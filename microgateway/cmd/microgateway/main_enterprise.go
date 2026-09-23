//go:build enterprise
// +build enterprise

package main

import (
	// The Semantic Router engine registers itself with pkg/semanticrouting.
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/semantic_router/engine"
)
