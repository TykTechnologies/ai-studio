//go:build enterprise
// +build enterprise

package main

import (
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/all" // Register every enterprise feature
)
