package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/v2/models"
	sr "github.com/TykTechnologies/midsommar/v2/pkg/semanticrouting"
	"github.com/TykTechnologies/midsommar/v2/pkg/semanticrouting/llmclient"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// SemanticRouterService compiles the Semantic Routers synced from the hub
// with the Enterprise engine and classifies requests for them. A router whose
// configuration has not changed keeps its compiled form across reloads, so
// its example vectors (embedded in the background) and its session affinity
// survive a configuration sync.
//
// In a build without the engine (Community Edition) no router loads, and a
// router slug is an unknown route like any other.
type SemanticRouterService struct {
	db *gorm.DB

	mu      sync.RWMutex
	engine  sr.Engine
	routers map[string]*compiledSemanticRouter // slug -> router

	lookupMu sync.RWMutex
	lookup   llmclient.LLMLookup
	decrypt  llmclient.Decrypter
}

type compiledSemanticRouter struct {
	ID         uint
	Slug       string
	configJSON string
	Config     sr.Config
	Router     sr.Router
}

// NewSemanticRouterService returns an empty service; LoadRouters fills it.
func NewSemanticRouterService(db *gorm.DB) *SemanticRouterService {
	return &SemanticRouterService{db: db, routers: map[string]*compiledSemanticRouter{}}
}

// SetLLMLookup tells the service how to reach the gateway's LLMs (for the
// embedding and judge calls). Until it is set those calls fail, and routers
// fall back as they would for any classifier error (and retry embedding their
// examples later).
func (s *SemanticRouterService) SetLLMLookup(lookup llmclient.LLMLookup) {
	s.lookupMu.Lock()
	defer s.lookupMu.Unlock()
	s.lookup = lookup
}

// SetDecrypter tells the service how to decrypt the key of a standalone
// embedder, which the hub sends inline in the router's configuration.
func (s *SemanticRouterService) SetDecrypter(d llmclient.Decrypter) {
	s.lookupMu.Lock()
	defer s.lookupMu.Unlock()
	s.decrypt = d
}

func (s *SemanticRouterService) decryptKey(ciphertext string) (string, error) {
	s.lookupMu.RLock()
	d := s.decrypt
	s.lookupMu.RUnlock()
	if d == nil {
		return "", errors.New("embedder keys cannot be decrypted yet")
	}
	return d(ciphertext)
}

func (s *SemanticRouterService) llm(id uint) (*models.LLM, error) {
	s.lookupMu.RLock()
	lookup := s.lookup
	s.lookupMu.RUnlock()
	if lookup == nil {
		return nil, errors.New("LLMs are not available to the semantic router yet")
	}
	return lookup(id)
}

func (s *SemanticRouterService) engineLocked() (sr.Engine, error) {
	if s.engine != nil {
		return s.engine, nil
	}
	e, err := sr.NewEngine(llmclient.New(s.llm, llmclient.WithDecrypter(s.decryptKey)).Deps())
	if err != nil {
		return nil, err
	}
	s.engine = e
	return e, nil
}

// LoadRouters (re)loads the active routers of the namespace and the global
// ones. Unchanged routers keep their compiled form.
func (s *SemanticRouterService) LoadRouters(namespace string) error {
	var rows []database.SemanticRouter
	q := s.db.Where("is_active = ?", true)
	if namespace != "" {
		q = q.Where("(namespace = ? OR namespace = '')", namespace)
	}
	if err := q.Order("id ASC").Find(&rows).Error; err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if len(rows) > 0 && !sr.Available() {
		log.Warn().Int("count", len(rows)).Msg("Semantic Routers are configured but this gateway build has no semantic routing engine (Enterprise); they are not served")
		s.routers = map[string]*compiledSemanticRouter{}
		return nil
	}
	next := make(map[string]*compiledSemanticRouter, len(rows))
	for _, row := range rows {
		if prev, ok := s.routers[row.Slug]; ok && prev.ID == row.ID && prev.configJSON == row.ConfigJSON {
			next[row.Slug] = prev
			continue
		}
		compiled, err := s.compileLocked(row)
		if err != nil {
			log.Error().Err(err).Str("router", row.Slug).Msg("Failed to compile Semantic Router; not served")
			continue
		}
		next[row.Slug] = compiled
	}
	s.routers = next
	log.Info().Int("count", len(next)).Str("namespace", namespace).Msg("Semantic routers loaded")
	return nil
}

func (s *SemanticRouterService) compileLocked(row database.SemanticRouter) (*compiledSemanticRouter, error) {
	var cfg sr.Config
	if err := json.Unmarshal([]byte(row.ConfigJSON), &cfg); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	cfg.RouterID = row.ID
	cfg.Slug = row.Slug
	engine, err := s.engineLocked()
	if err != nil {
		return nil, err
	}
	router, err := engine.Compile(cfg)
	if err != nil {
		return nil, err
	}
	return &compiledSemanticRouter{ID: row.ID, Slug: row.Slug, configJSON: row.ConfigJSON, Config: cfg, Router: router}, nil
}

// GetRouter returns the compiled router for slug.
func (s *SemanticRouterService) GetRouter(slug string) (*compiledSemanticRouter, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.routers[slug]
	return r, ok
}

// GetRouterCount is the number of routers served.
func (s *SemanticRouterService) GetRouterCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.routers)
}

// Classify runs the router's classifier.
func (s *SemanticRouterService) Classify(ctx context.Context, r *compiledSemanticRouter, req sr.Request) sr.Decision {
	return r.Router.Classify(ctx, req)
}
