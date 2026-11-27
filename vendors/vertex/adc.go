package vertexVendor

// Opt-in host Application Default Credentials (step 3 of the credential
// chain, epic midsommar-clean-1af). Off by default: reading vendor
// credentials from the host is the exception, enabled only by an operator
// with AI_STUDIO_VERTEX_ADC=true. AI Studio and the Microgateway read the
// same variable through ADCEnabled, so a hub and its edges behave alike;
// each edge then needs its own GCP identity.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// adcRetryInterval spaces lookups after a failed one, so a host without
// credentials does not probe the metadata server on every request.
const adcRetryInterval = 30 * time.Second

// findDefaultCredentials looks the host's credentials up. Tests replace it.
var findDefaultCredentials = google.FindDefaultCredentialsWithParams

// ADCEnabled reports whether the operator set AI_STUDIO_VERTEX_ADC to a
// true value (1, t, true; case-insensitive). Anything else, unset included,
// is off.
func ADCEnabled() bool {
	v, err := strconv.ParseBool(strings.TrimSpace(os.Getenv(ADCSetting)))
	return err == nil && v
}

var (
	adcMu       sync.Mutex
	adcSource   oauth2.TokenSource
	adcKind     string
	adcLastErr  error
	adcFailedAt time.Time
)

// adcTokenSource returns the process-wide token source for the host's
// credentials, looking them up on first use. A successful lookup is kept
// for the life of the process; a failed one is retried after
// adcRetryInterval.
func adcTokenSource() (oauth2.TokenSource, error) {
	adcMu.Lock()
	defer adcMu.Unlock()
	if adcSource != nil {
		return adcSource, nil
	}
	if adcLastErr != nil && time.Since(adcFailedAt) < adcRetryInterval {
		return nil, adcLastErr
	}
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, tokenHTTPClient)
	creds, err := findDefaultCredentials(ctx, google.CredentialsParams{Scopes: []string{CloudPlatformScope}})
	if err != nil {
		slog.Warn("vertex: host Application Default Credentials not found", "setting", ADCSetting, "error", err)
		adcLastErr = fmt.Errorf("%w: %s is enabled but no host Google credentials were found (GOOGLE_APPLICATION_CREDENTIALS, gcloud credentials or the GCP metadata server)", ErrNoCredential, ADCSetting)
		adcFailedAt = time.Now()
		return nil, adcLastErr
	}
	adcSource = oauth2.ReuseTokenSource(nil, creds.TokenSource)
	adcKind = credentialKind(creds)
	adcLastErr = nil
	return adcSource, nil
}

// ADCState describes the host-credential setting for a startup log. It
// carries no credential material.
type ADCState struct {
	Enabled bool `json:"enabled"`
	// Resolved: host credentials were found and a token was obtained.
	Resolved bool `json:"resolved"`
	// Source: the kind of host credential, e.g. "service_account",
	// "external_account", "authorized_user" or "metadata_server".
	Source string `json:"source,omitempty"`
	Error  string `json:"error,omitempty"`
}

// CheckADC reports the ADC setting and, when it is on, whether the host's
// credentials resolve to a token. It may contact the metadata server and
// Google's token endpoint, so hosts call it off the startup path.
func CheckADC(ctx context.Context) ADCState {
	if !ADCEnabled() {
		return ADCState{}
	}
	state := ADCState{Enabled: true}
	type result struct {
		ts  oauth2.TokenSource
		err error
	}
	done := make(chan result, 1)
	go func() {
		ts, err := adcTokenSource()
		if err == nil {
			_, err = ts.Token()
		}
		done <- result{ts, err}
	}()
	select {
	case <-ctx.Done():
		state.Error = "timed out looking up host Google credentials"
		return state
	case r := <-done:
		if r.err != nil {
			state.Error = "no usable host Google credentials; Vertex LLMs without an API key will be refused"
			return state
		}
	}
	state.Resolved = true
	adcMu.Lock()
	state.Source = adcKind
	adcMu.Unlock()
	return state
}

// credentialKind names the kind of credential ADC found, from the "type"
// field of its JSON, or the metadata server when there was no file.
func credentialKind(creds *google.Credentials) string {
	if len(creds.JSON) == 0 {
		return "metadata_server"
	}
	var f struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(creds.JSON, &f) != nil || f.Type == "" {
		return "credentials_file"
	}
	return f.Type
}

// resetADC drops the cached host source. Tests only.
func resetADC() {
	adcMu.Lock()
	adcSource, adcKind, adcLastErr, adcFailedAt = nil, "", nil, time.Time{}
	adcMu.Unlock()
}
