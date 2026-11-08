package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mortendnk/portacontrol/server/internal/api"
	"github.com/mortendnk/portacontrol/server/internal/app"
	"github.com/mortendnk/portacontrol/server/internal/config"
	"github.com/mortendnk/portacontrol/server/internal/store"
)

func setup(t *testing.T) (*httptest.Server, *app.App) {
	t.Helper()

	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = st.Close() })

	cfg := config.Default()
	cfg.Simulate = true
	cfg.TimeZone = "Europe/Berlin"

	a := app.New(cfg, st, nil)
	if err := a.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(a.Close)

	srv := httptest.NewServer(api.New(a, nil).Router())
	t.Cleanup(srv.Close)

	return srv, a
}

func do(t *testing.T, srv *httptest.Server, method, path string, body any, out any) int {
	t.Helper()

	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}

	req, _ := http.NewRequest(method, srv.URL+path, rd)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if out != nil && resp.StatusCode < 300 {
		if err := json.Unmarshal(data, out); err != nil {
			t.Fatalf("%s %s: %v: %s", method, path, err, data)
		}
	}

	if resp.StatusCode >= 300 {
		t.Logf("%s %s -> %d %s", method, path, resp.StatusCode, data)
	}

	return resp.StatusCode
}

func TestAPIFlow(t *testing.T) {
	srv, a := setup(t)

	var devices []app.View

	// Wait for the first poll of the simulated device.
	deadline := time.Now().Add(15 * time.Second)

	for {
		do(t, srv, http.MethodGet, "/api/v1/devices", nil, &devices)

		if len(devices) == 1 && devices[0].State != nil || time.Now().After(deadline) {
			break
		}

		time.Sleep(300 * time.Millisecond)
	}

	if len(devices) != 1 || devices[0].State == nil {
		t.Fatalf("no simulated device: %+v", devices)
	}

	id := devices[0].ID
	if devices[0].Token != "" {
		t.Fatal("token leaked to the device list")
	}

	on, mode, target := true, "cool", 22.0

	var v app.View
	if code := do(t, srv, http.MethodPost, "/api/v1/devices/"+id+"/control", store.Control{Power: &on, Mode: &mode, Target: &target}, &v); code != 200 {
		t.Fatalf("control %d", code)
	}

	if !v.State.Power || v.State.ModeName != "cool" || v.State.TargetTemperature != 22 {
		t.Fatalf("state %+v", v.State)
	}

	bad := 40.0
	if code := do(t, srv, http.MethodPost, "/api/v1/devices/"+id+"/control", store.Control{Target: &bad}, nil); code != 400 {
		t.Fatalf("want 400, got %d", code)
	}

	// Scene
	off := false

	var sc store.Scene
	if code := do(t, srv, http.MethodPost, "/api/v1/scenes", store.Scene{Name: "Aus", Control: store.Control{Power: &off}}, &sc); code != 200 {
		t.Fatal(code)
	}

	if code := do(t, srv, http.MethodPost, "/api/v1/scenes/"+sc.ID+"/apply?device="+id, nil, nil); code != 200 {
		t.Fatal(code)
	}

	if v, _ := a.View(id, false); v.State.Power {
		t.Fatal("scene did not switch off")
	}

	// Schedule validation
	if code := do(t, srv, http.MethodPost, "/api/v1/schedules", store.Schedule{Name: "x", Time: "25:00", Action: store.Action{Kind: "off"}}, nil); code != 400 {
		t.Fatalf("want 400 for bad time, got %d", code)
	}

	if code := do(t, srv, http.MethodPost, "/api/v1/schedules", store.Schedule{Name: "Abends aus", Enabled: true, Time: "22:00", Days: []int{1, 2, 3}, Action: store.Action{Kind: "off"}}, nil); code != 200 {
		t.Fatal(code)
	}

	// Energy and history endpoints answer.
	var en map[string]any
	if code := do(t, srv, http.MethodGet, "/api/v1/devices/"+id+"/energy?period=week", nil, &en); code != 200 || len(en["buckets"].([]any)) != 7 {
		t.Fatalf("energy %d %v", code, en)
	}

	var diag map[string]any
	if code := do(t, srv, http.MethodGet, "/api/v1/devices/"+id+"/diagnostics", nil, &diag); code != 200 || diag["compressor_hz"] == nil {
		t.Fatalf("diagnostics %d %v", code, diag)
	}

	var link map[string]string
	if code := do(t, srv, http.MethodGet, "/api/v1/system/app-link", nil, &link); code != 200 || link["api_key"] == "" {
		t.Fatalf("app-link %d", code)
	}

	var backup map[string]any
	if code := do(t, srv, http.MethodGet, "/api/v1/backup", nil, &backup); code != 200 || len(backup["devices"].([]any)) != 1 {
		t.Fatalf("backup %d", code)
	}
}
