package main

import (
	"math"
	"testing"
	"time"
)

func at(d, h int) time.Time     { return time.Date(2026, 11, d, h, 0, 0, 0, time.UTC) }
func near(a, b, e float64) bool { return math.Abs(a-b) < e }

// Week resets Thu 12 Nov 10:00 => started Thu 5 Nov 10:00.
func TestPaceRing(t *testing.T) {
	reset := at(12, 10)
	// Fri 6 Nov 22:00 = 1.5 days in -> day 2, budget through today 28.57%.
	d := computeDaily(at(6, 22), Usage{WeeklyPct: 28, WeeklyReset: reset})
	if d.Day != 2 || !near(d.Budget, 200.0/7, 0.01) || !near(d.Frac, 28/(200.0/7), 0.01) {
		t.Fatalf("%+v", d)
	}
	if !near(d.Pace, 28-1.5*100/7, 0.01) {
		t.Fatalf("pace %+v", d)
	}
}

func TestUsingLessLeavesMoreToday(t *testing.T) {
	reset := at(12, 10)
	lo := computeDaily(at(8, 12), Usage{WeeklyPct: 10, WeeklyReset: reset})
	hi := computeDaily(at(8, 12), Usage{WeeklyPct: 30, WeeklyReset: reset})
	if lo.Left <= hi.Left || lo.Frac >= hi.Frac {
		t.Fatalf("%+v %+v", lo, hi)
	}
}

func TestOverlapAndLastDay(t *testing.T) {
	reset := at(12, 10)
	d := computeDaily(at(6, 22), Usage{WeeklyPct: 50, WeeklyReset: reset})
	if d.Frac <= 1 {
		t.Fatalf("want overlap, %+v", d)
	}
	d = computeDaily(at(12, 9), Usage{WeeklyPct: 90, WeeklyReset: reset})
	if d.Day != 7 || !near(d.Budget, 100, 0.01) {
		t.Fatalf("%+v", d)
	}
}

// Session burns 10 pts/h and each session pt costs 0.5 weekly pts.
func samplesFor(reset, sreset time.Time) []Sample {
	var s []Sample
	for i := 0; i <= 6; i++ { // every 10 min for an hour: +1.67 session pts each
		s = append(s, Sample{
			T: at(6, 20).Add(time.Duration(i) * 10 * time.Minute).Unix(), Session: 10 + float64(i)*10.0/6,
			SessionReset: sreset.Unix(), Weekly: 20 + float64(i)*10.0/6*0.5, Reset: reset.Unix(),
		})
	}
	return s
}

func TestForecast(t *testing.T) {
	reset, sreset := at(12, 10), at(6, 24)
	now := at(6, 21)
	u := Usage{SessionPct: 20, SessionReset: sreset, WeeklyPct: 25, WeeklyReset: reset}
	d := computeDaily(now, u)
	f := computeForecast(now, u, samplesFor(reset, sreset), d)
	if !f.Ready || !near(f.Rate, 10, 0.2) || !near(f.K, 0.5, 0.01) {
		t.Fatalf("%+v", f)
	}
	// 3h left * 10/h = +30 session = +15 weekly -> 40 weekly; budget today 28.6 -> overshoot likely.
	if !near(f.ProjWeekly, 40, 1) || f.POver < 0.9 {
		t.Fatalf("%+v", f)
	}
	// Same pace but nearly out of time -> far less likely.
	now2 := at(6, 23).Add(50 * time.Minute)
	f2 := computeForecast(now2, u, samplesFor(reset, sreset), computeDaily(now2, u))
	if f2.POver >= f.POver {
		t.Fatalf("%+v vs %+v", f2, f)
	}
}

func TestForecastNeedsData(t *testing.T) {
	reset := at(12, 10)
	u := Usage{SessionPct: 5, SessionReset: at(6, 24), WeeklyPct: 25, WeeklyReset: reset}
	if f := computeForecast(at(6, 21), u, nil, computeDaily(at(6, 21), u)); f.Ready {
		t.Fatal("should not be ready without samples")
	}
}

func TestBackoff(t *testing.T) {
	if backoff(1, 0) != 5*time.Minute || backoff(2, 0) != 10*time.Minute || backoff(9, 0) != backoffMax {
		t.Fatal("bad backoff")
	}
	if backoff(1, 2*time.Hour) != 2*time.Hour {
		t.Fatal("Retry-After ignored")
	}
}
