// Package api serves the REST API (/api/v1), live events (SSE) and the web UI.
package api

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mortendnk/portacontrol/server/internal/app"
	"github.com/mortendnk/portacontrol/server/internal/events"
	"github.com/mortendnk/portacontrol/server/internal/midea"
	"github.com/mortendnk/portacontrol/server/internal/store"
	"github.com/mortendnk/portacontrol/server/internal/version"
)

// Server holds the HTTP handlers.
type Server struct {
	App        *app.App
	Web        http.Handler
	MQTTStatus func() string
	started    time.Time
	limiter    *loginLimiter
}

// New creates the server.
func New(a *app.App, web http.Handler) *Server {
	return &Server{App: a, Web: web, started: time.Now(), limiter: &loginLimiter{fails: map[string][]time.Time{}}}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": msg})
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := dec.Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return false
	}

	return true
}

// fail maps errors to HTTP status codes.
func fail(w http.ResponseWriter, err error) {
	var na *midea.ErrNotApplied

	switch {
	case errors.Is(err, app.ErrUnknownDevice), errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, err.Error())
	case errors.Is(err, app.ErrBadRequest):
		writeErr(w, http.StatusBadRequest, err.Error())
	case errors.As(err, &na):
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error(), "state": na.State})
	case errors.Is(err, midea.ErrBusy):
		writeErr(w, http.StatusServiceUnavailable, err.Error())
	default:
		writeErr(w, http.StatusBadGateway, err.Error())
	}
}

// Router builds all routes.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(securityHeaders)

	r.Get("/health", s.health)

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(noStore)

		r.Get("/auth/me", s.me)
		r.Post("/auth/login", s.login)
		r.Post("/auth/logout", s.logout)
		r.Post("/auth/password", s.need(RoleAdmin, s.setPassword))

		r.Get("/events", s.need(RoleGuest, s.events))
		r.Get("/overview", s.need(RoleGuest, s.overview))

		r.Get("/devices", s.need(RoleGuest, s.listDevices))
		r.Post("/devices", s.need(RoleAdmin, s.addDevice))
		r.Get("/devices/{id}", s.need(RoleGuest, s.getDevice))
		r.Patch("/devices/{id}", s.need(RoleAdmin, s.patchDevice))
		r.Delete("/devices/{id}", s.need(RoleAdmin, s.deleteDevice))
		r.Post("/devices/{id}/control", s.need(RoleGuest, s.control))
		r.Post("/devices/{id}/refresh", s.need(RoleGuest, s.refresh))
		r.Get("/devices/{id}/diagnostics", s.need(RoleGuest, s.diagnostics))
		r.Post("/devices/{id}/query", s.need(RoleAdmin, s.rawQuery))
		r.Post("/devices/{id}/relocate", s.need(RoleAdmin, s.relocate))
		r.Get("/devices/{id}/history", s.need(RoleGuest, s.history))
		r.Get("/devices/{id}/history.csv", s.need(RoleGuest, s.historyCSV))
		r.Get("/devices/{id}/energy", s.need(RoleGuest, s.energy))
		r.Get("/devices/{id}/timers", s.need(RoleGuest, s.getTimers))
		r.Put("/devices/{id}/timers", s.need(RoleGuest, s.putTimers))
		r.Get("/devices/{id}/credentials", s.need(RoleAdmin, s.credentials))

		r.Post("/setup/discover", s.need(RoleAdmin, s.discover))
		r.Get("/setup/cloud", s.need(RoleAdmin, s.cloudStatus))
		r.Post("/setup/cloud/login", s.need(RoleAdmin, s.cloudLogin))
		r.Post("/setup/cloud/logout", s.need(RoleAdmin, s.cloudLogout))
		r.Post("/setup/import", s.need(RoleAdmin, s.importBackup))
		r.Get("/backup", s.need(RoleAdmin, s.exportBackup))

		r.Get("/scenes", s.need(RoleGuest, listHandler[store.Scene](s, store.CollScenes)))
		r.Post("/scenes", s.need(RoleAdmin, s.saveScene))
		r.Put("/scenes/{id}", s.need(RoleAdmin, s.saveScene))
		r.Delete("/scenes/{id}", s.need(RoleAdmin, deleteHandler(s, store.CollScenes, "scenes")))
		r.Post("/scenes/{id}/apply", s.need(RoleGuest, s.applyScene))

		r.Get("/schedules", s.need(RoleGuest, s.listSchedules))
		r.Post("/schedules", s.need(RoleAdmin, s.saveSchedule))
		r.Put("/schedules/{id}", s.need(RoleAdmin, s.saveSchedule))
		r.Delete("/schedules/{id}", s.need(RoleAdmin, deleteHandler(s, store.CollSchedules, "automation")))
		r.Post("/schedules/{id}/run", s.need(RoleAdmin, s.runSchedule))

		r.Get("/rules", s.need(RoleGuest, listHandler[store.Rule](s, store.CollRules)))
		r.Post("/rules", s.need(RoleAdmin, s.saveRule))
		r.Put("/rules/{id}", s.need(RoleAdmin, s.saveRule))
		r.Delete("/rules/{id}", s.need(RoleAdmin, deleteHandler(s, store.CollRules, "automation")))

		r.Get("/settings", s.need(RoleAdmin, s.getSettings))
		r.Put("/settings", s.need(RoleAdmin, s.putSettings))
		r.Post("/settings/test-notify", s.need(RoleAdmin, s.testNotify))

		r.Get("/log", s.need(RoleGuest, s.log))
		r.Get("/system", s.need(RoleGuest, s.system))
		r.Get("/system/app-link", s.need(RoleAdmin, s.appLink))
		r.Post("/system/api-key/rotate", s.need(RoleAdmin, s.rotateKey))
		r.Get("/system/update", s.need(RoleGuest, s.updateCheck))
	})

	if s.Web != nil {
		r.NotFound(s.Web.ServeHTTP)
	}

	return r
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		next.ServeHTTP(w, r)
	})
}

func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// PublicURL is the address clients (the Android app) use.
func (s *Server) PublicURL() string {
	if s.App.Cfg.PublicURL != "" {
		return strings.TrimRight(s.App.Cfg.PublicURL, "/")
	}

	_, port, _ := net.SplitHostPort(s.App.Cfg.Listen)
	if port == "" {
		port = "8080"
	}

	for _, n := range midea.LocalNetworks() {
		if ip := n.IP.To4(); ip != nil && isPrivate(ip) {
			return "http://" + ip.String() + ":" + port
		}
	}

	return "http://127.0.0.1:" + port
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"ok": true, "version": version.Version, "public_url": s.PublicURL()})
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	ch, cancel := s.App.Hub.Subscribe()
	defer cancel()

	// Initial snapshot so clients need no extra request.
	snapshot, _ := json.Marshal(events.Event{Type: "snapshot", Data: s.App.Views(false), Time: time.Now()})
	fmt.Fprintf(w, "retry: 3000\ndata: %s\n\n", snapshot)
	fl.Flush()

	// Wake all devices so the new client sees fresh values quickly.
	for _, v := range s.App.Views(false) {
		s.App.Wake(v.ID)
	}

	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		case e, ok := <-ch:
			if !ok {
				return
			}

			b, _ := json.Marshal(e)
			fmt.Fprintf(w, "data: %s\n\n", b)
			fl.Flush()
		}
	}
}

func (s *Server) listDevices(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.App.Views(false))
}

func (s *Server) getDevice(w http.ResponseWriter, r *http.Request) {
	v, err := s.App.View(chi.URLParam(r, "id"), false)
	if err != nil {
		fail(w, err)
		return
	}

	writeJSON(w, v)
}

func (s *Server) credentials(w http.ResponseWriter, r *http.Request) {
	v, err := s.App.View(chi.URLParam(r, "id"), true)
	if err != nil {
		fail(w, err)
		return
	}

	writeJSON(w, map[string]any{"device_id": v.ID, "ip": v.IP, "port": v.Port, "token": v.Token, "key": v.Key})
}

func (s *Server) addDevice(w http.ResponseWriter, r *http.Request) {
	var req app.AddRequest
	if !readJSON(w, r, &req) {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()

	v, err := s.App.AddDevice(ctx, req)
	if err != nil {
		fail(w, err)
		return
	}

	writeJSON(w, v)
}

func (s *Server) patchDevice(w http.ResponseWriter, r *http.Request) {
	var p app.DevicePatch
	if !readJSON(w, r, &p) {
		return
	}

	v, err := s.App.UpdateDevice(chi.URLParam(r, "id"), p)
	if err != nil {
		fail(w, err)
		return
	}

	writeJSON(w, v)
}

func (s *Server) deleteDevice(w http.ResponseWriter, r *http.Request) {
	if err := s.App.DeleteDevice(chi.URLParam(r, "id"), r.URL.Query().Get("history") == "1"); err != nil {
		fail(w, err)
		return
	}

	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) control(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var c store.Control
	if !readJSON(w, r, &c) {
		return
	}

	if v, err := s.App.View(id, false); err == nil && v.Locked && s.role(r) < RoleAdmin {
		writeErr(w, http.StatusForbidden, "this device is locked – only the administrator may change it")
		return
	}

	src := r.Header.Get("X-Client")
	if src == "" {
		src = "Web"
	}

	if _, err := s.App.Control(id, c, src); err != nil {
		fail(w, err)
		return
	}

	v, err := s.App.View(id, false)
	if err != nil {
		fail(w, err)
		return
	}

	writeJSON(w, v)
}

func (s *Server) refresh(w http.ResponseWriter, r *http.Request) {
	v, err := s.App.Refresh(chi.URLParam(r, "id"))
	if err != nil {
		fail(w, err)
		return
	}

	writeJSON(w, v)
}

func (s *Server) diagnostics(w http.ResponseWriter, r *http.Request) {
	d, err := s.App.Diagnostics(chi.URLParam(r, "id"))
	if err != nil {
		fail(w, err)
		return
	}

	writeJSON(w, d)
}

func (s *Server) rawQuery(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Payload string `json:"payload"`
	}

	if !readJSON(w, r, &req) {
		return
	}

	p, err := hex.DecodeString(strings.ReplaceAll(req.Payload, " ", ""))
	if err != nil || len(p) == 0 || len(p) > 64 {
		writeErr(w, http.StatusBadRequest, "payload must be 1–64 bytes hex")
		return
	}

	frames, err := s.App.RawQuery(chi.URLParam(r, "id"), p)
	if err != nil && len(frames) == 0 {
		fail(w, err)
		return
	}

	writeJSON(w, map[string]any{"frames": frames})
}

func (s *Server) relocate(w http.ResponseWriter, r *http.Request) {
	v, err := s.App.Relocate(chi.URLParam(r, "id"))
	if err != nil {
		fail(w, err)
		return
	}

	writeJSON(w, v)
}

func parseSpan(r *http.Request, loc *time.Location, defHours int) (time.Time, time.Time) {
	q := r.URL.Query()
	to := time.Now()
	from := to.Add(-time.Duration(defHours) * time.Hour)

	if h, err := strconv.Atoi(q.Get("hours")); err == nil && h > 0 {
		from = to.Add(-time.Duration(h) * time.Hour)
	}

	if d, err := strconv.Atoi(q.Get("days")); err == nil && d > 0 {
		from = to.AddDate(0, 0, -d)
	}

	if t, err := time.ParseInLocation(time.RFC3339, q.Get("from"), loc); err == nil {
		from = t
	}

	if t, err := time.ParseInLocation(time.RFC3339, q.Get("to"), loc); err == nil {
		to = t
	}

	return from, to
}

func (s *Server) history(w http.ResponseWriter, r *http.Request) {
	from, to := parseSpan(r, s.App.Loc, 24)
	points, _ := strconv.Atoi(r.URL.Query().Get("points"))

	if points <= 0 {
		points = 600
	}

	list, err := s.App.History(r.Context(), chi.URLParam(r, "id"), from, to, points)
	if err != nil {
		fail(w, err)
		return
	}

	if list == nil {
		list = []store.Sample{}
	}

	writeJSON(w, map[string]any{"from": from, "to": to, "points": list})
}

func (s *Server) historyCSV(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	from, to := parseSpan(r, s.App.Loc, 24*30)

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="portacontrol-%s-%s.csv"`, id, time.Now().Format("2006-01-02")))

	if err := s.App.WriteCSV(r.Context(), w, id, from, to, r.URL.Query().Get("comma") == "1"); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
	}
}

func (s *Server) energy(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	period := q.Get("period")
	offset, _ := strconv.Atoi(q.Get("offset"))
	from, to, size := s.App.EnergyRange(period, offset)

	list, err := s.App.Energy(chi.URLParam(r, "id"), size, from, to)
	if err != nil {
		fail(w, err)
		return
	}

	var total app.EnergySum

	for _, b := range list {
		total.KWh += b.KWh
		total.Cost += b.Cost
		total.CO2kg += b.CO2kg
		total.OnHours += b.OnHours
		total.Currency = b.Currency
	}

	writeJSON(w, map[string]any{"period": period, "offset": offset, "from": from, "to": to, "buckets": list, "total": total})
}

func (s *Server) getTimers(w http.ResponseWriter, r *http.Request) {
	t, _ := store.Get[store.Timers](s.App.Store, store.CollTimers, chi.URLParam(r, "id"))
	writeJSON(w, t)
}

func (s *Server) putTimers(w http.ResponseWriter, r *http.Request) {
	var t store.Timers
	if !readJSON(w, r, &t) {
		return
	}

	out, err := s.App.SetTimers(chi.URLParam(r, "id"), t)
	if err != nil {
		fail(w, err)
		return
	}

	writeJSON(w, out)
}

func (s *Server) discover(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Target string `json:"target"`
		Subnet bool   `json:"subnet"`
	}

	_ = json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req)

	list, err := s.App.Discover(strings.TrimSpace(req.Target), req.Subnet)
	if err != nil {
		fail(w, err)
		return
	}

	known := map[string]bool{}
	for _, d := range s.App.Store.Devices() {
		known[d.ID] = true
	}

	type item struct {
		midea.Discovered
		Known bool `json:"known"`
		AC    bool `json:"air_conditioner"`
	}

	out := []item{}
	for _, d := range list {
		out = append(out, item{Discovered: d, Known: known[strconv.FormatUint(d.DeviceID, 10)], AC: d.IsAirConditioner()})
	}

	writeJSON(w, out)
}

func (s *Server) cloudStatus(w http.ResponseWriter, _ *http.Request) {
	c := s.App.Store.Settings().Cloud
	writeJSON(w, map[string]any{"account": c.Account, "configured": c.Account != "" && c.Password != ""})
}

func (s *Server) cloudLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Account  string `json:"account"`
		Password string `json:"password"`
	}

	if !readJSON(w, r, &req) {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	list, err := s.App.CloudLogin(ctx, req.Account, req.Password)
	if err != nil {
		fail(w, err)
		return
	}

	writeJSON(w, map[string]any{"appliances": list})
}

func (s *Server) cloudLogout(w http.ResponseWriter, _ *http.Request) {
	if err := s.App.CloudLogout(); err != nil {
		fail(w, err)
		return
	}

	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) importBackup(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	d, sc, err := s.App.ImportBackup(ctx, raw)
	if err != nil {
		fail(w, err)
		return
	}

	writeJSON(w, map[string]any{"devices": d, "scenes": sc})
}

func (s *Server) exportBackup(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="portacontrol-backup-%s.json"`, time.Now().Format("2006-01-02")))
	w.Header().Set("Content-Type", "application/json")

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(s.App.ExportBackup())
}

func listHandler[T any](s *Server, coll string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		list, err := store.List[T](s.App.Store, coll)
		if err != nil {
			fail(w, err)
			return
		}

		if list == nil {
			list = []T{}
		}

		writeJSON(w, list)
	}
}

func deleteHandler(s *Server, coll, event string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := s.App.Store.Delete(coll, chi.URLParam(r, "id")); err != nil {
			fail(w, err)
			return
		}

		s.App.Hub.Publish(events.Event{Type: event})
		writeJSON(w, map[string]any{"ok": true})
	}
}

func (s *Server) saveScene(w http.ResponseWriter, r *http.Request) {
	var sc store.Scene
	if !readJSON(w, r, &sc) {
		return
	}

	if id := chi.URLParam(r, "id"); id != "" {
		sc.ID = id
	}

	if sc.ID == "" {
		sc.ID = store.NewID()
	}

	sc.Name = midea.CleanText(sc.Name, "Szene", 60)

	if err := app.Validate(sc.Control); err != nil {
		fail(w, err)
		return
	}

	if err := s.App.Store.Put(store.CollScenes, sc.ID, sc); err != nil {
		fail(w, err)
		return
	}

	s.App.Hub.Publish(events.Event{Type: "scenes"})
	writeJSON(w, sc)
}

func (s *Server) applyScene(w http.ResponseWriter, r *http.Request) {
	src := r.Header.Get("X-Client")
	if src == "" {
		src = "Web"
	}

	if err := s.App.ApplyScene(chi.URLParam(r, "id"), r.URL.Query().Get("device"), src); err != nil {
		fail(w, err)
		return
	}

	writeJSON(w, map[string]any{"ok": true})
}

type scheduleView struct {
	store.Schedule
	NextRun *time.Time `json:"next_run"`
}

func (s *Server) listSchedules(w http.ResponseWriter, _ *http.Request) {
	list, err := store.List[store.Schedule](s.App.Store, store.CollSchedules)
	if err != nil {
		fail(w, err)
		return
	}

	out := []scheduleView{}

	for _, sc := range list {
		v := scheduleView{Schedule: sc}
		if n := s.App.NextRun(sc, time.Now()); !n.IsZero() {
			v.NextRun = &n
		}

		out = append(out, v)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Time < out[j].Time })
	writeJSON(w, out)
}

func validAction(a store.Action) error {
	switch a.Kind {
	case "on", "off", "notify":
	case "scene":
		if a.SceneID == "" {
			return fmt.Errorf("%w: scene missing", app.ErrBadRequest)
		}
	case "control":
		if a.Control == nil {
			return fmt.Errorf("%w: control values missing", app.ErrBadRequest)
		}

		return app.Validate(*a.Control)
	default:
		return fmt.Errorf("%w: action must be on, off, scene, control or notify", app.ErrBadRequest)
	}

	if a.Control != nil {
		return app.Validate(*a.Control)
	}

	return nil
}

func (s *Server) saveSchedule(w http.ResponseWriter, r *http.Request) {
	var sc store.Schedule
	if !readJSON(w, r, &sc) {
		return
	}

	if id := chi.URLParam(r, "id"); id != "" {
		sc.ID = id
		if old, err := store.Get[store.Schedule](s.App.Store, store.CollSchedules, id); err == nil {
			sc.LastRun = old.LastRun
		}
	}

	if sc.ID == "" {
		sc.ID = store.NewID()
		sc.LastRun = time.Now()
	}

	if _, _, ok := app.ParseClock(sc.Time); !ok {
		fail(w, fmt.Errorf("%w: time must be HH:MM", app.ErrBadRequest))
		return
	}

	if err := validAction(sc.Action); err != nil {
		fail(w, err)
		return
	}

	sc.Name = midea.CleanText(sc.Name, "Programm", 60)

	if err := s.App.Store.Put(store.CollSchedules, sc.ID, sc); err != nil {
		fail(w, err)
		return
	}

	s.App.Hub.Publish(events.Event{Type: "automation"})
	writeJSON(w, sc)
}

func (s *Server) runSchedule(w http.ResponseWriter, r *http.Request) {
	sc, err := store.Get[store.Schedule](s.App.Store, store.CollSchedules, chi.URLParam(r, "id"))
	if err != nil {
		fail(w, err)
		return
	}

	if err := s.App.RunAction(sc.Action, sc.Devices, "Wochenprogramm "+sc.Name+" (manuell)"); err != nil {
		fail(w, err)
		return
	}

	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) saveRule(w http.ResponseWriter, r *http.Request) {
	var ru store.Rule
	if !readJSON(w, r, &ru) {
		return
	}

	if id := chi.URLParam(r, "id"); id != "" {
		ru.ID = id
	}

	if ru.ID == "" {
		ru.ID = store.NewID()
	}

	ru.Armed = true

	switch {
	case ru.Device == "":
		fail(w, fmt.Errorf("%w: device missing", app.ErrBadRequest))
		return
	case ru.Sensor != "indoor" && ru.Sensor != "outdoor" && ru.Sensor != "power_watts":
		fail(w, fmt.Errorf("%w: sensor must be indoor, outdoor or power_watts", app.ErrBadRequest))
		return
	case ru.Op != "above" && ru.Op != "below":
		fail(w, fmt.Errorf("%w: op must be above or below", app.ErrBadRequest))
		return
	}

	if err := validAction(ru.Action); err != nil {
		fail(w, err)
		return
	}

	ru.Name = midea.CleanText(ru.Name, "Regel", 60)

	if err := s.App.Store.Put(store.CollRules, ru.ID, ru); err != nil {
		fail(w, err)
		return
	}

	s.App.Hub.Publish(events.Event{Type: "automation"})
	writeJSON(w, ru)
}

func (s *Server) getSettings(w http.ResponseWriter, _ *http.Request) {
	st := s.App.Store.Settings()
	st.Cloud.Password, st.Cloud.Sessions, st.Cloud.ClientID = "", nil, ""

	pwSet := st.MQTT.Password != ""
	st.MQTT.Password = ""

	mqtt := ""
	if s.MQTTStatus != nil {
		mqtt = s.MQTTStatus()
	}

	writeJSON(w, map[string]any{"settings": st, "mqtt_password_set": pwSet, "mqtt_status": mqtt})
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	var st store.Settings
	if !readJSON(w, r, &st) {
		return
	}

	if st.PricePerKWh < 0 || st.PricePerKWh > 10 {
		writeErr(w, http.StatusBadRequest, "price per kWh out of range")
		return
	}

	for i := range st.Webhooks {
		if st.Webhooks[i].ID == "" {
			st.Webhooks[i].ID = store.NewID()
		}

		if u, err := url.Parse(st.Webhooks[i].URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			writeErr(w, http.StatusBadRequest, "webhook URL must start with http:// or https://")
			return
		}
	}

	// The cloud account is managed by /setup/cloud.
	st.Cloud = store.CloudSettings{}

	if err := s.App.SaveSettings(st); err != nil {
		fail(w, err)
		return
	}

	s.getSettings(w, r)
}

func (s *Server) testNotify(w http.ResponseWriter, _ *http.Request) {
	s.App.TestNotify()
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) log(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 1000 {
		limit = 200
	}

	list, err := s.App.Store.Log(r.URL.Query().Get("device"), limit)
	if err != nil {
		fail(w, err)
		return
	}

	if list == nil {
		list = []store.LogEntry{}
	}

	writeJSON(w, list)
}

func (s *Server) overview(w http.ResponseWriter, _ *http.Request) {
	views := s.App.Views(false)

	var kwh, cost, watts float64

	on := 0

	for _, v := range views {
		if v.Today != nil {
			kwh += v.Today.KWh
			cost += v.Today.Cost
		}

		if v.State != nil && v.State.Power {
			on++

			if v.State.PowerWatts != nil {
				watts += *v.State.PowerWatts
			}
		}
	}

	scheds, _ := store.List[store.Schedule](s.App.Store, store.CollSchedules)

	var next *scheduleView

	for _, sc := range scheds {
		n := s.App.NextRun(sc, time.Now())
		if n.IsZero() {
			continue
		}

		if next == nil || n.Before(*next.NextRun) {
			next = &scheduleView{Schedule: sc, NextRun: &n}
		}
	}

	st := s.App.Store.Settings()
	writeJSON(w, map[string]any{
		"devices": views, "devices_on": on, "watts": watts, "today_kwh": kwh, "today_cost": cost,
		"currency": st.Currency, "next_schedule": next,
	})
}

func (s *Server) system(w http.ResponseWriter, r *http.Request) {
	mqtt := ""
	if s.MQTTStatus != nil {
		mqtt = s.MQTTStatus()
	}

	writeJSON(w, map[string]any{
		"version": version.Version, "commit": version.Commit, "date": version.Date,
		"uptime_seconds": int(time.Since(s.started).Seconds()), "public_url": s.PublicURL(),
		"time_zone": s.App.Loc.String(), "mqtt": mqtt, "simulate": s.App.Cfg.Simulate,
		"role": roleName(s.role(r)),
	})
}

func (s *Server) appLink(w http.ResponseWriter, _ *http.Request) {
	key, err := EnsureAPIKey(s.App.Store)
	if err != nil {
		fail(w, err)
		return
	}

	u := s.PublicURL()
	link := "portacontrol://connect?url=" + url.QueryEscape(u) + "&key=" + url.QueryEscape(key)
	writeJSON(w, map[string]any{"url": u, "api_key": key, "link": link})
}

func (s *Server) rotateKey(w http.ResponseWriter, _ *http.Request) {
	if _, err := RotateAPIKey(s.App.Store); err != nil {
		fail(w, err)
		return
	}

	s.appLink(w, nil)
}

var updateCache struct {
	sync.Mutex
	at     time.Time
	latest string
	url    string
}

func (s *Server) updateCheck(w http.ResponseWriter, r *http.Request) {
	updateCache.Lock()
	defer updateCache.Unlock()

	if time.Since(updateCache.at) > 6*time.Hour {
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()

		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/mortendnk/PortaControl/releases/latest", nil)
		req.Header.Set("Accept", "application/vnd.github+json")

		if resp, err := http.DefaultClient.Do(req); err == nil {
			var rel struct {
				Tag string `json:"tag_name"`
				URL string `json:"html_url"`
			}

			if resp.StatusCode == http.StatusOK && json.NewDecoder(resp.Body).Decode(&rel) == nil {
				updateCache.latest, updateCache.url = rel.Tag, rel.URL
			}

			_ = resp.Body.Close()
			updateCache.at = time.Now()
		}
	}

	cur := version.Version
	writeJSON(w, map[string]any{
		"current": cur, "latest": updateCache.latest, "url": updateCache.url,
		"available": updateCache.latest != "" && cur != "dev" && updateCache.latest != cur,
	})
}
