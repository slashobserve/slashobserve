package webhooks

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/internal/activity"
)

const (
	typeTest   = "webhook.test"
	maxTitle   = 100
	testText   = "SlashObserve test event: this webhook is set up correctly."
	adaptive   = "application/vnd.microsoft.card.adaptive"
	cardSchema = "http://adaptivecards.io/schemas/adaptive-card.json"
)

type envelope struct {
	DeliveryID string              `json:"delivery_id"`
	WebhookID  string              `json:"webhook_id"`
	Type       string              `json:"type"`
	Event      *activity.EventJSON `json:"event"`
}

func deliveryIDOf(webhookID string, e activity.Event) string {
	return webhookID + "." + strconv.FormatInt(e.Seq, 10)
}

func eventPayload(webhookID, format string, e activity.Event) (body []byte, deliveryID string, err error) {
	deliveryID = deliveryIDOf(webhookID, e)
	event := activity.JSONOf(e)
	body, err = encode(format, envelope{DeliveryID: deliveryID, WebhookID: webhookID, Type: e.Type, Event: &event}, eventText(event))
	if err != nil {
		return nil, "", fmt.Errorf("encode webhook payload: %w", err)
	}
	return body, deliveryID, nil
}

func testPayload(webhookID, format string) (body []byte, deliveryID string, err error) {
	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		return nil, "", fmt.Errorf("test delivery id: %w", err)
	}
	deliveryID = webhookID + ".test." + hex.EncodeToString(suffix)
	body, err = encode(format, envelope{DeliveryID: deliveryID, WebhookID: webhookID, Type: typeTest}, testText)
	if err != nil {
		return nil, "", fmt.Errorf("encode webhook payload: %w", err)
	}
	return body, deliveryID, nil
}

func encode(format string, generic envelope, text string) ([]byte, error) {
	switch format {
	case FormatSlack:
		return json.Marshal(map[string]string{"text": slackEscape(text)})
	case FormatTeams:
		return json.Marshal(map[string]any{
			"type": "message",
			"attachments": []any{map[string]any{
				"contentType": adaptive,
				"content": map[string]any{
					"$schema": cardSchema,
					"type":    "AdaptiveCard",
					"version": "1.4",
					"body":    []any{map[string]any{"type": "TextBlock", "text": text, "wrap": true}},
				},
			}},
		})
	}
	return json.Marshal(generic)
}

func eventText(e activity.EventJSON) string {
	var data struct {
		Title          string `json:"title"`
		DashboardTitle string `json:"dashboard_title"`
	}
	_ = json.Unmarshal(e.Data, &data)
	subject := firstNonEmpty(data.Title, data.DashboardTitle)
	text := fmt.Sprintf("%s: %s", e.Actor.Label, e.Type)
	if subject != "" {
		text += " - " + truncate(subject, maxTitle)
	}
	return text
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func truncate(s string, runes int) string {
	if utf8.RuneCountInString(s) <= runes {
		return s
	}
	return string([]rune(s)[:runes]) + "..."
}

var slackEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

func slackEscape(text string) string { return slackEscaper.Replace(text) }
