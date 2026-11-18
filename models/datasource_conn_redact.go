package models

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/secrets"
)

// A datasource's connection string is metadata (which store, where) that can
// carry a credential: a password in the URL's userinfo (postgres://u:p@h/db,
// redis://:p@h), a secret query parameter (?api_key=...), or password= in a
// key=value DSN. API responses show it with those parts replaced by
// CredentialRedacted (Workspaces .58: datasources are read-global, D4, so
// reads must not carry credentials). A $SECRET/ or $ENV/ reference is shown
// as it is.

// connSecretParam matches query parameter names that carry a credential.
var connSecretParam = regexp.MustCompile(`(?i)^(password|passwd|pwd|secret|token|access[_-]?token|auth[_-]?token|api[_-]?key|apikey|key)$`)

// connDSNPassword matches password=... in a key=value DSN (quoted or not).
var connDSNPassword = regexp.MustCompile(`(?i)\b(password|passwd|pwd)(\s*=\s*)('[^']*'|"[^"]*"|[^\s;]+)`)

// connPlaceholder stands in for CredentialRedacted while a URL is encoded,
// so the marker is not percent-escaped.
const connPlaceholder = "REDACTEDCONNSECRETPLACEHOLDER"

// RedactConnString returns s with its credential parts replaced by
// CredentialRedacted. Empty values and references are returned as they are.
// A value with a scheme that does not parse as a URL is redacted whole
// (fails closed).
func RedactConnString(s string) string {
	if s == "" || secrets.IsReference(s) {
		return s
	}
	if strings.Contains(s, "://") {
		u, err := url.Parse(s)
		if err != nil {
			return CredentialRedacted
		}
		changed := false
		if u.User != nil {
			if _, has := u.User.Password(); has {
				u.User = url.UserPassword(u.User.Username(), connPlaceholder)
				changed = true
			}
		}
		if u.RawQuery != "" {
			q := u.Query()
			for k, vs := range q {
				if !connSecretParam.MatchString(k) {
					continue
				}
				for i, v := range vs {
					if v != "" {
						vs[i] = connPlaceholder
						changed = true
					}
				}
			}
			if changed {
				u.RawQuery = q.Encode()
			}
		}
		if !changed {
			return s
		}
		return strings.ReplaceAll(u.String(), connPlaceholder, CredentialRedacted)
	}
	return connDSNPassword.ReplaceAllString(s, "${1}${2}"+CredentialRedacted)
}

// ConnStringHidesCredential reports whether RedactConnString changes s.
func ConnStringHidesCredential(s string) bool {
	return RedactConnString(s) != s
}

// RestoreConnString resolves a connection string sent back by a client
// against the stored one: the redacted form of the stored value means
// "unchanged" and gives the stored value back. Any other value that still
// carries the redaction marker is refused (ok false): keeping the stored
// password under a changed host or user would hand it to whoever chose
// them, so the caller must enter it again.
func RestoreConnString(sent, stored string) (string, bool) {
	if sent == stored {
		return sent, true
	}
	if stored != "" && sent == RedactConnString(stored) {
		return stored, true
	}
	if strings.Contains(sent, CredentialRedacted) || strings.Contains(sent, url.QueryEscape(CredentialRedacted)) {
		return "", false
	}
	return sent, true
}
