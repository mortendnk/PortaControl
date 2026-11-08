// Package store persists devices, scenes, automations, settings, history
// samples and the event log in SQLite (pure Go driver, no cgo).
package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	_ "modernc.org/sqlite" // driver
)

// ErrNotFound is returned for missing records.
var ErrNotFound = errors.New("not found")

// Store is the database.
type Store struct {
	db *sql.DB
	mu sync.Mutex // serialises writers (SQLite)
}

const schema = `
CREATE TABLE IF NOT EXISTS kv (k TEXT PRIMARY KEY, v TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS docs (coll TEXT NOT NULL, id TEXT NOT NULL, v TEXT NOT NULL, PRIMARY KEY (coll, id));
CREATE TABLE IF NOT EXISTS samples (
  device TEXT NOT NULL, ts INTEGER NOT NULL, power INTEGER NOT NULL, mode TEXT NOT NULL, target REAL NOT NULL,
  indoor REAL, outdoor REAL, watts REAL, kwh REAL, fan INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (device, ts)) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS log (id INTEGER PRIMARY KEY AUTOINCREMENT, ts INTEGER NOT NULL, device TEXT NOT NULL DEFAULT '',
  level TEXT NOT NULL, text TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS log_ts ON log (ts);
CREATE TABLE IF NOT EXISTS sessions (token TEXT PRIMARY KEY, expires INTEGER NOT NULL);
`

// Open opens (and creates) data_dir/portacontrol.db.
func Open(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		return nil, err
	}

	path := filepath.Join(dataDir, "portacontrol.db")

	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(4)

	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("schema: %w", err)
	}

	_ = os.Chmod(path, 0o600)

	return &Store{db: db}, nil
}

// OpenMemory opens an in-memory database (tests).
func OpenMemory() (*Store, error) {
	db, err := sql.Open("sqlite", "file::memory:?cache=shared&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(1)

	if _, err := db.Exec(schema); err != nil {
		return nil, err
	}

	return &Store{db: db}, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// GetKV reads a JSON value; ErrNotFound if missing.
func (s *Store) GetKV(key string, v any) error {
	var raw string

	err := s.db.QueryRow(`SELECT v FROM kv WHERE k = ?`, key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}

	if err != nil {
		return err
	}

	return json.Unmarshal([]byte(raw), v)
}

// SetKV writes a JSON value.
func (s *Store) SetKV(key string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	_, err = s.db.Exec(`INSERT INTO kv (k, v) VALUES (?, ?) ON CONFLICT(k) DO UPDATE SET v = excluded.v`, key, string(b))

	return err
}

// DeleteKV removes a key.
func (s *Store) DeleteKV(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`DELETE FROM kv WHERE k = ?`, key)

	return err
}

// Collections.
const (
	CollDevices   = "devices"
	CollScenes    = "scenes"
	CollSchedules = "schedules"
	CollRules     = "rules"
	CollTimers    = "timers"
)

// List reads all documents of a collection into out (pointer to slice).
func List[T any](s *Store, coll string) ([]T, error) {
	rows, err := s.db.Query(`SELECT v FROM docs WHERE coll = ? ORDER BY id`, coll)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []T

	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}

		var v T
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			continue
		}

		out = append(out, v)
	}

	return out, rows.Err()
}

// Get reads one document.
func Get[T any](s *Store, coll, id string) (T, error) {
	var v T

	var raw string

	err := s.db.QueryRow(`SELECT v FROM docs WHERE coll = ? AND id = ?`, coll, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return v, ErrNotFound
	}

	if err != nil {
		return v, err
	}

	return v, json.Unmarshal([]byte(raw), &v)
}

// Put writes one document.
func (s *Store) Put(coll, id string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	_, err = s.db.Exec(`INSERT INTO docs (coll, id, v) VALUES (?, ?, ?) ON CONFLICT(coll, id) DO UPDATE SET v = excluded.v`, coll, id, string(b))

	return err
}

// Delete removes one document.
func (s *Store) Delete(coll, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	res, err := s.db.Exec(`DELETE FROM docs WHERE coll = ? AND id = ?`, coll, id)
	if err != nil {
		return err
	}

	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}

	return nil
}

// NewID returns a random id.
func NewID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)

	return hex.EncodeToString(b)
}

// Settings returns the runtime settings (defaults filled in).
func (s *Store) Settings() Settings {
	v := DefaultSettings()
	_ = s.GetKV("settings", &v)

	if v.PollSeconds < 5 {
		v.PollSeconds = 30
	}

	if v.FastPollSeconds < 2 {
		v.FastPollSeconds = 5
	}

	if v.SampleSeconds < 10 {
		v.SampleSeconds = 60
	}

	if v.IdleDisconnect < 2 {
		v.IdleDisconnect = 8
	}

	if v.HistoryDays <= 0 {
		v.HistoryDays = 400
	}

	if v.Currency == "" {
		v.Currency = "EUR"
	}

	if v.MQTT.Prefix == "" {
		v.MQTT.Prefix = "portacontrol"
	}

	if v.MQTT.DiscoveryPrefix == "" {
		v.MQTT.DiscoveryPrefix = "homeassistant"
	}

	return v
}

// SaveSettings stores the settings.
func (s *Store) SaveSettings(v Settings) error { return s.SetKV("settings", v) }

// AddSample stores one history point.
func (s *Store) AddSample(x Sample) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`INSERT OR REPLACE INTO samples (device, ts, power, mode, target, indoor, outdoor, watts, kwh, fan)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, x.Device, x.TS, x.Power, x.Mode, x.Target, x.Indoor, x.Outdoor, x.Watts, x.KWh, x.Fan)

	return err
}

// Samples returns the points of a device between from and to (unix seconds).
func (s *Store) Samples(ctx context.Context, device string, from, to int64) ([]Sample, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT ts, power, mode, target, indoor, outdoor, watts, kwh, fan FROM samples
		WHERE device = ? AND ts >= ? AND ts <= ? ORDER BY ts`, device, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Sample

	for rows.Next() {
		var (
			x                    Sample
			in, outd, watts, kwh sql.NullFloat64
		)

		if err := rows.Scan(&x.TS, &x.Power, &x.Mode, &x.Target, &in, &outd, &watts, &kwh, &x.Fan); err != nil {
			return nil, err
		}

		x.Device = device
		x.Indoor, x.Outdoor, x.Watts, x.KWh = nf(in), nf(outd), nf(watts), nf(kwh)
		out = append(out, x)
	}

	return out, rows.Err()
}

func nf(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}

	f := v.Float64

	return &f
}

// Prune deletes samples and log lines older than days.
func (s *Store) Prune(days int) error {
	cut := time.Now().AddDate(0, 0, -days).Unix()

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := s.db.Exec(`DELETE FROM samples WHERE ts < ?`, cut); err != nil {
		return err
	}

	_, err := s.db.Exec(`DELETE FROM log WHERE ts < ?`, time.Now().AddDate(0, 0, -60).Unix())

	return err
}

// DeleteSamples removes the history of a device.
func (s *Store) DeleteSamples(device string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`DELETE FROM samples WHERE device = ?`, device)

	return err
}

// AddLog writes one log line and returns it.
func (s *Store) AddLog(device, level, text string) LogEntry {
	e := LogEntry{Time: time.Now(), Device: device, Level: level, Text: text}

	s.mu.Lock()
	defer s.mu.Unlock()

	res, err := s.db.Exec(`INSERT INTO log (ts, device, level, text) VALUES (?, ?, ?, ?)`, e.Time.Unix(), device, level, text)
	if err == nil {
		e.ID, _ = res.LastInsertId()
	}

	return e
}

// Log returns the newest lines (limit), optionally for one device.
func (s *Store) Log(device string, limit int) ([]LogEntry, error) {
	q := `SELECT id, ts, device, level, text FROM log`
	args := []any{}

	if device != "" {
		q += ` WHERE device = ?`
		args = append(args, device)
	}

	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []LogEntry

	for rows.Next() {
		var (
			e  LogEntry
			ts int64
		)

		if err := rows.Scan(&e.ID, &ts, &e.Device, &e.Level, &e.Text); err != nil {
			return nil, err
		}

		e.Time = time.Unix(ts, 0)
		out = append(out, e)
	}

	return out, rows.Err()
}

// Sessions.

// CreateSession stores a login session and returns its token.
func (s *Store) CreateSession(d time.Duration) (string, error) {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	tok := hex.EncodeToString(b)

	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`INSERT INTO sessions (token, expires) VALUES (?, ?)`, tok, time.Now().Add(d).Unix())

	return tok, err
}

// ValidSession reports whether tok is a live session.
func (s *Store) ValidSession(tok string) bool {
	if tok == "" {
		return false
	}

	var exp int64

	err := s.db.QueryRow(`SELECT expires FROM sessions WHERE token = ?`, tok).Scan(&exp)

	return err == nil && exp > time.Now().Unix()
}

// DeleteSession logs out.
func (s *Store) DeleteSession(tok string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, _ = s.db.Exec(`DELETE FROM sessions WHERE token = ? OR expires < ?`, tok, time.Now().Unix())
}

// DeleteAllSessions logs everybody out (password change).
func (s *Store) DeleteAllSessions() {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, _ = s.db.Exec(`DELETE FROM sessions`)
}

// Devices returns the configured devices sorted by order and name.
func (s *Store) Devices() []Device {
	list, _ := List[Device](s, CollDevices)
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].Order != list[j].Order {
			return list[i].Order < list[j].Order
		}

		return list[i].Name < list[j].Name
	})

	return list
}

// Backup exports everything except history.
type Backup struct {
	Version   int        `json:"version"`
	Exported  time.Time  `json:"exported"`
	Devices   []Device   `json:"devices_full"`
	Scenes    []Scene    `json:"scenes_full"`
	Schedules []Schedule `json:"schedules"`
	Rules     []Rule     `json:"rules"`
	Settings  Settings   `json:"settings"`
	// Klimacontrol compatible part (can be read by Klimacontrol).
	KCDevices []KCDevice `json:"devices"`
	KCScenes  []KCScene  `json:"scenes"`
}

// KCDevice is the Klimacontrol/msmart backup format of a device.
type KCDevice struct {
	DeviceID       uint64 `json:"deviceId"`
	DiscoveredName string `json:"discoveredName"`
	IP             string `json:"ip"`
	Key            string `json:"key"`
	Port           int    `json:"port"`
	Token          string `json:"token"`
	Type           int    `json:"type"`
	Version        int    `json:"version"`
}

// KCScene is the Klimacontrol scene format.
type KCScene struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	PowerOn       bool    `json:"powerOn"`
	Mode          int     `json:"mode"`
	TempC         float64 `json:"tempC"`
	Fan           int     `json:"fan"`
	Swing         bool    `json:"swing"`
	Eco           bool    `json:"eco"`
	Anion         bool    `json:"anion"`
	OutdoorSilent bool    `json:"outdoorSilent"`
	SelfClean     bool    `json:"selfClean"`
}
