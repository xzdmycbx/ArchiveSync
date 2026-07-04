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

func init() { Register(models.NotifierTelegram, newTelegram) }

// telegramNotifier sends messages through the Telegram Bot API.
type telegramNotifier struct {
	token  string
	chatID string
	client *http.Client
}

// newTelegram builds a Telegram notifier, validating the bot token and chat ID.
func newTelegram(n models.Notifier) (Notifier, error) {
	c := n.Config
	if c.TGBotToken == "" {
		return nil, fmt.Errorf("telegram: tg_bot_token is required")
	}
	if c.TGChatID == "" {
		return nil, fmt.Errorf("telegram: tg_chat_id is required")
	}
	return &telegramNotifier{
		token:  c.TGBotToken,
		chatID: c.TGChatID,
		client: &http.Client{Timeout: 15 * time.Second},
	}, nil
}

// Kind returns the notifier type identifier.
func (t *telegramNotifier) Kind() string { return "telegram" }

// tgResponse is the envelope returned by the Telegram Bot API.
type tgResponse struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
	ErrorCode   int    `json:"error_code"`
}

// Send delivers ev as a plain-text Telegram message (no parse_mode, which is
// the safest option since the rendered text is not escaped).
func (t *telegramNotifier) Send(ctx context.Context, ev Event) error {
	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.token)

	payload := map[string]any{
		"chat_id": t.chatID,
		"text":    plainText(ev),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("telegram: marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("telegram: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("telegram: send: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var tr tgResponse
		_ = json.Unmarshal(raw, &tr)
		if tr.Description != "" {
			return fmt.Errorf("telegram: api error %d: %s", tr.ErrorCode, tr.Description)
		}
		return fmt.Errorf("telegram: unexpected status %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}

	var tr tgResponse
	if err := json.Unmarshal(raw, &tr); err == nil && !tr.OK {
		return fmt.Errorf("telegram: api error %d: %s", tr.ErrorCode, tr.Description)
	}
	return nil
}
