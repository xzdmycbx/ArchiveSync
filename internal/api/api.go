// Package api exposes the ArchiveSync REST API and serves the embedded Vue SPA.
package api

import (
	"encoding/json"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"

	"archivesync/internal/auth"
	"archivesync/internal/backup"
	"archivesync/internal/config"
	"archivesync/internal/models"
	"archivesync/internal/scheduler"
	"archivesync/internal/store"
	"archivesync/internal/version"
	webui "archivesync/web"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Server wires the HTTP layer to the store, backup engine and scheduler.
type Server struct {
	cfg       *config.Config
	store     store.Store
	engine    *backup.Engine
	sched     *scheduler.Scheduler
	auth      *auth.Authenticator // nil => dev/no-auth mode
	log       *slog.Logger
	startedAt time.Time
}

// New constructs an API Server. auth may be nil to run without IAM (dev mode),
// in which case every request is treated as a local administrator.
func New(cfg *config.Config, st store.Store, engine *backup.Engine, sched *scheduler.Scheduler, a *auth.Authenticator, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{
		cfg:       cfg,
		store:     st,
		engine:    engine,
		sched:     sched,
		auth:      a,
		log:       log,
		startedAt: time.Now(),
	}
}

// DevMode reports whether the server runs without IAM authentication.
func (s *Server) DevMode() bool { return s.auth == nil }

// Handler builds the HTTP handler (router + SPA).
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(s.requestLogger)
	r.Use(middleware.Recoverer)

	r.Route("/api", func(r chi.Router) {
		r.Get("/health", s.handleHealth)

		r.Route("/auth", func(r chi.Router) {
			if s.auth != nil {
				r.Get("/login", s.auth.LoginHandler)
				r.Get("/callback", s.auth.CallbackHandler)
				r.Post("/logout", s.auth.LogoutHandler)
			} else {
				r.Get("/login", s.devLogin)
				r.Get("/callback", s.devLogin)
				r.Post("/logout", s.devLogout)
			}
			r.Group(func(r chi.Router) {
				r.Use(s.authMiddleware())
				r.Get("/me", s.handleMe)
			})
		})

		r.Group(func(r chi.Router) {
			r.Use(s.authMiddleware())

			r.Route("/channels", func(r chi.Router) {
				r.Get("/", s.listChannels)
				r.Post("/", s.createChannel)
				r.Post("/test", s.testChannelBody)
				r.Get("/{id}", s.getChannel)
				r.Put("/{id}", s.updateChannel)
				r.Delete("/{id}", s.deleteChannel)
				r.Post("/{id}/test", s.testChannelByID)
				r.Get("/{id}/objects", s.listChannelObjects)
				r.Get("/{id}/download", s.downloadChannelObject)
			})

			r.Route("/notifiers", func(r chi.Router) {
				r.Get("/", s.listNotifiers)
				r.Post("/", s.createNotifier)
				r.Post("/test", s.testNotifierBody)
				r.Get("/{id}", s.getNotifier)
				r.Put("/{id}", s.updateNotifier)
				r.Delete("/{id}", s.deleteNotifier)
				r.Post("/{id}/test", s.testNotifierByID)
			})

			r.Route("/targets", func(r chi.Router) {
				r.Get("/", s.listTargets)
				r.Post("/", s.createTarget)
				r.Get("/{id}", s.getTarget)
				r.Put("/{id}", s.updateTarget)
				r.Delete("/{id}", s.deleteTarget)
				r.Post("/{id}/run", s.runTarget)
			})

			r.Route("/runs", func(r chi.Router) {
				r.Get("/", s.listRuns)
				r.Get("/{id}", s.getRun)
			})

			r.Get("/status", s.handleStatus)
			r.Get("/meta", s.handleMeta)
			r.Get("/fs", s.handleFsList)
		})
	})

	r.Handle("/*", s.spaHandler())
	return r
}

// authMiddleware returns the real IAM middleware, or a passthrough in dev mode.
func (s *Server) authMiddleware() func(http.Handler) http.Handler {
	if s.auth != nil {
		return s.auth.Middleware
	}
	return func(next http.Handler) http.Handler { return next }
}

// ---------------------------------------------------------------------------
// Auth-related endpoints (dev-mode variants + /me).
// ---------------------------------------------------------------------------

func devSession() *models.Session {
	now := time.Now()
	return &models.Session{
		ID:        "dev",
		UserID:    "local",
		Username:  "local",
		Name:      "本地管理员",
		Email:     "",
		Roles:     []string{"admin"},
		CreatedAt: now,
		ExpiresAt: now.Add(24 * time.Hour),
	}
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		writeJSON(w, http.StatusOK, devSession())
		return
	}
	sess := auth.SessionFrom(r.Context())
	if sess == nil {
		writeErr(w, http.StatusUnauthorized, "unauthorized", "未登录")
		return
	}
	writeJSON(w, http.StatusOK, sess)
}

func (s *Server) devLogin(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, auth.SafeReturnTo(r.URL.Query().Get("return_to")), http.StatusFound)
}

func (s *Server) devLogout(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"version": version.Version,
		"dev":     s.DevMode(),
	})
}

// handleMeta returns static metadata for the UI (available types, dev flag).
func (s *Server) handleMeta(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"version":        version.String(),
		"dev":            s.DevMode(),
		"channel_types":  []string{"s3", "local"},
		"notifier_types": []string{"discord", "telegram", "smtp", "webhook"},
	})
}

// ---------------------------------------------------------------------------
// SPA serving (embedded Vue build with client-side routing fallback).
// ---------------------------------------------------------------------------

func (s *Server) spaHandler() http.Handler {
	fsys, err := webui.FS()
	if err != nil {
		s.log.Error("load embedded frontend", "err", err)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "frontend unavailable", http.StatusInternalServerError)
		})
	}
	fileServer := http.FileServer(http.FS(fsys))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" {
			p = "index.html"
		}
		if f, err := fsys.Open(p); err == nil {
			f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}
		// Client-side route: serve index.html.
		s.serveIndex(w, fsys)
	})
}

func (s *Server) serveIndex(w http.ResponseWriter, fsys fs.FS) {
	idx, err := fsys.Open("index.html")
	if err != nil {
		http.Error(w, "前端尚未构建，请在 web/ 运行 npm run build", http.StatusInternalServerError)
		return
	}
	defer idx.Close()
	data, err := io.ReadAll(idx)
	if err != nil {
		http.Error(w, "read index", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(data)
}

// ---------------------------------------------------------------------------
// Shared helpers.
// ---------------------------------------------------------------------------

func (s *Server) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		if strings.HasPrefix(r.URL.Path, "/api/") {
			s.log.Info("http",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"dur_ms", time.Since(start).Milliseconds(),
			)
		}
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func writeErr(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": msg}})
}

// decode reads a JSON request body (capped) into v.
func decode(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	return dec.Decode(v)
}

func idParam(r *http.Request) string { return chi.URLParam(r, "id") }
