package server

import (
	"net/http"
	"net/http/pprof"
	"runtime"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/rs/zerolog/log"
)

// StartProfiling serves Go's pprof endpoints on their own listener
// (PROFILING_ADDR, loopback by default) when ENABLE_PROFILING is set, and
// turns on mutex and block sampling so /debug/pprof/mutex and
// /debug/pprof/block show where goroutines wait. It is off by default: the
// endpoints expose internals and the sampling costs a little CPU.
func StartProfiling(cfg *config.ObservabilityConfig) {
	if cfg == nil || !cfg.EnableProfiling {
		return
	}
	runtime.SetMutexProfileFraction(10)
	runtime.SetBlockProfileRate(int(100 * time.Microsecond))

	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	srv := &http.Server{Addr: cfg.ProfilingAddr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		log.Info().Str("addr", cfg.ProfilingAddr).Msg("pprof endpoints enabled")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Str("addr", cfg.ProfilingAddr).Msg("pprof listener stopped")
		}
	}()
}
