//go:build enterprise
// +build enterprise

package main

import (
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/budget" // Register enterprise budget service
)
