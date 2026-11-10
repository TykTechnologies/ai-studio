package config

import "testing"

func TestClusterNodeLabel(t *testing.T) {
	load := func(value string) *AppConf {
		vals := map[string]string{"CLUSTER_NODE_LABEL": value}
		return LoadFrom(func(k string) string { return vals[k] })
	}
	for value, want := range map[string]string{"": "", "site-a": "site-a", "  Site B (eu) ": "Site B (eu)"} {
		if got := load(value).ClusterNodeLabel; got != want {
			t.Errorf("CLUSTER_NODE_LABEL=%q: ClusterNodeLabel = %q, want %q", value, got, want)
		}
	}
}
