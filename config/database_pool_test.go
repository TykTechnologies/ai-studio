package config

import (
	"testing"
	"time"
)

func TestDatabasePoolSettings(t *testing.T) {
	load := func(vals map[string]string) *AppConf {
		return LoadFrom(func(k string) string { return vals[k] })
	}

	conf := load(nil)
	if conf.DatabaseMaxOpenConns != 0 || conf.DatabaseMaxIdleConns != 0 {
		t.Errorf("unset limits = %d open, %d idle; want the driver's defaults (0)", conf.DatabaseMaxOpenConns, conf.DatabaseMaxIdleConns)
	}
	if conf.DatabaseConnMaxLifetime != 30*time.Minute {
		t.Errorf("default DATABASE_CONN_MAX_LIFETIME = %s, want 30m", conf.DatabaseConnMaxLifetime)
	}
	if conf.DatabaseConnectTimeout != 10*time.Second {
		t.Errorf("default DATABASE_CONNECT_TIMEOUT = %s, want 10s", conf.DatabaseConnectTimeout)
	}

	conf = load(map[string]string{
		"DATABASE_MAX_OPEN_CONNS": "40", "DATABASE_MAX_IDLE_CONNS": "10",
		"DATABASE_CONN_MAX_LIFETIME": "0", "DATABASE_CONNECT_TIMEOUT": "3s",
	})
	if conf.DatabaseMaxOpenConns != 40 || conf.DatabaseMaxIdleConns != 10 {
		t.Errorf("limits = %d open, %d idle; want 40, 10", conf.DatabaseMaxOpenConns, conf.DatabaseMaxIdleConns)
	}
	if conf.DatabaseConnMaxLifetime != 0 {
		t.Errorf("DATABASE_CONN_MAX_LIFETIME=0 = %s, want 0 (never)", conf.DatabaseConnMaxLifetime)
	}
	if conf.DatabaseConnectTimeout != 3*time.Second {
		t.Errorf("DATABASE_CONNECT_TIMEOUT = %s, want 3s", conf.DatabaseConnectTimeout)
	}

	// Invalid values fall back to the defaults.
	conf = load(map[string]string{
		"DATABASE_MAX_OPEN_CONNS": "-1", "DATABASE_MAX_IDLE_CONNS": "many",
		"DATABASE_CONN_MAX_LIFETIME": "-5m", "DATABASE_CONNECT_TIMEOUT": "soon",
	})
	if conf.DatabaseMaxOpenConns != 0 || conf.DatabaseMaxIdleConns != 0 ||
		conf.DatabaseConnMaxLifetime != 30*time.Minute || conf.DatabaseConnectTimeout != 10*time.Second {
		t.Errorf("invalid values = %d, %d, %s, %s; want the defaults", conf.DatabaseMaxOpenConns, conf.DatabaseMaxIdleConns, conf.DatabaseConnMaxLifetime, conf.DatabaseConnectTimeout)
	}
}
