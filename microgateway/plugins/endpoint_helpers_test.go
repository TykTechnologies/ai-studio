package plugins

import (
	"testing"
)

func TestParsePluginEndpointPath(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantSlug   string
		wantSub    string
		wantOK     bool
	}{
		{
			name:     "basic path with sub-path",
			input:    "/plugins/my-echo/hello",
			wantSlug: "my-echo",
			wantSub:  "/hello",
			wantOK:   true,
		},
		{
			name:     "nested sub-path",
			input:    "/plugins/my-oauth/users/123/profile",
			wantSlug: "my-oauth",
			wantSub:  "/users/123/profile",
			wantOK:   true,
		},
		{
			name:     "slug only, no trailing slash",
			input:    "/plugins/my-echo",
			wantSlug: "my-echo",
			wantSub:  "/",
			wantOK:   true,
		},
		{
			name:     "slug with trailing slash",
			input:    "/plugins/my-echo/",
			wantSlug: "my-echo",
			wantSub:  "/",
			wantOK:   true,
		},
		{
			name:     "well-known path",
			input:    "/plugins/my-oauth/.well-known/openid-configuration",
			wantSlug: "my-oauth",
			wantSub:  "/.well-known/openid-configuration",
			wantOK:   true,
		},
		{
			name:   "empty after plugins prefix",
			input:  "/plugins/",
			wantOK: false,
		},
		{
			name:   "no plugins prefix",
			input:  "/llm/rest/gpt-4/chat",
			wantOK: false,
		},
		{
			name:   "just /plugins",
			input:  "/plugins",
			wantOK: false,
		},
		{
			name:   "empty string",
			input:  "",
			wantOK: false,
		},
		{
			name:   "root path",
			input:  "/",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slug, sub, ok := ParsePluginEndpointPath(tt.input)
			if ok != tt.wantOK {
				t.Errorf("ok = %v, want %v", ok, tt.wantOK)
				return
			}
			if !tt.wantOK {
				return
			}
			if slug != tt.wantSlug {
				t.Errorf("slug = %q, want %q", slug, tt.wantSlug)
			}
			if sub != tt.wantSub {
				t.Errorf("sub = %q, want %q", sub, tt.wantSub)
			}
		})
	}
}

func TestSplitPathSegments(t *testing.T) {
	tests := []struct {
		name string
		path string
		want []string
	}{
		{
			name: "multi-segment",
			path: "/users/123/profile",
			want: []string{"users", "123", "profile"},
		},
		{
			name: "single segment",
			path: "/hello",
			want: []string{"hello"},
		},
		{
			name: "root",
			path: "/",
			want: nil,
		},
		{
			name: "empty",
			path: "",
			want: nil,
		},
		{
			name: "well-known",
			path: "/.well-known/openid-configuration",
			want: []string{".well-known", "openid-configuration"},
		},
		{
			name: "no leading slash",
			path: "foo/bar",
			want: []string{"foo", "bar"},
		},
		{
			name: "trailing slash",
			path: "/users/123/",
			want: []string{"users", "123"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SplitPathSegments(tt.path)
			if tt.want == nil && got != nil {
				t.Errorf("got %v, want nil", got)
				return
			}
			if len(got) != len(tt.want) {
				t.Errorf("got %v (len %d), want %v (len %d)", got, len(got), tt.want, len(tt.want))
				return
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("segment[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestIsValidEndpointPath(t *testing.T) {
	tests := []struct {
		path  string
		valid bool
	}{
		{"/*", true},
		{"/hello", true},
		{"/users/123", true},
		{"/.well-known/openid-configuration", true},
		{"/a/b/c/d/e", true},
		{"", false},
		{"hello", false},             // no leading slash
		{"/../etc/passwd", false},     // directory traversal
		{"/foo/../bar", false},        // directory traversal mid-path
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := isValidEndpointPath(tt.path); got != tt.valid {
				t.Errorf("isValidEndpointPath(%q) = %v, want %v", tt.path, got, tt.valid)
			}
		})
	}
}

func TestValidateHTTPMethods(t *testing.T) {
	tests := []struct {
		name    string
		input   []string
		wantLen int
	}{
		{"all valid", []string{"GET", "POST", "PUT"}, 3},
		{"lowercase normalised", []string{"get", "post"}, 2},
		{"mixed case", []string{"Get", "pOsT", "DELETE"}, 3},
		{"with invalid", []string{"GET", "INVALID", "POST"}, 2},
		{"all invalid", []string{"INVALID", "NOPE"}, 0},
		{"empty", []string{}, 0},
		{"nil", nil, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateHTTPMethods(tt.input)
			if len(got) != tt.wantLen {
				t.Errorf("got %v (len %d), want len %d", got, len(got), tt.wantLen)
			}
		})
	}
}

func TestEndpointRouteKeyAndLookup(t *testing.T) {
	pm := NewPluginManager(nil)

	// Simulate registering routes
	route := &EndpointRoute{
		PluginID:       1,
		PluginName:     "Test Plugin",
		PluginSlug:     "test-plugin",
		Path:           "/*",
		Methods:        []string{"GET", "POST"},
		RequireAuth:    false,
		StreamResponse: false,
	}

	// Register wildcard route for GET and POST
	pm.endpointRoutes[endpointRouteKey("GET", "test-plugin", "/*")] = route
	pm.endpointRoutes[endpointRouteKey("POST", "test-plugin", "/*")] = route
	pm.pluginEndpoints[1] = []*EndpointRoute{route}

	// Test: wildcard match should work
	found := pm.GetEndpointRoute("GET", "test-plugin", "/hello")
	if found == nil {
		t.Fatal("expected to find route for GET /hello via wildcard, got nil")
	}
	if found.PluginSlug != "test-plugin" {
		t.Errorf("PluginSlug = %q, want %q", found.PluginSlug, "test-plugin")
	}

	// Test: POST should also match
	found = pm.GetEndpointRoute("POST", "test-plugin", "/any/path")
	if found == nil {
		t.Fatal("expected to find route for POST /any/path via wildcard, got nil")
	}

	// Test: DELETE should NOT match (not registered)
	found = pm.GetEndpointRoute("DELETE", "test-plugin", "/hello")
	if found != nil {
		t.Error("expected nil for DELETE (not registered), got route")
	}

	// Test: wrong slug should NOT match
	found = pm.GetEndpointRoute("GET", "wrong-slug", "/hello")
	if found != nil {
		t.Error("expected nil for wrong slug, got route")
	}

	// Test: exact match takes precedence over wildcard
	exactRoute := &EndpointRoute{
		PluginID:   1,
		PluginSlug: "test-plugin",
		Path:       "/specific",
		Methods:    []string{"GET"},
	}
	pm.endpointRoutes[endpointRouteKey("GET", "test-plugin", "/specific")] = exactRoute

	found = pm.GetEndpointRoute("GET", "test-plugin", "/specific")
	if found == nil {
		t.Fatal("expected to find exact route, got nil")
	}
	if found.Path != "/specific" {
		t.Errorf("Path = %q, want %q (exact match should take precedence)", found.Path, "/specific")
	}

	// Test: unregister removes all routes
	pm.mu.Lock()
	pm.unregisterPluginEndpoints(1)
	pm.mu.Unlock()

	found = pm.GetEndpointRoute("GET", "test-plugin", "/hello")
	if found != nil {
		t.Error("expected nil after unregister, got route")
	}
}
