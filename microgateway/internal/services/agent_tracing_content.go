package services

import (
	"encoding/json"
	"strconv"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// installAgentTracingContent configures the edge's content pipeline from an
// applied snapshot (spec §5.5): the hub's global content settings, with App
// and resource overrides read from the synced configuration. Content is
// redacted here, before it enters the pulse. Without the hub's settings
// (tracing off there), or in a Community build where no pipeline is
// registered, no content is captured: raw parts are dropped.
func installAgentTracingContent(p *pb.AgentTracingContentPolicy, db *gorm.DB) {
	// The hub's OTel exporter exports the assembled span under the gateway
	// span id; the edge then leaves out its own live span for captured
	// requests, as the hub does.
	agenttrace.SetExportActive(p != nil && p.AssembledExportActive)
	// The hub sends the policy only while its own tracing runs; without it
	// the edge captures nothing (it would be discarded at the hub).
	agenttrace.SetHubTracing(p != nil)
	if p == nil {
		agenttrace.SetContentProcessor(nil)
		return
	}
	g := agenttrace.ContentGlobals{
		Ceiling:            p.Ceiling, // "" from an older hub resolves to off
		DefaultMode:        p.DefaultMode,
		MaxRequestOverride: p.MaxRequestOverride,
		StoreCap:           p.StoreCap,
		MaxPartBytes:       int(p.MaxPartBytes),
		MaxSpanBytes:       int(p.MaxSpanBytes),
		StoreBinary:        p.StoreBinary,
		Detectors:          append([]string(nil), p.RedactionDetectors...),
	}
	agenttrace.SetContentProcessor(agenttrace.NewContentProcessor(g, newEdgeContentOverrides(db).Lookup))
}

// appOff is a span whose App cannot be read, is unknown or has tracing off:
// no content, and no request header may raise it (jyx.172).
var appOff = agenttrace.ContentOverrides{AppMode: agenttrace.ContentOff, AppMaxRequestOverride: agenttrace.ContentOff}

// edgeOverridesTTL bounds how stale a cached setting can be; a sync installs
// a fresh cache anyway.
const edgeOverridesTTL = 30 * time.Second

// edgeContentOverrides reads App and resource content settings from the
// edge database, caching each for edgeOverridesTTL. A row that cannot be
// read resolves to off.
type edgeContentOverrides struct {
	db  *gorm.DB
	now func() time.Time

	mu    sync.Mutex
	cache map[string]edgeCached
}

type edgeCached struct {
	o  agenttrace.ContentOverrides
	at time.Time
}

func newEdgeContentOverrides(db *gorm.DB) *edgeContentOverrides {
	return &edgeContentOverrides{db: db, now: time.Now, cache: map[string]edgeCached{}}
}

// Lookup implements agenttrace.ContentOverridesFunc.
func (e *edgeContentOverrides) Lookup(appID uint, res agenttrace.Resource) agenttrace.ContentOverrides {
	o := e.app(appID)
	o.ResourceMax = e.resource(res)
	return o
}

func (e *edgeContentOverrides) get(key string, load func() agenttrace.ContentOverrides) agenttrace.ContentOverrides {
	e.mu.Lock()
	if c, ok := e.cache[key]; ok && e.now().Sub(c.at) <= edgeOverridesTTL {
		e.mu.Unlock()
		return c.o
	}
	e.mu.Unlock()
	o := load()
	e.mu.Lock()
	if len(e.cache) > 50000 {
		e.cache = map[string]edgeCached{}
	}
	e.cache[key] = edgeCached{o: o, at: e.now()}
	e.mu.Unlock()
	return o
}

func (e *edgeContentOverrides) app(id uint) agenttrace.ContentOverrides {
	if id == 0 || e.db == nil {
		return appOff
	}
	return e.get("app:"+strconv.FormatUint(uint64(id), 10), func() agenttrace.ContentOverrides {
		var app database.App
		if err := e.db.Select("id", "tracing").First(&app, id).Error; err != nil {
			return appOff
		}
		var t struct {
			Enabled            *bool  `json:"enabled"`
			ContentMode        string `json:"content_mode"`
			MaxRequestOverride string `json:"max_request_override"`
		}
		if len(app.Tracing) > 0 && string(app.Tracing) != "null" {
			if err := json.Unmarshal(app.Tracing, &t); err != nil {
				return appOff
			}
		}
		if t.Enabled != nil && !*t.Enabled {
			return appOff
		}
		return agenttrace.ContentOverrides{AppMode: t.ContentMode, AppMaxRequestOverride: t.MaxRequestOverride}
	})
}

func (e *edgeContentOverrides) resource(res agenttrace.Resource) string {
	var model any
	switch res.Type {
	case agenttrace.ResourceLLM:
		model = &database.LLM{}
	case agenttrace.ResourceTool:
		model = &database.Tool{}
	case agenttrace.ResourceDatasource:
		model = &database.Datasource{}
	default:
		return ""
	}
	id, err := strconv.ParseUint(res.ID, 10, 64)
	if err != nil || e.db == nil {
		return agenttrace.ContentOff
	}
	return e.get(res.Type+":"+res.ID, func() agenttrace.ContentOverrides {
		var row struct{ TracingContentModeMax string }
		if err := e.db.Model(model).Select("tracing_content_mode_max").Where("id = ?", id).Take(&row).Error; err != nil {
			return agenttrace.ContentOverrides{ResourceMax: agenttrace.ContentOff}
		}
		return agenttrace.ContentOverrides{ResourceMax: row.TracingContentModeMax}
	}).ResourceMax
}
