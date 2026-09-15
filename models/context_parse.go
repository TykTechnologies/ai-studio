package models

import (
	"regexp"
	"strings"
)

// contextBlockRe matches the [CONTEXT]...[/CONTEXT] block the session prepends
// to stored human messages (RAG hits, uploaded files, tool documentation).
var contextBlockRe = regexp.MustCompile(`(?s)^\s*\[CONTEXT\]\s*(.*?)\s*\[/CONTEXT\]\s*`)

// ragContextPrefix is the header prepHumanMessage puts in front of RAG hits.
const ragContextPrefix = "Context for this message:"

// SplitContext separates the context block the session stores in front of a
// human message from the text the user actually typed. It is the single place
// that knows the storage format, so the v2 history endpoint and anything else
// that materialises stored messages agree with the session itself.
func SplitContext(stored string) (contextText, userText string, hasContext bool) {
	m := contextBlockRe.FindStringSubmatchIndex(stored)
	if m == nil {
		return "", stored, false
	}
	contextText = strings.TrimSpace(stored[m[2]:m[3]])
	contextText = strings.TrimSpace(strings.TrimPrefix(contextText, ragContextPrefix))
	userText = strings.TrimSpace(stored[m[1]:])
	return contextText, userText, true
}
