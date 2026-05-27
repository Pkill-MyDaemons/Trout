package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

var seenIDsFile = dataPath("gmail_seen.json")

func loadSeenIDs() map[string]bool {
	data, err := os.ReadFile(seenIDsFile)
	if err != nil {
		return map[string]bool{}
	}
	var ids []string
	if err := json.Unmarshal(data, &ids); err != nil {
		return map[string]bool{}
	}
	m := make(map[string]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m
}

func saveSeenIDs(seen map[string]bool) {
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	data, _ := json.Marshal(ids)
	_ = os.WriteFile(seenIDsFile, data, 0600)
}

type gmailListResponse struct {
	Messages []struct {
		ID string `json:"id"`
	} `json:"messages"`
}

type gmailMessage struct {
	ID      string `json:"id"`
	Payload struct {
		Headers []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"headers"`
		MimeType string `json:"mimeType"`
		Body     struct {
			Data string `json:"data"`
		} `json:"body"`
		Parts []gmailPart `json:"parts"`
	} `json:"payload"`
	Snippet string `json:"snippet"`
}

type gmailPart struct {
	MimeType string `json:"mimeType"`
	Body     struct {
		Data string `json:"data"`
	} `json:"body"`
	Parts []gmailPart `json:"parts"`
}

func gmailGet(token, path string, out interface{}) error {
	req, err := http.NewRequest("GET", "https://gmail.googleapis.com/gmail/v1/users/me/"+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("gmail api: status %d for %s", resp.StatusCode, path)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func gmailMarkRead(token, messageID string) error {
	body, _ := json.Marshal(map[string][]string{"removeLabelIds": {"UNREAD"}})
	req, err := http.NewRequest("POST",
		"https://gmail.googleapis.com/gmail/v1/users/me/messages/"+messageID+"/modify",
		bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("gmail api: status %d on mark-read", resp.StatusCode)
	}
	return nil
}

// extractPlainText walks the MIME part tree looking for the first text/plain leaf.
func extractPlainText(parts []gmailPart) string {
	for _, p := range parts {
		if p.MimeType == "text/plain" && p.Body.Data != "" {
			data, err := base64.URLEncoding.DecodeString(p.Body.Data)
			if err == nil {
				return strings.TrimSpace(string(data))
			}
		}
		if len(p.Parts) > 0 {
			if s := extractPlainText(p.Parts); s != "" {
				return s
			}
		}
	}
	return ""
}

func gmailMessageBody(msg gmailMessage) string {
	// Non-multipart message
	if msg.Payload.MimeType == "text/plain" && msg.Payload.Body.Data != "" {
		data, err := base64.URLEncoding.DecodeString(msg.Payload.Body.Data)
		if err == nil {
			return strings.TrimSpace(string(data))
		}
	}
	if s := extractPlainText(msg.Payload.Parts); s != "" {
		return s
	}
	return msg.Snippet
}

func gmailHeader(msg gmailMessage, name string) string {
	for _, h := range msg.Payload.Headers {
		if strings.EqualFold(h.Name, name) {
			return h.Value
		}
	}
	return ""
}

func pollGmailInbox(cfg *Config, store *Store) error {
	if !cfg.GmailPollEnabled || cfg.EmailProvider != "gmail" {
		return nil
	}
	token, err := getValidAccessToken(cfg)
	if err != nil {
		return fmt.Errorf("gmail poll: %w", err)
	}

	var list gmailListResponse
	if err := gmailGet(token, "messages?q=is:unread+in:inbox+category:primary&maxResults=20", &list); err != nil {
		return fmt.Errorf("gmail poll list: %w", err)
	}
	daemonLog("gmail poll: %d unread message(s) found", len(list.Messages))

	if len(list.Messages) == 0 {
		return nil
	}

	seen := loadSeenIDs()
	changed := false

	for _, m := range list.Messages {
		if seen[m.ID] {
			continue
		}
		var msg gmailMessage
		if err := gmailGet(token, "messages/"+m.ID+"?format=full", &msg); err != nil {
			daemonLog("gmail poll: fetch message %s: %v", m.ID, err)
			continue
		}

		from := gmailHeader(msg, "from")
		if isNoReplyAddress(from) {
			daemonLog("gmail poll: skipping no-reply from %s", from)
			seen[m.ID] = true
			changed = true
			continue
		}
		if gmailHeader(msg, "list-unsubscribe") != "" {
			daemonLog("gmail poll: skipping bulk/marketing from %s", from)
			seen[m.ID] = true
			changed = true
			continue
		}

		subject := gmailHeader(msg, "subject")
		if subject == "" {
			subject = "(no subject)"
		}
		body := gmailMessageBody(msg)
		description := fmt.Sprintf("From: %s\n\n%s", from, body)

		store.addTask(subject, description)
		daemonLog("gmail poll: created task %q from %s", subject, from)
		_ = gmailMarkRead(token, m.ID)

		seen[m.ID] = true
		changed = true
	}

	if changed {
		saveSeenIDs(seen)
	}
	return nil
}
