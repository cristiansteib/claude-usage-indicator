package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	pollEvery      = 10 * time.Minute // automatic refresh interval
	manualCooldown = 2 * time.Minute  // minimum gap between any two requests
	backoffBase    = 5 * time.Minute  // first backoff after a failure
	backoffMax     = 60 * time.Minute
	networkRetry   = 45 * time.Second // retry after errors that never reached the API
)

// Service owns the cached state and decides when a request is allowed.
type Service struct {
	mu sync.Mutex
	st State
}

func newService() *Service { return &Service{st: loadState()} }

func backoff(failures int, retryAfter time.Duration) time.Duration {
	d := backoffBase
	for i := 1; i < failures && d < backoffMax; i++ {
		d *= 2
	}
	if d > backoffMax {
		d = backoffMax
	}
	if retryAfter > d { // the server asked for even longer
		d = retryAfter
	}
	return d
}

// Refresh fetches new data if the rate-limit policy allows it, and always
// republishes the (possibly cached) state. force=true is a manual refresh.
// It returns a short human message about what happened.
func (s *Service) Refresh(force bool) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	msg := ""

	switch {
	case now.Unix() < s.st.BlockedUntil:
		msg = fmt.Sprintf("Rate-limit protection: next request allowed at %s",
			time.Unix(s.st.BlockedUntil, 0).Format("15:04"))
	case now.Sub(time.Unix(s.st.LastAttempt, 0)) < manualCooldown:
		msg = "Too soon: showing cached data"
	case !force && s.st.HasUsage && now.Sub(time.Unix(s.st.FetchedAt, 0)) < pollEvery:
		msg = "Cache is fresh"
	default:
		s.st.LastAttempt = now.Unix()
		u, err := fetchUsage()
		if err != nil {
			s.st.LastError = err.Error()
			if ae, ok := err.(*apiError); ok {
				// The server answered with an error: back off exponentially.
				s.st.Failures++
				s.st.BlockedUntil = now.Add(backoff(s.st.Failures, ae.RetryAfter)).Unix()
			} else {
				// No answer (network not up yet at login, DNS, missing
				// credentials...): nothing reached the API, so just retry soon.
				s.st.BlockedUntil = now.Add(networkRetry).Unix()
			}
			msg = err.Error()
		} else {
			s.st.Usage, s.st.HasUsage = u, true
			s.st.FetchedAt = now.Unix()
			s.st.Failures, s.st.LastError, s.st.BlockedUntil = 0, "", 0
			s.st.Samples = addSample(s.st.Samples, now, u)
			msg = "Updated"
		}
		s.st.save()
	}
	s.publishLocked(now, msg)
	return msg
}

func (s *Service) publishLocked(now time.Time, msg string) {
	p := Published{
		Usage: s.st.Usage, HasUsage: s.st.HasUsage, FetchedAt: s.st.FetchedAt,
		BlockedUntil: s.st.BlockedUntil, Message: msg, Status: "ok",
		NextPoll: s.nextPollLocked(now).Unix(),
	}
	if s.st.HasUsage {
		p.Daily = computeDaily(now, s.st.Usage)
		p.Forecast = computeForecast(now, s.st.Usage, s.st.Samples, p.Daily)
	}
	switch {
	case s.st.LastError != "" && now.Unix() < s.st.BlockedUntil:
		p.Status, p.Message = "rate_limited", s.st.LastError
		if s.st.HasUsage {
			p.Status = "stale"
		}
	case s.st.LastError != "":
		p.Status, p.Message = "error", s.st.LastError
	}
	publish(p)
}

func (s *Service) nextPollLocked(now time.Time) time.Time {
	next := time.Unix(s.st.FetchedAt, 0).Add(pollEvery)
	if b := time.Unix(s.st.BlockedUntil, 0); b.After(next) {
		next = b
	}
	if next.Before(now) {
		next = now
	}
	return next
}

// D-Bus API so the extension's Refresh button can talk to the daemon.
type dbusAPI struct{ s *Service }

func (a dbusAPI) Refresh() (string, *dbus.Error) { return a.s.Refresh(true), nil }

func serveDBus(s *Service) error {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return err
	}
	if err := conn.Export(dbusAPI{s}, "/local/ClaudeUsage", "local.ClaudeUsage"); err != nil {
		return err
	}
	r, err := conn.RequestName("local.ClaudeUsage", dbus.NameFlagDoNotQueue)
	if err != nil {
		return err
	}
	if r != dbus.RequestNameReplyPrimaryOwner {
		return fmt.Errorf("another claude-usage-indicator is already running")
	}
	return nil
}
