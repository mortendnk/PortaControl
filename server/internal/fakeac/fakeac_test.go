package fakeac_test

import (
	"errors"
	"testing"

	"github.com/mortendnk/portacontrol/server/internal/fakeac"
	"github.com/mortendnk/portacontrol/server/internal/midea"
)

func start(t *testing.T) (*fakeac.AC, *midea.Device) {
	t.Helper()

	ac := fakeac.New(20015998343868)
	if err := ac.Listen("127.0.0.1:0", ""); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(ac.Close)

	cfg := midea.DeviceConfig{DeviceID: ac.DeviceID, IP: "127.0.0.1", Port: ac.Port, Token: ac.TokenHex(), Key: ac.KeyHex()}

	d, err := midea.NewDevice(cfg, midea.ConnLAN, nil)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(d.Close)

	return ac, d
}

func TestEndToEnd(t *testing.T) {
	ac, d := start(t)

	s, err := d.Refresh(true)
	if err != nil {
		t.Fatal(err)
	}

	if s.Power || s.Mode != midea.ModeCool || s.TotalEnergyKWh == nil || s.Sound == nil || !*s.Sound {
		t.Fatalf("initial %+v", s)
	}

	s, err = d.Update(func(s *midea.State) {
		s.Power = true
		s.Mode = midea.ModeHeat
		s.TargetTemperature = 21
		s.FanSpeed = 55
		s.FreezeProtection = true
		s.Swing = midea.SwingVertical
	})
	if err != nil {
		t.Fatal(err)
	}

	if !s.Power || s.Mode != midea.ModeHeat || s.TargetTemperature != 21 || s.FanSpeed != 55 || !s.FreezeProtection || !s.SwingOn {
		t.Fatalf("after update %+v", s)
	}

	on := 120
	if s, err = d.Update(func(s *midea.State) { s.OffTimerMinutes = &on }); err != nil || s.OffTimerMinutes == nil {
		t.Fatalf("timer %v %+v", err, s)
	}

	if s, err = d.SetProperty(midea.PropOutSilent, true); err != nil || s.OutSilent == nil || !*s.OutSilent {
		t.Fatalf("outsilent %v %+v", err, s.OutSilent)
	}

	if s, err = d.SetProperty(midea.PropIECO, true); err != nil || !*s.IECO {
		t.Fatalf("ieco %v", err)
	}

	if s, err = d.SetRate(50); err != nil || *s.RatePercent != 50 {
		t.Fatalf("rate %v", err)
	}

	var na *midea.ErrNotApplied
	if _, err = d.SetRate(75); !errors.As(err, &na) {
		t.Fatalf("rate 75 should not apply: %v", err)
	}

	if s, err = d.SetDisplay(false); err != nil || s.DisplayOn {
		t.Fatalf("display %v %v", err, s.DisplayOn)
	}

	diag, err := d.Diagnostics()
	if err != nil || diag.CompressorHz == nil || diag.IndoorTemp2 == nil || diag.Identifier == "" {
		t.Fatalf("diag %v %+v", err, diag)
	}

	_ = ac
}

func TestWrongToken(t *testing.T) {
	ac := fakeac.New(1)
	if err := ac.Listen("127.0.0.1:0", ""); err != nil {
		t.Fatal(err)
	}
	defer ac.Close()

	other := fakeac.New(1)
	cfg := midea.DeviceConfig{DeviceID: 1, IP: "127.0.0.1", Port: ac.Port, Token: other.TokenHex(), Key: other.KeyHex()}

	if err := midea.Verify(cfg); !errors.Is(err, midea.ErrAuth) {
		t.Fatalf("want ErrAuth, got %v", err)
	}
}
