package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type TransportType string

const (
	TransportSerial  TransportType = "serial"
	TransportNetwork TransportType = "network"
	TransportUSB     TransportType = "usb"
	TransportNone    TransportType = ""
)

type Config struct {
	Transport   TransportType `json:"transport"`
	SerialPort  string        `json:"serialPort"`
	SerialBaud  int           `json:"serialBaud"`
	NetworkIP   string        `json:"networkIP"`
	NetworkPort int           `json:"networkPort"`
	USBPort     string        `json:"usbPort"`
	PaperWidth  int           `json:"paperWidth"`
	HTTPPort    int           `json:"httpPort"`
}

func DefaultConfig() *Config {
	return &Config{
		Transport:   TransportNone,
		SerialPort:  "",
		SerialBaud:  9600,
		NetworkIP:   "",
		NetworkPort: 9100,
		PaperWidth:  80,
		HTTPPort:    7777,
	}
}

// configDir returns the preferred config directory.
// Prefers PROGRAMDATA (shared between all Windows users — ideal when the
// bridge runs as a service under LocalSystem or when several cashiers share
// the same PC). Falls back to APPDATA if PROGRAMDATA is not writable
// (e.g. when the user runs the bridge without admin privileges on first launch).
func configDir() (string, bool, error) {
	// Try PROGRAMDATA first
	if programData := os.Getenv("PROGRAMDATA"); programData != "" {
		dir := filepath.Join(programData, "Mivy", "PrintBridge")
		if err := os.MkdirAll(dir, 0755); err == nil {
			// Probe writability
			probe := filepath.Join(dir, ".writetest")
			if err := os.WriteFile(probe, []byte{}, 0644); err == nil {
				os.Remove(probe)
				return dir, true, nil
			}
		}
	}
	// Fallback: per-user APPDATA
	appData := os.Getenv("APPDATA")
	if appData == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", false, err
		}
		appData = home
	}
	dir := filepath.Join(appData, "Mivy", "PrintBridge")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", false, err
	}
	return dir, false, nil
}

func configPath() (string, error) {
	dir, _, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// legacyPaths returns old config locations that should be migrated from.
// These cover:
//   1. Old brand (zenPOS) in both PROGRAMDATA and APPDATA
//   2. Pre-PROGRAMDATA per-user APPDATA under Mivy (very early installs)
// Migration is one-way: on first Load() we pick the first existing file
// and persist it into the current canonical path.
func legacyPaths() []string {
	var paths []string
	if pd := os.Getenv("PROGRAMDATA"); pd != "" {
		paths = append(paths, filepath.Join(pd, "zenPOS", "PrintBridge", "config.json"))
	}
	if ad := os.Getenv("APPDATA"); ad != "" {
		paths = append(paths,
			filepath.Join(ad, "Mivy", "PrintBridge", "config.json"),
			filepath.Join(ad, "zenPOS", "PrintBridge", "config.json"),
		)
	}
	return paths
}

func Load() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return DefaultConfig(), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Migration: try each legacy path in order and adopt the first hit.
			for _, legacy := range legacyPaths() {
				if legacy == path {
					continue
				}
				if legacyData, rerr := os.ReadFile(legacy); rerr == nil {
					data = legacyData
					// Best-effort: persist into the new location.
					_ = os.WriteFile(path, legacyData, 0644)
					goto parse
				}
			}
			return DefaultConfig(), nil
		}
		return DefaultConfig(), err
	}
parse:
	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return DefaultConfig(), err
	}
	if cfg.HTTPPort == 0 {
		cfg.HTTPPort = 7777
	}
	if cfg.SerialBaud == 0 {
		cfg.SerialBaud = 9600
	}
	if cfg.NetworkPort == 0 {
		cfg.NetworkPort = 9100
	}
	if cfg.PaperWidth == 0 {
		cfg.PaperWidth = 80
	}
	return cfg, nil
}

func (c *Config) Save() error {
	path, err := configPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
