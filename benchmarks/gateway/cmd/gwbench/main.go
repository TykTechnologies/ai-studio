// Command gwbench measures the latency the AI Gateway adds for an end user,
// and how it holds up under load. See benchmarks/gateway/README.md.
//
//	gwbench seed    configure Studio + edge for benchmarking, write the state file
//	gwbench run     run one or more scenario files, then write their reports
//	gwbench report  (re)generate the report for a finished run directory
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/TykTechnologies/midsommar/v2/benchmarks/gateway/internal/report"
	"github.com/TykTechnologies/midsommar/v2/benchmarks/gateway/internal/run"
	"github.com/TykTechnologies/midsommar/v2/benchmarks/gateway/internal/sampler"
	"github.com/TykTechnologies/midsommar/v2/benchmarks/gateway/internal/scenario"
	"github.com/TykTechnologies/midsommar/v2/benchmarks/gateway/internal/seed"
	"github.com/TykTechnologies/midsommar/v2/pkg/testinfra/vendorconformance"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var err error
	switch os.Args[1] {
	case "seed":
		err = cmdSeed(ctx, os.Args[2:])
	case "run":
		err = cmdRun(ctx, os.Args[2:])
	case "report":
		err = cmdReport(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  gwbench seed   [flags]                 configure Studio and the edge gateway
  gwbench run    [flags] scenario.yaml...  run scenarios and write reports
  gwbench report run-dir...              regenerate reports

Endpoints default from GWBENCH_* environment variables; run a subcommand with -h for flags.`)
	os.Exit(2)
}

func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "%s %s\n", time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
}

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// common holds the endpoint flags shared by seed and run.
type common struct {
	studio, gateway, mock, state string
}

func (c *common) register(fs *flag.FlagSet) {
	fs.StringVar(&c.studio, "studio", env("GWBENCH_STUDIO_URL", "http://localhost:18080"), "Studio API base URL (GWBENCH_STUDIO_URL)")
	fs.StringVar(&c.gateway, "gateway", env("GWBENCH_GATEWAY_URL", "http://localhost:18081"), "edge gateway base URL as the load generator reaches it (GWBENCH_GATEWAY_URL)")
	fs.StringVar(&c.mock, "mock", env("GWBENCH_MOCK_URL", "http://localhost:18099"), "mock upstream base URL as the load generator reaches it (GWBENCH_MOCK_URL)")
	fs.StringVar(&c.state, "state", env("GWBENCH_STATE", "benchmarks/gateway/.state/state.json"), "seed state file (holds the app secret; gitignored)")
}

func cmdSeed(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("seed", flag.ExitOnError)
	var c common
	c.register(fs)
	email := fs.String("email", env("GWBENCH_ADMIN_EMAIL", "bench-admin@example.com"), "admin email (first user on a fresh Studio)")
	password := fs.String("password", env("GWBENCH_ADMIN_PASSWORD", "Bench#Admin2026"), "admin password")
	mockUpstream := fs.String("mock-upstream", env("GWBENCH_MOCK_UPSTREAM_URL", "http://mockllm:9999"), "mock base URL as the *gateway* reaches it")
	vendors := fs.Bool("vendors", false, "also seed real OpenAI/Anthropic LLMs from test-secrets/vendors.env")
	minimal := fs.Bool("minimal", false, "also seed the minimal-configuration app and LLM (no budget, unpriced model, instant mock)")
	minEdges := fs.Int("min-edges", 1, "edges that must be registered before pushing config")
	timeout := fs.Duration("sync-timeout", 3*time.Minute, "how long to wait for edges to sync")
	_ = fs.Parse(args)

	st, err := seed.Run(ctx, seed.Config{
		StudioURL: c.studio, Email: *email, Password: *password, MockUpstreamURL: *mockUpstream,
		Vendors: *vendors, Minimal: *minimal, MinEdges: *minEdges, GatewayURL: c.gateway, SyncTimeout: *timeout,
	}, logf)
	if err != nil {
		return err
	}
	if err := seed.WriteState(c.state, st); err != nil {
		return err
	}
	logf("state written to %s", c.state)
	return nil
}

func cmdRun(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	var c common
	c.register(fs)
	out := fs.String("out", env("GWBENCH_OUT", "benchmarks/gateway/results"), "results root directory")
	quick := fs.Bool("quick", false, "shrink every scenario for a smoke run (not publishable)")
	label := fs.String("label", env("GWBENCH_LABEL", ""), "free-text label for the manifest, e.g. the environment")
	metricsURL := fs.String("metrics", env("GWBENCH_METRICS_URL", ""), "gateway Prometheus URL (default: <gateway>/metrics)")
	metricsToken := fs.String("metrics-token", env("GWBENCH_METRICS_TOKEN", ""), "bearer token for the metrics endpoint")
	noSample := fs.Bool("no-sample", false, "do not sample gateway resources")
	noAnalytics := fs.Bool("no-analytics-check", false, "skip the analytics completeness check")
	vendors := fs.Bool("vendors", false, "configure direct OpenAI/Anthropic targets from test-secrets/vendors.env")
	rateScale := fs.Float64("rate-scale", 1, "multiply every open-loop rate (size spike/soak runs from the ramp's knee)")
	_ = fs.Parse(args)
	if fs.NArg() == 0 {
		return fmt.Errorf("give at least one scenario file")
	}

	st, err := seed.ReadState(c.state)
	if err != nil {
		return err
	}
	targets := map[string]scenario.Target{
		"gateway": {BaseURL: c.gateway, Headers: map[string]string{"Authorization": "Bearer " + st.AppSecret}},
		"mock":    {BaseURL: c.mock},
	}
	if st.MinimalAppSecret != "" {
		targets["gateway-minimal"] = scenario.Target{BaseURL: c.gateway, Headers: map[string]string{"Authorization": "Bearer " + st.MinimalAppSecret}}
	}
	if *vendors {
		vc, err := vendorconformance.Load()
		if err != nil {
			return err
		}
		if v, ok := vc.Vendor("openai"); ok {
			targets["openai"] = scenario.Target{BaseURL: v.Endpoint, Headers: map[string]string{"Authorization": "Bearer " + v.APIKey}}
		}
		if v, ok := vc.Vendor("anthropic"); ok {
			version := v.Extra["api_version"]
			if version == "" {
				version = "2023-06-01"
			}
			targets["anthropic"] = scenario.Target{BaseURL: v.Endpoint,
				Headers: map[string]string{"x-api-key": v.APIKey, "anthropic-version": version}}
		}
	}

	var samp *sampler.Config
	if !*noSample {
		mu := *metricsURL
		if mu == "" {
			mu = strings.TrimSuffix(c.gateway, "/") + "/metrics"
		}
		samp = &sampler.Config{GatewayMetricsURL: mu, MetricsToken: *metricsToken,
			MockStatsURL: strings.TrimSuffix(c.mock, "/") + "/stats", Interval: time.Second}
	}

	var analytics func(context.Context) (int, error)
	if !*noAnalytics {
		studio := seed.NewStudio(st.StudioURL)
		studio.UseAPIKey(st.APIKey)
		analytics = func(ctx context.Context) (int, error) {
			now := time.Now()
			return studio.ProxyLogCount(ctx, st.AppID, now.Add(-24*time.Hour), now.Add(24*time.Hour))
		}
	}

	var dirs []string
	for _, path := range fs.Args() {
		sc, err := scenario.Load(path)
		if err != nil {
			return err
		}
		if *rateScale != 1 {
			sc.ScaleRate(*rateScale)
		}
		logf("scenario %s: %s", sc.Name, sc.Title)
		dir, err := run.Run(ctx, run.Options{
			Scenario: sc, Env: scenario.Env{Targets: targets, State: st}, OutRoot: *out, Quick: *quick,
			Label: *label, Sampler: samp, AnalyticsCount: analytics, Log: logf,
		})
		if dir != "" {
			dirs = append(dirs, dir)
			if s, rerr := report.Write(dir); rerr != nil {
				logf("report for %s failed: %v", dir, rerr)
			} else {
				verdict := "VALID"
				if !s.Valid {
					verdict = "INVALID"
				}
				logf("%s: %s -> %s", sc.Name, verdict, filepath.Join(dir, "report.html"))
			}
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func cmdReport(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("give at least one run directory")
	}
	for _, dir := range args {
		s, err := report.Write(dir)
		if err != nil {
			return fmt.Errorf("%s: %w", dir, err)
		}
		verdict := "VALID"
		if !s.Valid {
			verdict = "INVALID"
		}
		logf("%s: %s -> %s", s.Scenario, verdict, filepath.Join(dir, "report.html"))
	}
	return nil
}
