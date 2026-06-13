package control

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/sphragis-oss/sphragis/internal/config"
)

func PidPath() string   { return filepath.Join(config.Home(), "sphragis.pid") }
func LogPath() string   { return filepath.Join(config.Home(), "sphragis.log") }
func statePath() string { return filepath.Join(config.Home(), "state.json") }

// EnsureHome creates the sphragis home directory.
func EnsureHome() error { return os.MkdirAll(config.Home(), 0o755) }

// WritePID records the daemon pid.
func WritePID(pid int) error {
	if err := EnsureHome(); err != nil {
		return err
	}
	return os.WriteFile(PidPath(), []byte(strconv.Itoa(pid)), 0o644)
}

// ReadPID returns the recorded pid, if any.
func ReadPID() (int, bool) {
	b, err := os.ReadFile(PidPath())
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return 0, false
	}
	return pid, true
}

// RemovePID deletes the pid file.
func RemovePID() { os.Remove(PidPath()) }

// Running reports whether the recorded pid is a live process.
func Running() (int, bool) {
	pid, ok := ReadPID()
	if !ok {
		return 0, false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return 0, false
	}
	if err := p.Signal(syscall.Signal(0)); err != nil {
		return 0, false
	}
	return pid, true
}

// State is the persisted daemon control state.
type State struct {
	AutoAnchor bool   `json:"auto_anchor"`
	Interval   string `json:"interval"`
}

// LoadState reads state.json, returning defaults if absent.
func LoadState() State {
	s := State{AutoAnchor: false, Interval: "24h"}
	b, err := os.ReadFile(statePath())
	if err != nil {
		return s
	}
	_ = json.Unmarshal(b, &s)
	if s.Interval == "" {
		s.Interval = "24h"
	}
	return s
}

// SaveState writes state.json.
func SaveState(s State) error {
	if err := EnsureHome(); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(s, "", "  ")
	return os.WriteFile(statePath(), b, 0o644)
}
