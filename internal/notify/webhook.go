package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"archivesync/internal/models"
)

func init() { Register(models.NotifierWebhook, newWebhook) }

// webhookNotifier POSTs a JSON payload to an arbitrary HTTP endpoint.
type webhookNotifier struct {
	url     string
	method  string
	headers map[string]string
	client  *http.Client
}

// newWebhook builds a webhook notifier, validating the target URL.
func newWebhook(n models.Notifier) (Notifier, error) {
	c := n.Config
	if c.WebhookURL == "" {
		return nil, fmt.Errorf("webhook: webhook_url is required")
	}
	method := strings.ToUpper(strings.TrimSpace(c.WebhookMethod))
	if method == "" {
		method = http.MethodPost
	}
	return &webhookNotifier{
		url:     c.WebhookURL,
		method:  method,
		headers: c.WebhookHeaders,
		client:  &http.Client{Timeout: 15 * time.Second},
	}, nil
}

// Kind returns the notifier type identifier.
func (w *webhookNotifier) Kind() string { return "webhook" }

func isDiscordWebhook(u string) bool {
	return strings.Contains(u, "discord.com/api/webhooks") || strings.Contains(u, "discordapp.com/api/webhooks")
}

func isSlackWebhook(u string) bool {
	return strings.Contains(u, "hooks.slack.com/")
}

// Send marshals ev into a JSON payload and delivers it to the endpoint.
func (w *webhookNotifier) Send(ctx context.Context, ev Event) error {
	ts := ev.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}

	msg := ev.Message
	if msg == "" && ev.Run != nil {
		msg = ev.Run.Message
	}

	text := plainText(ev)

	// Discord / Slack incoming webhooks expect their own payload shapes; send a
	// proper embed card to Discord and a text message to Slack. Any other
	// endpoint gets a structured payload (with content/text for compatibility).
	var payload map[string]any
	switch {
	case isDiscordWebhook(w.url):
		payload = map[string]any{"embeds": []any{discordEmbed(ev)}}
	case isSlackWebhook(w.url):
		payload = map[string]any{"text": text}
	default:
		payload = map[string]any{
			"type":      ev.Type,
			"target":    ev.TargetName,
			"title":     subject(ev),
			"message":   msg,
			"content":   text, // Discord-compatible
			"text":      text, // Slack-compatible
			"timestamp": ts.UTC().Format(time.RFC3339),
			"fields":    ev.Fields,
		}
		if ev.Run != nil {
			payload["run"] = ev.Run
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("webhook: marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, w.method, w.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("webhook: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range w.headers {
		req.Header.Set(k, v)
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook: send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		rb, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("webhook: unexpected status %s: %s", resp.Status, strings.TrimSpace(string(rb)))
	}
	return nil
}
