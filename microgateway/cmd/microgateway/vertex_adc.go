package main

import (
	"context"
	"time"

	vertexVendor "github.com/TykTechnologies/midsommar/v2/vendors/vertex"
	"github.com/rs/zerolog/log"
)

// logVertexADCState says at startup whether key-less Vertex LLMs may use
// this edge's own Google credentials (AI_STUDIO_VERTEX_ADC, the same setting
// AI Studio reads) and whether those resolve. Each edge needs its own GCP
// identity. It may wait on the GCP metadata server, so it runs in the
// background. Nothing logged is credential material.
func logVertexADCState() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	st := vertexVendor.CheckADC(ctx)
	ev := log.Info()
	if st.Enabled && !st.Resolved {
		ev = log.Warn()
	} else if !st.Enabled {
		ev = log.Debug()
	}
	ev.Str("setting", vertexVendor.ADCSetting).Bool("enabled", st.Enabled).Bool("resolved", st.Resolved).
		Str("source", st.Source).Str("error", st.Error).Msg("Vertex host credentials")
}
