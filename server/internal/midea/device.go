package midea

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"
)

// ConnMode selects how the device is reached.
type ConnMode string

// Connection modes.
const (
	ConnAuto  ConnMode = "auto"  // LAN first, cloud as fallback
	ConnLAN   ConnMode = "lan"   // LAN only
	ConnCloud ConnMode = "cloud" // cloud only
)

// ErrNotApplied is returned when the device did not take a change after
// three attempts; the error carries the current state.
type ErrNotApplied struct {
	State  State
	Fields []string
}

func (e *ErrNotApplied) Error() string {
	return fmt.Sprintf("the device did not apply the change %v – maybe another app controls it right now", e.Fields)
}

// DeviceConfig are the connection data of one device.
type DeviceConfig struct {
	DeviceID uint64
	IP       string
	Port     int
	Token    string // 128 hex chars
	Key      string // 64 hex chars
}

// HasLAN reports whether LAN credentials exist.
func (c DeviceConfig) HasLAN() bool { return c.IP != "" && c.Token != "" && c.Key != "" }

// Validate checks address and credentials.
func (c DeviceConfig) Validate() error {
	if c.IP == "" && c.Token == "" && c.Key == "" {
		return nil
	}

	if !IsIPv4(c.IP) {
		return errors.New("the address of the device is not a valid IPv4 address")
	}

	if c.Port <= 0 || c.Port > 65535 {
		return errors.New("invalid port")
	}

	t, err1 := hex.DecodeString(c.Token)
	k, err2 := hex.DecodeString(c.Key)

	if err1 != nil || err2 != nil || len(t) != 64 || len(k) != 32 {
		return errors.New("token or key are no valid hex (expected 128 and 64 characters)")
	}

	return nil
}

// Device controls one air conditioner over LAN and/or cloud. All jobs run
// serialised; after IdleDisconnect the LAN connection is closed because the
// device accepts only one connection at a time.
type Device struct {
	Config DeviceConfig

	// Beep: signal tone with commands.
	Beep           bool
	IdleDisconnect time.Duration
	CommandBudget  time.Duration
	Debugf         func(format string, args ...any)

	mu        sync.Mutex
	lan       *LANClient
	cloud     *CloudAccount
	mode      ConnMode
	path      string
	last      *State
	lastAt    time.Time
	idleTimer *time.Timer

	lanFailedAt       time.Time
	lanRetry          time.Duration
	cloudOfflineUntil time.Time
}

const (
	lanRetryInterval     = 60 * time.Second
	lanMaxRetryInterval  = 300 * time.Second
	cloudOfflineInterval = 5 * time.Minute
	freshStateAge        = 3 * time.Second
)

// NewDevice creates a device; cloud may be nil.
func NewDevice(cfg DeviceConfig, mode ConnMode, cloud *CloudAccount) (*Device, error) {
	d := &Device{Config: cfg, Beep: true, IdleDisconnect: 8 * time.Second, CommandBudget: 12 * time.Second,
		cloud: cloud, mode: mode, lanRetry: lanRetryInterval}

	if cfg.HasLAN() {
		if err := cfg.Validate(); err != nil {
			return nil, err
		}

		t, _ := hex.DecodeString(cfg.Token)
		k, _ := hex.DecodeString(cfg.Key)
		d.lan = &LANClient{Host: cfg.IP, Port: cfg.Port, DeviceID: cfg.DeviceID, Token: t, Key: k}
		d.lan.Debugf = func(f string, a ...any) {
			if d.Debugf != nil {
				d.Debugf(f, a...)
			}
		}
	}

	if mode == "" {
		d.mode = ConnAuto
	}

	return d, nil
}

// SetMode switches the connection mode and clears all backoffs.
func (d *Device) SetMode(m ConnMode) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.mode = m
	d.resetBackoffLocked()
}

// SetOptions changes beep and idle disconnect while jobs may run.
func (d *Device) SetOptions(beep bool, idle time.Duration) {
	d.mu.Lock()
	d.Beep = beep
	d.IdleDisconnect = idle
	d.mu.Unlock()
}

// SetCloud replaces the cloud account (nil = none).
func (d *Device) SetCloud(c *CloudAccount) {
	d.mu.Lock()
	d.cloud = c
	d.mu.Unlock()
}

// Path is the way the last answer came ("lan", "cloud" or "").
func (d *Device) Path() string {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.path
}

// ResetBackoff lets the next job try every path again (manual refresh).
func (d *Device) ResetBackoff() {
	d.mu.Lock()
	d.resetBackoffLocked()
	d.mu.Unlock()
}

func (d *Device) resetBackoffLocked() {
	d.lanFailedAt = time.Time{}
	d.lanRetry = lanRetryInterval
	d.cloudOfflineUntil = time.Time{}
}

// Close drops the LAN connection.
func (d *Device) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.idleTimer != nil {
		d.idleTimer.Stop()
	}

	if d.lan != nil {
		d.lan.Close()
	}
}

// run executes a job serialised with a time budget and schedules the idle
// disconnect afterwards.
func (d *Device) run(fn func() error) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.idleTimer != nil {
		d.idleTimer.Stop()
	}

	if d.lan != nil {
		d.lan.Deadline = time.Now().Add(d.CommandBudget)
	}

	err := fn()

	if d.lan != nil {
		d.lan.Deadline = time.Time{}

		lan := d.lan
		d.idleTimer = time.AfterFunc(d.IdleDisconnect, func() {
			d.mu.Lock()
			defer d.mu.Unlock()

			lan.Close()
		})
	}

	return err
}

func (d *Device) cloudSend(frame []byte) ([][]byte, error) {
	if time.Now().Before(d.cloudOfflineUntil) {
		return nil, &CloudError{Code: 3123}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	frames, err := d.cloud.Send(ctx, frame, d.Config.DeviceID)
	if IsApplianceOffline(err) {
		d.cloudOfflineUntil = time.Now().Add(cloudOfflineInterval)
	}

	return frames, err
}

func (d *Device) use(p string) {
	if d.path != p && d.path == "lan" && d.lan != nil {
		d.lan.Close()
	}

	d.path = p
}

func isControl(frame []byte) bool { return len(frame) > 9 && frame[9] == FrameControl }

// send chooses the path: auto = LAN first, cloud on connection errors. LAN is
// skipped for lanRetry after a failure while the cloud answers.
func (d *Device) send(frame []byte, retries int, expect byte) ([][]byte, error) {
	switch d.mode {
	case ConnLAN:
		if d.lan == nil {
			return nil, errors.New("no LAN credentials – set the device up once with the account or token/key")
		}

		f, err := d.lan.Send(frame, retries, expect)
		if err == nil {
			d.use("lan")
		}

		return f, err
	case ConnCloud:
		if d.cloud == nil {
			return nil, errors.New("cloud not set up – sign in with the Midea account once")
		}

		f, err := d.cloudSend(frame)
		if err == nil {
			d.use("cloud")
		}

		return f, err
	}

	if d.lan == nil && d.cloud == nil {
		return nil, ErrNoPath
	}

	var errs []error

	lanBlocked := !d.lanFailedAt.IsZero() && time.Since(d.lanFailedAt) < d.lanRetry && d.cloud != nil
	if d.lan != nil && !lanBlocked {
		lanRetries := retries
		if d.cloud != nil {
			lanRetries = min(retries, 1)
		}

		f, err := d.lan.Send(frame, lanRetries, expect)
		if err == nil {
			d.lanFailedAt = time.Time{}
			d.lanRetry = lanRetryInterval
			d.use("lan")

			return f, nil
		}

		errs = append(errs, fmt.Errorf("LAN: %w", err))

		if !d.lanFailedAt.IsZero() {
			d.lanRetry = min(lanMaxRetryInterval, d.lanRetry*2)
		}

		d.lanFailedAt = time.Now()

		// A control frame that went out is not silently repeated via the cloud.
		if d.cloud == nil || isControl(frame) && d.lan.FailedAfterWrite() {
			return nil, err
		}
	}

	if d.cloud != nil {
		f, err := d.cloudSend(frame)
		if err == nil {
			d.use("cloud")
			return f, nil
		}

		errs = append(errs, fmt.Errorf("cloud: %w", err))

		if !IsApplianceOffline(err) {
			// Cloud failed for network/login reasons: try LAN first next time.
			d.lanFailedAt = time.Time{}
		}
	}

	return nil, errors.Join(errs...)
}

// sendSingle is one attempt on the current path: no retry, no fallback, no
// blocking of the path (optional queries).
func (d *Device) sendSingle(frame []byte, expect byte) ([][]byte, error) {
	usePath := d.path
	if usePath == "" {
		switch {
		case d.mode == ConnCloud:
			usePath = "cloud"
		case d.lan != nil:
			usePath = "lan"
		default:
			usePath = "cloud"
		}
	}

	if usePath == "lan" && d.lan != nil {
		return d.lan.Send(frame, 0, expect)
	}

	if d.cloud != nil {
		return d.cloudSend(frame)
	}

	return nil, ErrNoPath
}

func stateFrom(frames [][]byte) (State, bool) {
	for i := len(frames) - 1; i >= 0; i-- {
		p, err := FramePayload(frames[i])
		if err != nil {
			continue
		}

		if s, ok := ParseState(p); ok {
			return s, true
		}
	}

	return State{}, false
}

func (d *Device) queryState() (State, error) {
	frames, err := d.send(FrameGetState(), 2, RespState)
	if err != nil {
		return State{}, err
	}

	s, ok := stateFrom(frames)
	if !ok {
		return State{}, fmt.Errorf("%w: no state report", ErrProtocol)
	}

	return s, nil
}

var stateProps = []PropertyID{PropIECO, PropOutSilent, PropSelfClean, PropSound, PropRateSelect}

func (d *Device) queryProperties(optional bool) (map[PropertyID][]byte, error) {
	frame := FrameGetProperties(stateProps...)

	var (
		frames [][]byte
		err    error
	)

	if optional {
		frames, err = d.sendSingle(frame, RespProperties)
	} else {
		frames, err = d.send(frame, 2, RespProperties)
	}

	if err != nil {
		return nil, err
	}

	out := map[PropertyID][]byte{}

	for _, f := range frames {
		p, err := FramePayload(f)
		if err != nil || len(p) == 0 || (p[0] != RespProperties && p[0] != RespSetProps) {
			continue
		}

		for k, v := range ParseProperties(p) {
			out[k] = v
		}
	}

	return out, nil
}

func (d *Device) queryEnergy() (w, total, run *float64, ok bool) {
	frames, err := d.sendSingle(FrameGetGroup(4), RespGroup)
	if err != nil {
		return nil, nil, nil, false
	}

	for _, f := range frames {
		if !IsGroupReply(f, 4) {
			continue
		}

		p, err := FramePayload(f)
		if err != nil {
			continue
		}

		w, total, run = ParseEnergy(p)

		return w, total, run, true
	}

	return nil, nil, nil, false
}

func applyProperties(props map[PropertyID][]byte, s *State) {
	if props == nil {
		return
	}

	if r, ok := props[PropRateSelect]; ok && len(r) > 0 {
		v := int(r[0])
		s.RatePercent = &v
	}

	for id, dst := range map[PropertyID]**bool{PropOutSilent: &s.OutSilent, PropSelfClean: &s.SelfClean, PropIECO: &s.IECO, PropSound: &s.Sound} {
		if raw, ok := props[id]; ok {
			if v, ok := id.Decode(raw); ok {
				*dst = &v
			}
		}
	}
}

// carry takes over values that are not part of the state report.
func (d *Device) carry(s State) State {
	if d.last == nil {
		return s
	}

	l := d.last
	s.OutSilent, s.SelfClean, s.IECO, s.Sound, s.RatePercent = l.OutSilent, l.SelfClean, l.IECO, l.Sound, l.RatePercent
	s.PowerWatts, s.TotalEnergyKWh, s.RunEnergyKWh = l.PowerWatts, l.TotalEnergyKWh, l.RunEnergyKWh

	return s
}

func (d *Device) remember(s State) {
	s.Normalize()
	d.last = &s
	d.lastAt = time.Now()
}

func (d *Device) freshState() (State, error) {
	if d.last != nil && time.Since(d.lastAt) <= freshStateAge {
		return *d.last, nil
	}

	s, err := d.queryState()
	if err != nil {
		return State{}, err
	}

	return d.carry(s), nil
}

func (d *Device) extras(s *State) {
	props, err := d.queryProperties(true)
	if err == nil {
		applyProperties(props, s)

		if s.Sound != nil {
			d.Beep = *s.Sound
		}
	}

	if w, t, r, ok := d.queryEnergy(); ok {
		s.PowerWatts, s.TotalEnergyKWh, s.RunEnergyKWh = w, t, r
	}
}

// Refresh reads the state; full adds properties and energy.
func (d *Device) Refresh(full bool) (State, error) {
	var out State

	err := d.run(func() error {
		s, err := d.queryState()
		if err != nil {
			return err
		}

		s = d.carry(s)
		if full {
			d.extras(&s)
		}

		d.remember(s)
		out = *d.last

		return nil
	})

	return out, err
}

// Last returns the last known state (nil if none).
func (d *Device) Last() *State {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.last == nil {
		return nil
	}

	s := *d.last

	return &s
}

type controlField struct {
	name string
	same func(a, b State) bool
}

var controlFields = []controlField{
	{"power", func(a, b State) bool { return a.Power == b.Power }},
	{"mode", func(a, b State) bool { return a.Mode == b.Mode }},
	{"target", func(a, b State) bool { return a.TargetTemperature == b.TargetTemperature }},
	{"fan", func(a, b State) bool { return a.FanSpeed == b.FanSpeed }},
	{"swing", func(a, b State) bool { return a.Swing == b.Swing }},
	{"eco", func(a, b State) bool { return a.Eco == b.Eco }},
	{"turbo", func(a, b State) bool { return a.Turbo == b.Turbo }},
	{"sleep", func(a, b State) bool { return a.Sleep == b.Sleep }},
	{"purifier", func(a, b State) bool { return a.Purifier == b.Purifier }},
	{"freeze", func(a, b State) bool { return a.FreezeProtection == b.FreezeProtection }},
	{"fahrenheit", func(a, b State) bool { return a.Fahrenheit == b.Fahrenheit }},
	{"timer", func(a, b State) bool {
		return eqIntPtr(a.OnTimerMinutes, b.OnTimerMinutes) && eqIntPtr(a.OffTimerMinutes, b.OffTimerMinutes)
	}},
}

func eqIntPtr(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	// The device counts down: allow a few minutes difference.
	return math.Abs(float64(*a-*b)) <= 2
}

func mismatch(result, target State, changed map[string]bool) []string {
	var out []string

	for _, f := range controlFields {
		if !changed[f.name] || f.same(result, target) {
			continue
		}

		if f.name != "power" && !target.Power && f.name != "timer" {
			continue
		}

		if f.name == "fan" && target.Mode == ModeDry {
			continue
		}

		out = append(out, f.name)
	}

	return out
}

// Update changes the basic state (power, mode, temperature, fan, swing, eco,
// turbo, sleep, ionizer, frost protection, device timers) and verifies it.
func (d *Device) Update(change func(*State)) (State, error) {
	var out State

	err := d.run(func() error {
		before, err := d.freshState()
		if err != nil {
			return err
		}

		target := before
		change(&target)

		if target.TargetTemperature != before.TargetTemperature {
			// Whole degrees only (like Klimacontrol: the PortaSplit ignores halves).
			target.TargetTemperature = math.Min(30, math.Max(16, math.Round(target.TargetTemperature)))
		}

		if target.Turbo {
			target.Eco = false
			target.Sleep = false
		}

		if target.Mode != ModeHeat {
			target.FreezeProtection = false
		}

		if target.FanSpeed != FanAuto {
			target.FanSpeed = max(1, min(100, target.FanSpeed))
		}

		changed := map[string]bool{}
		for _, f := range controlFields {
			if !f.same(before, target) {
				changed[f.name] = true
			}
		}

		result := before

		for attempt := 1; attempt <= 3; attempt++ {
			if attempt > 1 {
				time.Sleep(retryPause)
			}

			frames, err := d.send(FrameSetState(target, d.Beep), 2, RespState)
			if err != nil {
				return unclearOutcome(err)
			}

			s, ok := stateFrom(frames)
			if !ok {
				if s, err = d.queryState(); err != nil {
					return unclearOutcome(err)
				}
			}

			result = d.carry(s)
			if len(mismatch(result, target, changed)) == 0 {
				d.remember(result)
				out = *d.last

				return nil
			}
		}

		d.remember(result)

		return &ErrNotApplied{State: *d.last, Fields: mismatch(result, target, changed)}
	})

	if err != nil {
		var na *ErrNotApplied
		if errors.As(err, &na) {
			return na.State, err
		}
	}

	return out, err
}

func unclearOutcome(err error) error {
	var na *ErrNotApplied

	var ou *ErrOutcomeUnknown

	if errors.As(err, &na) || errors.As(err, &ou) || errors.Is(err, ErrAuth) || IsApplianceOffline(err) {
		return err
	}

	return &ErrOutcomeUnknown{Err: err}
}

// NearestRate maps a percentage to the power limit steps 50, 75 or 100.
func NearestRate(p int) int {
	best := 100
	for _, r := range []int{50, 75, 100} {
		if abs(r-p) < abs(best-p) {
			best = r
		}
	}

	return best
}

func abs(v int) int {
	if v < 0 {
		return -v
	}

	return v
}

// SetProperty sets a boolean property (outdoor silent, self clean, iECO,
// sound) and reads it back.
func (d *Device) SetProperty(id PropertyID, on bool) (State, error) {
	var out State

	err := d.run(func() error {
		var s State

		applied := true

		for attempt := range 2 {
			if attempt > 0 {
				time.Sleep(retryPause)
			}

			if _, err := d.send(FrameSetProperty(id, on), 2, 0); err != nil {
				return unclearOutcome(err)
			}

			st, err := d.queryState()
			if err != nil {
				return unclearOutcome(err)
			}

			s = d.carry(st)

			props, err := d.queryProperties(false)
			if err == nil {
				applyProperties(props, &s)
			}

			raw, ok := props[id]
			if !ok {
				applied = true
				break
			}

			v, _ := id.Decode(raw)
			if applied = v == on; applied {
				break
			}
		}

		if id == PropSound {
			d.Beep = on
		}

		d.remember(s)
		out = *d.last

		if !applied {
			return &ErrNotApplied{State: out, Fields: []string{fmt.Sprintf("property 0x%04X", uint16(id))}}
		}

		return nil
	})

	return out, err
}

// SetRate sets the power limit (100 = none, 75, 50).
func (d *Device) SetRate(percent int) (State, error) {
	value := NearestRate(percent)

	var out State

	err := d.run(func() error {
		var s State

		readBack := false

		for attempt := range 2 {
			if attempt > 0 {
				time.Sleep(retryPause)
			}

			if _, err := d.send(FrameSetRate(value), 2, 0); err != nil {
				return unclearOutcome(err)
			}

			st, err := d.queryState()
			if err != nil {
				return unclearOutcome(err)
			}

			s = d.carry(st)

			props, err := d.queryProperties(false)
			if err == nil {
				applyProperties(props, &s)
				_, readBack = props[PropRateSelect]
			}

			if s.RatePercent != nil && *s.RatePercent == value {
				break
			}
		}

		d.remember(s)
		out = *d.last

		if readBack && (s.RatePercent == nil || *s.RatePercent != value) {
			return &ErrNotApplied{State: out, Fields: []string{"rate"}}
		}

		return nil
	})

	return out, err
}

// SetDisplay switches the display to on (the device only knows "toggle", so
// it toggles only when the state differs, and checks once).
func (d *Device) SetDisplay(on bool) (State, error) {
	var out State

	err := d.run(func() error {
		before, err := d.freshState()
		if err != nil {
			return err
		}

		if before.DisplayOn == on {
			d.remember(before)
			out = before

			return nil
		}

		var s State

		for attempt := range 2 {
			if attempt > 0 {
				time.Sleep(retryPause)
			}

			frames, _ := d.sendSingle(FrameToggleDisplay(d.Beep), RespState)

			st, ok := stateFrom(frames)
			if !ok {
				if st, err = d.queryState(); err != nil {
					return unclearOutcome(err)
				}
			}

			s = d.carry(st)
			if s.DisplayOn == on {
				break
			}
		}

		d.remember(s)
		out = *d.last

		return nil
	})

	return out, err
}

// Group reads the raw 0xC1 answer frames of the given groups.
func (d *Device) groups(groups []byte) map[byte][]byte {
	out := map[byte][]byte{}

	for _, g := range groups {
		frames, err := d.sendSingle(FrameGetGroup(g), RespGroup)
		if err != nil {
			continue
		}

		for _, f := range frames {
			if IsGroupReply(f, g) {
				out[g] = f
			}
		}
	}

	return out
}

func (d *Device) propertyData(id PropertyID) []byte {
	frames, err := d.sendSingle(FrameGetProperties(id), RespProperties)
	if err != nil {
		return nil
	}

	for _, f := range frames {
		p, err := FramePayload(f)
		if err != nil || len(p) == 0 || p[0] != RespProperties {
			continue
		}

		if v, ok := ParseProperties(p)[id]; ok {
			return v
		}
	}

	return nil
}

// Diagnostics reads the group data, the second indoor sensor and the device
// identifier.
func (d *Device) Diagnostics() (Diagnostics, error) {
	var diag Diagnostics

	err := d.run(func() error {
		s, err := d.freshState()
		if err != nil {
			return err
		}

		for g, f := range d.groups(DiagnosticGroups) {
			if p, err := FramePayload(f); err == nil {
				diag.Apply(g, p)
			}
		}

		diag.DropStandbyPlaceholders(s.Power)

		// 0x00E0: count byte, then six 16 bit LE temperatures in 1/100 °C;
		// value 2 is the second indoor sensor.
		if e0 := d.propertyData(PropIndoorT2); len(e0) >= 5 {
			if v := int(e0[3]) | int(e0[4])<<8; v > 0 && v < 12000 {
				diag.IndoorTemp2 = fp(float64(v) / 100)
			}
		}

		if id := d.propertyData(PropIdentifier); len(id) > 0 {
			diag.Identifier = CleanText(string(id), "", 64)
		}

		return nil
	})

	return diag, err
}

// Query sends a raw read-only query payload (frame type 0x03) and returns all
// answer frames as hex (analysis).
func (d *Device) Query(payload []byte) ([]string, error) {
	var out []string

	err := d.run(func() error {
		frames, err := d.sendSingle(BuildFrame(FrameQuery, payload, -1), 0)
		for _, f := range frames {
			out = append(out, hexs(f))
		}

		return err
	})

	return out, err
}

// Verify checks token and key with a handshake and a state read.
func Verify(cfg DeviceConfig) error {
	d, err := NewDevice(cfg, ConnLAN, nil)
	if err != nil {
		return err
	}
	defer d.Close()

	d.CommandBudget = 8 * time.Second
	_, err = d.Refresh(false)

	return err
}

// FetchConfig gets token and key for a discovered device from the cloud and
// checks them at the device (big endian udpid first, then little endian).
func FetchConfig(ctx context.Context, acc *CloudAccount, dev Discovered) (DeviceConfig, error) {
	lastErr := error(ErrCloudNoToken)

	for _, k := range CloudKinds {
		for _, big := range []bool{true, false} {
			tok, key, err := acc.Token(ctx, k, UDPID(dev.DeviceID, big), dev.DeviceID)
			if err != nil {
				lastErr = err
				if !errors.Is(err, ErrCloudNoToken) {
					break
				}

				continue
			}

			cfg := DeviceConfig{DeviceID: dev.DeviceID, IP: dev.IP, Port: dev.Port, Token: tok, Key: key}
			if cfg.Port == 0 {
				cfg.Port = 6444
			}

			if err := Verify(cfg); err == nil {
				return cfg, nil
			}

			lastErr = ErrTokenRejected
		}
	}

	return DeviceConfig{}, lastErr
}
