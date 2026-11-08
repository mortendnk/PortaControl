// Package config loads the YAML configuration of portacontrold.
package config

import (
	"errors"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the file configuration. Everything a user changes at runtime
// (devices, scenes, prices, MQTT, ...) lives in the database instead.
type Config struct {
	Listen    string `yaml:"listen"`
	PublicURL string `yaml:"public_url"`
	DataDir   string `yaml:"data_dir"`
	TimeZone  string `yaml:"time_zone"`
	Log       struct {
		Level string `yaml:"level"`
		JSON  bool   `yaml:"json"`
	} `yaml:"log"`
	// Simulate starts a built-in simulated PortaSplit (demo without hardware).
	Simulate bool `yaml:"simulate"`
}

// Default returns the defaults.
func Default() Config {
	c := Config{Listen: ":8080", DataDir: "/var/lib/portacontrol", TimeZone: "Europe/Berlin"}
	c.Log.Level = "info"

	return c
}

// Load reads path; a missing file yields the defaults. Environment variables
// PORTACONTROL_LISTEN, PORTACONTROL_DATA_DIR, PORTACONTROL_PUBLIC_URL,
// PORTACONTROL_TZ and PORTACONTROL_SIMULATE override the file.
func Load(path string) (Config, error) {
	c := Default()

	b, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := yaml.Unmarshal(b, &c); err != nil {
			return c, fmt.Errorf("config %s: %w", path, err)
		}
	case errors.Is(err, os.ErrNotExist):
	default:
		return c, err
	}

	if v := os.Getenv("PORTACONTROL_LISTEN"); v != "" {
		c.Listen = v
	}

	if v := os.Getenv("PORTACONTROL_DATA_DIR"); v != "" {
		c.DataDir = v
	}

	if v := os.Getenv("PORTACONTROL_PUBLIC_URL"); v != "" {
		c.PublicURL = v
	}

	if v := os.Getenv("PORTACONTROL_TZ"); v != "" {
		c.TimeZone = v
	}

	if os.Getenv("PORTACONTROL_SIMULATE") == "1" {
		c.Simulate = true
	}

	return c, nil
}

// Location returns the configured time zone (local time as fallback).
func (c Config) Location() *time.Location {
	if c.TimeZone != "" {
		if l, err := time.LoadLocation(c.TimeZone); err == nil {
			return l
		}
	}

	return time.Local
}
