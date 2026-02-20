package main

// Build-time variables (set via -ldflags "-X main.Version=...")
var (
	Version   = "dev"
	BuildHash = "unknown"
	BuildTime = "unknown"
)
