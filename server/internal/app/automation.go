package app

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mortendnk/portacontrol/server/internal/events"
	"github.com/mortendnk/portacontrol/server/internal/midea"
	"github.com/mortendnk/portacontrol/server/internal/store"
)

// scheduleGrace: a schedule that was due less than this ago still runs (e.g.
// after a restart).
const scheduleGrace = 10 * time.Minute

func (a *App) automationLoop() {
	defer a.wg.Done()

	t := time.NewTicker(15 * time.Second)
	defer t.Stop()

	for {
		select {
		case <-a.ctx.Done():
			return
		case now := <-t.C:
			a.runTimers(now)
			a.runSchedules(now)
		}
	}
}

// ParseClock parses "HH:MM".
func ParseClock(s string) (int, int, bool) {
	h, m, ok := strings.Cut(strings.TrimSpace(s), ":")
	if !ok {
		return 0, 0, false
	}

	hh, err1 := strconv.Atoi(h)
	mm, err2 := strconv.Atoi(m)

	if err1 != nil || err2 != nil || hh < 0 || hh > 23 || mm < 0 || mm > 59 {
		return 0, 0, false
	}

	return hh, mm, true
}

func (a *App) runTimers(now time.Time) {
	for _, d := range a.Store.Devices() {
		t, err := store.Get[store.Timers](a.Store, store.CollTimers, d.ID)
		if err != nil {
			continue
		}

		changed := false

		if t.OnAt != nil && !now.Before(*t.OnAt) {
			due := *t.OnAt
			scene := t.OnSceneID
			t.OnAt, t.OnSceneID = nil, ""
			changed = true

			go func() {
				var err error
				if scene != "" {
					err = a.ApplyScene(scene, d.ID, "Timer")
				} else {
					on := true
					_, err = a.Control(d.ID, store.Control{Power: &on}, "Timer")
				}

				a.timerResult(d, "Einschalt-Timer", due, err)
			}()
		}

		if t.OffAt != nil && !now.Before(*t.OffAt) {
			due := *t.OffAt
			t.OffAt = nil
			changed = true

			go func() {
				off := false
				_, err := a.Control(d.ID, store.Control{Power: &off}, "Timer")
				a.timerResult(d, "Ausschalt-Timer", due, err)
			}()
		}

		if changed {
			_ = a.Store.Put(store.CollTimers, d.ID, t)
			a.Hub.Publish(events.Event{Type: "timers", Device: d.ID, Data: t})
		}
	}
}

func (a *App) timerResult(d store.Device, name string, due time.Time, err error) {
	if err != nil {
		a.notify(d.ID, "automation", fmt.Sprintf("%s: %s fehlgeschlagen", d.Name, name), err.Error())
		return
	}

	if a.Store.Settings().Notify.Automation {
		a.notify(d.ID, "automation", fmt.Sprintf("%s: %s ausgeführt (%s)", d.Name, name, due.In(a.Loc).Format("15:04")), "")
	}
}

// SetTimers stores the app timers of a device. onAt in the past means "the
// next occurrence of that clock time".
func (a *App) SetTimers(id string, t store.Timers) (store.Timers, error) {
	if _, err := a.entry(id); err != nil {
		return t, err
	}

	now := time.Now()

	if t.OnAt != nil && !t.OnAt.After(now) {
		n := t.OnAt.In(a.Loc)
		next := time.Date(now.Year(), now.Month(), now.Day(), n.Hour(), n.Minute(), 0, 0, a.Loc)

		if !next.After(now) {
			next = next.AddDate(0, 0, 1)
		}

		t.OnAt = &next
	}

	if t.OffAt != nil && !t.OffAt.After(now) {
		t.OffAt = nil
	}

	if err := a.Store.Put(store.CollTimers, id, t); err != nil {
		return t, err
	}

	a.Hub.Publish(events.Event{Type: "timers", Device: id, Data: t})

	return t, nil
}

func dayMatches(days []int, wd time.Weekday) bool {
	if len(days) == 0 {
		return true
	}

	for _, d := range days {
		if d == int(wd) {
			return true
		}
	}

	return false
}

// NextRun is the next time a schedule fires (zero if never).
func (a *App) NextRun(s store.Schedule, after time.Time) time.Time {
	h, m, ok := ParseClock(s.Time)
	if !ok || !s.Enabled {
		return time.Time{}
	}

	after = after.In(a.Loc)

	for i := range 8 {
		d := after.AddDate(0, 0, i)
		t := time.Date(d.Year(), d.Month(), d.Day(), h, m, 0, 0, a.Loc)

		if t.After(after) && dayMatches(s.Days, t.Weekday()) {
			return t
		}
	}

	return time.Time{}
}

func (a *App) runSchedules(now time.Time) {
	list, err := store.List[store.Schedule](a.Store, store.CollSchedules)
	if err != nil {
		return
	}

	now = now.In(a.Loc)

	for _, s := range list {
		h, m, ok := ParseClock(s.Time)
		if !s.Enabled || !ok {
			continue
		}

		due := time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, a.Loc)
		if now.Before(due) || now.Sub(due) > scheduleGrace || !s.LastRun.Before(due) || !dayMatches(s.Days, due.Weekday()) {
			continue
		}

		s.LastRun = now
		skip := s.SkipNext
		s.SkipNext = false
		_ = a.Store.Put(store.CollSchedules, s.ID, s)

		if skip {
			a.logf("", "info", "Wochenprogramm %q heute einmal übersprungen", s.Name)
			continue
		}

		go func(s store.Schedule) {
			err := a.RunAction(s.Action, s.Devices, "Wochenprogramm "+s.Name)
			if err != nil {
				a.notify("", "automation", "Wochenprogramm "+s.Name+" fehlgeschlagen", err.Error())
			} else if a.Store.Settings().Notify.Automation {
				a.notify("", "automation", "Wochenprogramm "+s.Name+" ausgeführt", "")
			}

			a.Hub.Publish(events.Event{Type: "automation"})
		}(s)
	}
}

func sensorValue(st *midea.State, sensor string) (float64, bool) {
	switch sensor {
	case "indoor":
		if st.IndoorTemperature != nil {
			return *st.IndoorTemperature, true
		}
	case "outdoor":
		if st.OutdoorTemp != nil {
			return *st.OutdoorTemp, true
		}
	case "power_watts":
		if st.PowerWatts != nil {
			return *st.PowerWatts, true
		}
	}

	return 0, false
}

func inWindow(now time.Time, from, to string) bool {
	if from == "" || to == "" {
		return true
	}

	fh, fm, ok1 := ParseClock(from)
	th, tm, ok2 := ParseClock(to)

	if !ok1 || !ok2 {
		return true
	}

	cur := now.Hour()*60 + now.Minute()
	f, t := fh*60+fm, th*60+tm

	if f <= t {
		return cur >= f && cur < t
	}

	return cur >= f || cur < t // over midnight
}

// evaluateRules runs the climate rules of a device after a fresh read.
// Rules are edge triggered: a rule fires once when its condition becomes
// true and re-arms when the value is back beyond the hysteresis.
func (a *App) evaluateRules(id string, st *midea.State) {
	list, err := store.List[store.Rule](a.Store, store.CollRules)
	if err != nil {
		return
	}

	now := time.Now().In(a.Loc)

	for _, r := range list {
		if !r.Enabled || r.Device != id {
			continue
		}

		v, ok := sensorValue(st, r.Sensor)
		if !ok {
			continue
		}

		hyst := r.Hysteresis
		if hyst <= 0 {
			hyst = 0.5
		}

		var cond, rearm bool

		switch r.Op {
		case "below":
			cond, rearm = v < r.Value, v >= r.Value+hyst
		default:
			cond, rearm = v > r.Value, v <= r.Value-hyst
		}

		changed := false

		if rearm && !r.Armed {
			r.Armed, changed = true, true
		}

		fire := cond && r.Armed && inWindow(now, r.From, r.To) &&
			(r.Cooldown <= 0 || now.Sub(r.LastFired) >= time.Duration(r.Cooldown)*time.Minute) &&
			!(r.OnlyIfOff && st.Power) && !(r.OnlyIfOn && !st.Power)

		if fire {
			r.Armed, r.LastFired, changed = false, now, true
		}

		if changed {
			_ = a.Store.Put(store.CollRules, r.ID, r)
		}

		if fire {
			go func(r store.Rule, v float64) {
				src := fmt.Sprintf("Regel %s (%.1f)", r.Name, v)

				err := a.RunAction(r.Action, []string{id}, src)
				if err != nil {
					a.notify(id, "automation", "Regel "+r.Name+" fehlgeschlagen", err.Error())
				} else if a.Store.Settings().Notify.Automation {
					a.notify(id, "automation", "Regel "+r.Name+" ausgelöst", src)
				}

				a.Hub.Publish(events.Event{Type: "automation"})
			}(r, v)
		}
	}
}
