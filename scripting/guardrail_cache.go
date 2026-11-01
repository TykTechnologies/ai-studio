package scripting

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/metrics"
	"github.com/TykTechnologies/midsommar/v2/pkg/lrucache"
)

// The verdict cache remembers a provider's verdict on one request segment,
// so text a client resends in every request (the conversation history, and
// above all the tool results in it) is classified once rather than once per
// turn. It is per process and in memory: each replica and each edge keeps
// its own.
//
// A key covers everything the verdict depends on: the filter version, its
// whole normalised config with connection values resolved (provider,
// detectors, thresholds, endpoint, redaction, budgets), the provider's
// Capabilities.Version and ToolSegments (whether tool results go to it as
// role tool), the direction, whether a rewrite was asked for, the segment's
// role, kind and tool name, and its text. It also covers the
// tenant (the app, else the user, else the chat session), so whether one
// app's text is in the cache cannot be read from another app's latency.
// Positional fields (index, path, tool call id) are left out, so a history
// rescan hits. Request metadata other than the tenant is not in the key: a
// provider whose verdict depends on it (an HTTP classifier reading meta)
// should run with the filter's cache switched off.
//
// Keys are HMAC-SHA256 under a key drawn at start-up, so a key reveals
// nothing about the resolved credentials in the config even if it leaked;
// keys are never logged. Only verdicts of provider calls that succeeded are
// stored; errors, timeouts, fail-mode outcomes and unscanned segments never
// are.

// Environment variables (or AppConf.FilterLimits) that size the cache.
const (
	EnvVerdictCacheEntries  = "GUARDRAIL_VERDICT_CACHE_ENTRIES"
	EnvVerdictCacheMaxBytes = "GUARDRAIL_VERDICT_CACHE_MAX_BYTES"
	// EnvVerdictCacheTTL is how long a verdict is reused; "0" switches the
	// cache off. A bare integer is read as seconds.
	EnvVerdictCacheTTL = "GUARDRAIL_VERDICT_CACHE_TTL"

	defaultVerdictCacheEntries  = 100_000
	defaultVerdictCacheMaxBytes = 64 << 20
	defaultVerdictCacheTTL      = 10 * time.Minute
)

// cachedVerdict is one segment's verdict. Findings carry no index or span.
type cachedVerdict struct {
	findings  []guardrails.Finding
	rewrite   string
	rewritten bool
	// templateHandled: the provider acknowledged running the segment's
	// template backstop in its call (Verdict.TemplatesHandled).
	templateHandled bool
	at              time.Time
}

// weight approximates the memory an entry holds.
func (v cachedVerdict) weight(key string) int64 {
	w := int64(len(key) + len(v.rewrite) + 96)
	for _, f := range v.findings {
		w += int64(len(f.Detector) + len(f.Category) + len(f.Label) + len(f.Severity) + 64)
	}
	return w
}

type verdictCache struct {
	lru *lrucache.Cache[cachedVerdict]
	ttl time.Duration
}

var (
	verdictCacheMu     sync.Mutex
	sharedVerdictCache *verdictCache
)

// currentVerdictCache returns the process's cache, sized from the
// environment on first use, or nil when it is switched off.
func currentVerdictCache() *verdictCache {
	verdictCacheMu.Lock()
	defer verdictCacheMu.Unlock()
	if sharedVerdictCache == nil {
		sharedVerdictCache = newVerdictCache(
			tunableInt(EnvVerdictCacheEntries, defaultVerdictCacheEntries),
			int64(tunableInt(EnvVerdictCacheMaxBytes, defaultVerdictCacheMaxBytes)),
			tunableDuration(EnvVerdictCacheTTL, defaultVerdictCacheTTL),
		)
	}
	if sharedVerdictCache.ttl <= 0 {
		return nil
	}
	return sharedVerdictCache
}

func newVerdictCache(entries int, maxBytes int64, ttl time.Duration) *verdictCache {
	return &verdictCache{
		lru: lrucache.NewWeighted[cachedVerdict](entries, maxBytes, func(key string, v cachedVerdict) int64 { return v.weight(key) }),
		ttl: ttl,
	}
}

// tunableInt is a positive tuning limit (config.FilterLimit), else def.
func tunableInt(name string, def int) int {
	if n := config.FilterLimitInt64(name, 0); n > 0 && n <= math.MaxInt32 {
		return int(n)
	}
	return def
}

// tunableDuration is a non-negative duration tuning limit
// (config.FilterLimit), else def.
func tunableDuration(name string, def time.Duration) time.Duration {
	if d := config.FilterLimitDuration(name, def); d >= 0 {
		return d
	}
	return def
}

// verdictKeyer derives one run's cache keys and reads and writes the cache
// for it. A nil keyer is a run without a cache.
type verdictKeyer struct {
	ctx      context.Context
	cache    *verdictCache
	provider string
	// prefix is the run-wide key material.
	prefix []byte
	// keys holds the key of each segment looked up, by position, so a store
	// does not hash the segment's text again.
	keys map[int]string
}

// newVerdictKeyer returns the keyer for a run, or nil when the run must not
// use the cache: the response side (a streaming buffer changes on every
// evaluation), a filter that is not a saved version (the test endpoint's),
// a contextual provider, a filter with the cache switched off, a request
// with no tenant to scope it to, or a config that cannot be digested.
func newVerdictKeyer(ctx context.Context, p *preparedGuardrail, input *ScriptInput, direction guardrails.Direction, redact bool) *verdictKeyer {
	if direction != guardrails.DirectionRequest || p.caps.Contextual || !p.cfg.CacheEnabled() {
		return nil
	}
	version := p.key
	tenant := tenantScope(input.Context)
	if version == "" || tenant == "" {
		return nil
	}
	digest := p.configDigest()
	cache := currentVerdictCache()
	if cache == nil || digest == nil {
		return nil
	}
	var b bytes.Buffer
	for _, field := range []string{"v1", version, string(digest), p.caps.Version, strconv.FormatBool(p.caps.ToolSegments), string(direction), strconv.FormatBool(redact), tenant} {
		writeField(&b, field)
	}
	if p.caps.WantsGoal {
		// Tool calls are sent with their context only to such providers.
		writeField(&b, "wants_goal")
	}
	return &verdictKeyer{ctx: ctx, cache: cache, provider: p.cfg.Provider, prefix: b.Bytes(), keys: map[int]string{}}
}

// tenantScope names whom the request belongs to: its app, else its user,
// else its chat session. Empty when none is known.
func tenantScope(reqCtx map[string]interface{}) string {
	for _, k := range []string{"app_id", "user_id", "session_id"} {
		v, ok := reqCtx[k]
		if !ok || v == nil {
			continue
		}
		if s := fmt.Sprint(v); s != "" && s != "0" {
			return k + ":" + s
		}
	}
	return ""
}

func (k *verdictKeyer) key(s guardrails.Segment) string {
	mac := guardrails.NewCacheMAC()
	mac.Write(k.prefix)
	for _, field := range []string{s.Role, s.Kind, s.ToolName, s.Text} {
		writeField(mac, field)
	}
	if s.IsToolCall() && s.Context != nil {
		// The same arguments under another goal are another question. Only
		// tool calls carry a context, so other keys are as they were.
		for _, field := range []string{"context", s.Context.UserGoal, s.Context.System} {
			writeField(mac, field)
		}
	}
	if t := s.Template; t != nil {
		// A residual is scored under its template: another template or
		// version, or another backstop, is another question. Only matched
		// messages carry one, so other keys are as they were.
		for _, field := range []string{"template", t.ID, strconv.Itoa(t.Version), t.Backstop, t.FullText} {
			writeField(mac, field)
		}
		if t.BackstopMinScore > 0 {
			// A provider may drop backstop findings below it.
			writeField(mac, "backstop_min_score")
			writeField(mac, strconv.FormatFloat(t.BackstopMinScore, 'g', -1, 64))
		}
	}
	return string(mac.Sum(nil))
}

// writeField writes s length-prefixed, so no two field lists encode alike.
func writeField(w io.Writer, s string) {
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(len(s)))
	w.Write(n[:])
	io.WriteString(w, s)
}

// lookup returns the cached verdict for s, the segment at position i.
func (k *verdictKeyer) lookup(i int, s guardrails.Segment) (cachedVerdict, bool) {
	if k == nil {
		return cachedVerdict{}, false
	}
	key := k.key(s)
	k.keys[i] = key
	v, ok := k.cache.lru.Get(key)
	if ok && time.Since(v.at) > k.cache.ttl {
		k.cache.lru.Remove(key)
		ok = false
	}
	outcome := "miss"
	if ok {
		outcome = "hit"
	}
	metrics.RecordGuardrailCache(k.ctx, k.provider, outcome)
	return v, ok
}

// store records the verdict on s, the segment at position i of a run whose
// outcome is res.
func (k *verdictKeyer) store(s guardrails.Segment, i int, res scanResult) {
	if k == nil {
		return
	}
	v := cachedVerdict{at: time.Now()}
	for _, f := range res.findings {
		if f.SegmentIndex == i {
			f.SegmentIndex = 0
			v.findings = append(v.findings, f)
		}
	}
	v.rewrite, v.rewritten = res.rewritten[i]
	v.templateHandled = res.templatesHandled[i]
	key, ok := k.keys[i]
	if !ok {
		key = k.key(s)
	}
	k.cache.lru.Add(key, v)
}

// configDigest is a digest of the prepared config, connection values
// resolved, computed once per prepared filter; nil when the config cannot
// be encoded, which keeps the filter out of the cache rather than sharing
// one digest among all such configs.
func (p *preparedGuardrail) configDigest() []byte {
	p.digestOnce.Do(func() {
		if b, err := json.Marshal(p.cfg); err == nil {
			p.digest = []byte(guardrails.CacheDigest(b))
		}
	})
	return p.digest
}
