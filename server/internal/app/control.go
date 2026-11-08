package app

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/mortendnk/portacontrol/server/internal/midea"
	"github.com/mortendnk/portacontrol/server/internal/store"
)

// ErrBadRequest marks invalid control values.
var ErrBadRequest = errors.New("invalid request")

// baseChange reports whether c touches the basic state frame.
func baseChange(c store.Control) bool {
	return c.Power != nil || c.Mode != nil || c.Target != nil || c.Fan != nil || c.Swing != nil || c.Eco != nil ||
		c.Turbo != nil || c.Sleep != nil || c.Purifier != nil || c.FreezeProtection != nil || c.Fahrenheit != nil ||
		c.OnTimerMinutes != nil || c.OffTimerMinutes != nil
}

// Validate checks the values of a control request.
func Validate(c store.Control) error {
	if c.Mode != nil {
		if _, ok := midea.ParseMode(*c.Mode); !ok {
			return fmt.Errorf("%w: mode must be auto, cool, dry, heat or fan", ErrBadRequest)
		}
	}

	if c.Target != nil && (math.IsNaN(*c.Target) || *c.Target < 16 || *c.Target > 30) {
		return fmt.Errorf("%w: target temperature must be 16–30 °C", ErrBadRequest)
	}

	if c.Fan != nil && (*c.Fan < 1 || *c.Fan > 100) && *c.Fan != midea.FanAuto {
		return fmt.Errorf("%w: fan speed must be 1–100 or 102 (auto)", ErrBadRequest)
	}

	for _, m := range []*int{c.OnTimerMinutes, c.OffTimerMinutes} {
		if m != nil && (*m < 0 || *m > midea.MaxTimerMinutes) {
			return fmt.Errorf("%w: timer must be 0–1440 minutes", ErrBadRequest)
		}
	}

	if c.Rate != nil && *c.Rate != 50 && *c.Rate != 75 && *c.Rate != 100 {
		return fmt.Errorf("%w: power limit must be 50, 75 or 100", ErrBadRequest)
	}

	return nil
}

// applyBase changes s like the request asks; functions that the mode does not
// support are dropped (the device would ignore them and the command would be
// retried three times).
func applyBase(s *midea.State, c store.Control, rec store.Device) {
	if c.Power != nil {
		s.Power = *c.Power
	}

	if c.Mode != nil {
		m, _ := midea.ParseMode(*c.Mode)
		s.Mode = m
	}

	if c.Target != nil {
		t := *c.Target
		if rec.MinTarget > 0 {
			t = math.Max(rec.MinTarget, t)
		}

		if rec.MaxTarget > 0 {
			t = math.Min(rec.MaxTarget, t)
		}

		s.TargetTemperature = t
	}

	if c.Fan != nil {
		s.FanSpeed = *c.Fan
	}

	if c.Swing != nil {
		s.Swing = midea.SwingOff
		if *c.Swing {
			s.Swing = midea.SwingVertical
		}
	}

	set := func(dst *bool, v *bool) {
		if v != nil {
			*dst = *v
		}
	}

	set(&s.Eco, c.Eco)
	set(&s.Turbo, c.Turbo)
	set(&s.Sleep, c.Sleep)
	set(&s.Purifier, c.Purifier)
	set(&s.FreezeProtection, c.FreezeProtection)
	set(&s.Fahrenheit, c.Fahrenheit)

	if c.Eco != nil && *c.Eco {
		s.Turbo = false
	}

	if c.Turbo != nil && *c.Turbo {
		s.Eco, s.Sleep = false, false
	}

	av := availability(s)
	if !av.Eco {
		s.Eco = false
	}

	if !av.Turbo {
		s.Turbo = false
	}

	if !av.Freeze {
		s.FreezeProtection = false
	}

	if !av.Sleep {
		s.Sleep = false
	}

	timer := func(dst **int, v *int) {
		if v == nil {
			return
		}

		if *v == 0 {
			*dst = nil
			return
		}

		m := *v
		*dst = &m
	}

	timer(&s.OnTimerMinutes, c.OnTimerMinutes)
	timer(&s.OffTimerMinutes, c.OffTimerMinutes)
}

// Control applies a change to one device and returns the new state.
// source is shown in the log (web, app, schedule, rule, mqtt, ...).
func (a *App) Control(id string, c store.Control, source string) (midea.State, error) {
	if err := Validate(c); err != nil {
		return midea.State{}, err
	}

	e, err := a.entry(id)
	if err != nil {
		return midea.State{}, err
	}

	e.mu.Lock()
	e.busyJob = true
	rec := e.Rec
	e.mu.Unlock()

	defer func() {
		e.mu.Lock()
		e.busyJob = false
		e.mu.Unlock()
	}()

	var (
		st   midea.State
		errs []error
		have bool
	)

	step := func(s midea.State, err error) {
		if err != nil {
			errs = append(errs, err)

			var na *midea.ErrNotApplied
			if !errors.As(err, &na) {
				return
			}
		}

		st, have = s, true
	}

	if baseChange(c) {
		step(e.Dev.Update(func(s *midea.State) { applyBase(s, c, rec) }))
	}

	props := []struct {
		v  *bool
		id midea.PropertyID
	}{{c.OutdoorSilent, midea.PropOutSilent}, {c.IECO, midea.PropIECO}, {c.SelfClean, midea.PropSelfClean}, {c.Sound, midea.PropSound}}

	for _, p := range props {
		if p.v != nil && len(errs) == 0 {
			step(e.Dev.SetProperty(p.id, *p.v))
		}
	}

	if c.Rate != nil && len(errs) == 0 {
		step(e.Dev.SetRate(*c.Rate))
	}

	if c.Display != nil && len(errs) == 0 {
		step(e.Dev.SetDisplay(*c.Display))
	}

	err = errors.Join(errs...)

	if have {
		a.afterRead(e, st, nil, false)
	}

	desc := describe(c)
	if err != nil {
		a.logf(id, "error", "%s (%s): %s – %v", rec.Name, source, desc, err)
	} else {
		a.logf(id, "info", "%s (%s): %s", rec.Name, source, desc)
	}

	return st, err
}

// describe is a short German text of a control request for the log.
func describe(c store.Control) string {
	var p []string

	onOff := func(name string, v *bool) {
		if v == nil {
			return
		}

		if *v {
			p = append(p, name+" an")
		} else {
			p = append(p, name+" aus")
		}
	}

	onOff("Ein", c.Power)

	if c.Mode != nil {
		p = append(p, "Modus "+ModeLabel(*c.Mode))
	}

	if c.Target != nil {
		p = append(p, fmt.Sprintf("%.1f °C", *c.Target))
	}

	if c.Fan != nil {
		if *c.Fan == midea.FanAuto {
			p = append(p, "Lüfter Auto")
		} else {
			p = append(p, fmt.Sprintf("Lüfter %d %%", *c.Fan))
		}
	}

	onOff("Schwenken", c.Swing)
	onOff("Eco", c.Eco)
	onOff("Turbo", c.Turbo)
	onOff("Schlaf", c.Sleep)
	onOff("Ionisierer", c.Purifier)
	onOff("Frostschutz", c.FreezeProtection)
	onOff("Außen leise", c.OutdoorSilent)
	onOff("iECO", c.IECO)
	onOff("Selbstreinigung", c.SelfClean)
	onOff("Signalton", c.Sound)
	onOff("Anzeige", c.Display)

	if c.Rate != nil {
		p = append(p, fmt.Sprintf("Leistung %d %%", *c.Rate))
	}

	if c.OnTimerMinutes != nil {
		p = append(p, fmt.Sprintf("Gerätetimer ein %d min", *c.OnTimerMinutes))
	}

	if c.OffTimerMinutes != nil {
		p = append(p, fmt.Sprintf("Gerätetimer aus %d min", *c.OffTimerMinutes))
	}

	if len(p) == 0 {
		return "keine Änderung"
	}

	return strings.Join(p, ", ")
}

// ModeLabel is the German name of a mode.
func ModeLabel(m string) string {
	switch m {
	case "auto":
		return "Auto"
	case "cool":
		return "Kühlen"
	case "dry":
		return "Entfeuchten"
	case "heat":
		return "Heizen"
	case "fan":
		return "Lüften"
	}

	return m
}

// Refresh reads a device now (full).
func (a *App) Refresh(id string) (View, error) {
	e, err := a.entry(id)
	if err != nil {
		return View{}, err
	}

	e.Dev.ResetBackoff()
	st, err := e.Dev.Refresh(true)
	a.afterRead(e, st, err, true)

	if err != nil {
		return View{}, err
	}

	return a.View(id, false)
}

// ApplyScene applies a scene to its devices (or to target when given).
func (a *App) ApplyScene(sceneID, target, source string) error {
	sc, err := store.Get[store.Scene](a.Store, store.CollScenes, sceneID)
	if err != nil {
		return err
	}

	devs := sc.Devices
	if target != "" {
		devs = []string{target}
	}

	if len(devs) == 0 {
		for _, d := range a.Store.Devices() {
			devs = append(devs, d.ID)
		}
	}

	var errs []error

	for _, id := range devs {
		if _, err := a.Control(id, sc.Control, source+": "+sc.Name); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// RunAction executes an automation action on the devices.
func (a *App) RunAction(act store.Action, devices []string, source string) error {
	if len(devices) == 0 && act.Kind != "scene" {
		for _, d := range a.Store.Devices() {
			devices = append(devices, d.ID)
		}
	}

	on, off := true, false

	switch act.Kind {
	case "scene":
		if len(devices) == 0 {
			return a.ApplyScene(act.SceneID, "", source)
		}

		var errs []error

		for _, d := range devices {
			errs = append(errs, a.ApplyScene(act.SceneID, d, source))
		}

		return errors.Join(errs...)
	case "notify":
		a.notify(strings.Join(devices, ","), "automation", act.Text, source)
		return nil
	}

	var c store.Control

	switch act.Kind {
	case "on":
		c.Power = &on
		if act.Control != nil {
			c = *act.Control
			c.Power = &on
		}
	case "off":
		c.Power = &off
	case "control":
		if act.Control == nil {
			return fmt.Errorf("%w: control action without values", ErrBadRequest)
		}

		c = *act.Control
	default:
		return fmt.Errorf("%w: unknown action %q", ErrBadRequest, act.Kind)
	}

	var errs []error

	for _, d := range devices {
		if _, err := a.Control(d, c, source); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}
