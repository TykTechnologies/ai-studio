package extract

import (
	"regexp"
	"strings"
)

// DefaultToolNamePrefixes are the §11 correlation.tool_name_prefix_strip
// defaults, plus {server}_ from the extraction bead.
var DefaultToolNamePrefixes = []string{"mcp__{server}__", "{server}__", "{server}.", "{server}_"}

// NameNormalizer normalises tool names for the R2 match (§4.4): lower case,
// then the first configured prefix that matches is stripped.
type NameNormalizer struct {
	prefixes []*regexp.Regexp
}

// NewNameNormalizer compiles prefix templates. In a template with a literal
// lead, such as mcp__{server}__, {server} matches any server name. In one
// that starts with {server} it matches only one of servers (the App's MCP
// server names, case-insensitively), and with no servers that template is
// skipped: it would strip the first word of any name ("get_weather" to
// "weather").
func NewNameNormalizer(templates, servers []string) *NameNormalizer {
	var alt string
	if len(servers) > 0 {
		quoted := make([]string, 0, len(servers))
		for _, s := range servers {
			if s = strings.ToLower(strings.TrimSpace(s)); s != "" {
				quoted = append(quoted, regexp.QuoteMeta(s))
			}
		}
		if len(quoted) > 0 {
			alt = "(?:" + strings.Join(quoted, "|") + ")"
		}
	}
	n := &NameNormalizer{}
	for _, t := range templates {
		t = strings.ToLower(t)
		before, after, hasServer := strings.Cut(t, "{server}")
		var expr string
		switch {
		case !hasServer:
			expr = regexp.QuoteMeta(t)
		case before != "":
			expr = regexp.QuoteMeta(before) + ".+?" + regexp.QuoteMeta(after)
		case alt != "":
			expr = alt + regexp.QuoteMeta(after)
		default:
			continue
		}
		n.prefixes = append(n.prefixes, regexp.MustCompile("^"+expr))
	}
	return n
}

// Normalize returns the normalised name. A prefix is stripped only when what
// remains is a name: not empty, and not starting with a separator.
func (n *NameNormalizer) Normalize(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if n == nil {
		return name
	}
	for _, re := range n.prefixes {
		loc := re.FindStringIndex(name)
		if loc == nil || loc[1] >= len(name) {
			continue
		}
		if c := name[loc[1]]; c == '_' || c == '.' {
			continue
		}
		return name[loc[1]:]
	}
	return name
}
