// tests/helm_chart_test.go
//
// Renders the umbrella chart and asserts the invariants that are easy to break
// silently. Every check here corresponds to something that would only surface on
// a real cluster — a rejected upgrade, a pod that never registers, a Prometheus
// target that scrapes 404 forever — so they are worth pinning in CI rather than
// re-checking by hand.
//
// Skips when helm is not installed.
package tests

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const microgatewayChartPath = "../helm"

func helmTemplate(t *testing.T, setArgs ...string) string {
	t.Helper()
	helm, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm not installed; skipping chart render tests")
	}
	chart, err := filepath.Abs(microgatewayChartPath)
	if err != nil {
		t.Fatal(err)
	}

	args := append([]string{"template", "test", chart}, setArgs...)
	out, err := exec.Command(helm, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("helm template failed: %v\n%s", err, out)
	}
	return string(out)
}

// helmTemplateExpectingError renders and returns the error output, for the
// cases where the chart should deliberately refuse to render.
func helmTemplateExpectingError(t *testing.T, setArgs ...string) string {
	t.Helper()
	helm, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm not installed; skipping chart render tests")
	}
	chart, err := filepath.Abs(microgatewayChartPath)
	if err != nil {
		t.Fatal(err)
	}

	args := append([]string{"template", "test", chart}, setArgs...)
	out, err := exec.Command(helm, args...).CombinedOutput()
	if err == nil {
		t.Fatalf("expected helm template to fail, but it succeeded:\n%s", out)
	}
	return string(out)
}

// microgatewayWorkload returns the parsed Deployment or StatefulSet for the
// microgateway from a rendered manifest stream.
func microgatewayWorkload(t *testing.T, rendered string) map[string]any {
	t.Helper()
	dec := yaml.NewDecoder(strings.NewReader(rendered))
	for {
		var doc map[string]any
		err := dec.Decode(&doc)
		if err != nil {
			break
		}
		if doc == nil {
			continue
		}
		kind, _ := doc["kind"].(string)
		if kind != "Deployment" && kind != "StatefulSet" {
			continue
		}
		meta, _ := doc["metadata"].(map[string]any)
		if name, _ := meta["name"].(string); name == "test-microgateway" {
			return doc
		}
	}
	t.Fatal("no microgateway Deployment or StatefulSet found in the rendered chart")
	return nil
}

func dig(t *testing.T, m map[string]any, path ...string) any {
	t.Helper()
	var cur any = m
	for _, key := range path {
		asMap, ok := cur.(map[string]any)
		if !ok {
			t.Fatalf("path %v: %q is not a map", path, key)
		}
		cur = asMap[key]
	}
	return cur
}

// TestSelectorLabelsAreImmutable is the most important check in this file. A
// Deployment's spec.selector is immutable, so adding the standard
// app.kubernetes.io/* labels to it would make `helm upgrade` fail on every
// existing install. They belong on metadata and the pod template only.
func TestSelectorLabelsAreImmutable(t *testing.T) {
	workload := microgatewayWorkload(t, helmTemplate(t))

	selector, ok := dig(t, workload, "spec", "selector", "matchLabels").(map[string]any)
	if !ok {
		t.Fatal("spec.selector.matchLabels is missing")
	}
	if len(selector) != 1 || selector["app"] != "microgateway" {
		t.Errorf("selector.matchLabels = %v, want exactly {app: microgateway}; "+
			"changing it breaks in-place upgrades", selector)
	}

	// The standard labels must still be present where they are safe.
	podLabels, ok := dig(t, workload, "spec", "template", "metadata", "labels").(map[string]any)
	if !ok {
		t.Fatal("pod template labels are missing")
	}
	if podLabels["app.kubernetes.io/name"] != "microgateway" {
		t.Error("standard labels are missing from the pod template")
	}
	if podLabels["app"] != "microgateway" {
		t.Error("the selector label must remain on the pod template or the selector matches nothing")
	}
}

// TestEdgeIDIsPerPod guards the scale-out bug: a single EDGE_ID value in the
// ConfigMap made every replica register with the hub under one identity.
func TestEdgeIDIsPerPod(t *testing.T) {
	rendered := helmTemplate(t, "--set", "microgateway.replicaCount=3")

	if strings.Contains(rendered, "EDGE_ID: ") {
		t.Error("EDGE_ID is set as a literal ConfigMap value; every replica would share one edge identity")
	}

	workload := microgatewayWorkload(t, rendered)
	containers, ok := dig(t, workload, "spec", "template", "spec", "containers").([]any)
	if !ok || len(containers) == 0 {
		t.Fatal("no containers found")
	}
	container, _ := containers[0].(map[string]any)
	env, ok := container["env"].([]any)
	if !ok {
		t.Fatal("container has no env block")
	}

	var found bool
	for _, e := range env {
		entry, _ := e.(map[string]any)
		if entry["name"] != "EDGE_ID" {
			continue
		}
		found = true
		fieldPath := dig(t, entry, "valueFrom", "fieldRef", "fieldPath")
		if fieldPath != "metadata.name" {
			t.Errorf("EDGE_ID fieldPath = %v, want metadata.name", fieldPath)
		}
	}
	if !found {
		t.Error("EDGE_ID is not set from the downward API")
	}
}

// TestReadinessUsesReadyEndpoint: /health only reports that the process is up,
// while /ready checks the database and plugin health. Probing the wrong one
// leaves broken pods in the Service.
func TestReadinessUsesReadyEndpoint(t *testing.T) {
	workload := microgatewayWorkload(t, helmTemplate(t))
	containers, _ := dig(t, workload, "spec", "template", "spec", "containers").([]any)
	container, _ := containers[0].(map[string]any)

	if path := dig(t, container, "readinessProbe", "httpGet", "path"); path != "/ready" {
		t.Errorf("readinessProbe path = %v, want /ready", path)
	}
	if path := dig(t, container, "livenessProbe", "httpGet", "path"); path != "/health" {
		t.Errorf("livenessProbe path = %v, want /health", path)
	}
	if dig(t, container, "startupProbe", "httpGet", "path") == nil {
		t.Error("no startupProbe; a slow first config sync would be killed by the liveness probe")
	}
}

// TestGracePeriodCoversShutdownTimeout: the gateway drains for SHUTDOWN_TIMEOUT
// (30s). A shorter grace period means Kubernetes SIGKILLs it mid-drain.
func TestGracePeriodCoversShutdownTimeout(t *testing.T) {
	workload := microgatewayWorkload(t, helmTemplate(t))
	grace, ok := dig(t, workload, "spec", "template", "spec", "terminationGracePeriodSeconds").(int)
	if !ok {
		t.Fatal("terminationGracePeriodSeconds is not set")
	}
	if grace < 30 {
		t.Errorf("terminationGracePeriodSeconds = %d, must be >= the 30s SHUTDOWN_TIMEOUT", grace)
	}
}

// TestServiceMonitorRequiresScrapeAuth: the gateway does not register /metrics
// at all unless a token is set or unauthenticated access is allowed, so a
// ServiceMonitor without either would scrape a 404 indefinitely. The chart
// should refuse to render rather than ship that.
func TestServiceMonitorRequiresScrapeAuth(t *testing.T) {
	t.Run("refuses to render without scrape auth", func(t *testing.T) {
		out := helmTemplateExpectingError(t,
			"--set", "microgateway.metrics.serviceMonitor.enabled=true",
			"--set", "microgateway.metrics.allowUnauthenticated=false")
		if !strings.Contains(out, "metrics.serviceMonitor.enabled requires") {
			t.Errorf("expected a helpful failure message, got:\n%s", out)
		}
	})

	t.Run("renders with a bearer token secret", func(t *testing.T) {
		rendered := helmTemplate(t,
			"--set", "microgateway.metrics.serviceMonitor.enabled=true",
			"--set", "microgateway.metrics.allowUnauthenticated=false",
			"--set", "microgateway.metrics.bearerTokenSecret.name=mg-metrics")
		if !strings.Contains(rendered, "kind: ServiceMonitor") {
			t.Error("ServiceMonitor was not rendered")
		}
		if !strings.Contains(rendered, "name: mg-metrics") {
			t.Error("the bearer token secret is not referenced")
		}
	})

	t.Run("renders with unauthenticated scraping", func(t *testing.T) {
		rendered := helmTemplate(t,
			"--set", "microgateway.metrics.serviceMonitor.enabled=true",
			"--set", "microgateway.metrics.allowUnauthenticated=true")
		if !strings.Contains(rendered, "kind: ServiceMonitor") {
			t.Error("ServiceMonitor was not rendered")
		}
		if !strings.Contains(rendered, `METRICS_ALLOW_UNAUTHENTICATED: "true"`) {
			t.Error("the gateway was not configured to serve /metrics unauthenticated")
		}
	})
}

// TestExistingSecretSuppressesChartSecret: the point of the hook is that no
// plaintext credential has to live in values.yaml.
func TestExistingSecretSuppressesChartSecret(t *testing.T) {
	rendered := helmTemplate(t, "--set", "microgateway.secrets.existingSecret=external-creds")

	if strings.Contains(rendered, "test-microgateway-secrets") {
		t.Error("the chart still creates its own microgateway Secret despite existingSecret being set")
	}
	if !strings.Contains(rendered, "name: external-creds") {
		t.Error("the external secret is not referenced by envFrom")
	}
}

// TestAutoscalingOmitsReplicas: leaving a static replicas field alongside an HPA
// makes the two fight over the replica count.
func TestAutoscalingOmitsReplicas(t *testing.T) {
	rendered := helmTemplate(t, "--set", "microgateway.autoscaling.enabled=true")
	workload := microgatewayWorkload(t, rendered)

	if _, present := workload["spec"].(map[string]any)["replicas"]; present {
		t.Error("spec.replicas is set while an HPA is enabled; they would fight")
	}
	if !strings.Contains(rendered, "kind: HorizontalPodAutoscaler") {
		t.Error("no HPA was rendered")
	}
}

// TestPersistenceOnlyOnStatefulSet: volumeClaimTemplates are a StatefulSet-only
// field, so a Deployment must always fall back to an emptyDir rather than
// referencing a volume that does not exist.
func TestPersistenceOnlyOnStatefulSet(t *testing.T) {
	t.Run("statefulset gets a volume claim template", func(t *testing.T) {
		rendered := helmTemplate(t,
			"--set", "microgateway.kind=StatefulSet",
			"--set", "microgateway.persistence.enabled=true")
		workload := microgatewayWorkload(t, rendered)
		if workload["kind"] != "StatefulSet" {
			t.Fatalf("kind = %v, want StatefulSet", workload["kind"])
		}
		if workload["spec"].(map[string]any)["volumeClaimTemplates"] == nil {
			t.Error("no volumeClaimTemplates on the StatefulSet")
		}
	})

	t.Run("deployment with persistence still has a data volume", func(t *testing.T) {
		rendered := helmTemplate(t, "--set", "microgateway.persistence.enabled=true")
		workload := microgatewayWorkload(t, rendered)
		volumes, _ := dig(t, workload, "spec", "template", "spec", "volumes").([]any)

		var names []string
		for _, v := range volumes {
			vol, _ := v.(map[string]any)
			names = append(names, vol["name"].(string))
		}
		// Every mount must have a backing volume or the pod will not start.
		for _, required := range []string{"data", "tmp", "plugins", "analytics-config"} {
			var found bool
			for _, n := range names {
				if n == required {
					found = true
				}
			}
			if !found {
				t.Errorf("volume %q is mounted but not defined; volumes present: %v", required, names)
			}
		}
	})
}

// TestReadOnlyRootHasWritableTmp: readOnlyRootFilesystem is on by default, so
// anything writing outside a mounted volume — SQLite's temp files included —
// needs /tmp to be writable.
func TestReadOnlyRootHasWritableTmp(t *testing.T) {
	workload := microgatewayWorkload(t, helmTemplate(t))
	containers, _ := dig(t, workload, "spec", "template", "spec", "containers").([]any)
	container, _ := containers[0].(map[string]any)

	readOnly := dig(t, container, "securityContext", "readOnlyRootFilesystem")
	if readOnly != true {
		t.Fatalf("readOnlyRootFilesystem = %v; this test guards the writable-/tmp pairing", readOnly)
	}

	mounts, _ := container["volumeMounts"].([]any)
	var hasTmp bool
	for _, m := range mounts {
		mount, _ := m.(map[string]any)
		if mount["mountPath"] == "/tmp" {
			hasTmp = true
		}
	}
	if !hasTmp {
		t.Error("readOnlyRootFilesystem is set but /tmp is not writable; SQLite temp files would fail")
	}
}
