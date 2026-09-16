package netguard

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestURLPolicy_Validate(t *testing.T) {
	strict := NewURLPolicy(PolicyOptions{})
	allowInternal := NewURLPolicy(PolicyOptions{AllowInternal: true})
	listed := NewURLPolicy(PolicyOptions{
		AllowedHosts: []string{"dash.example.com", ".partner.example.org", "10.0.0.5"},
		DeniedHosts:  []string{"evil.partner.example.org"},
	})
	exempt := NewURLPolicy(PolicyOptions{ExemptHosts: []string{"dashboard.internal"}})
	custom := errors.New("custom sentinel")
	sentinel := NewURLPolicy(PolicyOptions{Err: custom, InternalHint: "set the switch"})

	cases := []struct {
		name   string
		p      *URLPolicy
		url    string
		wantOK bool
	}{
		{"https public", strict, "https://dash.example.com/", true},
		{"http public", strict, "http://dash.example.com:8080/api?x=1", true},
		{"ftp", strict, "ftp://dash.example.com/", false},
		{"userinfo", strict, "https://user:pw@dash.example.com/", false},
		{"fragment", strict, "https://dash.example.com/#frag", false},
		{"no host", strict, "https:///path", false},
		{"bad port", strict, "https://dash.example.com:99999/", false},
		{"loopback literal", strict, "http://127.0.0.1:3000/", false},
		{"rfc1918 literal", strict, "http://10.1.2.3/", false},
		{"metadata", strict, "http://169.254.169.254/latest/meta-data", false},
		{"ipv6 loopback", strict, "http://[::1]:3000/", false},
		{"localhost name", strict, "http://localhost:3000/", false},
		{".internal name", strict, "http://dashboard.internal/", false},
		{"loopback allowed", allowInternal, "http://127.0.0.1:3000/", true},
		{"localhost allowed", allowInternal, "http://localhost:3000/", true},
		{"allowlist exact", listed, "https://dash.example.com/", true},
		{"allowlist suffix", listed, "https://a.partner.example.org/", true},
		{"allowlist bare suffix domain", listed, "https://partner.example.org/", true},
		{"allowlist miss", listed, "https://other.example.com/", false},
		{"deny wins", listed, "https://evil.partner.example.org/", false},
		{"explicit internal allowed", listed, "http://10.0.0.5/", true},
		{"exempt internal host", exempt, "http://dashboard.internal:3000/", true},
		{"exempt does not open others", exempt, "http://other.internal:3000/", false},
		{"empty", strict, "", false},
		{"too long", strict, "https://dash.example.com/" + strings.Repeat("a", 2100), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			u, err := c.p.Validate(c.url)
			if c.wantOK {
				require.NoError(t, err)
				assert.NotNil(t, u)
			} else {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrURLPolicy)
			}
		})
	}

	_, err := sentinel.Validate("http://localhost:3000/")
	require.Error(t, err)
	assert.ErrorIs(t, err, custom)
	assert.Contains(t, err.Error(), "set the switch")
}

func TestURLPolicy_WithExemptHosts(t *testing.T) {
	base := NewURLPolicy(PolicyOptions{})
	_, err := base.Validate("http://localhost:3000/")
	require.Error(t, err)
	_, err = base.WithExemptHosts("localhost").Validate("http://localhost:3000/")
	require.NoError(t, err)
	// The original is unchanged.
	_, err = base.Validate("http://localhost:3000/")
	require.Error(t, err)
}

func TestURLPolicy_DialerBlocksInternalAtConnectTime(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer srv.Close()

	strict := NewURLPolicy(PolicyOptions{})
	_, err := strict.NewHTTPClient(2 * time.Second).Get(srv.URL)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrPolicyDial), "expected policy dial error, got %v", err)

	open := NewURLPolicy(PolicyOptions{AllowInternal: true})
	resp, err := open.NewHTTPClient(2 * time.Second).Get(srv.URL)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, 200, resp.StatusCode)

	exempt := NewURLPolicy(PolicyOptions{ExemptHosts: []string{"127.0.0.1"}})
	resp, err = exempt.NewHTTPClient(2 * time.Second).Get(srv.URL)
	require.NoError(t, err)
	resp.Body.Close()
}

func TestURLPolicy_ClientDoesNotFollowRedirects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://elsewhere.example.com/", http.StatusFound)
	}))
	defer srv.Close()
	open := NewURLPolicy(PolicyOptions{AllowInternal: true})
	resp, err := open.NewHTTPClient(2 * time.Second).Get(srv.URL)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusFound, resp.StatusCode)
}

func TestHostMatches(t *testing.T) {
	assert.True(t, HostMatches("a.example.com", ".example.com"))
	assert.True(t, HostMatches("example.com", ".example.com"))
	assert.False(t, HostMatches("notexample.com", ".example.com"))
	assert.True(t, HostMatches("example.com", "example.com"))
	assert.False(t, HostMatches("a.example.com", "example.com"))
}
