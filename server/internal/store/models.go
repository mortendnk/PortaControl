package store

import (
	"time"

	"github.com/mortendnk/portacontrol/server/internal/midea"
)

// Device is a configured air conditioner. Token and Key are secrets and are
// never sent to non-admin clients.
type Device struct {
	ID          string    `json:"id"` // Midea device id (decimal)
	Name        string    `json:"name"`
	Room        string    `json:"room"`
	IP          string    `json:"ip"`
	Port        int       `json:"port"`
	Token       string    `json:"token,omitempty"`
	Key         string    `json:"key,omitempty"`
	SN          string    `json:"sn,omitempty"`
	Mode        string    `json:"connection"` // auto, lan, cloud
	Order       int       `json:"order"`
	Favorite    bool      `json:"favorite"`
	Simulated   bool      `json:"simulated,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	MinTarget   float64   `json:"min_target,omitempty"` // child lock style limits
	MaxTarget   float64   `json:"max_target,omitempty"`
	Locked      bool      `json:"locked,omitempty"` // guests may not control
	PowerRating float64   `json:"power_rating,omitempty"`
}

// Control is a (partial) change request. Nil fields stay unchanged. Scenes,
// schedules, rules, MQTT and the API all use it.
type Control struct {
	Power            *bool    `json:"power,omitempty"`
	Mode             *string  `json:"mode,omitempty"`
	Target           *float64 `json:"target_temperature,omitempty"`
	Fan              *int     `json:"fan_speed,omitempty"`
	Swing            *bool    `json:"swing,omitempty"`
	Eco              *bool    `json:"eco,omitempty"`
	Turbo            *bool    `json:"turbo,omitempty"`
	Sleep            *bool    `json:"sleep,omitempty"`
	Purifier         *bool    `json:"purifier,omitempty"`
	FreezeProtection *bool    `json:"freeze_protection,omitempty"`
	Fahrenheit       *bool    `json:"fahrenheit,omitempty"`
	OnTimerMinutes   *int     `json:"on_timer_minutes,omitempty"`  // 0 clears
	OffTimerMinutes  *int     `json:"off_timer_minutes,omitempty"` // 0 clears
	OutdoorSilent    *bool    `json:"outdoor_silent,omitempty"`
	SelfClean        *bool    `json:"self_clean,omitempty"`
	IECO             *bool    `json:"ieco,omitempty"`
	Sound            *bool    `json:"sound,omitempty"`
	Rate             *int     `json:"rate_percent,omitempty"`
	Display          *bool    `json:"display_on,omitempty"`
}

// Empty reports whether c changes nothing.
func (c Control) Empty() bool { return c == Control{} }

// Scene is a stored wish state.
type Scene struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Icon    string   `json:"icon"`
	Devices []string `json:"devices"` // empty = the device it is applied to / all
	Control Control  `json:"control"`
	Order   int      `json:"order"`
}

// Action is what an automation does.
type Action struct {
	Kind    string   `json:"kind"` // on, off, scene, control, notify
	SceneID string   `json:"scene_id,omitempty"`
	Control *Control `json:"control,omitempty"`
	Text    string   `json:"text,omitempty"`
}

// Schedule is a weekly program entry.
type Schedule struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Enabled bool      `json:"enabled"`
	Devices []string  `json:"devices"` // empty = all
	Days    []int     `json:"days"`    // 0 = Sunday … 6 = Saturday; empty = every day
	Time    string    `json:"time"`    // HH:MM
	Action  Action    `json:"action"`
	LastRun time.Time `json:"last_run"`
	// SkipNext skips the next run once (vacation, today different).
	SkipNext bool `json:"skip_next"`
}

// Rule is a climate automation: when the sensor crosses the threshold, run
// the action (edge triggered, with hysteresis and an optional time window).
type Rule struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Enabled    bool      `json:"enabled"`
	Device     string    `json:"device"`
	Sensor     string    `json:"sensor"` // indoor, outdoor, power_watts, humidity
	Op         string    `json:"op"`     // above, below
	Value      float64   `json:"value"`
	Hysteresis float64   `json:"hysteresis"`
	From       string    `json:"from,omitempty"` // HH:MM window start
	To         string    `json:"to,omitempty"`
	OnlyIfOff  bool      `json:"only_if_off,omitempty"`
	OnlyIfOn   bool      `json:"only_if_on,omitempty"`
	Action     Action    `json:"action"`
	Cooldown   int       `json:"cooldown_minutes"`
	LastFired  time.Time `json:"last_fired"`
	Armed      bool      `json:"armed"`
}

// Timers are app side timers of one device (run by the server, also while
// no client is open). The device's own timers are part of Control.
type Timers struct {
	OnAt      *time.Time `json:"on_at"`
	OnSceneID string     `json:"on_scene_id,omitempty"`
	OffAt     *time.Time `json:"off_at"`
}

// Webhook is an outgoing HTTP POST on events.
type Webhook struct {
	ID      string   `json:"id"`
	URL     string   `json:"url"`
	Events  []string `json:"events"` // empty = all: state, notice, automation
	Enabled bool     `json:"enabled"`
}

// MQTTSettings configure the MQTT bridge (Home Assistant discovery).
type MQTTSettings struct {
	Enabled         bool   `json:"enabled"`
	Broker          string `json:"broker"` // tcp://host:1883
	Username        string `json:"username"`
	Password        string `json:"password,omitempty"`
	Prefix          string `json:"prefix"`
	Discovery       bool   `json:"discovery"`
	DiscoveryPrefix string `json:"discovery_prefix"`
}

// CloudSettings is the Midea account (optional).
type CloudSettings struct {
	Account  string                `json:"account"`
	Password string                `json:"password,omitempty"`
	ClientID string                `json:"client_id,omitempty"`
	Sessions []*midea.CloudSession `json:"sessions,omitempty"`
}

// NotifySettings choose push notifications (ntfy compatible).
type NotifySettings struct {
	NtfyURL      string   `json:"ntfy_url"` // https://ntfy.sh/my-topic
	FilterAlert  bool     `json:"filter_alert"`
	ErrorCode    bool     `json:"error_code"`
	Offline      bool     `json:"offline"`
	Automation   bool     `json:"automation"`
	TempAlarmMax *float64 `json:"temp_alarm_max,omitempty"`
	TempAlarmMin *float64 `json:"temp_alarm_min,omitempty"`
}

// Settings are runtime settings stored in the database.
type Settings struct {
	PricePerKWh       float64        `json:"price_per_kwh"`
	Currency          string         `json:"currency"`
	PollSeconds       int            `json:"poll_seconds"`
	FastPollSeconds   int            `json:"fast_poll_seconds"`
	IdleDisconnect    int            `json:"idle_disconnect_seconds"`
	SampleSeconds     int            `json:"sample_seconds"`
	HistoryDays       int            `json:"history_days"`
	GuestControl      bool           `json:"guest_control"`
	Language          string         `json:"language"`
	TemperatureUnit   string         `json:"temperature_unit"` // C or F (display)
	Beep              bool           `json:"beep"`
	Cloud             CloudSettings  `json:"cloud"`
	MQTT              MQTTSettings   `json:"mqtt"`
	Webhooks          []Webhook      `json:"webhooks"`
	Notify            NotifySettings `json:"notify"`
	CO2GramsPerKWh    float64        `json:"co2_grams_per_kwh"`
	MonthlyBudgetKWh  float64        `json:"monthly_budget_kwh"`
	SolarSurplusTopic string         `json:"solar_surplus_topic,omitempty"`
}

// DefaultSettings are used on first start.
func DefaultSettings() Settings {
	return Settings{
		PricePerKWh: 0.35, Currency: "EUR", PollSeconds: 30, FastPollSeconds: 5, IdleDisconnect: 8,
		SampleSeconds: 60, HistoryDays: 400, GuestControl: true, Language: "de", TemperatureUnit: "C",
		Beep: true, CO2GramsPerKWh: 380,
		MQTT:   MQTTSettings{Prefix: "portacontrol", Discovery: true, DiscoveryPrefix: "homeassistant"},
		Notify: NotifySettings{FilterAlert: true, ErrorCode: true, Offline: true},
	}
}

// Sample is one history point.
type Sample struct {
	Device  string   `json:"-"`
	TS      int64    `json:"t"`
	Power   bool     `json:"on"`
	Mode    string   `json:"mode"`
	Target  float64  `json:"target"`
	Indoor  *float64 `json:"indoor"`
	Outdoor *float64 `json:"outdoor"`
	Watts   *float64 `json:"watts"`
	KWh     *float64 `json:"kwh"`
	Fan     int      `json:"fan"`
}

// LogEntry is one line of the event log.
type LogEntry struct {
	ID     int64     `json:"id"`
	Time   time.Time `json:"time"`
	Device string    `json:"device,omitempty"`
	Level  string    `json:"level"` // info, warn, error
	Text   string    `json:"text"`
}
