//go:build tools

// Package gormpin holds the versions of the gorm modules copied into
// third_party/gorm.io. It is never built; the imports keep go mod tidy from
// dropping the requirements.
package gormpin

import (
	_ "gorm.io/driver/postgres"
	_ "gorm.io/driver/sqlite"
	_ "gorm.io/gorm"
)
