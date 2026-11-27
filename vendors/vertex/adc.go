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

// adcLookupTimeout bounds how long a request waits for the first lookup of
// the host's credentials (the GCP metadata server probe can be slow).
const adcLookupTimeout = 15 * time.Second

var (
	adcMu       sync.Mutex
	adcSource   oauth2.TokenSource
	adcKind     string
	adcLastErr  error
	adcFailedAt time.Time
	// adcInflight is closed when the running lookup finishes; nil when no
	// lookup runs.
	adcInflight chan struct{}
)

// adcTokenSource returns the process-wide token source for the host's
// credentials, looking them up on first use. A successful lookup is kept
// for the life of the process; a failed one is retried after
// adcRetryInterval.
func adcTokenSource() (oauth2.TokenSource, error) {
	ctx, cancel := context.WithTimeout(context.Background(), adcLookupTimeout)
	defer cancel()
	return adcTokenSourceCtx(ctx)
}

// adcTokenSourceCtx is adcTokenSource waiting at most until ctx is done.
// One lookup runs at a time, without the lock held, and every caller waits
// for it: the lookup's own context must outlive the request, since the
// token source keeps it for refreshes.
func adcTokenSourceCtx(ctx context.Context) (oauth2.TokenSource, error) {
	adcMu.Lock()
	if adcSource != nil {
		ts := adcSource
		adcMu.Unlock()
		return ts, nil
	}
	if adcLastErr != nil && time.Since(adcFailedAt) < adcRetryInterval {
		err := adcLastErr
		adcMu.Unlock()
		return nil, err
	}
	wait := adcInflight
	if wait == nil {
		wait = make(chan struct{})
		adcInflight = wait
		go runADCLookup(wait)
	}
	adcMu.Unlock()

	select {
	case <-wait:
	case <-ctx.Done():
		return nil, fmt.Errorf("%w: %s is enabled but looking up the host Google credentials timed out", ErrNoCredential, ADCSetting)
	}
	adcMu.Lock()
	defer adcMu.Unlock()
	if adcSource != nil {
		return adcSource, nil
	}
	return nil, adcLastErr
}

// runADCLookup looks the host's credentials up and records the result.
func runADCLookup(done chan struct{}) {
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, tokenHTTPClient)
	creds, err := findDefaultCredentials(ctx, google.CredentialsParams{Scopes: []string{CloudPlatformScope}})

	adcMu.Lock()
	defer func() {
		adcInflight = nil
		adcMu.Unlock()
		close(done)
	}()
	if err != nil {
		slog.Warn("vertex: host Application Default Credentials not found", "setting", ADCSetting, "error", err)
		adcLastErr = fmt.Errorf("%w: %s is enabled but no host Google credentials were found (GOOGLE_APPLICATION_CREDENTIALS, gcloud credentials or the GCP metadata server)", ErrNoCredential, ADCSetting)
		adcFailedAt = time.Now()
		return
	}
	adcSource = oauth2.ReuseTokenSource(nil, creds.TokenSource)
	adcKind = credentialKind(creds)
	adcLastErr = nil
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
// credentials resolve to a token. The lookup waits at most until ctx is
// done; the token request is bounded by the token client's timeout. It may
// contact the metadata server and Google's token endpoint, so hosts call it
// off the startup path.
func CheckADC(ctx context.Context) ADCState {
	if !ADCEnabled() {
		return ADCState{}
	}
	state := ADCState{Enabled: true}
	ts, err := adcTokenSourceCtx(ctx)
	if err == nil {
		_, err = ts.Token()
	}
	if err != nil {
		state.Error = "no usable host Google credentials; Vertex LLMs without an API key will be refused"
		return state
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
	adcSource, adcKind, adcLastErr, adcFailedAt, adcInflight = nil, "", nil, time.Time{}, nil
	adcMu.Unlock()
}
