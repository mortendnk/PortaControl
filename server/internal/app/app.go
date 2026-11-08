// Package app wires devices, polling, history, automation and notifications.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/mortendnk/portacontrol/server/internal/config"
	"github.com/mortendnk/portacontrol/server/internal/events"
	"github.com/mortendnk/portacontrol/server/internal/fakeac"
	"github.com/mortendnk/portacontrol/server/internal/midea"
	"github.com/mortendnk/portacontrol/server/internal/store"
)

// ErrUnknownDevice is returned for unknown device ids.
var ErrUnknownDevice = errors.New("unknown device")

// App is the running service.
type App struct {
	Cfg   config.Config
	Store *store.Store
	Hub   *events.Hub
	Log   *slog.Logger
	Loc   *time.Location

	mu      sync.RWMutex
	entries map[string]*Entry
	cloud   *midea.CloudAccount
	sim     *fakeac.AC
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup

	// OnState is called after every state change (MQTT bridge).
	OnState func(id string)
	// OnDevices is called when devices were added or removed.
	OnDevices func()
	// OnSettings is called after settings were saved.
	OnSettings func()

	notifyMu sync.Mutex
}

// Entry is the runtime of one device.
type Entry struct {
	mu         sync.Mutex
	Rec        store.Device
	Dev        *midea.Device
	State      *midea.State
	Online     bool
	Error      string
	LastSeen   time.Time
	lastFull   time.Time
	lastSample time.Time
	fails      int
	wake       chan struct{}
	stop       chan struct{}
	offlineMsg bool
	busyJob    bool
}

// New creates the app (devices are started with Start).
func New(cfg config.Config, st *store.Store, log *slog.Logger) *App {
	if log == nil {
		log = slog.Default()
	}

	return &App{Cfg: cfg, Store: st, Hub: events.New(), Log: log, Loc: cfg.Location(), entries: map[string]*Entry{}}
}

// Start loads devices and starts polling and automation.
func (a *App) Start(ctx context.Context) error {
	a.ctx, a.cancel = context.WithCancel(ctx)

	if a.Cfg.Simulate {
		if err := a.startSimulator(); err != nil {
			a.Log.Warn("simulator", "err", err)
		}
	}

	a.reloadCloud()

	for _, d := range a.Store.Devices() {
		if err := a.startDevice(d); err != nil {
			a.logf(d.ID, "error", "device %s not started: %v", d.Name, err)
		}
	}

	a.wg.Add(2)

	go a.automationLoop()
	go a.maintenanceLoop()

	return nil
}

// Close stops everything.
func (a *App) Close() {
	if a.cancel != nil {
		a.cancel()
	}

	a.mu.Lock()
	for _, e := range a.entries {
		close(e.stop)
		e.Dev.Close()
	}

	a.entries = map[string]*Entry{}
	a.mu.Unlock()

	a.wg.Wait()

	if a.sim != nil {
		a.sim.Close()
	}
}

func (a *App) startSimulator() error {
	ac := fakeac.New(99000000000001)
	if err := ac.Listen("127.0.0.1:0", ""); err != nil {
		return err
	}

	a.sim = ac

	for _, d := range a.Store.Devices() {
		if d.Simulated {
			_ = a.Store.Delete(store.CollDevices, d.ID)
		}
	}

	rec := store.Device{ID: strconv.FormatUint(ac.DeviceID, 10), Name: "PortaSplit (Demo)", Room: "Demo", IP: "127.0.0.1", Port: ac.Port,
		Token: ac.TokenHex(), Key: ac.KeyHex(), Mode: string(midea.ConnLAN), Simulated: true, CreatedAt: time.Now()}

	return a.Store.Put(store.CollDevices, rec.ID, rec)
}

func (a *App) logf(device, level, format string, args ...any) store.LogEntry {
	text := fmt.Sprintf(format, args...)
	e := a.Store.AddLog(device, level, text)

	switch level {
	case "error":
		a.Log.Error(text, "device", device)
	case "warn":
		a.Log.Warn(text, "device", device)
	default:
		a.Log.Info(text, "device", device)
	}

	a.Hub.Publish(events.Event{Type: "log", Device: device, Data: e})

	return e
}

// reloadCloud (re)creates the cloud account from the settings.
func (a *App) reloadCloud() {
	s := a.Store.Settings()

	var acc *midea.CloudAccount

	if s.Cloud.Account != "" && s.Cloud.Password != "" {
		if s.Cloud.ClientID == "" {
			s.Cloud.ClientID = store.NewID()
			_ = a.Store.SaveSettings(s)
		}

		acc = midea.NewCloudAccount(s.Cloud.Account, s.Cloud.Password, s.Cloud.ClientID, s.Cloud.Sessions)
		acc.OnSession = a.saveCloudSession
	}

	a.mu.Lock()
	a.cloud = acc
	for _, e := range a.entries {
		e.Dev.SetCloud(acc)
	}
	a.mu.Unlock()
}

func (a *App) saveCloudSession(cs *midea.CloudSession) {
	s := a.Store.Settings()
	kept := []*midea.CloudSession{cs}

	for _, old := range s.Cloud.Sessions {
		if old != nil && old.Kind != cs.Kind {
			kept = append(kept, old)
		}
	}

	s.Cloud.Sessions = kept
	_ = a.Store.SaveSettings(s)
}

// Cloud returns the cloud account (nil if none).
func (a *App) Cloud() *midea.CloudAccount {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return a.cloud
}

func (a *App) startDevice(rec store.Device) error {
	id, err := strconv.ParseUint(rec.ID, 10, 64)
	if err != nil {
		return fmt.Errorf("bad device id %q", rec.ID)
	}

	if rec.Port == 0 {
		rec.Port = 6444
	}

	cfg := midea.DeviceConfig{DeviceID: id, IP: rec.IP, Port: rec.Port, Token: rec.Token, Key: rec.Key}

	mode := midea.ConnMode(rec.Mode)
	if mode == "" {
		mode = midea.ConnAuto
	}

	dev, err := midea.NewDevice(cfg, mode, a.Cloud())
	if err != nil {
		return err
	}

	s := a.Store.Settings()
	dev.Beep = s.Beep
	dev.IdleDisconnect = time.Duration(s.IdleDisconnect) * time.Second
	dev.Debugf = func(format string, args ...any) { a.Log.Debug(fmt.Sprintf(format, args...), "device", rec.ID) }

	e := &Entry{Rec: rec, Dev: dev, wake: make(chan struct{}, 1), stop: make(chan struct{})}

	a.mu.Lock()
	if old, ok := a.entries[rec.ID]; ok {
		close(old.stop)
		old.Dev.Close()
	}

	a.entries[rec.ID] = e
	a.mu.Unlock()

	a.wg.Add(1)

	go a.pollLoop(e)

	return nil
}

func (a *App) entry(id string) (*Entry, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	e, ok := a.entries[id]
	if !ok {
		return nil, ErrUnknownDevice
	}

	return e, nil
}

// Wake triggers an immediate poll of a device.
func (a *App) Wake(id string) {
	if e, err := a.entry(id); err == nil {
		e.Dev.ResetBackoff()

		select {
		case e.wake <- struct{}{}:
		default:
		}
	}
}

func (a *App) pollLoop(e *Entry) {
	defer a.wg.Done()

	timer := time.NewTimer(500 * time.Millisecond)
	defer timer.Stop()

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-e.stop:
			return
		case <-e.wake:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		case <-timer.C:
		}

		a.poll(e)

		s := a.Store.Settings()
		interval := time.Duration(s.PollSeconds) * time.Second

		if a.Hub.Count() > 0 {
			interval = time.Duration(s.FastPollSeconds) * time.Second
		}

		e.mu.Lock()
		if !e.Online && e.fails > 3 {
			// Back off while the device is unreachable.
			interval = max(interval, time.Duration(min(e.fails, 10))*30*time.Second)
		}
		e.mu.Unlock()

		timer.Reset(interval)
	}
}

func (a *App) poll(e *Entry) {
	s := a.Store.Settings()

	e.mu.Lock()
	full := time.Since(e.lastFull) >= time.Duration(s.PollSeconds)*time.Second-time.Second
	busy := e.busyJob
	e.mu.Unlock()

	if busy {
		return
	}

	st, err := e.Dev.Refresh(full)
	a.afterRead(e, st, err, full)
}

// afterRead updates the entry, history, rules and notifications.
func (a *App) afterRead(e *Entry, st midea.State, err error, full bool) {
	s := a.Store.Settings()
	now := time.Now()

	e.mu.Lock()

	if err != nil {
		e.fails++
		e.Error = err.Error()

		wasOnline := e.Online
		if e.fails >= 3 {
			e.Online = false
		}

		goneOffline := wasOnline && !e.Online
		notify := goneOffline && !e.offlineMsg

		if notify {
			e.offlineMsg = true
		}

		name := e.Rec.Name
		e.mu.Unlock()

		if e.fails == 1 || goneOffline {
			a.logf(e.Rec.ID, "warn", "%s: %v", name, err)
		}

		if goneOffline {
			a.publishDevice(e)
		}

		if notify && s.Notify.Offline {
			a.notify(e.Rec.ID, "offline", name+": nicht erreichbar", err.Error())
		}

		return
	}

	prev := e.State
	e.State = &st
	backOnline := !e.Online && e.offlineMsg
	e.Online = true
	e.Error = ""
	e.fails = 0
	e.offlineMsg = false
	e.LastSeen = now

	if full {
		e.lastFull = now
	}

	doSample := now.Sub(e.lastSample) >= time.Duration(s.SampleSeconds)*time.Second
	if doSample {
		e.lastSample = now
	}

	rec := e.Rec
	e.mu.Unlock()

	if doSample {
		_ = a.Store.AddSample(store.Sample{
			Device: rec.ID, TS: now.Unix(), Power: st.Power, Mode: st.ModeName, Target: st.TargetTemperature,
			Indoor: st.IndoorTemperature, Outdoor: st.OutdoorTemp, Watts: st.PowerWatts, KWh: st.TotalEnergyKWh, Fan: st.FanSpeed,
		})
	}

	if backOnline && s.Notify.Offline {
		a.notify(rec.ID, "online", rec.Name+": wieder erreichbar", "")
	}

	a.checkTransitions(rec, prev, &st, s)
	a.publishDevice(e)

	if significant(prev, &st) {
		a.StateWebhook(rec.ID)
	}

	if a.OnState != nil {
		a.OnState(rec.ID)
	}

	a.evaluateRules(rec.ID, &st)
}

func (a *App) checkTransitions(rec store.Device, prev, cur *midea.State, s store.Settings) {
	if prev == nil {
		return
	}

	if cur.FilterAlert && !prev.FilterAlert && s.Notify.FilterAlert {
		a.notify(rec.ID, "filter", rec.Name+": Filter reinigen", "Die Anlage meldet, dass der Filter gereinigt werden sollte.")
	}

	if cur.ErrorCode != 0 && cur.ErrorCode != prev.ErrorCode && s.Notify.ErrorCode {
		a.notify(rec.ID, "error", fmt.Sprintf("%s: Fehlercode %d", rec.Name, cur.ErrorCode), ErrorCodeText(cur.ErrorCode))
	}

	if prev.SelfClean != nil && cur.SelfClean != nil && *prev.SelfClean && !*cur.SelfClean {
		a.notify(rec.ID, "selfclean", rec.Name+": Selbstreinigung beendet", "")
	}

	if t := cur.IndoorTemperature; t != nil && prev.IndoorTemperature != nil {
		if m := s.Notify.TempAlarmMax; m != nil && *t > *m && *prev.IndoorTemperature <= *m {
			a.notify(rec.ID, "temperature", fmt.Sprintf("%s: %.1f °C – über %.1f °C", rec.Name, *t, *m), "")
		}

		if m := s.Notify.TempAlarmMin; m != nil && *t < *m && *prev.IndoorTemperature >= *m {
			a.notify(rec.ID, "temperature", fmt.Sprintf("%s: %.1f °C – unter %.1f °C", rec.Name, *t, *m), "")
		}
	}

	if prev.Power != cur.Power {
		state := "aus"
		if cur.Power {
			state = "ein"
		}

		a.logf(rec.ID, "info", "%s: %s", rec.Name, state)
	}
}

// significant reports a change worth a state webhook (not every poll).
func significant(prev, cur *midea.State) bool {
	if prev == nil {
		return true
	}

	round := func(v *float64) float64 {
		if v == nil {
			return -999
		}

		return math.Round(*v)
	}

	return prev.Power != cur.Power || prev.Mode != cur.Mode || prev.TargetTemperature != cur.TargetTemperature ||
		prev.FanSpeed != cur.FanSpeed || prev.Eco != cur.Eco || prev.Turbo != cur.Turbo || prev.Sleep != cur.Sleep ||
		round(prev.IndoorTemperature) != round(cur.IndoorTemperature) || prev.ErrorCode != cur.ErrorCode
}

// ErrorCodeText is the notification text for an error code. The meaning
// of the codes differs between models, so it points to the manual.
func ErrorCodeText(code int) string {
	return fmt.Sprintf("Die Anlage meldet Fehlercode %d. Bedeutung siehe Fehlertabelle in der Bedienungsanleitung.", code)
}

// View is the API representation of a device.
type View struct {
	store.Device
	State     *midea.State `json:"state"`
	Online    bool         `json:"online"`
	Error     string       `json:"error,omitempty"`
	LastSeen  *time.Time   `json:"last_seen"`
	Path      string       `json:"path"`
	Timers    store.Timers `json:"timers"`
	HasLAN    bool         `json:"has_lan"`
	HasCloud  bool         `json:"has_cloud"`
	Today     *EnergySum   `json:"today,omitempty"`
	CostHour  *float64     `json:"cost_per_hour,omitempty"`
	Available Availability `json:"available"`
}

// Availability tells which functions are usable in the current mode
// (like the official app greys them out).
type Availability struct {
	Eco           bool `json:"eco"`
	Turbo         bool `json:"turbo"`
	OutdoorSilent bool `json:"outdoor_silent"`
	Freeze        bool `json:"freeze_protection"`
	Sleep         bool `json:"sleep"`
	Target        bool `json:"target"`
	Fan           bool `json:"fan"`
}

func availability(s *midea.State) Availability {
	if s == nil {
		return Availability{}
	}

	m := s.Mode

	return Availability{
		Eco:           m == midea.ModeCool,
		Turbo:         m == midea.ModeCool || m == midea.ModeHeat,
		OutdoorSilent: m == midea.ModeCool || m == midea.ModeHeat,
		Freeze:        m == midea.ModeHeat,
		Sleep:         m == midea.ModeCool || m == midea.ModeHeat || m == midea.ModeAuto,
		Target:        m != midea.ModeFan,
		Fan:           m != midea.ModeDry,
	}
}

// Views returns all devices.
func (a *App) Views(withSecrets bool) []View {
	a.mu.RLock()
	ids := make([]string, 0, len(a.entries))
	for id := range a.entries {
		ids = append(ids, id)
	}
	a.mu.RUnlock()

	out := []View{}

	for _, rec := range a.Store.Devices() {
		for _, id := range ids {
			if id == rec.ID {
				if v, err := a.View(id, withSecrets); err == nil {
					out = append(out, v)
				}
			}
		}
	}

	return out
}

// View returns one device.
func (a *App) View(id string, withSecrets bool) (View, error) {
	e, err := a.entry(id)
	if err != nil {
		return View{}, err
	}

	e.mu.Lock()
	v := View{Device: e.Rec, Online: e.Online, Error: e.Error, Path: e.Dev.Path()}

	if e.State != nil {
		s := *e.State
		v.State = &s
	}

	if !e.LastSeen.IsZero() {
		t := e.LastSeen
		v.LastSeen = &t
	}
	e.mu.Unlock()

	v.HasLAN = v.Token != "" && v.Key != "" && v.IP != ""
	v.HasCloud = a.Cloud() != nil

	if !withSecrets {
		v.Token, v.Key = "", ""
	}

	v.Timers, _ = store.Get[store.Timers](a.Store, store.CollTimers, id)
	v.Available = availability(v.State)

	if sum, err := a.EnergyToday(id); err == nil {
		v.Today = &sum
	}

	if v.State != nil && v.State.PowerWatts != nil {
		c := math.Round(*v.State.PowerWatts/1000*a.Store.Settings().PricePerKWh*1000) / 1000
		v.CostHour = &c
	}

	return v, nil
}

func (a *App) publishDevice(e *Entry) {
	v, err := a.View(e.Rec.ID, false)
	if err == nil {
		a.Hub.Publish(events.Event{Type: "device", Device: e.Rec.ID, Data: v})
	}
}

// Diagnostics reads the diagnostic values.
func (a *App) Diagnostics(id string) (midea.Diagnostics, error) {
	e, err := a.entry(id)
	if err != nil {
		return midea.Diagnostics{}, err
	}

	return e.Dev.Diagnostics()
}

// RawQuery sends a read-only query frame payload (hex).
func (a *App) RawQuery(id string, payload []byte) ([]string, error) {
	e, err := a.entry(id)
	if err != nil {
		return nil, err
	}

	return e.Dev.Query(payload)
}

func (a *App) maintenanceLoop() {
	defer a.wg.Done()

	t := time.NewTicker(6 * time.Hour)
	defer t.Stop()

	_ = a.Store.Prune(a.Store.Settings().HistoryDays)

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-t.C:
			_ = a.Store.Prune(a.Store.Settings().HistoryDays)
		}
	}
}

// Simulator returns the built-in simulator (nil if off).
func (a *App) Simulator() *fakeac.AC { return a.sim }
