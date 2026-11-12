// tests/helm_bundled_edge_test.go
//
// The microgateway subchart's edge in a default install must reach the AI
// Studio Service of its own release and authenticate to it
// (midsommar-clean-ntg2): the default CONTROL_ENDPOINT used to be
// "<release>:50051" while the Service is "<release>-midsommar", so the
// bundled edge crash-loopped. Skips when helm is missing.
package tests

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helmTemplateRelease renders the umbrella chart as release name.
func helmTemplateRelease(t *testing.T, name string, setArgs ...string) (string, error) {
	t.Helper()
	helm, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm not installed; skipping chart render tests")
	}
	chart, err := filepath.Abs(microgatewayChartPath)
	require.NoError(t, err)
	out, err := exec.Command(helm, append([]string{"template", name, chart}, setArgs...)...).CombinedOutput()
	return string(out), err
}

// studioGRPC is the rendered AI Studio Service's name and its gRPC port.
func studioGRPC(t *testing.T, rendered, release string) string {
	t.Helper()
	for _, name := range []string{release + "-midsommar", release} {
		svc := renderedDoc(t, rendered, "Service", name)
		if svc == nil {
			continue
		}
		ports, _ := dig(t, svc, "spec", "ports").([]any)
		for _, p := range ports {
			pm := p.(map[string]any)
			if pm["name"] == "grpc" {
				return fmt.Sprintf("%s:%v", name, pm["port"])
			}
		}
	}
	t.Fatalf("no AI Studio Service with a grpc port for release %s", release)
	return ""
}

// The default edge connects to the Service the same render creates, on its
// gRPC port, whatever the release is called.
func TestHelmBundledEdgeReachesItsStudio(t *testing.T) {
	for _, release := range []string{"test", "studio", "midsommar-prod", "ai-studio-eu-west-production-cluster-with-a-long-name"} {
		t.Run(release, func(t *testing.T) {
			rendered, err := helmTemplateRelease(t, release)
			require.NoError(t, err, rendered)
			cm := renderedDoc(t, rendered, "ConfigMap", release+"-microgateway-config")
			require.NotNil(t, cm)
			data, _ := cm["data"].(map[string]any)
			endpoint := studioGRPC(t, rendered, release)
			assert.Equal(t, endpoint, data["CONTROL_ENDPOINT"])

			// Studio listens where the Service sends gRPC.
			service, port, _ := strings.Cut(endpoint, ":")
			studioCM := renderedDoc(t, rendered, "ConfigMap", service+"-config")
			require.NotNil(t, studioCM)
			studioData, _ := studioCM["data"].(map[string]any)
			assert.Equal(t, port, studioData["GRPC_PORT"])
		})
	}
}

// An explicit endpoint is used as given, and a render whose default would
// not reach this release's Studio refuses with the value to set.
func TestHelmBundledEdgeEndpointChecks(t *testing.T) {
	rendered, err := helmTemplateRelease(t, "test", "--set", "microgateway.config.controlEndpoint=studio.elsewhere:50051")
	require.NoError(t, err, rendered)
	data, _ := renderedDoc(t, rendered, "ConfigMap", "test-microgateway-config")["data"].(map[string]any)
	assert.Equal(t, "studio.elsewhere:50051", data["CONTROL_ENDPOINT"])

	out, err := helmTemplateRelease(t, "test", "--set", "fullnameOverride=ai")
	require.Error(t, err)
	assert.Contains(t, out, "set microgateway.config.controlEndpoint=ai:50051")

	// ... unless the endpoint is set: then the override is fine.
	_, err = helmTemplateRelease(t, "test", "--set", "fullnameOverride=ai", "--set", "microgateway.config.controlEndpoint=ai:50051")
	require.NoError(t, err)

	out, err = helmTemplateRelease(t, "test", "--set", "config.grpcPort=50052")
	require.Error(t, err)
	assert.Contains(t, out, "AI Studio listens for gRPC on config.grpcPort 50052")
}

// The edge's token and key must be Studio's, and it must use TLS when
// Studio serves gRPC over TLS.
func TestHelmBundledEdgeCredentialsAndTLS(t *testing.T) {
	_, err := helmTemplateRelease(t, "test",
		"--set", "config.grpcAuthToken=tok", "--set", "microgateway.secrets.edgeAuthToken=tok",
		"--set", "config.microgatewayEncryptionKey=0123456789abcdef0123456789abcdef", "--set", "microgateway.secrets.encryptionKey=0123456789abcdef0123456789abcdef")
	require.NoError(t, err, "matching pairs render")

	out, err := helmTemplateRelease(t, "test", "--set", "config.grpcAuthToken=tok", "--set", "microgateway.secrets.edgeAuthToken=other")
	require.Error(t, err)
	assert.Contains(t, out, "microgateway.secrets.edgeAuthToken must equal config.grpcAuthToken")

	out, err = helmTemplateRelease(t, "test", "--set", "config.microgatewayEncryptionKey=a", "--set", "microgateway.secrets.encryptionKey=b")
	require.Error(t, err)
	assert.Contains(t, out, "microgateway.secrets.encryptionKey must equal config.microgatewayEncryptionKey")

	// An existing Secret is not inspected.
	_, err = helmTemplateRelease(t, "test", "--set", "config.grpcAuthToken=tok", "--set", "microgateway.secrets.existingSecret=edge-creds", "--set", "microgateway.secrets.edgeAuthToken=other")
	require.NoError(t, err)

	out, err = helmTemplateRelease(t, "test", "--set-string", "config.grpcTlsInsecure=false")
	require.Error(t, err)
	assert.Contains(t, out, "microgateway.config.tlsEnabled is not true")
	_, err = helmTemplateRelease(t, "test", "--set-string", "config.grpcTlsInsecure=false", "--set-string", "microgateway.config.tlsEnabled=true")
	require.NoError(t, err)

	// The bundled edge off: nothing to check.
	_, err = helmTemplateRelease(t, "test", "--set", "microgateway.enabled=false", "--set", "fullnameOverride=ai")
	require.NoError(t, err)
}
