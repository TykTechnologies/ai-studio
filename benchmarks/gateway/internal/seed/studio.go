// Package seed configures a fresh Studio (hub) and its edge gateway for a
// benchmark run, headlessly, over Studio's REST API: first admin, API key,
// LLMs pointing at the mock and the real vendors, model prices, an app with a
// budget and an active credential, then a config push to the edges and a wait
// until every edge has loaded it.
package seed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Studio is a minimal client for the Studio admin API.
type Studio struct {
	base   string
	http   *http.Client
	apiKey string
	csrf   string
}

// NewStudio returns a client for the Studio API at base (e.g. http://studio:8080).
func NewStudio(base string) *Studio {
	return &Studio{base: strings.TrimSuffix(base, "/"), http: &http.Client{Jar: &hostJar{}, Timeout: 30 * time.Second}}
}

// hostJar is a cookie jar for a client that only ever talks to one Studio. A
// production Studio marks its session cookie Secure; net/http/cookiejar then
// never returns it over plain HTTP, which is how the benchmark network reaches
// Studio. This jar keeps every cookie by name and sends them all.
type hostJar struct {
	mu      sync.Mutex
	cookies map[string]*http.Cookie
}

func (j *hostJar) SetCookies(_ *url.URL, cookies []*http.Cookie) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.cookies == nil {
		j.cookies = map[string]*http.Cookie{}
	}
	for _, c := range cookies {
		j.cookies[c.Name] = &http.Cookie{Name: c.Name, Value: c.Value}
	}
}

func (j *hostJar) Cookies(_ *url.URL) []*http.Cookie {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := make([]*http.Cookie, 0, len(j.cookies))
	for _, c := range j.cookies {
		out = append(out, c)
	}
	return out
}

// UseAPIKey authenticates with an API key saved by an earlier Login.
func (s *Studio) UseAPIKey(key string) { s.apiKey = key }

// Login registers the first user if the database is empty (that user becomes
// an admin), logs in, and rolls an API key. From then on requests carry the
// key, which also exempts them from CSRF.
func (s *Studio) Login(ctx context.Context, email, password string) error {
	if err := s.fetchCSRF(ctx); err != nil {
		return err
	}
	reg := map[string]any{"data": map[string]any{"type": "users", "attributes": map[string]any{
		"email": email, "name": "Benchmark Admin", "password": password, "with_portal": true, "with_chat": true,
	}}}
	status, body, err := s.do(ctx, http.MethodPost, "/auth/register", reg, nil)
	if err != nil {
		return err
	}
	// 400 is expected when the user exists from an earlier seed.
	if status != http.StatusCreated && status != http.StatusBadRequest {
		return fmt.Errorf("register: HTTP %d: %s", status, body)
	}

	login := map[string]any{"data": map[string]any{"type": "login", "attributes": map[string]any{
		"email": email, "password": password,
	}}}
	if status, body, err = s.do(ctx, http.MethodPost, "/auth/login", login, nil); err != nil {
		return err
	} else if status != http.StatusOK {
		return fmt.Errorf("login: HTTP %d: %s (is ALLOW_REGISTRATIONS=true on a fresh database?)", status, body)
	}
	var admin struct {
		IsAdmin bool `json:"is_admin"`
	}
	_ = json.Unmarshal(body, &admin)
	if !admin.IsAdmin {
		return fmt.Errorf("%s is not an admin: seed a fresh Studio database, where the first registered user becomes admin", email)
	}

	var rolled struct {
		Data struct {
			APIKey string `json:"api_key"`
		} `json:"data"`
	}
	if status, body, err = s.do(ctx, http.MethodPost, "/common/me/api-key/roll", nil, &rolled); err != nil {
		return err
	} else if status != http.StatusOK || rolled.Data.APIKey == "" {
		return fmt.Errorf("roll api key: HTTP %d: %s", status, body)
	}
	s.apiKey = rolled.Data.APIKey
	return nil
}

func (s *Studio) fetchCSRF(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.base+"/csrf-token", nil)
	if err != nil {
		return err
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return fmt.Errorf("studio unreachable at %s: %w", s.base, err)
	}
	resp.Body.Close()
	s.csrf = resp.Header.Get("X-CSRF-Token")
	if s.csrf == "" {
		return fmt.Errorf("GET /csrf-token returned no X-CSRF-Token header (HTTP %d)", resp.StatusCode)
	}
	return nil
}

// do sends a JSON request and decodes a JSON response into out (if non-nil).
func (s *Studio) do(ctx context.Context, method, path string, in, out any) (int, []byte, error) {
	var rd io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return 0, nil, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, s.base+path, rd)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if s.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.apiKey)
	} else if s.csrf != "" {
		req.Header.Set("X-CSRF-Token", s.csrf)
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if out != nil && resp.StatusCode < 300 && len(body) > 0 {
		if err := json.Unmarshal(body, out); err != nil {
			return resp.StatusCode, body, fmt.Errorf("%s %s: decoding response: %w", method, path, err)
		}
	}
	return resp.StatusCode, body, nil
}

func (s *Studio) mustOK(ctx context.Context, method, path string, in, out any, want ...int) error {
	status, body, err := s.do(ctx, method, path, in, out)
	if err != nil {
		return err
	}
	for _, w := range want {
		if status == w {
			return nil
		}
	}
	return fmt.Errorf("%s %s: HTTP %d: %s", method, path, status, truncate(body))
}

// jsonAPI is the Studio JSON:API envelope for one resource.
type jsonAPI[T any] struct {
	Data struct {
		ID         string `json:"id"`
		Attributes T      `json:"attributes"`
	} `json:"data"`
}

type jsonAPIList[T any] struct {
	Data []struct {
		ID         string `json:"id"`
		Attributes T      `json:"attributes"`
	} `json:"data"`
}

// LLMSpec is one LLM row to create.
type LLMSpec struct {
	Name         string // the route slug is derived from it (slug.Make)
	Vendor       string // openai | anthropic
	Endpoint     string
	APIKey       string
	DefaultModel string
}

type llmAttrs struct {
	Name string `json:"name"`
}

// EnsureLLM creates the LLM, or updates it in place when one with the same
// name exists, and returns its id.
func (s *Studio) EnsureLLM(ctx context.Context, l LLMSpec) (int, error) {
	attrs := map[string]any{
		"name": l.Name, "vendor": l.Vendor, "api_endpoint": l.Endpoint, "api_key": l.APIKey,
		"default_model": l.DefaultModel, "active": true, "privacy_score": 50,
	}
	var list jsonAPIList[llmAttrs]
	if err := s.mustOK(ctx, http.MethodGet, "/api/v1/llms?page_size=100&all=true", nil, &list, http.StatusOK); err != nil {
		return 0, err
	}
	for _, d := range list.Data {
		if d.Attributes.Name == l.Name {
			body := map[string]any{"data": map[string]any{"type": "llms", "id": d.ID, "attributes": attrs}}
			if err := s.mustOK(ctx, http.MethodPatch, "/api/v1/llms/"+d.ID, body, nil, http.StatusOK); err != nil {
				return 0, err
			}
			return strconv.Atoi(d.ID)
		}
	}
	var created jsonAPI[llmAttrs]
	body := map[string]any{"data": map[string]any{"type": "llms", "attributes": attrs}}
	if err := s.mustOK(ctx, http.MethodPost, "/api/v1/llms", body, &created, http.StatusCreated); err != nil {
		return 0, err
	}
	return strconv.Atoi(created.Data.ID)
}

// EnsurePrice creates a model price; an existing one for the same model and
// vendor is left as it is.
func (s *Studio) EnsurePrice(ctx context.Context, model, vendor string, outPerToken, inPerToken float64) error {
	body := map[string]any{"data": map[string]any{"type": "model-prices", "attributes": map[string]any{
		"model_name": model, "vendor": vendor, "cpt": outPerToken, "cpit": inPerToken, "currency": "USD",
	}}}
	status, resp, err := s.do(ctx, http.MethodPost, "/api/v1/model-prices", body, nil)
	if err != nil {
		return err
	}
	if status == http.StatusCreated || status == http.StatusConflict ||
		(status >= 400 && strings.Contains(strings.ToLower(string(resp)), "exist")) ||
		(status >= 400 && strings.Contains(strings.ToLower(string(resp)), "unique")) {
		return nil
	}
	return fmt.Errorf("create model price %s/%s: HTTP %d: %s", vendor, model, status, truncate(resp))
}

type appAttrs struct {
	Name         string `json:"name"`
	CredentialID int    `json:"credential_id"`
}

// App is the seeded benchmark app and its gateway credential.
type App struct {
	ID     int
	Secret string
}

// EnsureApp creates (or updates) the benchmark app with access to llmIDs and a
// budget large enough never to block, activates its credential and returns the
// secret clients present as a Bearer token.
func (s *Studio) EnsureApp(ctx context.Context, name string, userID int, llmIDs []int) (App, error) {
	now := time.Now().UTC()
	attrs := map[string]any{
		"name": name, "description": "Gateway latency benchmark", "user_id": userID, "llm_ids": llmIDs,
		"monthly_budget":    1_000_000.0,
		"budget_start_date": time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"is_active":         true,
	}
	var list jsonAPIList[appAttrs]
	if err := s.mustOK(ctx, http.MethodGet, "/api/v1/apps?page_size=100&all=true", nil, &list, http.StatusOK); err != nil {
		return App{}, err
	}
	var id string
	var credID int
	for _, d := range list.Data {
		if d.Attributes.Name == name {
			id, credID = d.ID, d.Attributes.CredentialID
			body := map[string]any{"data": map[string]any{"type": "apps", "id": id, "attributes": attrs}}
			if err := s.mustOK(ctx, http.MethodPatch, "/api/v1/apps/"+id, body, nil, http.StatusOK); err != nil {
				return App{}, err
			}
		}
	}
	if id == "" {
		var created jsonAPI[appAttrs]
		body := map[string]any{"data": map[string]any{"type": "apps", "attributes": attrs}}
		if err := s.mustOK(ctx, http.MethodPost, "/api/v1/apps", body, &created, http.StatusCreated); err != nil {
			return App{}, err
		}
		id, credID = created.Data.ID, created.Data.Attributes.CredentialID
	}
	if err := s.mustOK(ctx, http.MethodPost, "/api/v1/apps/"+id+"/activate-credential", nil, nil,
		http.StatusNoContent, http.StatusOK); err != nil {
		return App{}, err
	}
	var cred jsonAPI[struct {
		Secret string `json:"secret"`
		Active bool   `json:"active"`
	}]
	if err := s.mustOK(ctx, http.MethodGet, "/api/v1/credentials/"+strconv.Itoa(credID), nil, &cred, http.StatusOK); err != nil {
		return App{}, err
	}
	if cred.Data.Attributes.Secret == "" || !cred.Data.Attributes.Active {
		return App{}, fmt.Errorf("credential %d for app %s is not active or has no secret", credID, id)
	}
	appID, _ := strconv.Atoi(id)
	return App{ID: appID, Secret: cred.Data.Attributes.Secret}, nil
}

// MyUserID returns the id of the logged-in user.
func (s *Studio) MyUserID(ctx context.Context) (int, error) {
	// /common/me answers with the resource at the top level, not in a
	// {data} envelope; accept either.
	var me struct {
		ID   string `json:"id"`
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := s.mustOK(ctx, http.MethodGet, "/common/me", nil, &me, http.StatusOK); err != nil {
		return 0, err
	}
	if me.ID == "" {
		me.ID = me.Data.ID
	}
	return strconv.Atoi(me.ID)
}

// SyncStatus is the namespace view from /api/v1/sync/status/{ns}.
type SyncStatus struct {
	Namespace struct {
		ExpectedChecksum string `json:"expected_checksum"`
		SyncedCount      int    `json:"synced_count"`
		TotalEdges       int    `json:"total_edges"`
	} `json:"namespace"`
}

// PushAndWaitForEdges pushes config to the namespace and waits until every
// registered edge reports the expected checksum. Studio recomputes sync state
// on each edge heartbeat, so keep EDGE_HEARTBEAT_INTERVAL short.
func (s *Studio) PushAndWaitForEdges(ctx context.Context, namespace string, minEdges int, timeout time.Duration) (SyncStatus, error) {
	deadline := time.Now().Add(timeout)
	var st SyncStatus
	for {
		if err := s.mustOK(ctx, http.MethodGet, "/api/v1/sync/status/"+namespace, nil, &st, http.StatusOK); err != nil {
			return st, err
		}
		if st.Namespace.TotalEdges >= minEdges {
			break
		}
		if time.Now().After(deadline) {
			return st, fmt.Errorf("only %d edge(s) registered in namespace %q after %v, want %d", st.Namespace.TotalEdges, namespace, timeout, minEdges)
		}
		time.Sleep(2 * time.Second)
	}
	var op jsonAPI[struct {
		OperationID string `json:"operation_id"`
	}]
	if err := s.mustOK(ctx, http.MethodPost, "/api/v1/namespaces/"+namespace+"/reload", map[string]any{}, &op,
		http.StatusAccepted, http.StatusOK); err != nil {
		return st, err
	}
	// The sync status alone is not enough: it has been seen reporting every
	// edge in sync right after a reload the edge failed to apply. Wait for the
	// reload operation itself to finish (Enterprise; CE has no endpoint).
	if id := op.Data.Attributes.OperationID; id != "" {
		for {
			var status jsonAPI[struct {
				Status  string `json:"status"`
				Message string `json:"message"`
			}]
			code, body, err := s.do(ctx, http.MethodGet, "/api/v1/reload-operations/"+id+"/status", nil, &status)
			if err != nil {
				return st, err
			}
			if code != http.StatusOK {
				if code == http.StatusPaymentRequired || code == http.StatusNotFound {
					break
				}
				return st, fmt.Errorf("reload operation %s: HTTP %d: %s", id, code, truncate(body))
			}
			switch status.Data.Attributes.Status {
			case "completed":
			case "failed", "timed_out":
				return st, fmt.Errorf("edge config reload %s: %s: %s (check the edge's logs)", id,
					status.Data.Attributes.Status, status.Data.Attributes.Message)
			default:
				if time.Now().After(deadline) {
					return st, fmt.Errorf("edge config reload %s still %q after %v", id, status.Data.Attributes.Status, timeout)
				}
				time.Sleep(time.Second)
				continue
			}
			break
		}
	}
	for {
		if err := s.mustOK(ctx, http.MethodGet, "/api/v1/sync/status/"+namespace, nil, &st, http.StatusOK); err != nil {
			return st, err
		}
		if st.Namespace.TotalEdges > 0 && st.Namespace.SyncedCount == st.Namespace.TotalEdges {
			return st, nil
		}
		if time.Now().After(deadline) {
			return st, fmt.Errorf("edges not in sync after %v: %d/%d", timeout, st.Namespace.SyncedCount, st.Namespace.TotalEdges)
		}
		time.Sleep(2 * time.Second)
	}
}

// ProxyLogCount returns how many proxy log rows Studio holds for the app
// between from and to (dates, inclusive).
func (s *Studio) ProxyLogCount(ctx context.Context, appID int, from, to time.Time) (int, error) {
	var out struct {
		Meta struct {
			TotalCount int `json:"total_count"`
		} `json:"meta"`
	}
	path := fmt.Sprintf("/api/v1/analytics/proxy-logs-for-app?app_id=%d&start_date=%s&end_date=%s&page_size=1",
		appID, from.UTC().Format("2006-01-02"), to.UTC().Format("2006-01-02"))
	if err := s.mustOK(ctx, http.MethodGet, path, nil, &out, http.StatusOK); err != nil {
		return 0, err
	}
	return out.Meta.TotalCount, nil
}

// BudgetUsage returns the app's current-period spend as Studio reports it.
func (s *Studio) BudgetUsage(ctx context.Context, appID int) (float64, error) {
	var out struct {
		CurrentUsage float64 `json:"current_usage"`
	}
	path := fmt.Sprintf("/api/v1/analytics/budget-usage-for-app?app_id=%d", appID)
	if err := s.mustOK(ctx, http.MethodGet, path, nil, &out, http.StatusOK); err != nil {
		return 0, err
	}
	return out.CurrentUsage, nil
}

func truncate(b []byte) string {
	if len(b) > 300 {
		return string(b[:300]) + "..."
	}
	return string(b)
}
