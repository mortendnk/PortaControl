package app

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"strconv"
	"time"

	"github.com/mortendnk/portacontrol/server/internal/store"
)

// EnergySum is consumption and runtime over a span.
type EnergySum struct {
	KWh       float64  `json:"kwh"`
	Cost      float64  `json:"cost"`
	CO2kg     float64  `json:"co2_kg"`
	OnHours   float64  `json:"on_hours"`
	AvgIndoor *float64 `json:"avg_indoor,omitempty"`
	MaxWatts  float64  `json:"max_watts"`
	Currency  string   `json:"currency"`
}

// Bucket is one bar of the energy chart.
type Bucket struct {
	Start time.Time `json:"start"`
	EnergySum
}

// maxGap is the longest gap between samples that still counts as continuous.
const maxGap = 15 * time.Minute

type accum struct {
	kwh, onSec, maxW float64
	inSum            float64
	inN              int
}

// integrate walks the samples and calls add for every interval: the energy
// comes from the device counter (binary, 0.01 kWh steps) and falls back to
// power × time when there is no counter.
func integrate(samples []store.Sample, add func(t time.Time, kwh, onSec float64, s store.Sample)) {
	for i := 1; i < len(samples); i++ {
		p, c := samples[i-1], samples[i]
		dt := float64(c.TS - p.TS)

		if dt <= 0 {
			continue
		}

		if dt > maxGap.Seconds() {
			add(time.Unix(c.TS, 0), 0, 0, c)
			continue
		}

		kwh := 0.0

		switch {
		case p.KWh != nil && c.KWh != nil:
			d := *c.KWh - *p.KWh
			// A device draws ~1.1 kW at most; larger jumps are counter resets.
			if d > 0 && d < 1.5*dt/3600+0.05 {
				kwh = d
			}
		case p.Watts != nil:
			kwh = *p.Watts / 1000 * dt / 3600
		}

		on := 0.0
		if p.Power {
			on = dt
		}

		add(time.Unix(c.TS, 0), kwh, on, c)
	}
}

func (a *App) finish(acc accum) EnergySum {
	s := a.Store.Settings()
	e := EnergySum{
		KWh:      math.Round(acc.kwh*1000) / 1000,
		OnHours:  math.Round(acc.onSec/36) / 100,
		MaxWatts: acc.maxW,
		Currency: s.Currency,
	}
	e.Cost = math.Round(acc.kwh*s.PricePerKWh*100) / 100
	e.CO2kg = math.Round(acc.kwh*s.CO2GramsPerKWh) / 1000

	if acc.inN > 0 {
		v := math.Round(acc.inSum/float64(acc.inN)*10) / 10
		e.AvgIndoor = &v
	}

	return e
}

func (a *App) startOfDay(t time.Time) time.Time {
	t = t.In(a.Loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, a.Loc)
}

// EnergyToday sums the consumption since local midnight.
func (a *App) EnergyToday(id string) (EnergySum, error) {
	from := a.startOfDay(time.Now())

	list, err := a.Energy(id, "day", from, from.AddDate(0, 0, 1))
	if err != nil {
		return EnergySum{}, err
	}

	var acc accum

	for _, b := range list {
		acc.kwh += b.KWh
		acc.onSec += b.OnHours * 3600
		acc.maxW = math.Max(acc.maxW, b.MaxWatts)
	}

	return a.finish(acc), nil
}

// Energy returns buckets of the given size (hour, day, month) between from
// and to.
func (a *App) Energy(id, size string, from, to time.Time) ([]Bucket, error) {
	samples, err := a.Store.Samples(context.Background(), id, from.Add(-maxGap).Unix(), to.Unix())
	if err != nil {
		return nil, err
	}

	bucketOf := func(t time.Time) time.Time {
		t = t.In(a.Loc)

		switch size {
		case "hour", "day":
			return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, a.Loc)
		case "month", "year":
			return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, a.Loc)
		default:
			return a.startOfDay(t)
		}
	}

	next := func(t time.Time) time.Time {
		switch size {
		case "hour", "day":
			return t.Add(time.Hour)
		case "month", "year":
			return t.AddDate(0, 1, 0)
		default:
			return t.AddDate(0, 0, 1)
		}
	}

	accs := map[time.Time]*accum{}

	integrate(samples, func(t time.Time, kwh, on float64, s store.Sample) {
		if t.Before(from) || !t.Before(to) {
			return
		}

		b := bucketOf(t)
		acc := accs[b]

		if acc == nil {
			acc = &accum{}
			accs[b] = acc
		}

		acc.kwh += kwh
		acc.onSec += on

		if s.Watts != nil {
			acc.maxW = math.Max(acc.maxW, *s.Watts)
		}

		if s.Indoor != nil {
			acc.inSum += *s.Indoor
			acc.inN++
		}
	})

	var out []Bucket

	for b := bucketOf(from); b.Before(to); b = next(b) {
		acc := accs[b]
		if acc == nil {
			acc = &accum{}
		}

		out = append(out, Bucket{Start: b, EnergySum: a.finish(*acc)})
	}

	return out, nil
}

// EnergyRange resolves a period name (day, week, month, year) and an offset
// (0 = current, -1 = previous, ...) into span and bucket size.
func (a *App) EnergyRange(period string, offset int) (from, to time.Time, size string) {
	now := time.Now().In(a.Loc)
	today := a.startOfDay(now)

	switch period {
	case "week":
		wd := (int(today.Weekday()) + 6) % 7 // Monday first
		from = today.AddDate(0, 0, -wd+7*offset)

		return from, from.AddDate(0, 0, 7), "week"
	case "month":
		from = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, a.Loc).AddDate(0, offset, 0)
		return from, from.AddDate(0, 1, 0), "month-days"
	case "year":
		from = time.Date(now.Year()+offset, 1, 1, 0, 0, 0, 0, a.Loc)
		return from, from.AddDate(1, 0, 0), "year"
	default:
		from = today.AddDate(0, 0, offset)
		return from, from.AddDate(0, 0, 1), "day"
	}
}

// HistoryPoint is a downsampled history entry.
type HistoryPoint = store.Sample

// History returns samples downsampled to at most maxPoints.
func (a *App) History(ctx context.Context, id string, from, to time.Time, maxPoints int) ([]store.Sample, error) {
	list, err := a.Store.Samples(ctx, id, from.Unix(), to.Unix())
	if err != nil || len(list) <= maxPoints || maxPoints <= 0 {
		return list, err
	}

	step := float64(len(list)) / float64(maxPoints)
	out := make([]store.Sample, 0, maxPoints)

	for i := 0.0; int(i) < len(list); i += step {
		out = append(out, list[int(i)])
	}

	return out, nil
}

// WriteCSV writes the history of a device as CSV (semicolon, German Excel
// friendly decimal comma optional).
func (a *App) WriteCSV(ctx context.Context, w io.Writer, id string, from, to time.Time, decimalComma bool) error {
	list, err := a.Store.Samples(ctx, id, from.Unix(), to.Unix())
	if err != nil {
		return err
	}

	cw := csv.NewWriter(w)
	cw.Comma = ';'

	f := func(v *float64) string {
		if v == nil {
			return ""
		}

		s := strconv.FormatFloat(*v, 'f', -1, 64)
		if decimalComma {
			s = replaceDot(s)
		}

		return s
	}

	_ = cw.Write([]string{"time", "power", "mode", "target_c", "indoor_c", "outdoor_c", "watts", "energy_kwh", "fan"})

	for _, s := range list {
		t := s.Target
		_ = cw.Write([]string{
			time.Unix(s.TS, 0).In(a.Loc).Format("2006-01-02 15:04:05"), fmt.Sprint(b01(s.Power)), s.Mode,
			f(&t), f(s.Indoor), f(s.Outdoor), f(s.Watts), f(s.KWh), strconv.Itoa(s.Fan),
		})
	}

	cw.Flush()

	return cw.Error()
}

func replaceDot(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c == '.' {
			b[i] = ','
		}
	}

	return string(b)
}

func b01(v bool) int {
	if v {
		return 1
	}

	return 0
}
