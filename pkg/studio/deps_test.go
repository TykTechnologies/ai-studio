package studio

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// cgoOnlyPackages must stay out of pkg/studio's imports. A host that embeds
// Studio may build with CGO_ENABLED=0 (the Tyk Dashboard's dev builds do):
// these packages then either fail to compile (chroma-go's tokenizer, the ONNX
// runtime) or compile to a stub that fails at runtime (go-sqlite3).
var cgoOnlyPackages = []string{
	"github.com/mattn/go-sqlite3",
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite",
	"github.com/amikos-tech/chroma-go/pkg/api/v2",
	"github.com/amikos-tech/chroma-go/pkg/tokenizers/libtokenizers",
	"github.com/yalue/onnxruntime_go",
}

func TestHostBuildHasNoCgoOnlyDependencies(t *testing.T) {
	checkNoCgoOnlyDependencies(t, "studio_noui")
}

// checkNoCgoOnlyDependencies lists pkg/studio's dependencies the way a
// CGO_ENABLED=0 host build sees them and fails on any cgo-only package.
func checkNoCgoOnlyDependencies(t *testing.T, tags string) {
	t.Helper()
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go command not found")
	}
	cmd := exec.Command(goBin, "list", "-deps", "-tags", tags, ".")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps -tags %q: %v\n%s", tags, err, out)
	}
	deps := make(map[string]bool)
	for _, line := range strings.Split(string(out), "\n") {
		deps[strings.TrimSpace(line)] = true
	}
	for _, pkg := range cgoOnlyPackages {
		if deps[pkg] {
			t.Errorf("pkg/studio (tags %q) depends on %s, which needs cgo; keep it behind pkg/studio/sqlitedb or a cgo build tag", tags, pkg)
		}
	}
}
