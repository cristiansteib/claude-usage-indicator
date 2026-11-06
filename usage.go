package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Usage holds the utilization (0-100) of the two Claude limits we display.
type Usage struct {
	SessionPct   float64   `json:"session_pct"`
	SessionReset time.Time `json:"session_reset"`
	WeeklyPct    float64   `json:"weekly_pct"`
	WeeklyReset  time.Time `json:"weekly_reset"`
}

type limit struct {
	Utilization float64   `json:"utilization"`
	ResetsAt    time.Time `json:"resets_at"`
}

type usageResp struct {
	FiveHour limit `json:"five_hour"`
	SevenDay limit `json:"seven_day"`
}

// apiError carries the HTTP status so the scheduler can back off properly.
type apiError struct {
	Status     int
	RetryAfter time.Duration
}

func (e *apiError) Error() string {
	if e.Status == http.StatusTooManyRequests {
		return "rate limited by the usage API (429)"
	}
	return fmt.Sprintf("usage API returned HTTP %d", e.Status)
}

// token re-reads the credentials file on every call: Claude Code refreshes it
// in place, so we never have to deal with the refresh flow ourselves.
func token() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(filepath.Join(home, ".claude", ".credentials.json"))
	if err != nil {
		return "", err
	}
	var c struct {
		OAuth struct {
			AccessToken string `json:"accessToken"`
		} `json:"claudeAiOauth"`
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return "", err
	}
	if c.OAuth.AccessToken == "" {
		return "", fmt.Errorf("no access token in credentials (run `claude` and log in)")
	}
	return c.OAuth.AccessToken, nil
}

// fetchUsage performs exactly one HTTP request to the usage API.
func fetchUsage() (Usage, error) {
	tok, err := token()
	if err != nil {
		return Usage{}, err
	}
	req, _ := http.NewRequest("GET", "https://api.anthropic.com/api/oauth/usage", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")
	req.Header.Set("User-Agent", "claude-usage-indicator")
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return Usage{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		ra, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
		return Usage{}, &apiError{Status: resp.StatusCode, RetryAfter: time.Duration(ra) * time.Second}
	}
	var r usageResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return Usage{}, err
	}
	return Usage{
		SessionPct: r.FiveHour.Utilization, SessionReset: r.FiveHour.ResetsAt,
		WeeklyPct: r.SevenDay.Utilization, WeeklyReset: r.SevenDay.ResetsAt,
	}, nil
}
