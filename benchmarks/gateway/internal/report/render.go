package report

import (
	"fmt"
	"html"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/benchmarks/gateway/internal/stats"
)

// Write analyses dir and writes summary.json, report.md and report.html.
func Write(dir string) (*Summary, error) {
	s, err := Analyze(dir)
	if err != nil {
		return nil, err
	}
	b, err := marshalSafe(s)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, "summary.json"), b, 0o644); err != nil {
		return nil, err
	}
	doc := build(s)
	if err := os.WriteFile(filepath.Join(dir, "report.md"), []byte(doc.markdown()), 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, "report.html"), []byte(doc.html(s.Title)), 0o644); err != nil {
		return nil, err
	}
	return s, nil
}

// --- document model ------------------------------------------------------------

type block interface{}

type heading struct {
	level int
	text  string
}
type para struct{ text string }
type note struct {
	ok   bool
	text string
}
type table struct {
	head []string
	rows [][]string
}
type chart struct {
	title  string
	xLabel string
	yLabel string
	series []series
}
type series struct {
	name string
	x, y []float64
}

type document struct{ blocks []block }

func (d *document) add(b ...block) { d.blocks = append(d.blocks, b...) }

// --- content -------------------------------------------------------------------

func build(s *Summary) *document {
	d := &document{}
	title := s.Title
	if title == "" {
		title = s.Scenario
	}
	d.add(heading{1, title})
	verdict := "VALID: every validity check passed."
	if !s.Valid {
		verdict = "INVALID: a validity check failed (see below). Do not quote these numbers."
	}
	d.add(note{s.Valid, verdict})
	if s.Quick {
		d.add(note{false, "Quick (smoke) run: sample sizes are too small to publish."})
	}

	sha := s.GitSHA
	if len(sha) > 12 {
		sha = sha[:12]
	}
	if s.GitDirty {
		sha += " (uncommitted changes)"
	}
	meta := [][]string{
		{"Scenario", s.Scenario},
		{"Mode", modeText(s.Mode)},
		{"Started", s.StartedAt.Format("2006-01-02 15:04:05 MST")},
		{"Duration", s.FinishedAt.Sub(s.StartedAt).Round(1e9).String()},
		{"Code", sha},
		{"Load generator", fmt.Sprintf("%s, %d CPUs, %s/%s, %s", s.Host.Hostname, s.Host.CPUs, s.Host.OS, s.Host.Arch, s.Host.Kernel)},
		{"Baseline arm", s.Baseline},
	}
	if s.Label != "" {
		meta = append(meta, []string{"Label", s.Label})
	}
	var tnames []string
	for k := range s.Targets {
		tnames = append(tnames, k)
	}
	sort.Strings(tnames)
	for _, k := range tnames {
		meta = append(meta, []string{"Target " + k, s.Targets[k]})
	}
	d.add(table{[]string{"", ""}, meta})
	if s.Description != "" {
		d.add(para{strings.TrimSpace(s.Description)})
	}
	d.add(para{"Latencies are in milliseconds, measured by the load generator from each request's intended send time. " +
		"TTFT is time to the first content token (for non-streaming requests, the complete response). " +
		"Overhead is the arm's percentile minus the baseline arm's, with a 95% bootstrap confidence interval. " +
		"Server-Timing figures are the gateway's own account of its time, excluding network hops."})

	d.add(heading{2, "Validity checks"})
	var rows [][]string
	for _, c := range s.Checks {
		mark := "pass"
		if !c.Passed {
			mark = strings.ToUpper(c.Severity)
		}
		rows = append(rows, []string{mark, c.Name, c.Cell, c.Detail})
	}
	d.add(table{[]string{"Result", "Check", "Cell", "Detail"}, rows})
	if len(s.Skipped) > 0 {
		var sk [][]string
		for k, v := range s.Skipped {
			sk = append(sk, []string{k, v})
		}
		sort.Slice(sk, func(i, j int) bool { return sk[i][0] < sk[j][0] })
		d.add(heading{3, "Skipped cells"}, table{[]string{"Cell", "Reason"}, sk})
	}

	// Headline: overhead per cell, the numbers a reader wants first.
	d.add(heading{2, "Overhead summary"})
	var head [][]string
	for _, c := range s.Cells {
		for _, o := range c.Overhead {
			row := []string{c.Name, o.Arm, strings.ToUpper(o.Metric), ci(o.P50), ci(o.P90), ci(o.P99)}
			if o.Paired != nil {
				row = append(row, fmt.Sprintf("%s (%d pairs)", ci(*o.Paired), o.Pairs))
			} else {
				row = append(row, "")
			}
			head = append(head, row)
		}
	}
	d.add(table{[]string{"Cell", "Arm", "Metric", "p50 overhead", "p90 overhead", "p99 overhead", "Paired median"}, head})

	for _, c := range s.Cells {
		d.add(heading{2, "Cell: " + c.Name})
		if c.StoppedBy != "" {
			d.add(note{false, "Stopped early: " + c.StoppedBy})
		}
		var ar [][]string
		for _, a := range c.Arms {
			ar = append(ar, []string{a.Arm, fmt.Sprint(a.Requests), fmt.Sprint(a.Errors), f1(a.AchievedRPS),
				f2(a.TTFB.P50), f2(a.TTFT.P50), f2(a.TTFT.P90), f2(a.TTFT.P99), f2(a.TTFT.P999),
				f2(a.Total.P50), f2(a.Total.P99), f2(a.Total.Max), f3(a.Lag.P99)})
		}
		d.add(table{[]string{"Arm", "n", "Errors", "req/s", "TTFB p50", "TTFT p50", "TTFT p90", "TTFT p99", "TTFT p99.9",
			"Total p50", "Total p99", "Total max", "Lag p99"}, ar})

		var st [][]string
		for _, a := range c.Arms {
			if len(a.Timing) == 0 {
				continue
			}
			// A metric the gateway did not report shows as "–", not 0.
			tm := func(name string, p float64) string {
				s, ok := a.Timing[name]
				if !ok {
					return f3(math.NaN())
				}
				if p == 50 {
					return f3(s.P50)
				}
				return f3(s.P99)
			}
			st = append(st, []string{a.Arm, tm("gw-pre", 50), tm("gw-pre", 99),
				tm("gw", 50), tm("gw", 99), tm("gw-ttfb", 50), tm("gw-ttfb", 99),
				f1(a.UpstreamNewConnPct) + "%"})
		}
		if len(st) > 0 {
			d.add(heading{3, "Gateway Server-Timing (self-reported)"},
				table{[]string{"Arm", "gw-pre p50", "gw-pre p99", "gw p50", "gw p99", "gw-ttfb p50", "gw-ttfb p99", "New upstream conns"}, st},
				para{"gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. " +
					"gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway " +
					"opened a fresh connection (TCP+TLS) to the upstream instead of reusing one."})
		}

		if len(c.Windows) > 0 {
			d.add(heading{3, "Over time"})
			var arms []string
			for arm := range c.Windows[0].Arms {
				arms = append(arms, arm)
			}
			sort.Strings(arms)
			var p99s, rps []series
			for _, arm := range arms {
				sp := series{name: arm + " TTFT p99"}
				sr := series{name: arm + " req/s"}
				for _, w := range c.Windows {
					if st, ok := w.Arms[arm]; ok {
						sp.x, sp.y = append(sp.x, w.StartS), append(sp.y, st.TTFTP99)
						sr.x, sr.y = append(sr.x, w.StartS), append(sr.y, st.RPS)
					}
				}
				p99s, rps = append(p99s, sp), append(rps, sr)
			}
			d.add(chart{"TTFT p99 by window", "seconds", "ms", p99s}, chart{"Request rate by window", "seconds", "req/s", rps})
			var wr [][]string
			for _, w := range c.Windows {
				for _, arm := range arms {
					st, ok := w.Arms[arm]
					if !ok {
						continue
					}
					wr = append(wr, []string{f0(w.StartS), arm, f1(st.RPS), fmt.Sprint(st.N), pct(st.ErrorRate),
						f2(st.TTFTP50), f2(st.TTFTP99), f2(st.TotalP99), f3(st.GWP99)})
				}
			}
			d.add(table{[]string{"t (s)", "Arm", "req/s", "n", "Errors", "TTFT p50", "TTFT p99", "Total p99", "gw p99"}, wr})
		}
		if c.Knee != nil {
			k := c.Knee
			txt := fmt.Sprintf("Sustained %.0f req/s within the %.0fms p99 overhead SLO.", k.SustainedRPS, k.SLOMS)
			switch {
			case k.BrokeAtRPS > 0:
				txt += fmt.Sprintf(" First breach at %.0f req/s: %s.", k.BrokeAtRPS, k.Reason)
			case k.StoppedBy != "":
				txt += " The run's stop condition ended the ramp during the next step: " + k.StoppedBy + "."
			default:
				txt += " No breach within the ramp: the ceiling is above the highest rate tested."
			}
			txt += fmt.Sprintf(" A step breaches on errors, or on p99 overhead measured by the gateway's Server-Timing or "+
				"against the baseline pooled over the ramp; latency is judged only from %d or more samples.", kneeMinSamples)
			d.add(heading{3, "Capacity"}, para{txt})
		}
		if r := c.Resources; r != nil {
			d.add(heading{3, "Gateway resources"})
			rows := [][]string{
				{"RSS (MB) start / end / max", fmt.Sprintf("%s / %s / %s", f1(r.RSSStartMB), f1(r.RSSEndMB), f1(r.RSSMaxMB))},
				{"Goroutines start / end / max", fmt.Sprintf("%s / %s / %s", f0(r.GoroutinesStart), f0(r.GoroutinesEnd), f0(r.GoroutinesMax))},
				{"Open FDs max", f0(r.FDsMax)},
				{"CPU cores avg / max", fmt.Sprintf("%s / %s", f2(r.CPUCoresAvg), f2(r.CPUCoresMax))},
			}
			if r.RSSSlopeMBPerH != 0 || r.GoroutineSlopePH != 0 {
				rows = append(rows, []string{"RSS growth (MB/hour)", f1(r.RSSSlopeMBPerH)},
					[]string{"Goroutine growth (per hour)", f1(r.GoroutineSlopePH)})
			}
			d.add(table{[]string{"", ""}, rows})
			if len(r.Series) > 1 {
				var rs, gs series
				rs.name, gs.name = "RSS MB", "goroutines"
				for _, p := range r.Series {
					rs.x, rs.y = append(rs.x, p.T), append(rs.y, p.RSSMB)
					gs.x, gs.y = append(gs.x, p.T), append(gs.y, p.Goroutines)
				}
				d.add(chart{"Gateway RSS", "seconds", "MB", []series{rs}}, chart{"Gateway goroutines", "seconds", "count", []series{gs}})
			}
		}
	}
	return d
}

func modeText(m string) string {
	switch m {
	case "closed":
		return "closed loop (fixed concurrency, back-to-back requests)"
	case "open":
		return "open loop (fixed arrival schedule, coordinated-omission corrected)"
	case "paired":
		return "paired ABBA blocks against real upstreams"
	}
	return m
}

func f0(v float64) string { return num(v, 0) }
func f1(v float64) string { return num(v, 1) }
func f2(v float64) string { return num(v, 2) }
func f3(v float64) string { return num(v, 3) }

func num(v float64, prec int) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "–"
	}
	return fmt.Sprintf("%.*f", prec, v)
}

func pct(v float64) string {
	if math.IsNaN(v) {
		return "–"
	}
	return fmt.Sprintf("%.2f%%", 100*v)
}

// ci renders an estimate with its interval: "1.23 [0.98, 1.51]".
func ci(c stats.CI) string {
	if math.IsNaN(c.Estimate) {
		return "–"
	}
	return fmt.Sprintf("%.2f [%.2f, %.2f]", c.Estimate, c.Lo, c.Hi)
}

// --- markdown --------------------------------------------------------------------

func (d *document) markdown() string {
	var b strings.Builder
	for _, bl := range d.blocks {
		switch v := bl.(type) {
		case heading:
			fmt.Fprintf(&b, "%s %s\n\n", strings.Repeat("#", v.level), v.text)
		case para:
			fmt.Fprintf(&b, "%s\n\n", v.text)
		case note:
			fmt.Fprintf(&b, "> **%s**\n\n", v.text)
		case table:
			if len(v.rows) == 0 {
				b.WriteString("_none_\n\n")
				continue
			}
			b.WriteString("| " + strings.Join(escapeMD(v.head), " | ") + " |\n")
			b.WriteString("|" + strings.Repeat(" --- |", len(v.head)) + "\n")
			for _, r := range v.rows {
				b.WriteString("| " + strings.Join(escapeMD(r), " | ") + " |\n")
			}
			b.WriteString("\n")
		case chart:
			// Charts are HTML-only; the tables carry the same numbers.
		}
	}
	return b.String()
}

func escapeMD(cells []string) []string {
	out := make([]string, len(cells))
	for i, c := range cells {
		out[i] = strings.ReplaceAll(c, "|", `\|`)
	}
	return out
}

// --- html --------------------------------------------------------------------------

var palette = []string{"var(--c1)", "var(--c2)", "var(--c3)", "var(--c4)", "var(--c5)", "var(--c6)"}

func (d *document) html(title string) string {
	var b strings.Builder
	b.WriteString(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">`)
	fmt.Fprintf(&b, "<title>%s</title>", html.EscapeString(title))
	b.WriteString(`<style>
:root{--bg:#fff;--fg:#1d1d1f;--muted:#6e6e73;--line:#e5e5ea;--ok:#1a7f37;--bad:#b42318;--okbg:#e9f7ee;--badbg:#fdecea;
--c1:#2563eb;--c2:#dc2626;--c3:#059669;--c4:#d97706;--c5:#7c3aed;--c6:#0891b2}
@media (prefers-color-scheme:dark){:root{--bg:#141416;--fg:#f2f2f7;--muted:#a1a1aa;--line:#2c2c30;--ok:#4ade80;--bad:#f87171;--okbg:#10291a;--badbg:#2d1414;
--c1:#60a5fa;--c2:#f87171;--c3:#34d399;--c4:#fbbf24;--c5:#a78bfa;--c6:#22d3ee}}
body{background:var(--bg);color:var(--fg);font:14px/1.5 system-ui,-apple-system,sans-serif;margin:0 auto;max-width:1200px;padding:24px 16px}
h1{font-size:24px}h2{font-size:19px;margin-top:36px;border-bottom:1px solid var(--line);padding-bottom:4px}h3{font-size:15px;margin-top:22px}
.tw{overflow-x:auto}table{border-collapse:collapse;margin:8px 0 16px;font-variant-numeric:tabular-nums}
th,td{border-bottom:1px solid var(--line);padding:5px 10px;text-align:right;white-space:nowrap}th{color:var(--muted);font-weight:600}
td:first-child,th:first-child,td:nth-child(2){text-align:left}
.note{padding:10px 14px;border-radius:8px;font-weight:600;margin:12px 0}.ok{background:var(--okbg);color:var(--ok)}.bad{background:var(--badbg);color:var(--bad)}
.charts{display:flex;flex-wrap:wrap;gap:16px}figure{margin:0;flex:1 1 440px;min-width:0}figcaption{color:var(--muted);font-size:13px}
svg{width:100%;height:auto}svg text{fill:var(--muted);font-size:11px}.axis{stroke:var(--line)}
.legend span{display:inline-block;margin-right:12px;font-size:12px}.legend i{display:inline-block;width:10px;height:10px;border-radius:2px;margin-right:4px;vertical-align:middle}
</style></head><body>`)
	inCharts := false
	closeCharts := func() {
		if inCharts {
			b.WriteString("</div>")
			inCharts = false
		}
	}
	for _, bl := range d.blocks {
		if _, ok := bl.(chart); !ok {
			closeCharts()
		}
		switch v := bl.(type) {
		case heading:
			fmt.Fprintf(&b, "<h%d>%s</h%d>", v.level, html.EscapeString(v.text), v.level)
		case para:
			fmt.Fprintf(&b, "<p>%s</p>", html.EscapeString(v.text))
		case note:
			cls := "ok"
			if !v.ok {
				cls = "bad"
			}
			fmt.Fprintf(&b, `<div class="note %s">%s</div>`, cls, html.EscapeString(v.text))
		case table:
			if len(v.rows) == 0 {
				b.WriteString("<p><em>none</em></p>")
				continue
			}
			b.WriteString(`<div class="tw"><table><thead><tr>`)
			for _, h := range v.head {
				fmt.Fprintf(&b, "<th>%s</th>", html.EscapeString(h))
			}
			b.WriteString("</tr></thead><tbody>")
			for _, r := range v.rows {
				b.WriteString("<tr>")
				for _, c := range r {
					fmt.Fprintf(&b, "<td>%s</td>", html.EscapeString(c))
				}
				b.WriteString("</tr>")
			}
			b.WriteString("</tbody></table></div>")
		case chart:
			if !inCharts {
				b.WriteString(`<div class="charts">`)
				inCharts = true
			}
			b.WriteString(svgChart(v))
		}
	}
	closeCharts()
	b.WriteString("</body></html>\n")
	return b.String()
}

func svgChart(c chart) string {
	const w, h, l, r, t, bt = 560.0, 260.0, 56.0, 12.0, 12.0, 34.0
	minX, maxX, maxY := math.Inf(1), math.Inf(-1), 0.0
	for _, s := range c.series {
		for i := range s.x {
			if math.IsNaN(s.y[i]) {
				continue
			}
			minX, maxX = math.Min(minX, s.x[i]), math.Max(maxX, s.x[i])
			maxY = math.Max(maxY, s.y[i])
		}
	}
	if math.IsInf(minX, 0) {
		return ""
	}
	if maxX == minX {
		maxX = minX + 1
	}
	if maxY == 0 {
		maxY = 1
	}
	maxY = niceCeil(maxY)
	px := func(x float64) float64 { return l + (x-minX)/(maxX-minX)*(w-l-r) }
	py := func(y float64) float64 { return t + (1-y/maxY)*(h-t-bt) }

	var b strings.Builder
	fmt.Fprintf(&b, `<figure><figcaption>%s</figcaption><svg viewBox="0 0 %.0f %.0f" role="img" aria-label="%s">`,
		html.EscapeString(c.title), w, h, html.EscapeString(c.title))
	for i := 0; i <= 4; i++ {
		y := maxY * float64(i) / 4
		fmt.Fprintf(&b, `<line class="axis" x1="%.1f" x2="%.1f" y1="%.1f" y2="%.1f"/>`, l, w-r, py(y), py(y))
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" text-anchor="end">%s</text>`, l-6, py(y)+4, trimNum(y))
	}
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f">%s</text>`, l, h-8, trimNum(minX))
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" text-anchor="end">%s %s</text>`, w-r, h-8, trimNum(maxX), html.EscapeString(c.xLabel))
	fmt.Fprintf(&b, `<text x="8" y="%.1f">%s</text>`, t+8, html.EscapeString(c.yLabel))
	for i, s := range c.series {
		var pts []string
		for j := range s.x {
			if !math.IsNaN(s.y[j]) {
				pts = append(pts, fmt.Sprintf("%.1f,%.1f", px(s.x[j]), py(s.y[j])))
			}
		}
		col := palette[i%len(palette)]
		fmt.Fprintf(&b, `<polyline fill="none" stroke="%s" stroke-width="2" points="%s"/>`, col, strings.Join(pts, " "))
	}
	b.WriteString(`</svg><div class="legend">`)
	for i, s := range c.series {
		fmt.Fprintf(&b, `<span><i style="background:%s"></i>%s</span>`, palette[i%len(palette)], html.EscapeString(s.name))
	}
	b.WriteString("</div></figure>")
	return b.String()
}

func niceCeil(v float64) float64 {
	mag := math.Pow(10, math.Floor(math.Log10(v)))
	for _, m := range []float64{1, 2, 2.5, 5, 10} {
		if m*mag >= v {
			return m * mag
		}
	}
	return 10 * mag
}

func trimNum(v float64) string {
	if v >= 100 || v == math.Trunc(v) {
		return fmt.Sprintf("%.0f", v)
	}
	return fmt.Sprintf("%.2g", v)
}
