// Command mockllm is the benchmark upstream: an OpenAI- and Anthropic-shaped
// LLM server that paces its output to a configurable profile (first-token
// delay, token rate, output length) and records nothing per request, so it can
// sustain far more load than the gateway under test.
//
// The default profile is set with -profile; any request can layer its own on
// top through a /p/{spec}/ path prefix or the X-Mock-Profile header. See
// pkg/testinfra/mockllm.Server.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/testinfra/mockllm"
)

func main() {
	addr := flag.String("addr", ":9999", "listen address")
	spec := flag.String("profile", "tokens=16", "default profile, e.g. ttft=300ms,ttft_p99=900ms,tps=50,tokens=200")
	flag.Parse()

	profile, err := mockllm.ParseProfile(mockllm.DefaultProfile(), *spec)
	if err != nil {
		log.Fatalf("invalid -profile: %v", err)
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           mockllm.NewServer(profile),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       5 * time.Minute,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()

	log.Printf("mockllm listening on %s with profile %+v", *addr, profile)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
