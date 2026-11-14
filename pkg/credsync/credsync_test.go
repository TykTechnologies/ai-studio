package credsync

import (
	"testing"

	"google.golang.org/grpc"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/secrets"
)

// The hook holds one handler for the process; nil removes it. (The row
// reader is tested through the control server, in package grpc, on SQLite
// and Postgres.)
func TestRegister(t *testing.T) {
	t.Cleanup(func() { Register(nil) })
	Register(nil)
	if Registered() != nil {
		t.Fatal("a handler is registered before any was")
	}
	called := false
	Register(func(Source, *pb.CredentialSyncRequest, grpc.ServerStreamingServer[pb.CredentialSyncPage]) error {
		called = true
		return nil
	})
	h := Registered()
	if h == nil {
		t.Fatal("the registered handler is not returned")
	}
	if err := h(nil, nil, nil); err != nil || !called {
		t.Fatalf("the registered handler was not the one called: %v", err)
	}
	Register(nil)
	if Registered() != nil {
		t.Fatal("nil did not remove the handler")
	}
}

// A credential can be hashed for edges only when its secret was read.
func TestRowHashable(t *testing.T) {
	for secret, want := range map[string]bool{"a-key": true, "": false, secrets.RefusedCredential: false} {
		if got := (Row{Credential: models.Credential{Secret: secret}}).Hashable(); got != want {
			t.Errorf("Hashable(%q) = %v, want %v", secret, got, want)
		}
	}
}
