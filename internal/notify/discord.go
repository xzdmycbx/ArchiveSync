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

func init() { Register(models.NotifierDiscord, newDiscord) }

// discordNotifier posts messages to a Discord channel via the bot API.
type discordNotifier struct {
	token     string
	channelID string
	guildID   string // kept for reference; not required to post
	client    *http.Client
}

// newDiscord builds a Discord notifier, validating the bot token and channel.
func newDiscord(n models.Notifier) (Notifier, error) {
	c := n.Config
	if c.BotToken == "" {
		return nil, fmt.Errorf("discord: bot_token is required")
	}
	if c.ChannelID == "" {
		return nil, fmt.Errorf("discord: channel_id is required")
	}
	return &discordNotifier{
		token:     c.BotToken,
		channelID: c.ChannelID,
		guildID:   c.GuildID,
		client:    &http.Client{Timeout: 15 * time.Second},
	}, nil
}

// Kind returns the notifier type identifier.
func (d *discordNotifier) Kind() string { return "discord" }

// Send posts an embed card describing ev to the configured channel.
func (d *discordNotifier) Send(ctx context.Context, ev Event) error {
	endpoint := fmt.Sprintf("https://discord.com/api/v10/channels/%s/messages", d.channelID)

	payload := map[string]any{"embeds": []any{discordEmbed(ev)}}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("discord: marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("discord: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bot "+d.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("discord: send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		rb, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("discord: unexpected status %s: %s", resp.Status, strings.TrimSpace(string(rb)))
	}
	return nil
}
