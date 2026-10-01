// Command hostcompat reports where AI Studio would raise a host's dependency
// versions. Go's minimal version selection picks, for every module, the
// highest version any go.mod in the build requires, so each Studio
// requirement above the host's own silently upgrades the host when it
// imports Studio. That includes Studio's test-only requirements, and the go
// directive, which sets the host's minimum Go version.
//
//	go run ./tools/hostcompat -host dashboard.go.mod -studio go.mod [-studio enterprise/go.mod] [-allow scripts/host-compat-allow.tyk-analytics.txt]
//
// It exits 1 when Studio raises something the allowlist does not name. The
// allowlist holds one module path per line, each with a reason after a '#'.
// scripts/host-compat.sh runs it once per host (the Dashboard, MDCB), each
// with its own allowlist; see features/Embedding.md.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/semver"
)

type multi []string

func (m *multi) String() string     { return strings.Join(*m, ",") }
func (m *multi) Set(v string) error { *m = append(*m, v); return nil }

type raise struct {
	path, studio, host, from string
	direct                   bool
}

func main() {
	hostPath := flag.String("host", "", "the host's go.mod")
	allowPath := flag.String("allow", "", "allowlist of module paths Studio may raise")
	var studioPaths multi
	flag.Var(&studioPaths, "studio", "a Studio go.mod (repeatable)")
	flag.Parse()
	if *hostPath == "" || len(studioPaths) == 0 {
		flag.Usage()
		os.Exit(2)
	}

	host := parse(*hostPath)
	hostVers := map[string]string{}
	for _, r := range host.Require {
		hostVers[r.Mod.Path] = r.Mod.Version
	}
	// A host replace pins that module for the whole build; Studio's
	// requirement cannot move it.
	hostReplaced := map[string]bool{}
	for _, r := range host.Replace {
		hostReplaced[r.Old.Path] = true
	}
	allowed := readAllow(*allowPath)

	var raises []raise
	seen := map[string]int{}
	goRaise := ""
	for _, p := range studioPaths {
		f := parse(p)
		if f.Go != nil && host.Go != nil && semver.Compare("v"+f.Go.Version, "v"+host.Go.Version) > 0 {
			goRaise = fmt.Sprintf("go directive: %s requires go %s, the host declares go %s", p, f.Go.Version, host.Go.Version)
		}
		for _, r := range f.Require {
			hv, ok := hostVers[r.Mod.Path]
			if !ok || hostReplaced[r.Mod.Path] {
				continue
			}
			if semver.Compare(r.Mod.Version, hv) > 0 {
				if prev, ok := seen[r.Mod.Path]; ok {
					if semver.Compare(r.Mod.Version, raises[prev].studio) > 0 {
						raises[prev].studio = r.Mod.Version
					}
					raises[prev].direct = raises[prev].direct || !r.Indirect
					raises[prev].from += ", " + p
					continue
				}
				seen[r.Mod.Path] = len(raises)
				raises = append(raises, raise{r.Mod.Path, r.Mod.Version, hv, p, !r.Indirect})
			}
		}
	}
	sort.Slice(raises, func(i, j int) bool { return raises[i].path < raises[j].path })

	failed := false
	if goRaise != "" {
		fmt.Println(goRaise)
		if !allowed["go"] {
			failed = true
		}
	}
	for _, r := range raises {
		kind := "indirect"
		if r.direct {
			kind = "direct"
		}
		mark := "RAISES"
		if allowed[r.path] {
			mark = "allowed"
		} else {
			failed = true
		}
		fmt.Printf("%-8s %-60s host %-40s studio %-40s (%s, %s)\n", mark, r.path, r.host, r.studio, kind, r.from)
	}
	if len(raises) == 0 && goRaise == "" {
		fmt.Println("Studio raises none of the host's module versions.")
	}
	if failed {
		fmt.Fprintln(os.Stderr, "hostcompat: Studio would upgrade the host's dependencies above. Lower Studio's requirement where nothing needs the newer version, or add the module to the allowlist with the reason.")
		os.Exit(1)
	}
}

func parse(path string) *modfile.File {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "hostcompat:", err)
		os.Exit(2)
	}
	f, err := modfile.Parse(path, data, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "hostcompat:", err)
		os.Exit(2)
	}
	return f
}

func readAllow(path string) map[string]bool {
	allowed := map[string]bool{}
	if path == "" {
		return allowed
	}
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "hostcompat:", err)
		os.Exit(2)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(strings.SplitN(sc.Text(), "#", 2)[0])
		if line != "" {
			allowed[line] = true
		}
	}
	return allowed
}
