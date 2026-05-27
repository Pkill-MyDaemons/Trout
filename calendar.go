package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func calendarGet(token, path string, out interface{}) error {
	req, err := http.NewRequest("GET", "https://www.googleapis.com/calendar/v3/"+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var errBody struct {
			Error struct{ Message string `json:"message"` } `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&errBody) //nolint:errcheck
		if errBody.Error.Message != "" {
			return fmt.Errorf("calendar api: %s", errBody.Error.Message)
		}
		return fmt.Errorf("calendar api: status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func calendarPatch(token, path string, body []byte, out interface{}) error {
	req, err := http.NewRequest("PATCH", "https://www.googleapis.com/calendar/v3/"+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var errBody struct {
			Error struct{ Message string `json:"message"` } `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&errBody) //nolint:errcheck
		if errBody.Error.Message != "" {
			return fmt.Errorf("calendar api: %s", errBody.Error.Message)
		}
		return fmt.Errorf("calendar api: status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func calendarPost(token, path string, body []byte, out interface{}) error {
	req, err := http.NewRequest("POST", "https://www.googleapis.com/calendar/v3/"+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var errBody struct {
			Error struct{ Message string `json:"message"` } `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&errBody) //nolint:errcheck
		if errBody.Error.Message != "" {
			return fmt.Errorf("calendar api: %s", errBody.Error.Message)
		}
		return fmt.Errorf("calendar api: status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (e *Executor) listCalendarEvents(days int) (string, error) {
	if e.cfg == nil {
		return "", fmt.Errorf("no config available")
	}
	token, err := getValidAccessToken(e.cfg)
	if err != nil {
		return "", fmt.Errorf("calendar: %w", err)
	}

	now := time.Now()
	timeMin := now.Format(time.RFC3339)
	timeMax := now.AddDate(0, 0, days).Format(time.RFC3339)

	var result struct {
		Items []struct {
			ID          string `json:"id"`
			Summary     string `json:"summary"`
			Description string `json:"description"`
			Location    string `json:"location"`
			Start       struct {
				DateTime string `json:"dateTime"`
				Date     string `json:"date"`
			} `json:"start"`
			End struct {
				DateTime string `json:"dateTime"`
				Date     string `json:"date"`
			} `json:"end"`
		} `json:"items"`
	}

	path := fmt.Sprintf("calendars/primary/events?timeMin=%s&timeMax=%s&maxResults=20&orderBy=startTime&singleEvents=true",
		timeMin, timeMax)
	if err := calendarGet(token, path, &result); err != nil {
		return "", err
	}

	if len(result.Items) == 0 {
		return fmt.Sprintf("No events in the next %d day(s).", days), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Upcoming events (next %d day(s)):\n\n", days)
	for _, ev := range result.Items {
		start := ev.Start.DateTime
		if start == "" {
			start = ev.Start.Date + " (all day)"
		} else {
			if t, err := time.Parse(time.RFC3339, start); err == nil {
				start = t.Format("Mon Jan 2, 3:04 PM")
			}
		}
		end := ev.End.DateTime
		if end == "" {
			end = ev.End.Date
		} else {
			if t, err := time.Parse(time.RFC3339, end); err == nil {
				end = t.Format("3:04 PM")
			}
		}
		fmt.Fprintf(&sb, "• [id:%s] %s — %s to %s", ev.ID, ev.Summary, start, end)
		if ev.Location != "" {
			fmt.Fprintf(&sb, " @ %s", ev.Location)
		}
		if ev.Description != "" {
			desc := strings.ReplaceAll(strings.TrimSpace(ev.Description), "\n", " ")
			if len(desc) > 100 {
				desc = desc[:100] + "…"
			}
			fmt.Fprintf(&sb, "\n  %s", desc)
		}
		sb.WriteString("\n")
	}
	return strings.TrimSpace(sb.String()), nil
}

func (e *Executor) createCalendarEvent(title, start, end, description, location string) (string, error) {
	if e.cfg == nil {
		return "", fmt.Errorf("no config available")
	}
	token, err := getValidAccessToken(e.cfg)
	if err != nil {
		return "", fmt.Errorf("calendar: %w", err)
	}

	event := map[string]any{
		"summary": title,
		"start":   map[string]string{"dateTime": start},
		"end":     map[string]string{"dateTime": end},
	}
	if description != "" {
		event["description"] = description
	}
	if location != "" {
		event["location"] = location
	}

	body, err := json.Marshal(event)
	if err != nil {
		return "", err
	}

	var created struct {
		ID      string `json:"id"`
		HtmlLink string `json:"htmlLink"`
		Summary string `json:"summary"`
	}
	if err := calendarPost(token, "calendars/primary/events", body, &created); err != nil {
		return "", err
	}
	return fmt.Sprintf("Created event %q — %s", created.Summary, created.HtmlLink), nil
}

func (e *Executor) updateCalendarEvent(eventID, title, start, end, description, location string) (string, error) {
	if e.cfg == nil {
		return "", fmt.Errorf("no config available")
	}
	if eventID == "" {
		return "", fmt.Errorf("event_id is required — use list_calendar_events to find it")
	}
	token, err := getValidAccessToken(e.cfg)
	if err != nil {
		return "", fmt.Errorf("calendar: %w", err)
	}

	patch := map[string]any{}
	if title != "" {
		patch["summary"] = title
	}
	if start != "" {
		patch["start"] = map[string]string{"dateTime": start}
	}
	if end != "" {
		patch["end"] = map[string]string{"dateTime": end}
	}
	if description != "" {
		patch["description"] = description
	}
	if location != "" {
		patch["location"] = location
	}

	body, err := json.Marshal(patch)
	if err != nil {
		return "", err
	}

	var updated struct {
		Summary  string `json:"summary"`
		HtmlLink string `json:"htmlLink"`
	}
	if err := calendarPatch(token, "calendars/primary/events/"+eventID, body, &updated); err != nil {
		return "", err
	}
	return fmt.Sprintf("Updated event %q — %s", updated.Summary, updated.HtmlLink), nil
}
