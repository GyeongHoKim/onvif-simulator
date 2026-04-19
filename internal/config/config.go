// Package config loads and validates onvif-simulator.json.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
)

// FileName is the default on-disk config file name in the working directory (see README).
const FileName = "onvif-simulator.json"

// CurrentVersion is the only supported config schema version.
const CurrentVersion = 1

// Config is the on-disk JSON shape (see onvif-simulator.example.json).
type Config struct {
	Version  int    `json:"version"`
	MainRTSP string `json:"main_rtsp_uri"`
	SubRTSP  string `json:"sub_rtsp_uri"`
}

var (
	// ErrInvalidVersion means config.version does not match a supported schema.
	ErrInvalidVersion = errors.New("config: unsupported version")
	// ErrEmptyRTSPURI means a required rtsp URI field was empty.
	ErrEmptyRTSPURI = errors.New("config: rtsp uri must not be empty")
	// ErrInvalidRTSPURL means the value is not a valid URL.
	ErrInvalidRTSPURL = errors.New("config: invalid rtsp url")
	// ErrInvalidRTSPScheme means the URL scheme is not rtsp.
	ErrInvalidRTSPScheme = errors.New("config: rtsp uri scheme must be rtsp")
	// ErrRTSPHostRequired means the URL has no host component.
	ErrRTSPHostRequired = errors.New("config: rtsp uri must include host")
)

// Validate checks required fields and RTSP URL shape.
func Validate(c Config) error {
	if c.Version != CurrentVersion {
		return fmt.Errorf("%w (got %d, want %d)", ErrInvalidVersion, c.Version, CurrentVersion)
	}
	if err := validateRTSPURI("main_rtsp_uri", c.MainRTSP); err != nil {
		return err
	}
	return validateRTSPURI("sub_rtsp_uri", c.SubRTSP)
}

func validateRTSPURI(field, raw string) error {
	if raw == "" {
		return fmt.Errorf("%w (%s)", ErrEmptyRTSPURI, field)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%w (%s): %w", ErrInvalidRTSPURL, field, err)
	}
	if u.Scheme != "rtsp" {
		return fmt.Errorf("%w (%s): got scheme %q", ErrInvalidRTSPScheme, field, u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("%w (%s)", ErrRTSPHostRequired, field)
	}
	return nil
}

// Load reads and validates ./onvif-simulator.json relative to the process working directory.
func Load() (Config, error) {
	wd, err := os.Getwd()
	if err != nil {
		return Config{}, fmt.Errorf("config: get working directory: %w", err)
	}
	data, err := fs.ReadFile(os.DirFS(wd), FileName)
	if err != nil {
		return Config{}, fmt.Errorf("config: read %s: %w", FileName, err)
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, fmt.Errorf("config: parse %s: %w", FileName, err)
	}
	if err := Validate(c); err != nil {
		return Config{}, err
	}
	return c, nil
}

// Save writes cfg to ./onvif-simulator.json relative to the working directory.
// It validates before writing and replaces the destination atomically when possible
// (same directory rename).
func Save(cfg Config) error {
	if err := Validate(cfg); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("config: marshal: %w", err)
	}
	data = append(data, '\n')

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("config: get working directory: %w", err)
	}
	path := filepath.Join(wd, FileName)

	tmp, err := os.CreateTemp(wd, "."+FileName+".tmp-*")
	if err != nil {
		return fmt.Errorf("config: create temp in %q: %w", wd, err)
	}
	tmpPath := tmp.Name()

	if _, werr := tmp.Write(data); werr != nil {
		return joinSaveErrors(
			fmt.Errorf("config: write temp: %w", werr),
			tmp.Close(),
			os.Remove(tmpPath),
		)
	}
	if cerr := tmp.Close(); cerr != nil {
		return joinSaveErrors(
			fmt.Errorf("config: close temp: %w", cerr),
			os.Remove(tmpPath),
		)
	}
	if rerr := os.Rename(tmpPath, path); rerr != nil {
		return joinSaveErrors(
			fmt.Errorf("config: rename to %s: %w", FileName, rerr),
			os.Remove(tmpPath),
		)
	}
	return nil
}

func joinSaveErrors(primary error, rest ...error) error {
	return errors.Join(append([]error{primary}, rest...)...)
}
