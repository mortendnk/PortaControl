// Package mqtt bridges PortaControl to an MQTT broker with Home Assistant
// discovery (climate entity plus sensors, switches, select and button).
package mqtt

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"

	"github.com/mortendnk/portacontrol/server/internal/app"
	"github.com/mortendnk/portacontrol/server/internal/midea"
	"github.com/mortendnk/portacontrol/server/internal/store"
	"github.com/mortendnk/portacontrol/server/internal/version"
)

// Bridge is the MQTT connection; Reload applies changed settings.
type Bridge struct {
	App *app.App
	Log *slog.Logger

	mu     sync.Mutex
	client paho.Client
	cfg    store.MQTTSettings
	status string
}

// Status is shown in the settings.
func (b *Bridge) Status() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.status == "" {
		return "aus"
	}

	return b.status
}

func (b *Bridge) setStatus(s string) {
	b.mu.Lock()
	b.status = s
	b.mu.Unlock()
}

// Reload (re)connects according to the settings.
func (b *Bridge) Reload() {
	s := b.App.Store.Settings().MQTT

	b.mu.Lock()
	old := b.client
	b.client = nil
	b.cfg = s
	b.mu.Unlock()

	if old != nil {
		b.publishAvailability(old, "offline")
		old.Disconnect(500)
	}

	if !s.Enabled || s.Broker == "" {
		b.setStatus("aus")
		return
	}

	broker := s.Broker
	if !strings.Contains(broker, "://") {
		broker = "tcp://" + broker
	}

	if !strings.Contains(strings.SplitN(broker, "://", 2)[1], ":") {
		broker += ":1883"
	}

	opts := paho.NewClientOptions().AddBroker(broker).SetClientID("portacontrol-"+store.NewID()[:6]).
		SetUsername(s.Username).SetPassword(s.Password).SetAutoReconnect(true).SetConnectRetry(true).
		SetConnectRetryInterval(10*time.Second).SetKeepAlive(30*time.Second).
		SetWill(s.Prefix+"/bridge/availability", "offline", 1, true)

	opts.SetOnConnectHandler(func(c paho.Client) {
		b.setStatus("verbunden mit " + broker)
		b.Log.Info("mqtt connected", "broker", broker)
		c.Publish(s.Prefix+"/bridge/availability", 1, true, "online")
		c.Subscribe(s.Prefix+"/+/set/+", 1, b.onCommand)
		c.Subscribe(s.Prefix+"/+/set", 1, b.onCommand)
		c.Subscribe(s.DiscoveryPrefix+"/status", 1, func(paho.Client, paho.Message) { b.announceAll() })
		b.announceAll()
	})
	opts.SetConnectionLostHandler(func(_ paho.Client, err error) {
		b.setStatus("getrennt: " + err.Error())
	})

	c := paho.NewClient(opts)

	b.mu.Lock()
	b.client = c
	b.mu.Unlock()
	b.setStatus("verbinde mit " + broker + " …")

	c.Connect()
}

// Close disconnects.
func (b *Bridge) Close() {
	b.mu.Lock()
	c := b.client
	b.client = nil
	b.mu.Unlock()

	if c != nil {
		b.publishAvailability(c, "offline")
		c.Disconnect(500)
	}
}

func (b *Bridge) publishAvailability(c paho.Client, v string) {
	if c.IsConnected() {
		c.Publish(b.cfg.Prefix+"/bridge/availability", 1, true, v).WaitTimeout(time.Second)
	}
}

func (b *Bridge) conn() (paho.Client, store.MQTTSettings) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.client == nil || !b.client.IsConnected() {
		return nil, b.cfg
	}

	return b.client, b.cfg
}

func haMode(s *midea.State) string {
	if s == nil || !s.Power {
		return "off"
	}

	if s.Mode == midea.ModeFan {
		return "fan_only"
	}

	return s.Mode.String()
}

var fanNames = map[int]string{midea.FanAuto: "auto", midea.FanSilent: "silent", midea.FanLow: "low", midea.FanMedium: "medium", midea.FanHigh: "high", midea.FanMax: "max"}

func haFan(s *midea.State) string {
	if s == nil {
		return "auto"
	}

	if n, ok := fanNames[s.FanSpeed]; ok {
		return n
	}

	// Stepless value: nearest preset.
	best, name := 1000, "auto"
	for v, n := range fanNames {
		if v == midea.FanAuto {
			continue
		}

		if d := abs(v - s.FanSpeed); d < best {
			best, name = d, n
		}
	}

	return name
}

func abs(v int) int {
	if v < 0 {
		return -v
	}

	return v
}

func haPreset(s *midea.State) string {
	switch {
	case s == nil:
		return "none"
	case s.Turbo:
		return "boost"
	case s.Eco:
		return "eco"
	case s.Sleep:
		return "sleep"
	}

	return "none"
}

func onOff(v *bool) string {
	if v != nil && *v {
		return "ON"
	}

	return "OFF"
}

// PublishState sends the state of one device (called after every read).
func (b *Bridge) PublishState(id string) {
	c, cfg := b.conn()
	if c == nil {
		return
	}

	v, err := b.App.View(id, false)
	if err != nil {
		return
	}

	avail := "offline"
	if v.Online {
		avail = "online"
	}

	c.Publish(cfg.Prefix+"/"+id+"/availability", 1, true, avail)

	if v.State == nil {
		return
	}

	s := v.State
	payload := map[string]any{
		"power": onOff(&s.Power), "mode": s.ModeName, "ha_mode": haMode(s), "ha_fan": haFan(s), "ha_preset": haPreset(s),
		"ha_swing":           map[bool]string{true: "vertical", false: "off"}[s.SwingOn],
		"target_temperature": s.TargetTemperature, "indoor_temperature": s.IndoorTemperature, "outdoor_temperature": s.OutdoorTemp,
		"fan_speed": s.FanSpeed, "power_watts": s.PowerWatts, "total_energy_kwh": s.TotalEnergyKWh,
		"error_code": s.ErrorCode, "filter_alert": onOff(&s.FilterAlert),
		"outdoor_silent": onOff(s.OutSilent), "ieco": onOff(s.IECO), "purifier": onOff(&s.Purifier),
		"display_on": onOff(&s.DisplayOn), "freeze_protection": onOff(&s.FreezeProtection), "sound": onOff(s.Sound),
		"self_clean": onOff(s.SelfClean), "eco": onOff(&s.Eco), "turbo": onOff(&s.Turbo), "sleep": onOff(&s.Sleep),
		"swing": onOff(&s.SwingOn), "path": v.Path,
	}

	if s.RatePercent != nil {
		payload["rate_percent"] = strconv.Itoa(*s.RatePercent)
	}

	if v.Today != nil {
		payload["today_kwh"] = v.Today.KWh
		payload["today_cost"] = v.Today.Cost
	}

	js, _ := json.Marshal(payload)
	c.Publish(cfg.Prefix+"/"+id+"/state", 0, true, js)
}

func (b *Bridge) announceAll() {
	for _, v := range b.App.Views(false) {
		b.announce(v)
		b.PublishState(v.ID)
	}
}

// Announce publishes the Home Assistant discovery of all devices again
// (after devices changed).
func (b *Bridge) Announce() { b.announceAll() }

func (b *Bridge) announce(v app.View) {
	c, cfg := b.conn()
	if c == nil || !cfg.Discovery {
		return
	}

	p := cfg.Prefix + "/" + v.ID
	uid := "portacontrol_" + v.ID
	dev := map[string]any{"identifiers": []string{uid}, "name": v.Name, "manufacturer": "Midea", "model": "PortaSplit",
		"sw_version": "PortaControl " + version.Version, "suggested_area": v.Room}
	avail := []map[string]string{{"topic": cfg.Prefix + "/bridge/availability"}, {"topic": p + "/availability"}}
	state := p + "/state"

	pub := func(component, object string, cfg2 map[string]any) {
		cfg2["unique_id"] = uid + "_" + object
		cfg2["object_id"] = uid + "_" + object
		cfg2["device"] = dev
		cfg2["availability"] = avail
		cfg2["availability_mode"] = "all"
		js, _ := json.Marshal(cfg2)
		c.Publish(fmt.Sprintf("%s/%s/%s/%s/config", cfg.DiscoveryPrefix, component, uid, object), 1, true, js)
	}

	pub("climate", "climate", map[string]any{
		"name": nil, "modes": []string{"off", "auto", "cool", "dry", "heat", "fan_only"},
		"mode_state_topic": state, "mode_state_template": "{{ value_json.ha_mode }}", "mode_command_topic": p + "/set/mode",
		"temperature_state_topic": state, "temperature_state_template": "{{ value_json.target_temperature }}",
		"temperature_command_topic": p + "/set/target_temperature",
		"current_temperature_topic": state, "current_temperature_template": "{{ value_json.indoor_temperature }}",
		"fan_modes": []string{"auto", "silent", "low", "medium", "high", "max"}, "fan_mode_state_topic": state,
		"fan_mode_state_template": "{{ value_json.ha_fan }}", "fan_mode_command_topic": p + "/set/fan_mode",
		"swing_modes": []string{"off", "vertical"}, "swing_mode_state_topic": state,
		"swing_mode_state_template": "{{ value_json.ha_swing }}", "swing_mode_command_topic": p + "/set/swing_mode",
		"preset_modes": []string{"eco", "boost", "sleep"}, "preset_mode_state_topic": state,
		"preset_mode_value_template": "{{ value_json.ha_preset }}", "preset_mode_command_topic": p + "/set/preset_mode",
		"power_command_topic": p + "/set/power", "min_temp": 16, "max_temp": 30, "temp_step": 1, "precision": 0.1,
		"temperature_unit": "C",
	})

	sensor := func(object, name, tpl, unit, class, stateClass string) {
		m := map[string]any{"name": name, "state_topic": state, "value_template": tpl}
		if unit != "" {
			m["unit_of_measurement"] = unit
		}

		if class != "" {
			m["device_class"] = class
		}

		if stateClass != "" {
			m["state_class"] = stateClass
		}

		pub("sensor", object, m)
	}

	sensor("indoor", "Innentemperatur", "{{ value_json.indoor_temperature }}", "°C", "temperature", "measurement")
	sensor("outdoor", "Außentemperatur", "{{ value_json.outdoor_temperature }}", "°C", "temperature", "measurement")
	sensor("power", "Leistung", "{{ value_json.power_watts }}", "W", "power", "measurement")
	sensor("energy", "Verbrauch gesamt", "{{ value_json.total_energy_kwh }}", "kWh", "energy", "total_increasing")
	sensor("today_kwh", "Verbrauch heute", "{{ value_json.today_kwh }}", "kWh", "energy", "")
	sensor("error", "Fehlercode", "{{ value_json.error_code }}", "", "", "")

	pub("binary_sensor", "filter", map[string]any{"name": "Filter reinigen", "state_topic": state,
		"value_template": "{{ value_json.filter_alert }}", "device_class": "problem"})

	sw := func(object, name, icon string) {
		pub("switch", object, map[string]any{"name": name, "state_topic": state, "value_template": "{{ value_json." + object + " }}",
			"command_topic": p + "/set/" + object, "icon": icon})
	}

	sw("outdoor_silent", "Außengerät leise", "mdi:volume-off")
	sw("ieco", "iECO", "mdi:leaf-circle")
	sw("purifier", "Ionisierer", "mdi:creation")
	sw("display_on", "Anzeige", "mdi:monitor")
	sw("freeze_protection", "Frostschutz", "mdi:snowflake-thermometer")
	sw("sound", "Signalton", "mdi:volume-high")
	sw("swing", "Schwenken", "mdi:arrow-oscillating")

	pub("select", "rate_percent", map[string]any{"name": "Leistungsbegrenzung", "state_topic": state,
		"value_template": "{{ value_json.rate_percent }}", "command_topic": p + "/set/rate_percent",
		"options": []string{"50", "75", "100"}, "icon": "mdi:gauge"})

	pub("button", "self_clean", map[string]any{"name": "Selbstreinigung starten", "command_topic": p + "/set/self_clean",
		"payload_press": "ON", "icon": "mdi:spray-bottle"})
}

// onCommand handles <prefix>/<id>/set/<field> and <prefix>/<id>/set (JSON).
func (b *Bridge) onCommand(_ paho.Client, m paho.Message) {
	_, cfg := b.conn()
	parts := strings.Split(strings.TrimPrefix(m.Topic(), cfg.Prefix+"/"), "/")

	if len(parts) < 2 || parts[1] != "set" {
		return
	}

	id := parts[0]
	val := strings.TrimSpace(string(m.Payload()))

	var c store.Control

	if len(parts) == 2 {
		if err := json.Unmarshal(m.Payload(), &c); err != nil {
			b.Log.Warn("mqtt: bad JSON command", "err", err)
			return
		}
	} else if !b.parseField(&c, parts[2], val, id) {
		return
	}

	go func() {
		if _, err := b.App.Control(id, c, "MQTT"); err != nil {
			b.Log.Warn("mqtt command", "device", id, "err", err)
		}

		b.PublishState(id)
	}()
}

func (b *Bridge) parseField(c *store.Control, field, val, id string) bool {
	t, f := true, false
	isOn := strings.EqualFold(val, "ON") || val == "1" || strings.EqualFold(val, "true")
	bv := &f

	if isOn {
		bv = &t
	}

	switch field {
	case "power":
		c.Power = bv
	case "mode":
		if val == "off" {
			c.Power = &f
			return true
		}

		if val == "fan_only" {
			val = "fan"
		}

		if _, ok := midea.ParseMode(val); !ok {
			return false
		}

		c.Power, c.Mode = &t, &val
	case "target_temperature":
		v, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return false
		}

		c.Target = &v
	case "fan_mode", "fan_speed":
		for v, n := range fanNames {
			if n == val {
				c.Fan = &v
				return true
			}
		}

		v, err := strconv.Atoi(val)
		if err != nil {
			return false
		}

		c.Fan = &v
	case "swing_mode":
		on := val == "vertical"
		c.Swing = &on
	case "preset_mode":
		c.Eco, c.Turbo, c.Sleep = &f, &f, &f

		switch val {
		case "eco":
			c.Eco = &t
		case "boost":
			c.Turbo = &t
		case "sleep":
			c.Sleep = &t
		}
	case "outdoor_silent":
		c.OutdoorSilent = bv
	case "ieco":
		c.IECO = bv
	case "purifier":
		c.Purifier = bv
	case "display_on":
		c.Display = bv
	case "freeze_protection":
		c.FreezeProtection = bv
	case "sound":
		c.Sound = bv
	case "swing":
		c.Swing = bv
	case "eco":
		c.Eco = bv
	case "turbo":
		c.Turbo = bv
	case "sleep":
		c.Sleep = bv
	case "self_clean":
		c.SelfClean = bv
	case "rate_percent":
		v, err := strconv.Atoi(val)
		if err != nil {
			return false
		}

		c.Rate = &v
	case "scene":
		go func() { _ = b.App.ApplyScene(val, id, "MQTT") }()
		return false
	default:
		return false
	}

	return true
}
