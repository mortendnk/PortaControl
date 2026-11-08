package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/mortendnk/portacontrol/server/internal/events"
)

// Notice is a user facing notification.
type Notice struct {
	Kind   string    `json:"kind"` // filter, error, offline, online, selfclean, temperature, automation, test
	Device string    `json:"device,omitempty"`
	Title  string    `json:"title"`
	Text   string    `json:"text,omitempty"`
	Time   time.Time `json:"time"`
}

var notifyHTTP = &http.Client{Timeout: 10 * time.Second}

// notify logs, publishes and pushes a notice (ntfy and webhooks).
func (a *App) notify(device, kind, title, text string) {
	n := Notice{Kind: kind, Device: device, Title: title, Text: text, Time: time.Now()}
	level := "info"

	if kind == "error" || kind == "offline" {
		level = "warn"
	}

	line := title
	if text != "" {
		line += " – " + text
	}

	a.logf(device, level, "%s", line)
	a.Hub.Publish(events.Event{Type: "notice", Device: device, Data: n})

	s := a.Store.Settings()

	go func() {
		a.notifyMu.Lock()
		defer a.notifyMu.Unlock()

		if s.Notify.NtfyURL != "" {
			a.pushNtfy(s.Notify.NtfyURL, n)
		}

		a.sendWebhooks("notice", n)
	}()
}

func (a *App) pushNtfy(url string, n Notice) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	body := n.Text
	if body == "" {
		body = n.Title
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return
	}

	req.Header.Set("Title", n.Title)
	req.Header.Set("Tags", "snowflake")

	if n.Kind == "error" || n.Kind == "offline" {
		req.Header.Set("Priority", "high")
	}

	resp, err := notifyHTTP.Do(req)
	if err != nil {
		a.Log.Warn("ntfy", "err", err)
		return
	}

	_ = resp.Body.Close()
}

// sendWebhooks posts {"event": kind, ...data} to every matching webhook.
func (a *App) sendWebhooks(kind string, data any) {
	for _, w := range a.Store.Settings().Webhooks {
		if !w.Enabled || w.URL == "" || (len(w.Events) > 0 && !slices.Contains(w.Events, kind)) {
			continue
		}

		b, _ := json.Marshal(map[string]any{"event": kind, "data": data, "time": time.Now()})

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.URL, bytes.NewReader(b))

		if err == nil {
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("User-Agent", "PortaControl")

			if resp, err := notifyHTTP.Do(req); err != nil {
				a.Log.Warn("webhook", "url", w.URL, "err", err)
			} else {
				_ = resp.Body.Close()
			}
		}

		cancel()
	}
}

// TestNotify sends a test notification.
func (a *App) TestNotify() {
	a.notify("", "test", "PortaControl: Testnachricht", "Benachrichtigungen funktionieren.")
}

// StateWebhook sends the state of a device to webhooks subscribed to "state".
func (a *App) StateWebhook(id string) {
	v, err := a.View(id, false)
	if err != nil {
		return
	}

	go a.sendWebhooks("state", v)
}
