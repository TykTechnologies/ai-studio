// cmd/mgw/main.go
package main

import (
	"os"

	"github.com/TykTechnologies/midsommar/microgateway/cmd/mgw/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}