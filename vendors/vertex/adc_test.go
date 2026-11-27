package vertexVendor

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// fakeADC replaces the host credential lookup and counts calls.
func fakeADC(t *testing.T, creds *google.Credentials, err error) *atomic.Int32 {
	t.Helper()
	var calls atomic.Int32
	prev := findDefaultCredentials
	findDefaultCredentials = func(context.Context, google.CredentialsParams) (*google.Credentials, error) {
		calls.Add(1)
		return creds, err
	}
	resetADC()
	t.Cleanup(func() {
		findDefaultCredentials = prev
		resetADC()
	})
	return &calls
}

func metadataCreds(token string) *google.Credentials {
	return &google.Credentials{TokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})}
}

func TestADCEnabled_Values(t *testing.T) {
	for v, want := range map[string]bool{"": false, "false": false, "0": false, "yes": false, "true": true, "TRUE": true, "1": true, " true ": true} {
		t.Setenv(ADCSetting, v)
		assert.Equal(t, want, ADCEnabled(), "%q", v)
	}
}

func TestADC_OffEmptyKeyNeverLooksUpHost(t *testing.T) {
	allowLocalEndpoints(t)
	calls := fakeADC(t, metadataCreds("host-token"), nil)
	t.Setenv(ADCSetting, "")

	h, err := authHeader(t, &models.LLM{ID: 30, Name: "keyless", Vendor: models.VERTEX})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNoCredential), err)
	assert.Contains(t, err.Error(), ADCSetting)
	assert.Empty(t, h)
	assert.Zero(t, calls.Load(), "FindDefaultCredentials is never called while the setting is off")
	assert.Equal(t, ADCState{}, CheckADC(context.Background()))
	assert.Zero(t, calls.Load())
}

func TestADC_OnEmptyKeyUsesHostToken(t *testing.T) {
	allowLocalEndpoints(t)
	calls := fakeADC(t, metadataCreds("host-token"), nil)
	t.Setenv(ADCSetting, "true")

	for i := 0; i < 3; i++ {
		h, err := authHeader(t, &models.LLM{ID: 31 + uint(i), Name: "keyless", Vendor: models.VERTEX})
		require.NoError(t, err)
		assert.Equal(t, "Bearer host-token", h)
	}
	assert.EqualValues(t, 1, calls.Load(), "one host source per process")
}

func TestADC_ExplicitKeyWins(t *testing.T) {
	allowLocalEndpoints(t)
	g := newFakeGoogle(t)
	calls := fakeADC(t, metadataCreds("host-token"), nil)
	t.Setenv(ADCSetting, "true")

	h, err := authHeader(t, &models.LLM{ID: 35, Name: "keyed", Vendor: models.VERTEX, APIKey: mustJSON(t, serviceAccountJSON(t, g.srv.URL+"/token"))})
	require.NoError(t, err)
	assert.Equal(t, "Bearer sa-1", h)
	h, err = authHeader(t, &models.LLM{ID: 36, Name: "tok", Vendor: models.VERTEX, APIKey: "ya29.explicit"})
	require.NoError(t, err)
	assert.Equal(t, "Bearer ya29.explicit", h)
	assert.Zero(t, calls.Load())
}

func TestADC_OnButNotFound(t *testing.T) {
	allowLocalEndpoints(t)
	calls := fakeADC(t, nil, errors.New("google: could not find default credentials"))
	t.Setenv(ADCSetting, "true")

	_, err := authHeader(t, &models.LLM{ID: 37, Name: "keyless", Vendor: models.VERTEX})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNoCredential), err)
	_, err = authHeader(t, &models.LLM{ID: 37, Name: "keyless", Vendor: models.VERTEX})
	require.Error(t, err)
	assert.EqualValues(t, 1, calls.Load(), "a failed lookup is not repeated on every request")

	state := CheckADC(context.Background())
	assert.True(t, state.Enabled)
	assert.False(t, state.Resolved)
	assert.NotEmpty(t, state.Error)
}

func TestCheckADC_ReportsSourceWithoutSecrets(t *testing.T) {
	const secret = "PRIVATE-KEY-MATERIAL"
	t.Setenv(ADCSetting, "true")

	fakeADC(t, metadataCreds("host-token"), nil)
	assert.Equal(t, ADCState{Enabled: true, Resolved: true, Source: "metadata_server"}, CheckADC(context.Background()))

	fakeADC(t, &google.Credentials{
		JSON:        []byte(`{"type":"service_account","private_key":"` + secret + `"}`),
		TokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "t"}),
	}, nil)
	state := CheckADC(context.Background())
	assert.Equal(t, ADCState{Enabled: true, Resolved: true, Source: "service_account"}, state)
	assert.NotContains(t, state.Source+state.Error, secret)
}

func TestADC_ConcurrentFirstLookupRunsOnceWithoutHoldingTheLock(t *testing.T) {
	release := make(chan struct{})
	var calls atomic.Int32
	prev := findDefaultCredentials
	findDefaultCredentials = func(context.Context, google.CredentialsParams) (*google.Credentials, error) {
		calls.Add(1)
		<-release
		return metadataCreds("host-token"), nil
	}
	resetADC()
	t.Cleanup(func() {
		findDefaultCredentials = prev
		resetADC()
	})

	// A caller whose deadline passes gives up; the lookup keeps running.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := adcTokenSourceCtx(ctx)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNoCredential))

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ts, err := adcTokenSourceCtx(context.Background())
			if assert.NoError(t, err) {
				tok, err := ts.Token()
				assert.NoError(t, err)
				assert.Equal(t, "host-token", tok.AccessToken)
			}
		}()
	}
	time.Sleep(20 * time.Millisecond)
	close(release)
	wg.Wait()
	assert.EqualValues(t, 1, calls.Load(), "one lookup for every waiting caller")
}
