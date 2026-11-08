package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mortendnk/portacontrol/server/internal/events"
	"github.com/mortendnk/portacontrol/server/internal/midea"
	"github.com/mortendnk/portacontrol/server/internal/store"
)

// Discover searches the LAN (broadcast, optionally a subnet scan or one IP).
func (a *App) Discover(target string, subnet bool) ([]midea.Discovered, error) {
	var targets []string

	switch {
	case target != "":
		if !midea.IsIPv4(target) {
			return nil, fmt.Errorf("%w: %q is no IPv4 address", ErrBadRequest, target)
		}

		targets = []string{target}
	case subnet:
		targets = append(midea.BroadcastTargets(), midea.SubnetTargets()...)
	}

	list, err := midea.Scan(targets, 4*time.Second, nil)
	if err != nil {
		return nil, err
	}

	if sim := a.sim; sim != nil {
		list = append(list, midea.Discovered{IP: "127.0.0.1", Port: sim.Port, DeviceID: sim.DeviceID, SN: sim.SN,
			Name: sim.Name, Type: 0xAC, Version: 3})
	}

	return list, nil
}

// CloudLogin stores the Midea account and lists its devices.
func (a *App) CloudLogin(ctx context.Context, account, password string) ([]midea.CloudAppliance, error) {
	account = strings.TrimSpace(account)
	if account == "" || password == "" {
		return nil, fmt.Errorf("%w: account and password required", ErrBadRequest)
	}

	s := a.Store.Settings()
	if s.Cloud.ClientID == "" {
		s.Cloud.ClientID = store.NewID()
	}

	acc := midea.NewCloudAccount(account, password, s.Cloud.ClientID, nil)
	acc.OnSession = a.saveCloudSession

	list, err := acc.Appliances(ctx)
	if err != nil {
		return nil, err
	}

	s = a.Store.Settings()
	s.Cloud.Account, s.Cloud.Password = account, password
	s.Cloud.ClientID = acc.ClientID

	if err := a.Store.SaveSettings(s); err != nil {
		return nil, err
	}

	a.reloadCloud()
	a.logf("", "info", "Midea-Konto %s angemeldet, %d Anlage(n) im Konto", maskAccount(account), len(list))

	return list, nil
}

// CloudLogout forgets the account.
func (a *App) CloudLogout() error {
	s := a.Store.Settings()
	s.Cloud = store.CloudSettings{ClientID: s.Cloud.ClientID}

	if err := a.Store.SaveSettings(s); err != nil {
		return err
	}

	a.reloadCloud()

	return nil
}

func maskAccount(s string) string {
	at := strings.Index(s, "@")
	if at <= 1 {
		return "***"
	}

	return s[:1] + "***" + s[at:]
}

// AddRequest adds a device: either from discovery + cloud token, or manual.
type AddRequest struct {
	Name     string `json:"name"`
	Room     string `json:"room"`
	IP       string `json:"ip"`
	Port     int    `json:"port"`
	DeviceID string `json:"device_id"`
	Token    string `json:"token"`
	Key      string `json:"key"`
	SN       string `json:"sn"`
	// FromCloud fetches token/key with the stored Midea account.
	FromCloud bool `json:"from_cloud"`
	// CloudOnly adds a device without LAN credentials (cloud control only).
	CloudOnly bool   `json:"cloud_only"`
	Mode      string `json:"connection"`
}

// AddDevice verifies and stores a device.
func (a *App) AddDevice(ctx context.Context, r AddRequest) (View, error) {
	id, err := strconv.ParseUint(strings.TrimSpace(r.DeviceID), 10, 64)
	if err != nil || id == 0 {
		return View{}, fmt.Errorf("%w: device id must be a decimal number", ErrBadRequest)
	}

	if r.Port == 0 {
		r.Port = 6444
	}

	cfg := midea.DeviceConfig{DeviceID: id, IP: strings.TrimSpace(r.IP), Port: r.Port,
		Token: strings.ToLower(strings.TrimSpace(r.Token)), Key: strings.ToLower(strings.TrimSpace(r.Key))}

	switch {
	case r.CloudOnly:
		if a.Cloud() == nil {
			return View{}, fmt.Errorf("%w: sign in with the Midea account first", ErrBadRequest)
		}

		cfg.IP, cfg.Token, cfg.Key = "", "", ""
		r.Mode = string(midea.ConnCloud)
	case r.FromCloud:
		acc := a.Cloud()
		if acc == nil {
			return View{}, fmt.Errorf("%w: sign in with the Midea account first", ErrBadRequest)
		}

		acc.ResetPause()

		got, err := midea.FetchConfig(ctx, acc, midea.Discovered{IP: cfg.IP, Port: cfg.Port, DeviceID: id})
		if err != nil {
			return View{}, err
		}

		cfg = got
	default:
		if err := cfg.Validate(); err != nil {
			return View{}, fmt.Errorf("%w: %v", ErrBadRequest, err)
		}

		if err := midea.Verify(cfg); err != nil {
			return View{}, fmt.Errorf("check at the device failed: %w", err)
		}
	}

	name := midea.CleanText(r.Name, "PortaSplit", 60)
	mode := r.Mode

	if mode == "" {
		mode = string(midea.ConnAuto)
	}

	rec := store.Device{ID: strconv.FormatUint(id, 10), Name: name, Room: midea.CleanText(r.Room, "", 40), IP: cfg.IP, Port: cfg.Port,
		Token: cfg.Token, Key: cfg.Key, SN: midea.CleanText(r.SN, "", 40), Mode: mode, CreatedAt: time.Now(), Order: len(a.Store.Devices())}

	if old, err := store.Get[store.Device](a.Store, store.CollDevices, rec.ID); err == nil {
		rec.CreatedAt, rec.Order, rec.Favorite = old.CreatedAt, old.Order, old.Favorite
		if r.Name == "" {
			rec.Name = old.Name
		}
	}

	if err := a.Store.Put(store.CollDevices, rec.ID, rec); err != nil {
		return View{}, err
	}

	if err := a.startDevice(rec); err != nil {
		return View{}, err
	}

	a.logf(rec.ID, "info", "Anlage %s eingerichtet (%s)", rec.Name, rec.IP)
	a.devicesChanged()

	return a.View(rec.ID, false)
}

// DevicePatch changes the settings of a device.
type DevicePatch struct {
	Name      *string  `json:"name"`
	Room      *string  `json:"room"`
	IP        *string  `json:"ip"`
	Port      *int     `json:"port"`
	Mode      *string  `json:"connection"`
	Order     *int     `json:"order"`
	Favorite  *bool    `json:"favorite"`
	MinTarget *float64 `json:"min_target"`
	MaxTarget *float64 `json:"max_target"`
	Locked    *bool    `json:"locked"`
	Token     *string  `json:"token"`
	Key       *string  `json:"key"`
}

// UpdateDevice applies a patch and restarts the device runtime if needed.
func (a *App) UpdateDevice(id string, p DevicePatch) (View, error) {
	rec, err := store.Get[store.Device](a.Store, store.CollDevices, id)
	if err != nil {
		return View{}, ErrUnknownDevice
	}

	restart := false

	if p.Name != nil {
		rec.Name = midea.CleanText(*p.Name, rec.Name, 60)
	}

	if p.Room != nil {
		rec.Room = midea.CleanText(*p.Room, "", 40)
	}

	if p.IP != nil && *p.IP != rec.IP {
		if !midea.IsIPv4(*p.IP) {
			return View{}, fmt.Errorf("%w: invalid IPv4 address", ErrBadRequest)
		}

		rec.IP, restart = *p.IP, true
	}

	if p.Port != nil && *p.Port != rec.Port {
		rec.Port, restart = *p.Port, true
	}

	if p.Token != nil && p.Key != nil {
		rec.Token, rec.Key, restart = strings.ToLower(*p.Token), strings.ToLower(*p.Key), true
	}

	if p.Mode != nil {
		switch midea.ConnMode(*p.Mode) {
		case midea.ConnAuto, midea.ConnLAN, midea.ConnCloud:
			rec.Mode = *p.Mode
		default:
			return View{}, fmt.Errorf("%w: connection must be auto, lan or cloud", ErrBadRequest)
		}
	}

	if p.Order != nil {
		rec.Order = *p.Order
	}

	if p.Favorite != nil {
		rec.Favorite = *p.Favorite
	}

	if p.MinTarget != nil {
		rec.MinTarget = *p.MinTarget
	}

	if p.MaxTarget != nil {
		rec.MaxTarget = *p.MaxTarget
	}

	if p.Locked != nil {
		rec.Locked = *p.Locked
	}

	if err := (midea.DeviceConfig{IP: rec.IP, Port: rec.Port, Token: rec.Token, Key: rec.Key}).Validate(); err != nil {
		return View{}, fmt.Errorf("%w: %v", ErrBadRequest, err)
	}

	if err := a.Store.Put(store.CollDevices, id, rec); err != nil {
		return View{}, err
	}

	e, err := a.entry(id)
	if err != nil || restart {
		if err := a.startDevice(rec); err != nil {
			return View{}, err
		}
	} else {
		e.mu.Lock()
		e.Rec = rec
		e.mu.Unlock()

		if p.Mode != nil {
			e.Dev.SetMode(midea.ConnMode(rec.Mode))
		}
	}

	a.devicesChanged()

	return a.View(id, false)
}

// DeleteDevice removes a device (history optional).
func (a *App) DeleteDevice(id string, history bool) error {
	if err := a.Store.Delete(store.CollDevices, id); err != nil {
		return ErrUnknownDevice
	}

	a.mu.Lock()
	if e, ok := a.entries[id]; ok {
		close(e.stop)
		e.Dev.Close()
		delete(a.entries, id)
	}
	a.mu.Unlock()

	_ = a.Store.Delete(store.CollTimers, id)

	if history {
		_ = a.Store.DeleteSamples(id)
	}

	a.logf(id, "info", "Anlage entfernt")
	a.devicesChanged()

	return nil
}

func (a *App) devicesChanged() {
	a.Hub.Publish(events.Event{Type: "devices"})

	if a.OnDevices != nil {
		a.OnDevices()
	}
}

// Relocate searches the device on the LAN (after an IP change) and takes
// the new address only after a handshake with the stored token worked.
func (a *App) Relocate(id string) (View, error) {
	rec, err := store.Get[store.Device](a.Store, store.CollDevices, id)
	if err != nil {
		return View{}, ErrUnknownDevice
	}

	did, _ := strconv.ParseUint(id, 10, 64)

	d, err := midea.Find(did, 4*time.Second)
	if err != nil {
		return View{}, err
	}

	cfg := midea.DeviceConfig{DeviceID: did, IP: d.IP, Port: d.Port, Token: rec.Token, Key: rec.Key}
	if err := midea.Verify(cfg); err != nil {
		return View{}, fmt.Errorf("found at %s, but the handshake failed: %w", d.IP, err)
	}

	ip, port := d.IP, d.Port

	a.logf(id, "info", "%s: neue Adresse %s", rec.Name, ip)

	return a.UpdateDevice(id, DevicePatch{IP: &ip, Port: &port})
}

// ImportBackup reads a Klimacontrol/msmart backup (devices + scenes) or a
// PortaControl backup.
func (a *App) ImportBackup(ctx context.Context, raw []byte) (int, int, error) {
	var b store.Backup
	if err := json.Unmarshal(raw, &b); err != nil {
		return 0, 0, fmt.Errorf("%w: no backup file (%v)", ErrBadRequest, err)
	}

	devices, scenes := 0, 0

	if len(b.Devices) > 0 {
		for _, d := range b.Devices {
			if err := a.Store.Put(store.CollDevices, d.ID, d); err == nil {
				_ = a.startDevice(d)
				devices++
			}
		}

		for _, s := range b.Scenes {
			if a.Store.Put(store.CollScenes, s.ID, s) == nil {
				scenes++
			}
		}

		for _, s := range b.Schedules {
			_ = a.Store.Put(store.CollSchedules, s.ID, s)
		}

		for _, r := range b.Rules {
			_ = a.Store.Put(store.CollRules, r.ID, r)
		}
	} else {
		var errs []error

		for _, d := range b.KCDevices {
			if d.Version != 0 && d.Version != 3 {
				errs = append(errs, fmt.Errorf("protocol V%d is not supported (only V3)", d.Version))
				continue
			}

			_, err := a.AddDevice(ctx, AddRequest{Name: d.DiscoveredName, IP: d.IP, Port: d.Port,
				DeviceID: strconv.FormatUint(d.DeviceID, 10), Token: d.Token, Key: d.Key})
			if err != nil {
				errs = append(errs, err)
				continue
			}

			devices++
		}

		for _, s := range b.KCScenes {
			if a.Store.Put(store.CollScenes, s.ID, sceneFromKC(s)) == nil {
				scenes++
			}
		}

		if devices == 0 && len(errs) > 0 {
			return 0, scenes, errors.Join(errs...)
		}
	}

	a.devicesChanged()
	a.Hub.Publish(events.Event{Type: "scenes"})

	return devices, scenes, nil
}

func sceneFromKC(s store.KCScene) store.Scene {
	modes := map[int]string{1: "auto", 2: "cool", 3: "dry", 4: "heat", 5: "fan"}
	mode := modes[s.Mode]

	if mode == "" {
		mode = "auto"
	}

	t := s.TempC
	fan := s.Fan
	c := store.Control{Power: &s.PowerOn}

	if s.PowerOn {
		swing, eco, anion, silent := s.Swing, s.Eco, s.Anion, s.OutdoorSilent
		c.Mode, c.Target, c.Fan, c.Swing, c.Eco, c.Purifier, c.OutdoorSilent = &mode, &t, &fan, &swing, &eco, &anion, &silent
	}

	id := s.ID
	if id == "" {
		id = store.NewID()
	}

	return store.Scene{ID: id, Name: midea.CleanText(s.Name, "Szene", 60), Icon: "sparkles", Control: c}
}

// ExportBackup returns everything except history; secrets included.
func (a *App) ExportBackup() store.Backup {
	b := store.Backup{Version: 1, Exported: time.Now(), Devices: a.Store.Devices(), Settings: a.Store.Settings()}
	b.Scenes, _ = store.List[store.Scene](a.Store, store.CollScenes)
	b.Schedules, _ = store.List[store.Schedule](a.Store, store.CollSchedules)
	b.Rules, _ = store.List[store.Rule](a.Store, store.CollRules)

	for _, d := range b.Devices {
		id, _ := strconv.ParseUint(d.ID, 10, 64)
		if d.Token == "" {
			continue
		}

		b.KCDevices = append(b.KCDevices, store.KCDevice{DeviceID: id, DiscoveredName: d.Name, IP: d.IP, Key: d.Key,
			Port: d.Port, Token: d.Token, Type: 172, Version: 3})
	}

	modes := map[string]int{"auto": 1, "cool": 2, "dry": 3, "heat": 4, "fan": 5}

	for _, s := range b.Scenes {
		kc := store.KCScene{ID: s.ID, Name: s.Name, Mode: 1, TempC: 24, Fan: 102}
		c := s.Control

		if c.Power != nil {
			kc.PowerOn = *c.Power
		}

		if c.Mode != nil {
			kc.Mode = modes[*c.Mode]
		}

		if c.Target != nil {
			kc.TempC = *c.Target
		}

		if c.Fan != nil {
			kc.Fan = *c.Fan
		}

		kc.Swing = c.Swing != nil && *c.Swing
		kc.Eco = c.Eco != nil && *c.Eco
		kc.Anion = c.Purifier != nil && *c.Purifier
		kc.OutdoorSilent = c.OutdoorSilent != nil && *c.OutdoorSilent
		b.KCScenes = append(b.KCScenes, kc)
	}

	return b
}

// SaveSettings stores settings and applies them to the running devices.
func (a *App) SaveSettings(s store.Settings) error {
	old := a.Store.Settings()
	// Secrets that are not sent to clients stay unless replaced.
	if s.Cloud.Password == "" {
		s.Cloud = old.Cloud
	}

	if s.MQTT.Password == "" {
		s.MQTT.Password = old.MQTT.Password
	}

	if err := a.Store.SaveSettings(s); err != nil {
		return err
	}

	a.mu.RLock()
	for _, e := range a.entries {
		go e.Dev.SetOptions(s.Beep, time.Duration(s.IdleDisconnect)*time.Second)
	}
	a.mu.RUnlock()

	a.Hub.Publish(events.Event{Type: "settings"})

	if a.OnSettings != nil {
		a.OnSettings()
	}

	return nil
}
