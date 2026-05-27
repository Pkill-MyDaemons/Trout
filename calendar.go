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

func (e *Executor) createCalendarEvent(title, start, end, description, location, attendees string) (string, error) {
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
		// always request a Google Meet link
		"conferenceData": map[string]any{
			"createRequest": map[string]any{
				"requestId":             fmt.Sprintf("ta-%d", time.Now().UnixNano()),
				"conferenceSolutionKey": map[string]string{"type": "hangoutsMeet"},
			},
		},
	}
	if description != "" {
		event["description"] = description
	}
	if location != "" {
		event["location"] = location
	}
	if attendees != "" {
		var list []map[string]string
		for _, email := range strings.Split(attendees, ",") {
			if email = strings.TrimSpace(email); email != "" {
				list = append(list, map[string]string{"email": email})
			}
		}
		if len(list) > 0 {
			event["attendees"] = list
		}
	}

	body, err := json.Marshal(event)
	if err != nil {
		return "", err
	}

	var created struct {
		ID       string `json:"id"`
		HtmlLink string `json:"htmlLink"`
		Summary  string `json:"summary"`
		ConferenceData struct {
			EntryPoints []struct {
				EntryPointType string `json:"entryPointType"`
				URI            string `json:"uri"`
			} `json:"entryPoints"`
		} `json:"conferenceData"`
	}
	// conferenceDataVersion=1 is required for Meet link generation
	if err := calendarPost(token, "calendars/primary/events?conferenceDataVersion=1", body, &created); err != nil {
		return "", err
	}

	meetLink := ""
	for _, ep := range created.ConferenceData.EntryPoints {
		if ep.EntryPointType == "video" {
			meetLink = ep.URI
			break
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Created event %q\n", created.Summary)
	fmt.Fprintf(&sb, "Calendar link: %s\n", created.HtmlLink)
	if meetLink != "" {
		fmt.Fprintf(&sb, "Google Meet: %s\n", meetLink)
	}
	if attendees != "" {
		fmt.Fprintf(&sb, "Invited: %s\n", attendees)
	}
	return strings.TrimSpace(sb.String()), nil
}

func (e *Executor) updateCalendarEvent(eventID, title, start, end, description, location, attendees string) (string, error) {
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

	// GET the current event so we can merge rather than overwrite fields.
	var current struct {
		Summary     string `json:"summary"`
		Description string `json:"description"`
		Location    string `json:"location"`
		Start       struct {
			DateTime string `json:"dateTime"`
			TimeZone string `json:"timeZone"`
			Date     string `json:"date"`
		} `json:"start"`
		End struct {
			DateTime string `json:"dateTime"`
			TimeZone string `json:"timeZone"`
			Date     string `json:"date"`
		} `json:"end"`
		Attendees []struct {
			Email          string `json:"email"`
			Organizer      bool   `json:"organizer"`
			Self           bool   `json:"self"`
			ResponseStatus string `json:"responseStatus"`
		} `json:"attendees"`
		HtmlLink string `json:"htmlLink"`
	}
	if err := calendarGet(token, "calendars/primary/events/"+eventID, &current); err != nil {
		return "", fmt.Errorf("could not fetch event: %w", err)
	}

	// Build patch from current values, overriding only what was explicitly supplied.
	patch := map[string]any{
		"summary": current.Summary,
	}
	if title != "" {
		patch["summary"] = title
	}

	startDT := current.Start.DateTime
	startTZ := current.Start.TimeZone
	if start != "" {
		startDT = start
	}
	if startDT != "" {
		s := map[string]string{"dateTime": startDT}
		if startTZ != "" {
			s["timeZone"] = startTZ
		}
		patch["start"] = s
	} else if current.Start.Date != "" {
		patch["start"] = map[string]string{"date": current.Start.Date}
	}

	endDT := current.End.DateTime
	endTZ := current.End.TimeZone
	if end != "" {
		endDT = end
	}
	if endDT != "" {
		e2 := map[string]string{"dateTime": endDT}
		if endTZ != "" {
			e2["timeZone"] = endTZ
		}
		patch["end"] = e2
	} else if current.End.Date != "" {
		patch["end"] = map[string]string{"date": current.End.Date}
	}

	if description != "" {
		patch["description"] = description
	} else if current.Description != "" {
		patch["description"] = current.Description
	}
	if location != "" {
		patch["location"] = location
	} else if current.Location != "" {
		patch["location"] = current.Location
	}

	// Merge attendees: keep existing list and add new ones.
	emailSet := map[string]bool{}
	var mergedAttendees []map[string]string
	for _, a := range current.Attendees {
		emailSet[a.Email] = true
		mergedAttendees = append(mergedAttendees, map[string]string{"email": a.Email})
	}
	if attendees != "" {
		for _, email := range strings.Split(attendees, ",") {
			if email = strings.TrimSpace(email); email != "" && !emailSet[email] {
				mergedAttendees = append(mergedAttendees, map[string]string{"email": email})
			}
		}
	}
	if len(mergedAttendees) > 0 {
		patch["attendees"] = mergedAttendees
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

	var sb strings.Builder
	fmt.Fprintf(&sb, "Updated event %q\n", updated.Summary)
	fmt.Fprintf(&sb, "Calendar link: %s\n", updated.HtmlLink)
	if attendees != "" {
		fmt.Fprintf(&sb, "Added attendees: %s\n", attendees)
	}
	return strings.TrimSpace(sb.String()), nil
}
