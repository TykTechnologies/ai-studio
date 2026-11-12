// tests/helm_studio_replicas_test.go
//
// The Studio chart with several replicas (midsommar-clean-4x5y.22): what
// replicaCount and the external-database values render, and the renders the
// chart refuses. The first test reads the files and runs without helm; the
// others render the chart and skip when helm is missing.
package tests

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// renderedDoc returns the document of kind named name in a render, nil when
// there is none.
func renderedDoc(t *testing.T, rendered, kind, name string) map[string]any {
	t.Helper()
	dec := yaml.NewDecoder(strings.NewReader(rendered))
	for {
		var doc map[string]any
		if err := dec.Decode(&doc); err != nil {
			return nil
		}
		if doc == nil || doc["kind"] != kind {
			continue
		}
		if n, _ := dig(t, doc, "metadata", "name").(string); n == name {
			return doc
		}
	}
}

func studioContainer(t *testing.T, deployment map[string]any) map[string]any {
	t.Helper()
	containers, _ := dig(t, deployment, "spec", "template", "spec", "containers").([]any)
	require.NotEmpty(t, containers)
	c, _ := containers[0].(map[string]any)
	require.Equal(t, "midsommar", c["name"])
	return c
}

// externalReplicas are the values of two replicas on an external database.
var externalReplicas = []string{
	"--set", "replicaCount=2",
	"--set", "database.internal=false",
	"--set", "database.existingSecret=studio-database",
	"--set", "config.csrfKey=0123456789abcdef0123456789abcdef",
}

func TestHelmStudioReplicaDefaults(t *testing.T) {
	values := readValues(t, "../helm/values.yaml")
	assert.Equal(t, 1, lookup(t, values, "replicaCount"))
	assert.Empty(t, lookup(t, values, "midsommar", "strategy"), "Recreate for one replica, chosen in the template")
	assert.Equal(t, "", lookup(t, values, "database", "existingSecret"))
	assert.Equal(t, "", lookup(t, values, "config", "csrfKey"))
	assert.Equal(t, "", lookup(t, values, "config", "clusterNodeLabel"))
	assert.Equal(t, false, lookup(t, values, "probes", "readiness", "requireWrite"))
	_, hasAutoscaling := values["autoscaling"]
	assert.False(t, hasAutoscaling, "Studio is scaled by replicaCount; no unused autoscaling values")

	// The startup probe covers DATABASE_STARTUP_TIMEOUT (60s), the
	// migrations, and one more connection attempt (2 x 10s with
	// sslmode=prefer): at least 120s.
	period := lookup(t, values, "probes", "startup", "periodSeconds").(int)
	threshold := lookup(t, values, "probes", "startup", "failureThreshold").(int)
	assert.GreaterOrEqual(t, period*threshold, 120)
}

// One replica renders as before: one pod, Recreate, no PDB, no spread, no
// affinity annotations, /ready; the startup probe is the one addition.
func TestHelmStudioOneReplica(t *testing.T) {
	rendered := helmTemplate(t)
	d := renderedDoc(t, rendered, "Deployment", "test-midsommar")
	require.NotNil(t, d)
	assert.EqualValues(t, 1, dig(t, d, "spec", "replicas"))
	assert.Equal(t, map[string]any{"type": "Recreate"}, dig(t, d, "spec", "strategy"))
	assert.Nil(t, dig(t, d, "spec", "template", "spec", "topologySpreadConstraints"))
	assert.Nil(t, renderedDoc(t, rendered, "PodDisruptionBudget", "test-midsommar"))

	c := studioContainer(t, d)
	assert.Equal(t, "/ready", dig(t, c, "readinessProbe", "httpGet", "path"))
	assert.Equal(t, "/health", dig(t, c, "startupProbe", "httpGet", "path"))
	assert.Equal(t, "/health", dig(t, c, "livenessProbe", "httpGet", "path"))

	ing := renderedDoc(t, rendered, "Ingress", "test-midsommar")
	require.NotNil(t, ing)
	annotations, _ := dig(t, ing, "metadata", "annotations").(map[string]any)
	assert.NotContains(t, annotations, "nginx.ingress.kubernetes.io/affinity")

	cm := renderedDoc(t, rendered, "ConfigMap", "test-midsommar-config")
	require.NotNil(t, cm)
	data, _ := cm["data"].(map[string]any)
	for _, name := range []string{"CLUSTER_NODE_LABEL", "DATABASE_READY_GRACE", "DATABASE_STARTUP_TIMEOUT", "DATABASE_CONNECT_TIMEOUT"} {
		assert.NotContains(t, data, name, "unset keeps Studio's default")
	}
}

// Two replicas on an external database: a rolling update that keeps every
// replica serving, a PDB, spread over nodes and zones, the database URL
// from the Secret, CSRF_KEY in the chart's Secret, and cookie affinity on
// the Ingress.
func TestHelmStudioTwoReplicasOnAnExternalDatabase(t *testing.T) {
	rendered := helmTemplate(t, append(externalReplicas,
		"--set", "config.clusterNodeLabel=site-a",
		"--set", "config.databaseReadyGrace=0",
		"--set", "config.databaseStartupTimeout=90s",
	)...)
	d := renderedDoc(t, rendered, "Deployment", "test-midsommar")
	require.NotNil(t, d)
	assert.EqualValues(t, 2, dig(t, d, "spec", "replicas"))
	assert.Equal(t, "RollingUpdate", dig(t, d, "spec", "strategy", "type"))
	assert.EqualValues(t, 0, dig(t, d, "spec", "strategy", "rollingUpdate", "maxUnavailable"))

	spread, _ := dig(t, d, "spec", "template", "spec", "topologySpreadConstraints").([]any)
	require.Len(t, spread, 2)
	var keys []string
	for _, s := range spread {
		keys = append(keys, s.(map[string]any)["topologyKey"].(string))
	}
	assert.ElementsMatch(t, []string{"kubernetes.io/hostname", "topology.kubernetes.io/zone"}, keys)

	pdb := renderedDoc(t, rendered, "PodDisruptionBudget", "test-midsommar")
	require.NotNil(t, pdb)
	assert.EqualValues(t, 1, dig(t, pdb, "spec", "minAvailable"))
	assert.Equal(t, dig(t, d, "spec", "selector", "matchLabels"), dig(t, pdb, "spec", "selector", "matchLabels"))

	c := studioContainer(t, d)
	var dbURL map[string]any
	for _, e := range c["env"].([]any) {
		if e.(map[string]any)["name"] == "DATABASE_URL" {
			dbURL = e.(map[string]any)
		}
	}
	require.NotNil(t, dbURL)
	assert.Nil(t, dbURL["value"])
	assert.Equal(t, "studio-database", dig(t, dbURL, "valueFrom", "secretKeyRef", "name"))
	assert.Equal(t, "DATABASE_URL", dig(t, dbURL, "valueFrom", "secretKeyRef", "key"))

	cm := renderedDoc(t, rendered, "ConfigMap", "test-midsommar-config")
	data, _ := cm["data"].(map[string]any)
	assert.NotContains(t, data, "DATABASE_URL", "not in a ConfigMap when it comes from a Secret")
	assert.Equal(t, "site-a", data["CLUSTER_NODE_LABEL"])
	assert.Equal(t, "0", data["DATABASE_READY_GRACE"], "0 is rendered, not read as unset")
	assert.Equal(t, "90s", data["DATABASE_STARTUP_TIMEOUT"])

	secret := renderedDoc(t, rendered, "Secret", "test-midsommar-secrets")
	require.NotNil(t, secret)
	assert.Equal(t, "0123456789abcdef0123456789abcdef", dig(t, secret, "stringData", "CSRF_KEY"))

	ing := renderedDoc(t, rendered, "Ingress", "test-midsommar")
	annotations, _ := dig(t, ing, "metadata", "annotations").(map[string]any)
	assert.Equal(t, "cookie", annotations["nginx.ingress.kubernetes.io/affinity"])
	assert.Equal(t, "tyk-ai-studio-route", annotations["nginx.ingress.kubernetes.io/session-cookie-name"])
	assert.Equal(t, "true", annotations["nginx.ingress.kubernetes.io/ssl-redirect"], "the chart's own annotations stay")
}

// The values an operator sets win over the chart's choices.
func TestHelmStudioReplicaOverrides(t *testing.T) {
	rendered := helmTemplate(t, append(externalReplicas,
		"--set", "midsommar.strategy.type=Recreate",
		"--set", "podDisruptionBudget.maxUnavailable=1",
		"--set", "topologySpreadConstraints[0].maxSkew=2",
		"--set", "topologySpreadConstraints[0].topologyKey=topology.kubernetes.io/zone",
		"--set", "topologySpreadConstraints[0].whenUnsatisfiable=DoNotSchedule",
		"--set", "midsommar.ingress.annotations.nginx\\.ingress\\.kubernetes\\.io/session-cookie-name=mine",
		"--set", "probes.readiness.requireWrite=true",
	)...)
	d := renderedDoc(t, rendered, "Deployment", "test-midsommar")
	assert.Equal(t, map[string]any{"type": "Recreate"}, dig(t, d, "spec", "strategy"))
	spread, _ := dig(t, d, "spec", "template", "spec", "topologySpreadConstraints").([]any)
	require.Len(t, spread, 1)
	assert.Equal(t, "DoNotSchedule", spread[0].(map[string]any)["whenUnsatisfiable"])
	assert.Equal(t, "/ready?require=write", dig(t, studioContainer(t, d), "readinessProbe", "httpGet", "path"))

	pdb := renderedDoc(t, rendered, "PodDisruptionBudget", "test-midsommar")
	assert.EqualValues(t, 1, dig(t, pdb, "spec", "maxUnavailable"))
	assert.Nil(t, dig(t, pdb, "spec", "minAvailable"))

	ing := renderedDoc(t, rendered, "Ingress", "test-midsommar")
	annotations, _ := dig(t, ing, "metadata", "annotations").(map[string]any)
	assert.Equal(t, "mine", annotations["nginx.ingress.kubernetes.io/session-cookie-name"])

	// Affinity off, or another ingress class: no nginx affinity annotations.
	for _, extra := range [][]string{
		{"--set", "midsommar.ingress.sessionAffinity.enabled=false"},
		{"--set", "midsommar.ingress.className=traefik"},
	} {
		ing := renderedDoc(t, helmTemplate(t, append(append([]string{}, externalReplicas...), extra...)...), "Ingress", "test-midsommar")
		annotations, _ := dig(t, ing, "metadata", "annotations").(map[string]any)
		assert.NotContains(t, annotations, "nginx.ingress.kubernetes.io/affinity", "%v", extra)
	}
}

// Several replicas on a database they cannot share, or without one
// CSRF_KEY, are refused with the reason.
func TestHelmStudioReplicasRefused(t *testing.T) {
	out := helmTemplateExpectingError(t, "--set", "replicaCount=2", "--set", "config.csrfKey=x")
	assert.Contains(t, out, "replicaCount > 1 needs an external PostgreSQL")

	out = helmTemplateExpectingError(t, "--set", "replicaCount=2", "--set", "database.internal=false",
		"--set", "config.databaseType=sqlite", "--set", "config.csrfKey=x")
	assert.Contains(t, out, "replicaCount > 1 needs PostgreSQL: SQLite serves one Studio process")

	out = helmTemplateExpectingError(t, "--set", "replicaCount=2", "--set", "database.internal=false")
	assert.Contains(t, out, "replicaCount > 1 needs config.csrfKey")
}

// global.nodeSelector, global.tolerations and global.affinity place the
// AI Studio pods and the bundled PostgreSQL pod (one site per node pool);
// the top-level affinity, when set, replaces global.affinity for Studio.
func TestHelmStudioGlobalScheduling(t *testing.T) {
	rendered := helmTemplate(t,
		"--set", "global.nodeSelector.pool=site-a",
		"--set", "global.tolerations[0].key=site",
		"--set", "global.tolerations[0].operator=Equal",
		"--set", "global.tolerations[0].value=a",
		"--set", "global.tolerations[0].effect=NoSchedule",
		"--set", "global.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms[0].matchExpressions[0].key=zone",
		"--set", "global.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms[0].matchExpressions[0].operator=Exists",
	)
	for _, name := range []string{"test-midsommar", "test-midsommar-postgres"} {
		d := renderedDoc(t, rendered, "Deployment", name)
		require.NotNil(t, d, name)
		spec, _ := dig(t, d, "spec", "template", "spec").(map[string]any)
		assert.Equal(t, map[string]any{"pool": "site-a"}, spec["nodeSelector"], name)
		tolerations, _ := spec["tolerations"].([]any)
		require.Len(t, tolerations, 1, name)
		assert.Equal(t, "site", tolerations[0].(map[string]any)["key"], name)
		assert.NotNil(t, dig(t, spec, "affinity", "nodeAffinity"), name)
	}

	// The top-level affinity wins for Studio.
	rendered = helmTemplate(t,
		"--set", "global.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms[0].matchExpressions[0].key=zone",
		"--set", "global.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms[0].matchExpressions[0].operator=Exists",
		"--set", "affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[0].weight=100",
		"--set", "affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[0].podAffinityTerm.topologyKey=kubernetes.io/hostname",
	)
	d := renderedDoc(t, rendered, "Deployment", "test-midsommar")
	affinity, _ := dig(t, d, "spec", "template", "spec", "affinity").(map[string]any)
	assert.Contains(t, affinity, "podAntiAffinity")
	assert.NotContains(t, affinity, "nodeAffinity", "replaced, not merged")

	// Unset: none of them is rendered.
	d = renderedDoc(t, helmTemplate(t), "Deployment", "test-midsommar")
	spec, _ := dig(t, d, "spec", "template", "spec").(map[string]any)
	for _, k := range []string{"nodeSelector", "tolerations", "affinity"} {
		assert.NotContains(t, spec, k)
	}
}

// A Studio pod sleeps before it stops, so requests routed to it while
// Kubernetes takes it out of the Service are still answered, and its grace
// period covers the sleep and Studio's 30-second drain.
func TestHelmStudioStopDelay(t *testing.T) {
	values := readValues(t, "../helm/values.yaml")
	assert.Equal(t, 10, lookup(t, values, "shutdown", "preStopDelaySeconds"))

	studio := func(rendered string) map[string]any {
		d := renderedDoc(t, rendered, "Deployment", "test-midsommar")
		require.NotNil(t, d)
		return d
	}
	d := studio(helmTemplate(t))
	assert.EqualValues(t, 45, dig(t, d, "spec", "template", "spec", "terminationGracePeriodSeconds"), "10 + 30 + 5")
	assert.EqualValues(t, 10, dig(t, studioContainer(t, d), "lifecycle", "preStop", "sleep", "seconds"), "the sleep action: no shell needed")

	d = studio(helmTemplate(t, "--set", "shutdown.preStopDelaySeconds=0"))
	assert.Nil(t, studioContainer(t, d)["lifecycle"], "0: no preStop")
	assert.EqualValues(t, 35, dig(t, d, "spec", "template", "spec", "terminationGracePeriodSeconds"))

	d = studio(helmTemplate(t, "--set", "shutdown.preStopDelaySeconds=15", "--set", "shutdown.terminationGracePeriodSeconds=60"))
	assert.EqualValues(t, 60, dig(t, d, "spec", "template", "spec", "terminationGracePeriodSeconds"))
	assert.EqualValues(t, 15, dig(t, studioContainer(t, d), "lifecycle", "preStop", "sleep", "seconds"))

	out := helmTemplateExpectingError(t, "--set", "shutdown.terminationGracePeriodSeconds=20")
	assert.Contains(t, out, "must be at least shutdown.preStopDelaySeconds (10) plus Studio's 30-second shutdown: 40")
}
