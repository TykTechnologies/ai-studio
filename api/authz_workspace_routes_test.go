package api

import (
	"bufio"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// workspaceGlobalOnlyFile lists the routes whose permission is on a
// Workspace resource but which are NOT Workspace-checked: the route gate
// requires the permission globally there, so a caller whose grant is scoped
// to a Workspace is refused (fail closed, .37). A route leaves the list when
// its handler applies the object checks of authz_workspace.go and it is
// registered with permRouter.InWorkspace (the Workspaces B4 beads). A new
// route on a Workspace resource must either be Workspace-checked or be
// added here: either way it is a conscious decision.
//
// The file is the union over both editions (enterprise-only routes are
// listed too). Regenerate with UPDATE_WORKSPACE_ROUTES=1 go test ./api -run
// TestAuthzRoutes_WorkspaceClassification, in each edition: entries for
// routes the running edition does not register are kept.
const workspaceGlobalOnlyFile = "testdata/workspace_global_only_routes.txt"

func readRouteList(t *testing.T, path string) map[string]string {
	t.Helper()
	out := map[string]string{}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return out
	}
	require.NoError(t, err)
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, comment, _ := strings.Cut(line, "#")
		fields := strings.Fields(key)
		require.Len(t, fields, 2, "malformed line %q in %s", line, path)
		out[fields[0]+" "+fields[1]] = strings.TrimSpace(comment)
	}
	require.NoError(t, sc.Err())
	return out
}

// workspacePermsOf returns the Workspace-resource permissions of a fixed
// route annotation.
func workspacePermsOf(e permEntry) []authz.Permission {
	var out []authz.Permission
	for _, p := range e.permissions(nil) {
		if authz.ScopeOf(p).InWorkspace() {
			out = append(out, p)
		}
	}
	return out
}

func TestAuthzRoutes_WorkspaceClassification(t *testing.T) {
	api, _ := setupTestAPI(t)
	listed := readRouteList(t, workspaceGlobalOnlyFile)

	var unclassified, stale []string
	want := map[string]string{}
	for k, v := range listed {
		want[k] = v
	}
	for key, e := range api.routePerms {
		if e.inWorkspace {
			// Marked routes must not also be listed as global-only.
			if _, ok := listed[key]; ok {
				stale = append(stale, key)
				delete(want, key)
			}
			continue
		}
		if e.dynamic() {
			// Resolved per request; unmarked, so global-only by
			// construction (e.g. governed metadata on an LLM).
			continue
		}
		perms := workspacePermsOf(e)
		if len(perms) == 0 {
			continue
		}
		if _, ok := listed[key]; !ok {
			unclassified = append(unclassified, key+" ("+string(perms[0])+")")
			want[key] = ""
		}
	}

	if os.Getenv("UPDATE_WORKSPACE_ROUTES") == "1" {
		keys := make([]string, 0, len(want))
		for k := range want {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var b strings.Builder
		b.WriteString("# Routes on a Workspace resource that are NOT Workspace-checked: the route\n")
		b.WriteString("# gate requires the permission globally (fail closed for scoped callers).\n")
		b.WriteString("# See TestAuthzRoutes_WorkspaceClassification. Union over both editions.\n")
		for _, k := range keys {
			b.WriteString(k)
			if c := want[k]; c != "" {
				b.WriteString("  # " + c)
			}
			b.WriteString("\n")
		}
		require.NoError(t, os.WriteFile(workspaceGlobalOnlyFile, []byte(b.String()), 0o644))
		return
	}

	sort.Strings(unclassified)
	sort.Strings(stale)
	assert.Empty(t, unclassified, "routes on a Workspace resource that are neither Workspace-checked (permRouter.InWorkspace) nor listed in %s", workspaceGlobalOnlyFile)
	assert.Empty(t, stale, "Workspace-checked routes still listed as global-only in %s", workspaceGlobalOnlyFile)
}

// A Workspace-checked route must carry a Workspace permission: a global one
// would let the gate's any-Workspace rule apply to nothing, which hides a
// mistake.
func TestAuthzRoutes_InWorkspaceRejectsGlobalPermission(t *testing.T) {
	api, _ := setupTestAPI(t)
	g := api.permGroup(api.router.Group("/zz-test"))
	assert.Panics(t, func() {
		g.InWorkspace().GET("/users", authz.Read("users"), func(c *gin.Context) {})
	})
	assert.NotPanics(t, func() {
		g.InWorkspace().GET("/llms", authz.Read("llms"), func(c *gin.Context) {})
	})
	e := api.routePerms[routeKey("GET", "/zz-test/llms")]
	assert.True(t, e.inWorkspace)
	g.GET("/plain", authz.Read("llms"), func(c *gin.Context) {})
	assert.False(t, api.routePerms[routeKey("GET", "/zz-test/plain")].inWorkspace)
}
