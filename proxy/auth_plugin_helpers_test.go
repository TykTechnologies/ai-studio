package proxy

import (
	"net/http"
	"sync"
)

// acceptCredential is a CustomAuth hook that authenticates one credential as
// appID and leaves every other credential to app keys, as for an endpoint
// without auth plugins.
func acceptCredential(credential string, appID uint) func(*http.Request, AuthTarget, string, string) (AuthResult, error) {
	return func(_ *http.Request, _ AuthTarget, got, _ string) (AuthResult, error) {
		if got == credential {
			return AuthResult{Outcome: AuthAccepted, AppID: appID, PluginID: 1, PluginName: "test-auth"}, nil
		}
		return AuthResult{Outcome: AuthNotHandled}, nil
	}
}

// fakeAuthPlugins models auth plugins attached to some endpoints: a request
// to one of them is accepted for a known credential and rejected otherwise;
// other endpoints are left to app keys. It records each call.
type fakeAuthPlugins struct {
	mu       sync.Mutex
	attached map[string]bool // AuthTarget.Kind + ":" + Slug
	accept   map[string]uint // credential -> app id
	subject  string
	calls    []fakeAuthCall
}

type fakeAuthCall struct {
	Target     AuthTarget
	Credential string
	CredType   string
	Path       string
}

func newFakeAuthPlugins() *fakeAuthPlugins {
	return &fakeAuthPlugins{attached: map[string]bool{}, accept: map[string]uint{}}
}

func (f *fakeAuthPlugins) attach(kind, slug string) *fakeAuthPlugins {
	f.attached[kind+":"+slug] = true
	return f
}

func (f *fakeAuthPlugins) hook(r *http.Request, target AuthTarget, credential, credType string) (AuthResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.attached[target.Kind+":"+target.Slug] {
		return AuthResult{Outcome: AuthNotHandled}, nil
	}
	f.calls = append(f.calls, fakeAuthCall{Target: target, Credential: credential, CredType: credType, Path: r.URL.Path})
	if appID, ok := f.accept[credential]; ok {
		return AuthResult{
			Outcome: AuthAccepted, AppID: appID, PluginID: 42, PluginName: "fake-idp",
			Subject: f.subject, Claims: map[string]string{"auth_actor": "agent-1"},
		}, nil
	}
	return AuthResult{Outcome: AuthRejected, Reason: "unknown credential"}, nil
}

func (f *fakeAuthPlugins) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}
