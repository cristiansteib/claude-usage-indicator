package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// State is persisted on disk so that restarts and logins never trigger an
// unnecessary request, and so a rate-limit block survives a restart.
type State struct {
	Usage        Usage    `json:"usage"`
	HasUsage     bool     `json:"has_usage"`
	FetchedAt    int64    `json:"fetched_at"`    // last successful fetch
	LastAttempt  int64    `json:"last_attempt"`  // last request (ok or not)
	BlockedUntil int64    `json:"blocked_until"` // no requests before this
	Failures     int      `json:"failures"`      // consecutive failures (drives backoff)
	LastError    string   `json:"last_error"`
	Samples      []Sample `json:"samples"`
}

func cachePath() string {
	d, err := os.UserCacheDir()
	if err != nil {
		d = os.TempDir()
	}
	return filepath.Join(d, "claude-usage-indicator", "state.json")
}

func loadState() State {
	var s State
	if b, err := os.ReadFile(cachePath()); err == nil {
		_ = json.Unmarshal(b, &s)
	}
	return s
}

func (s *State) save() {
	p := cachePath()
	_ = os.MkdirAll(filepath.Dir(p), 0o700)
	b, _ := json.MarshalIndent(s, "", " ")
	writeAtomic(p, b)
}

func writeAtomic(path string, b []byte) {
	tmp := path + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		_ = os.Rename(tmp, path)
	}
}

// Published is what the GNOME Shell extension reads (in $XDG_RUNTIME_DIR).
type Published struct {
	Usage
	HasUsage     bool     `json:"has_usage"`
	FetchedAt    int64    `json:"fetched_at"`
	Updated      int64    `json:"updated"`
	Status       string   `json:"status"` // ok | stale | rate_limited | error
	Message      string   `json:"message"`
	BlockedUntil int64    `json:"blocked_until"`
	NextPoll     int64    `json:"next_poll"`
	Daily        Daily    `json:"daily"`
	Forecast     Forecast `json:"forecast"`
}

func publish(p Published) {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		return
	}
	p.Updated = time.Now().Unix()
	b, _ := json.Marshal(p)
	writeAtomic(filepath.Join(dir, "claude-usage.json"), b)
}
