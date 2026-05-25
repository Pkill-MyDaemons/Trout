package main

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

type EmailMode string

const (
	EmailModeSummaryOnly EmailMode = "summary_only"
	EmailModeDraftWait   EmailMode = "draft_wait"
	EmailModeAutoSend    EmailMode = "auto_send"
)

var emailModes = []EmailMode{
	EmailModeSummaryOnly,
	EmailModeDraftWait,
	EmailModeAutoSend,
}

func emailModeLabel(m EmailMode) string {
	switch m {
	case EmailModeDraftWait:
		return "Draft + wait"
	case EmailModeAutoSend:
		return "Auto send"
	default:
		return "Summary only"
	}
}

func cycleEmailMode(m EmailMode, forward bool) EmailMode {
	for i, v := range emailModes {
		if v == m {
			if forward {
				return emailModes[(i+1)%len(emailModes)]
			}
			return emailModes[(i+len(emailModes)-1)%len(emailModes)]
		}
	}
	return EmailModeSummaryOnly
}

// EmailDraft is an email reply drafted by the agent and stored on a Comment.
type EmailDraft struct {
	To      string     `json:"to"`
	Subject string     `json:"subject"`
	Body    string     `json:"body"`
	Status  string     `json:"status"` // "pending" | "sent" | "rejected"
	SentAt  *time.Time `json:"sent_at,omitempty"`
}

// sendEmail delivers a draft via SMTP.
// Port 465 uses implicit TLS; all others use STARTTLS via smtp.SendMail.
func sendEmail(cfg *Config, draft *EmailDraft) error {
	if cfg.SMTPHost == "" {
		return fmt.Errorf("SMTP host not configured — open config with 'c'")
	}
	if cfg.FromAddr == "" {
		return fmt.Errorf("from address not configured")
	}

	addr := fmt.Sprintf("%s:%d", cfg.SMTPHost, cfg.SMTPPort)
	from := cfg.FromAddr
	to := []string{draft.To}

	headers := fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n",
		from, draft.To, draft.Subject,
	)
	msg := []byte(headers + draft.Body)

	if cfg.SMTPPort == 465 {
		return sendTLS(addr, cfg.SMTPHost, cfg.SMTPUser, cfg.SMTPPassword, from, to, msg)
	}
	auth := smtp.PlainAuth("", cfg.SMTPUser, cfg.SMTPPassword, cfg.SMTPHost)
	return smtp.SendMail(addr, auth, from, to, msg)
}

// sendTLS handles implicit TLS (port 465 / SMTPS).
func sendTLS(addr, host, user, pass, from string, to []string, msg []byte) error {
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: host})
	if err != nil {
		// fallback: try plain TCP if TLS dial fails (some servers)
		plain, err2 := net.Dial("tcp", addr)
		if err2 != nil {
			return err
		}
		conn2 := tls.Client(plain, &tls.Config{ServerName: host})
		if err2 := conn2.Handshake(); err2 != nil {
			return err
		}
		c2, err2 := smtp.NewClient(conn2, host)
		if err2 != nil {
			return err
		}
		return smtpSend(c2, user, pass, host, from, to, msg)
	}
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	return smtpSend(c, user, pass, host, from, to, msg)
}

func smtpSend(c *smtp.Client, user, pass, host, from string, to []string, msg []byte) error {
	defer c.Quit()
	if err := c.Auth(smtp.PlainAuth("", user, pass, host)); err != nil {
		return err
	}
	if err := c.Mail(from); err != nil {
		return err
	}
	for _, addr := range to {
		if err := c.Rcpt(addr); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	defer w.Close()
	_, err = w.Write(msg)
	return err
}

// isNoReplySender returns true if any "From:" line in the task title or
// description contains "no-reply" or "noreply" — automated senders we skip.
func isNoReplySender(task *Task) bool {
	combined := strings.ToLower(task.Title + "\n" + task.Description)
	for _, line := range strings.Split(combined, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "from:") {
			continue
		}
		addr := line[5:]
		if strings.Contains(addr, "no-reply") || strings.Contains(addr, "noreply") {
			return true
		}
	}
	return false
}

// parseEmailDraft extracts a <!-- task-agent-email ... --> block from raw text.
// The block format is:
//
//	<!-- task-agent-email
//	to: recipient@example.com
//	subject: Re: Something
//	---
//	Body text here.
//	-->
func parseEmailDraft(raw string) (body string, draft *EmailDraft) {
	const startMarker = "<!-- task-agent-email"
	const endMarker = "-->"

	idx := strings.LastIndex(raw, startMarker)
	if idx == -1 {
		return strings.TrimSpace(raw), nil
	}

	body = strings.TrimSpace(raw[:idx])
	rest := raw[idx+len(startMarker):]
	closeIdx := strings.Index(rest, endMarker)
	if closeIdx == -1 {
		return body, nil
	}

	section := strings.TrimSpace(rest[:closeIdx])
	lines := strings.Split(section, "\n")

	d := &EmailDraft{Status: "pending"}
	bodyStart := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "---" {
			bodyStart = i + 1
			break
		}
		lower := strings.ToLower(line)
		if val, ok := strings.CutPrefix(lower, "to:"); ok {
			d.To = strings.TrimSpace(line[len("to:"):])
			_ = val
		} else if strings.HasPrefix(lower, "subject:") {
			d.Subject = strings.TrimSpace(line[len("subject:"):])
		}
	}

	if bodyStart >= 0 && bodyStart < len(lines) {
		d.Body = strings.TrimSpace(strings.Join(lines[bodyStart:], "\n"))
	}

	if d.To == "" && d.Subject == "" && d.Body == "" {
		return body, nil // malformed block — ignore
	}
	return body, d
}
