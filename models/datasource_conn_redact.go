package models

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/secrets"
)

// A datasource's connection string is metadata (which store, where) that can
// carry a credential: a password in the URL's userinfo (postgres://u:p@h/db,
// redis://:p@h), a secret query parameter (?api_key=...), or a secret
// key=value pair (password=, token=, api_key=... in a DSN or a JDBC URL). API responses show it with those parts replaced by
// CredentialRedacted (Workspaces .58: datasources are read-global, D4, so
// reads must not carry credentials). A $SECRET/ or $ENV/ reference is shown
// as it is.

// connSecretKeys are the parameter names that carry a credential, in a URL
// query or in a key=value DSN (lower case).
var connSecretKeys = []string{
	"password", "passwd", "pwd", "secret", "token", "access_token", "access-token", "accesstoken",
	"auth_token", "auth-token", "authtoken", "api_key", "api-key", "apikey", "key",
}

var connSecretKeySet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(connSecretKeys))
	for _, k := range connSecretKeys {
		m[k] = struct{}{}
	}
	return m
}()

// connKeyValueSecret matches key=value pairs with a secret key anywhere in
// the string: a DSN (host=h password=p), a JDBC URL (jdbc:sqlserver://h;
// password=p) or a DSN with a URL-valued field. The value runs to the next
// space, ';' or '&', or is quoted.
var connKeyValueSecret = func() *regexp.Regexp {
	alts := make([]string, len(connSecretKeys))
	for i, k := range connSecretKeys {
		alts[i] = regexp.QuoteMeta(k)
	}
	return regexp.MustCompile(`(?i)\b(` + strings.Join(alts, "|") + `)(\s*=\s*)('[^']*'|"[^"]*"|[^\s;&]+)`)
}()

// connURLScheme matches a string that starts as a standard URL.
var connURLScheme = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.\-]*://`)

// connPlaceholder stands in for CredentialRedacted while a URL is encoded,
// so the marker is not percent-escaped.
const connPlaceholder = "REDACTEDCONNSECRETPLACEHOLDER"

// RedactConnString returns s with its credential parts replaced by
// CredentialRedacted: the userinfo password and secret query parameters of
// a URL, and every secret key=value pair anywhere in the string. Empty
// values and references are returned as they are. A URL that does not parse
// is redacted whole (fails closed).
func RedactConnString(s string) string {
	if s == "" || secrets.IsReference(s) {
		return s
	}
	out := s
	if connURLScheme.MatchString(s) {
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
			queryChanged := false
			for k, vs := range q {
				if _, secret := connSecretKeySet[strings.ToLower(k)]; !secret {
					continue
				}
				for i, v := range vs {
					if v != "" {
						vs[i] = connPlaceholder
						queryChanged = true
					}
				}
			}
			if queryChanged {
				u.RawQuery = q.Encode()
				changed = true
			}
		}
		if changed {
			out = strings.ReplaceAll(u.String(), connPlaceholder, CredentialRedacted)
		}
	}
	return connKeyValueSecret.ReplaceAllString(out, "${1}${2}"+CredentialRedacted)
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
