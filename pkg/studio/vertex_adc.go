package studio

import (
	"context"
	"time"

	"github.com/TykTechnologies/midsommar/v2/logger"
	vertexVendor "github.com/TykTechnologies/midsommar/v2/vendors/vertex"
)

// logVertexADCState says at startup whether key-less Vertex LLMs may use the
// host's Google credentials (AI_STUDIO_VERTEX_ADC) and whether those
// resolve. It may wait on the GCP metadata server, so it runs in the
// background. Nothing logged is credential material.
func logVertexADCState() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	st := vertexVendor.CheckADC(ctx)
	switch {
	case !st.Enabled:
		logger.Debugf("Vertex host credentials (%s): disabled; key-less Vertex LLMs are refused on gateway routes", vertexVendor.ADCSetting)
	case st.Resolved:
		logger.Infof("Vertex host credentials (%s): enabled, resolved (%s)", vertexVendor.ADCSetting, st.Source)
	default:
		logger.Warnf("Vertex host credentials (%s): enabled, not resolved: %s", vertexVendor.ADCSetting, st.Error)
	}
}
