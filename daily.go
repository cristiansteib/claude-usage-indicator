package main

import (
	"math"
	"time"
)

// Sample is one observation, kept so the forecast can learn how fast the
// 5h session burns and how much of the weekly budget a session point costs.
type Sample struct {
	T            int64   `json:"t"`
	Session      float64 `json:"session"`
	SessionReset int64   `json:"session_reset"`
	Weekly       float64 `json:"weekly"`
	Reset        int64   `json:"reset"` // weekly reset: identifies the week
}

const tolerance = 15 * 60 // seconds; the API's reset timestamps jitter a little

func sameReset(a, b int64) bool { return math.Abs(float64(a-b)) < tolerance }

// Daily is the pace ring. The week is 7 days, so a day is 100/7 = 14.3% of
// the weekly budget. Budget accumulates day by day, so using less on earlier
// days automatically leaves more room today (and using more leaves less).
type Daily struct {
	Day      int     `json:"day"`       // 1..7, counted from the weekly reset
	PerDay   float64 `json:"per_day"`   // 14.29
	Budget   float64 `json:"budget"`    // cumulative budget through the end of today
	Weekly   float64 `json:"weekly"`    // used so far this week
	Frac     float64 `json:"frac"`      // weekly/budget; >1 = into tomorrow's budget (overlap)
	Pace     float64 `json:"pace"`      // weekly minus what an even burn would be by now (+ = ahead)
	Left     float64 `json:"left"`      // budget still available today (can be negative)
	DaysLeft float64 `json:"days_left"` // days until the weekly reset

}

func computeDaily(now time.Time, u Usage) Daily {
	const days = 7.0
	perDay := 100 / days
	elapsed := now.Sub(u.WeeklyReset.Add(-7*24*time.Hour)).Hours() / 24
	elapsed = math.Min(math.Max(elapsed, 0), days)
	idx := math.Min(math.Floor(elapsed), days-1)
	d := Daily{
		Day: int(idx) + 1, PerDay: perDay, Budget: (idx + 1) * perDay,
		Weekly: u.WeeklyPct, Pace: u.WeeklyPct - elapsed*perDay, DaysLeft: days - elapsed,
	}
	d.Frac = d.Weekly / d.Budget
	d.Left = d.Budget - d.Weekly
	return d
}

// Forecast is a probabilistic look-ahead for the rest of the current session.
type Forecast struct {
	Ready      bool    `json:"ready"`
	Note       string  `json:"note"`
	HoursLeft  float64 `json:"hours_left"`  // until the 5h session resets
	Rate       float64 `json:"rate"`        // session-% per hour (recent)
	K          float64 `json:"k"`           // weekly-% per 1 session-%
	AddSession float64 `json:"add_session"` // expected extra session-%
	AddWeekly  float64 `json:"add_weekly"`  // expected extra weekly-%
	ProjWeekly float64 `json:"proj_weekly"` // weekly-% expected when the session ends
	ProjFrac   float64 `json:"proj_frac"`   // same, as a fraction of today's budget
	POver      float64 `json:"p_over"`      // P(weekly ends above today's budget)
}

func normCDF(z float64) float64 { return 0.5 * math.Erfc(-z/math.Sqrt2) }

// computeForecast models the extra session-% X still to be burnt before the
// session resets as Normal(mu*R, s2*R) (mu = mean burn per hour, s2 = its
// variance per hour, both measured from this session's samples), converts it
// to weekly-% with the learned ratio k, and asks how likely it is to push
// the week past today's budget.
func computeForecast(now time.Time, u Usage, samples []Sample, d Daily) Forecast {
	f := Forecast{HoursLeft: math.Min(math.Max(u.SessionReset.Sub(now).Hours(), 0), 5)}
	srU, wrU := u.SessionReset.Unix(), u.WeeklyReset.Unix()

	// Pairs of consecutive samples inside the current session of the current week.
	var cur []Sample
	for _, s := range samples {
		if sameReset(s.SessionReset, srU) && sameReset(s.Reset, wrU) {
			cur = append(cur, s)
		}
	}
	var sumDS, sumDT float64
	type inc struct{ ds, dt float64 }
	var incs []inc
	for i := 1; i < len(cur); i++ {
		dt := float64(cur[i].T-cur[i-1].T) / 3600
		ds := cur[i].Session - cur[i-1].Session
		if dt <= 0.01 || ds < 0 {
			continue
		}
		incs = append(incs, inc{ds, dt})
		sumDS += ds
		sumDT += dt
	}
	mu := 0.0
	switch {
	case sumDT >= 0.25:
		mu = sumDS / sumDT
	case u.SessionPct > 0 && f.HoursLeft < 4.9: // no samples yet: whole-session average
		mu = u.SessionPct / (5 - f.HoursLeft)
	}
	s2 := 0.0
	if sumDT > 0 {
		for _, x := range incs {
			e := x.ds - mu*x.dt
			s2 += e * e
		}
		s2 /= sumDT
	}
	s2 = math.Max(s2, 0.25*mu) // never claim certainty from a handful of samples
	f.Rate = mu

	// k: weekly-% per session-%, learned from every pair this week.
	var kw, ks float64
	for i := 1; i < len(samples); i++ {
		a, b := samples[i-1], samples[i]
		if sameReset(a.Reset, wrU) && sameReset(b.Reset, wrU) && sameReset(a.SessionReset, b.SessionReset) &&
			b.Session > a.Session {
			ks += b.Session - a.Session
			kw += b.Weekly - a.Weekly
		}
	}
	if ks < 2 {
		f.Note = "learning how session use maps to the week (needs a bit more usage)"
		return f
	}
	f.K = kw / ks
	f.Ready = true

	room := math.Max(100-u.SessionPct, 0)
	f.AddSession = math.Min(mu*f.HoursLeft, room)
	f.AddWeekly = f.AddSession * f.K
	f.ProjWeekly = d.Weekly + f.AddWeekly
	f.ProjFrac = f.ProjWeekly / d.Budget

	need := d.Budget - d.Weekly // weekly-% of headroom left today
	switch {
	case need <= 0:
		f.POver = 1
	case f.K <= 0 || f.HoursLeft == 0 || mu == 0:
		f.POver = 0
	default:
		thr := need / f.K // session-% that would use up the headroom
		if thr > room {
			f.POver = 0
		} else {
			f.POver = 1 - normCDF((thr-mu*f.HoursLeft)/math.Sqrt(s2*f.HoursLeft))
		}
	}
	return f
}

// addSample records one successful observation and prunes old data.
func addSample(samples []Sample, now time.Time, u Usage) []Sample {
	s := Sample{T: now.Unix(), Session: u.SessionPct, SessionReset: u.SessionReset.Unix(),
		Weekly: u.WeeklyPct, Reset: u.WeeklyReset.Unix()}
	if n := len(samples); n > 0 && s.T-samples[n-1].T < 120 {
		return samples
	}
	samples = append(samples, s)
	cut := now.Add(-9 * 24 * time.Hour).Unix()
	out := samples[:0]
	for _, x := range samples {
		if x.T >= cut {
			out = append(out, x)
		}
	}
	return out
}
