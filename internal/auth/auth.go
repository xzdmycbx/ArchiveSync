// Package auth implements TransCircle IAM authentication for ArchiveSync using
// OIDC Authorization Code flow with PKCE (S256) and server-side sessions stored
// as HttpOnly cookies. See SPEC §4.6.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"archivesync/internal/config"
	"archivesync/internal/models"
	"archivesync/internal/store"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
)

// CookieName is the name of the session cookie set by the Authenticator.
const CookieName = "archive_sync_session"

// flowTTL bounds how long a pending login flow (state/nonce/PKCE) is valid.
const flowTTL = 10 * time.Minute

// ctxKey is the unexported context key type for the current session.
type ctxKey struct{}

var sessionCtxKey ctxKey

// flowState holds the transient per-login data kept between LoginHandler and
// CallbackHandler, keyed by the OAuth2 "state" value.
type flowState struct {
	nonce        string
	pkceVerifier string
	returnTo     string
	expiresAt    time.Time
}

// userClaims maps the TransCircle IAM userinfo response.
type userClaims struct {
	Sub               string   `json:"sub"`
	PreferredUsername string   `json:"preferred_username"`
	Name              string   `json:"name"`
	Email             string   `json:"email"`
	Picture           string   `json:"picture"`
	Roles             []string `json:"tc_roles"`
	Permissions       []string `json:"tc_permissions"`
	Groups            []string `json:"tc_groups"`
	PermVersion       string   `json:"tc_perm_version"`
}

// Authenticator handles the OIDC login/callback flow and guards protected
// routes via its Middleware. It is safe for concurrent use.
type Authenticator struct {
	oauth        oauth2.Config
	provider     *oidc.Provider
	verifier     *oidc.IDTokenVerifier
	store        store.Store
	sessionTTL   time.Duration
	secureCookie bool

	requiredPermission string
	requiredRole       string

	mu    sync.Mutex
	flows map[string]flowState
}

// New builds an Authenticator by discovering the OIDC provider at cfg.Issuer and
// wiring up the OAuth2 config and ID-token verifier. sessionTTL controls both the
// session lifetime and the cookie MaxAge; secureCookie sets the cookie Secure flag.
func New(ctx context.Context, cfg config.IAMConfig, st store.Store, sessionTTL time.Duration, secureCookie bool) (*Authenticator, error) {
	provider, err := oidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("auth: discover oidc provider %q: %w", cfg.Issuer, err)
	}
	oauthCfg := oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       cfg.Scopes,
	}
	verifier := provider.Verifier(&oidc.Config{ClientID: cfg.ClientID})
	return &Authenticator{
		oauth:              oauthCfg,
		provider:           provider,
		verifier:           verifier,
		store:              st,
		sessionTTL:         sessionTTL,
		secureCookie:       secureCookie,
		requiredPermission: cfg.RequiredPermission,
		requiredRole:       cfg.RequiredRole,
		flows:              make(map[string]flowState),
	}, nil
}

// ---------------------------------------------------------------------------
// Pending-flow bookkeeping.
// ---------------------------------------------------------------------------

func (a *Authenticator) storeFlow(state string, fs flowState) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pruneFlowsLocked()
	a.flows[state] = fs
}

// takeFlow atomically returns and removes the flow for state (single use).
func (a *Authenticator) takeFlow(state string) (flowState, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pruneFlowsLocked()
	fs, ok := a.flows[state]
	if ok {
		delete(a.flows, state)
	}
	return fs, ok
}

func (a *Authenticator) pruneFlowsLocked() {
	now := time.Now()
	for k, v := range a.flows {
		if now.After(v.expiresAt) {
			delete(a.flows, k)
		}
	}
}

// ---------------------------------------------------------------------------
// HTTP handlers.
// ---------------------------------------------------------------------------

// LoginHandler starts the OIDC Authorization Code + PKCE flow. It generates a
// state, nonce and PKCE verifier, stores them, and 302-redirects the browser to
// the IAM authorization endpoint. An optional ?return_to= records where to send
// the user after a successful callback.
func (a *Authenticator) LoginHandler(w http.ResponseWriter, r *http.Request) {
	returnTo := SafeReturnTo(r.URL.Query().Get("return_to"))
	state, err := randomToken()
	if err != nil {
		a.serverError(w, "generate state", err)
		return
	}
	nonce, err := randomToken()
	if err != nil {
		a.serverError(w, "generate nonce", err)
		return
	}
	pkce := oauth2.GenerateVerifier()
	a.storeFlow(state, flowState{
		nonce:        nonce,
		pkceVerifier: pkce,
		returnTo:     returnTo,
		expiresAt:    time.Now().Add(flowTTL),
	})
	authURL := a.oauth.AuthCodeURL(state,
		oauth2.S256ChallengeOption(pkce),
		oidc.Nonce(nonce),
	)
	http.Redirect(w, r, authURL, http.StatusFound)
}

// CallbackHandler completes the OIDC flow: it validates the state, exchanges the
// authorization code (with the PKCE verifier), verifies the ID token and nonce,
// fetches userinfo, creates a server-side session, sets the cookie and redirects
// back to the recorded return_to (default "/"). RequiredPermission/RequiredRole
// are enforced here; failing users get a 403.
func (a *Authenticator) CallbackHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()

	if authErr := q.Get("error"); authErr != "" {
		desc := q.Get("error_description")
		slog.Warn("auth: callback returned error", "error", authErr, "description", desc)
		http.Error(w, "authentication failed: "+authErr, http.StatusBadRequest)
		return
	}

	state := q.Get("state")
	code := q.Get("code")
	if state == "" || code == "" {
		http.Error(w, "missing code or state", http.StatusBadRequest)
		return
	}

	flow, ok := a.takeFlow(state)
	if !ok {
		http.Error(w, "invalid or expired login state", http.StatusBadRequest)
		return
	}

	tok, err := a.oauth.Exchange(ctx, code, oauth2.VerifierOption(flow.pkceVerifier))
	if err != nil {
		a.serverError(w, "exchange authorization code", err)
		return
	}

	rawID, ok := tok.Extra("id_token").(string)
	if !ok || rawID == "" {
		a.serverError(w, "token response missing id_token", errors.New("no id_token claim"))
		return
	}

	idt, err := a.verifier.Verify(ctx, rawID)
	if err != nil {
		a.serverError(w, "verify id_token", err)
		return
	}
	if idt.Nonce != flow.nonce {
		slog.Warn("auth: id_token nonce mismatch")
		http.Error(w, "nonce mismatch", http.StatusBadRequest)
		return
	}

	userInfo, err := a.provider.UserInfo(ctx, oauth2.StaticTokenSource(tok))
	if err != nil {
		a.serverError(w, "fetch userinfo", err)
		return
	}
	var claims userClaims
	if err := userInfo.Claims(&claims); err != nil {
		a.serverError(w, "parse userinfo claims", err)
		return
	}

	now := time.Now()
	sess := &models.Session{
		ID:          uuid.NewString(),
		UserID:      claims.Sub,
		Username:    claims.PreferredUsername,
		Name:        claims.Name,
		Email:       claims.Email,
		Picture:     claims.Picture,
		Roles:       claims.Roles,
		Permissions: claims.Permissions,
		Groups:      claims.Groups,
		PermVersion: claims.PermVersion,
		CreatedAt:   now,
		ExpiresAt:   now.Add(a.sessionTTL),
	}

	if !sess.HasPermission(a.requiredPermission) || !sess.HasRole(a.requiredRole) {
		slog.Warn("auth: user lacks required access",
			"user", sess.Username,
			"required_permission", a.requiredPermission,
			"required_role", a.requiredRole,
		)
		a.forbiddenPage(w)
		return
	}

	if err := a.store.CreateSession(ctx, sess); err != nil {
		a.serverError(w, "create session", err)
		return
	}
	a.setCookie(w, sess.ID)
	http.Redirect(w, r, flow.returnTo, http.StatusFound)
}

// LogoutHandler deletes the current session and clears the cookie.
func (a *Authenticator) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(CookieName); err == nil && c.Value != "" {
		if err := a.store.DeleteSession(r.Context(), c.Value); err != nil {
			slog.Warn("auth: delete session on logout", "error", err)
		}
	}
	a.clearCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

// MeHandler returns the current session user as JSON, or 401 if unauthenticated.
// It relies on Middleware having injected the session into the request context.
func (a *Authenticator) MeHandler(w http.ResponseWriter, r *http.Request) {
	sess := SessionFrom(r.Context())
	if sess == nil {
		a.unauthorized(w, "not authenticated")
		return
	}
	writeJSON(w, http.StatusOK, sess)
}

// Middleware guards protected routes. It reads the session cookie, loads and
// validates the session (existence, expiry, and RequiredPermission/RequiredRole),
// injects the *models.Session into the request context, and returns a JSON 401 on
// any missing or invalid session (or 403 when access requirements are unmet).
func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(CookieName)
		if err != nil || c.Value == "" {
			a.unauthorized(w, "authentication required")
			return
		}
		sess, err := a.store.GetSession(r.Context(), c.Value)
		if err != nil || sess == nil {
			a.clearCookie(w)
			a.unauthorized(w, "invalid session")
			return
		}
		if time.Now().After(sess.ExpiresAt) {
			if delErr := a.store.DeleteSession(r.Context(), sess.ID); delErr != nil {
				slog.Warn("auth: delete expired session", "error", delErr)
			}
			a.clearCookie(w)
			a.unauthorized(w, "session expired")
			return
		}
		if !sess.HasPermission(a.requiredPermission) || !sess.HasRole(a.requiredRole) {
			a.forbiddenJSON(w)
			return
		}
		ctx := context.WithValue(r.Context(), sessionCtxKey, sess)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// SessionFrom returns the authenticated session stored in ctx by Middleware, or
// nil if none is present.
func SessionFrom(ctx context.Context) *models.Session {
	sess, _ := ctx.Value(sessionCtxKey).(*models.Session)
	return sess
}

// SafeReturnTo returns rt only when it is a same-origin, in-app relative path;
// otherwise "/". This prevents the login flow from being used as an open
// redirect (absolute URLs, protocol-relative "//host", or backslash tricks).
func SafeReturnTo(rt string) string {
	if rt == "" || !strings.HasPrefix(rt, "/") || strings.HasPrefix(rt, "//") || strings.HasPrefix(rt, "/\\") {
		return "/"
	}
	if u, err := url.Parse(rt); err != nil || u.IsAbs() || u.Host != "" {
		return "/"
	}
	return rt
}

// ---------------------------------------------------------------------------
// Cookie + response helpers.
// ---------------------------------------------------------------------------

func (a *Authenticator) setCookie(w http.ResponseWriter, id string) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		Secure:   a.secureCookie,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(a.sessionTTL.Seconds()),
	})
}

func (a *Authenticator) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   a.secureCookie,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func (a *Authenticator) serverError(w http.ResponseWriter, what string, err error) {
	slog.Error("auth: "+what, "error", err)
	http.Error(w, "authentication error: "+what, http.StatusInternalServerError)
}

func (a *Authenticator) unauthorized(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusUnauthorized, errorBody("unauthorized", msg))
}

func (a *Authenticator) forbiddenJSON(w http.ResponseWriter) {
	writeJSON(w, http.StatusForbidden, errorBody("forbidden", "you do not have the required permission"))
}

func (a *Authenticator) forbiddenPage(w http.ResponseWriter) {
	http.Error(w, "access denied: your account does not have the required permission or role", http.StatusForbidden)
}

// randomToken returns a URL-safe, cryptographically random 256-bit token.
func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("auth: read random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func errorBody(code, message string) map[string]any {
	return map[string]any{"error": map[string]string{"code": code, "message": message}}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("auth: encode json response", "error", err)
	}
}
