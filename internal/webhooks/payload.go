package webhooks

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/slashobserve/slashobserve/internal/activity"
)

const typeTest = "webhook.test"

type envelope struct {
	DeliveryID string              `json:"delivery_id"`
	WebhookID  string              `json:"webhook_id"`
	Type       string              `json:"type"`
	Event      *activity.EventJSON `json:"event"`
}

func eventPayload(webhookID string, e activity.Event) (body []byte, deliveryID string, err error) {
	deliveryID = webhookID + "." + strconv.FormatInt(e.Seq, 10)
	event := activity.JSONOf(e)
	body, err = json.Marshal(envelope{DeliveryID: deliveryID, WebhookID: webhookID, Type: e.Type, Event: &event})
	if err != nil {
		return nil, "", fmt.Errorf("encode webhook payload: %w", err)
	}
	return body, deliveryID, nil
}

func testPayload(webhookID string) (body []byte, deliveryID string, err error) {
	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		return nil, "", fmt.Errorf("test delivery id: %w", err)
	}
	deliveryID = webhookID + ".test." + hex.EncodeToString(suffix)
	body, err = json.Marshal(envelope{DeliveryID: deliveryID, WebhookID: webhookID, Type: typeTest})
	if err != nil {
		return nil, "", fmt.Errorf("encode webhook payload: %w", err)
	}
	return body, deliveryID, nil
}
