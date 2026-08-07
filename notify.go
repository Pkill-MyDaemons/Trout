package main

import (
	"encoding/json"
	"fmt"
)

var webhookFormats = []string{"slack", "discord", "generic"}

func cycleWebhookFormat(current string, forward bool) string {
	for i, v := range webhookFormats {
		if v == current {
			if forward {
				return webhookFormats[(i+1)%len(webhookFormats)]
			}
			return webhookFormats[(i+len(webhookFormats)-1)%len(webhookFormats)]
		}
	}
	return webhookFormats[0]
}

// sendWebhookNotification posts a short message to cfg.NotifyWebhookURL when
// the agent finishes replying to a task. A failed or unconfigured webhook
// must never fail the task itself — callers run this best-effort and log
// errors rather than surfacing them to the user.
func sendWebhookNotification(cfg *Config, taskTitle, body string) error {
	if cfg.NotifyWebhookURL == "" {
		return nil
	}

	text := fmt.Sprintf("Trout: %s\n\n%s", taskTitle, truncate(body, 1000))

	var payload any
	switch cfg.NotifyWebhookFormat {
	case "slack":
		payload = map[string]string{"text": text}
	case "discord":
		payload = map[string]string{"content": text}
	default: // generic
		payload = map[string]string{"title": taskTitle, "body": body}
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = httpPost(cfg.NotifyWebhookURL, data, map[string]string{
		"content-type": "application/json",
	})
	return err
}
