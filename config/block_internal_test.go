package config

import (
	"strings"
	"testing"
)

// LLM_UPSTREAM_BLOCK_INTERNAL is on unless set to false, and a value that is
// not a boolean is a startup error, never a silent default (sf5).
func TestBlockInternalSetting(t *testing.T) {
	load := func(v string) *AppConf {
		return LoadFrom(func(k string) string {
			if k == "LLM_UPSTREAM_BLOCK_INTERNAL" {
				return v
			}
			return ""
		})
	}
	if conf := load(""); conf.AllowInternalUpstreams || conf.Err() != nil {
		t.Errorf("unset: AllowInternalUpstreams = %v, err %v; want blocking on", conf.AllowInternalUpstreams, conf.Err())
	}
	if conf := load(" false "); !conf.AllowInternalUpstreams || conf.Err() != nil {
		t.Errorf("false: AllowInternalUpstreams = %v, err %v", conf.AllowInternalUpstreams, conf.Err())
	}
	for _, v := range []string{"ture", "yes", "off"} {
		err := load(v).Err()
		if err == nil || !strings.Contains(err.Error(), "LLM_UPSTREAM_BLOCK_INTERNAL") {
			t.Errorf("%q: err = %v, want an error naming the variable", v, err)
		}
	}
}
