// previews renders the README images. It mirrors the geometry and colors of
// extension/claude-usage@local/main.js using filled SVG shapes, and converts
// them with ImageMagick (`convert`). These are renders, not screenshots.
package main

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const scale = 3.0 // output pixels per CSS pixel

type svg struct{ b strings.Builder }

func (s *svg) f(format string, a ...any) { fmt.Fprintf(&s.b, format+"\n", a...) }

func pt(cx, cy, r, a float64) (float64, float64) { return cx + r*math.Cos(a), cy + r*math.Sin(a) }

// arc draws an annular sector (lap = fraction of a full turn) with round caps.
func (s *svg) arc(cx, cy, r, w, lap float64, color string, op float64) {
	if lap <= 0 {
		return
	}
	if lap >= 0.999 {
		s.f(`<circle cx="%.2f" cy="%.2f" r="%.2f" fill="none" stroke="%s" stroke-opacity="%.2f" stroke-width="%.2f"/>`, cx, cy, r, color, op, w)
		return
	}
	a0 := -math.Pi / 2
	a1 := a0 + lap*2*math.Pi
	n := int(math.Max(8, lap*90))
	var pts []string
	for i := 0; i <= n; i++ {
		x, y := pt(cx, cy, r+w/2, a0+(a1-a0)*float64(i)/float64(n))
		pts = append(pts, fmt.Sprintf("%.2f,%.2f", x, y))
	}
	for i := n; i >= 0; i-- {
		x, y := pt(cx, cy, r-w/2, a0+(a1-a0)*float64(i)/float64(n))
		pts = append(pts, fmt.Sprintf("%.2f,%.2f", x, y))
	}
	s.f(`<polygon points="%s" fill="%s" fill-opacity="%.2f"/>`, strings.Join(pts, " "), color, op)
	for _, a := range []float64{a0, a1} {
		x, y := pt(cx, cy, r, a)
		s.f(`<circle cx="%.2f" cy="%.2f" r="%.2f" fill="%s" fill-opacity="%.2f"/>`, x, y, w/2, color, op)
	}
}

func lapColor(v float64) string {
	if v >= 0.85 {
		return "#f5a623"
	}
	return "#4cd964"
}

func (s *svg) laps(cx, cy, r, w, v, op float64) {
	if v > 1 {
		s.arc(cx, cy, r, w, 1, "#e0443e", op)
		s.arc(cx, cy, r, w, math.Min(v-1, 1), "#ffb3d1", op)
	} else {
		s.arc(cx, cy, r, w, v, lapColor(v), op)
	}
}

func barColor(p float64) string {
	switch {
	case p >= 85:
		return "#f24b4b"
	case p >= 60:
		return "#f5a623"
	}
	return "#4cd964"
}

// widget draws ring + 2 bars + logo at (x,y) in CSS px; returns its width.
func (s *svg) widget(x, y, frac, proj, session, weekly float64) float64 {
	const ring, logo, barW, barH = 30.0, 24.0, 200.0, 7.0
	cy := y + 16
	// ring
	cx := x + ring/2
	w := 4.5
	r := ring/2 - w/2 - 0.5
	s.arc(cx, cy, r, w, 1, "#ffffff", 0.22)
	if proj > frac+0.005 {
		s.laps(cx, cy, r, w, proj, 0.38)
	}
	s.laps(cx, cy, r, w, frac, 1)
	s.f(`<text x="%.2f" y="%.2f" font-family="DejaVu Sans" font-weight="bold" font-size="%.1f" fill="#fff" text-anchor="middle">%d</text>`,
		cx, cy+3.4, map[bool]float64{true: 8, false: 9.5}[frac >= 0.995], int(math.Round(frac*100)))
	// bars
	bx := x + ring + 9
	for i, p := range []float64{session, weekly} {
		by := cy - (2*barH+4)/2 + float64(i)*(barH+4)
		s.f(`<rect x="%.2f" y="%.2f" width="%.0f" height="%.0f" rx="3" fill="#fff" fill-opacity="0.25"/>`, bx, by, barW, barH)
		s.f(`<rect x="%.2f" y="%.2f" width="%.2f" height="%.0f" rx="3" fill="%s"/>`, bx, by, barW*p/100, barH, barColor(p))
	}
	// Claude starburst
	lx := bx + barW + 9
	lc := lx + logo/2
	ln := []float64{1, .78, .95, .72, 1, .8, .92, .74, 1, .78, .95, .72}
	for i := 0; i < 12; i++ {
		a := float64(i)*2*math.Pi/12 - math.Pi/2
		x1, y1 := pt(lc, cy, logo/2*0.2, a)
		x2, y2 := pt(lc, cy, (logo/2-1.5)*ln[i], a)
		dx, dy := -math.Sin(a)*1.2, math.Cos(a)*1.2
		s.f(`<polygon points="%.2f,%.2f %.2f,%.2f %.2f,%.2f %.2f,%.2f" fill="#d97757"/>`, x1+dx, y1+dy, x2+dx, y2+dy, x2-dx, y2-dy, x1-dx, y1-dy)
		s.f(`<circle cx="%.2f" cy="%.2f" r="1.2" fill="#d97757"/>`, x2, y2)
	}
	return lx + logo - x
}

func (s *svg) text(x, y float64, size float64, color, txt string, bold bool) {
	wt := "normal"
	if bold {
		wt = "bold"
	}
	s.f(`<text x="%.1f" y="%.1f" font-family="DejaVu Sans" font-weight="%s" font-size="%.1f" fill="%s">%s</text>`, x, y, wt, size, color, txt)
}

func render(path string, w, h float64, body func(*svg)) error {
	s := &svg{}
	s.f(`<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f">`, w*scale, h*scale, w, h)
	s.f(`<g transform="scale(1)">`)
	body(s)
	s.f(`</g></svg>`)
	tmp := path + ".svg"
	if err := os.WriteFile(tmp, []byte(s.b.String()), 0o644); err != nil {
		return err
	}
	defer os.Remove(tmp)
	out, err := exec.Command("convert", "-background", "none", tmp, path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("convert: %v\n%s", err, out)
	}
	return nil
}

func panelBG(s *svg, w, h float64) {
	s.f(`<rect width="%.0f" height="%.0f" fill="#1b1b1b"/>`, w, h)
}

func main() {
	dir := "docs"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	_ = os.MkdirAll(dir, 0o755)
	must := func(err error) {
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	// Hero: the top bar with the widget next to the date.
	must(render(filepath.Join(dir, "panel.png"), 640, 32, func(s *svg) {
		panelBG(s, 640, 32)
		s.text(16, 21, 13, "#e6e6e6", "Activities", false)
		s.text(206, 21, 13, "#ffffff", "nov 6  21:29", true)
		s.widget(332, 0, 0.98, 1.2, 53, 28)
		s.text(612, 21, 13, "#e6e6e6", "●", false)
	}))

	// The ring in its three situations, each with the full widget.
	type row struct {
		cap                  string
		frac, proj, sess, wk float64
	}
	rows := []row{
		{"On pace: under today's budget; the faint arc is the forecast", 0.55, 0.72, 20, 18},
		{"Close to today's budget", 0.92, 0.97, 61, 26},
		{"Overlap: past 100% a second (pink) lap starts, you are spending tomorrow's budget", 1.38, 1.5, 88, 41},
	}
	must(render(filepath.Join(dir, "states.png"), 640, 3*58, func(s *svg) {
		panelBG(s, 640, 3*58)
		for i, r := range rows {
			y := float64(i) * 58
			s.widget(16, y, r.frac, r.proj, r.sess, r.wk)
			s.text(16, y+50, 11, "#a8a8a8", r.cap, false)
		}
	}))

	// The popup menu.
	lines := []struct {
		t    string
		dim  bool
		sep  bool
		gap  bool
		bold bool
	}{
		{t: "Session (5h): 53%  — resets Sat 01:30"},
		{t: "Weekly (7d): 28%  — resets Thu 11:00"},
		{t: "Day 2 of 7 · 14.3% per day"},
		{t: "Budget through today: 28.6% · used 28.0% (98%)"},
		{t: "Pace: +7.0% vs an even burn · 5.5 days left"},
		{t: "Forecast at session end (3.3 h): weekly ≈ 31.2% (+3.2%)"},
		{t: "Chance of exceeding today's budget: 64%  (burn 6.1%/h, 1% session ≈ 0.50% week)"},
		{sep: true},
		{t: "Last update: Fri 22:17 (3 min ago)"},
		{t: "Next automatic refresh ≈ 22:27", dim: true},
		{t: "↻  Refresh now", bold: true},
	}
	const mw, lh = 620.0, 26.0
	mh := 16 + float64(len(lines))*lh
	must(render(filepath.Join(dir, "menu.png"), mw+32, mh+32, func(s *svg) {
		s.f(`<rect width="%.0f" height="%.0f" fill="#1b1b1b"/>`, mw+32, mh+32)
		s.f(`<rect x="16" y="16" width="%.0f" height="%.0f" rx="14" fill="#2b2b2b" stroke="#444" stroke-width="1"/>`, mw, mh)
		y := 16 + 8.0
		for _, l := range lines {
			if l.sep {
				s.f(`<rect x="32" y="%.1f" width="%.0f" height="1" fill="#555"/>`, y+lh/2, mw-32)
			} else {
				c := "#f0f0f0"
				if l.dim {
					c = "#9a9a9a"
				}
				s.text(34, y+lh/2+5, 13, c, l.t, l.bold)
			}
			y += lh
		}
	}))
	fmt.Println("wrote previews to", dir)
}
