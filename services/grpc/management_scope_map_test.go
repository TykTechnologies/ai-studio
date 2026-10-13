package grpc

import (
	"testing"

	pb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
)

// Every RPC of the management service a plugin can reach over its broker
// must have a scope: the broker's interceptor refuses unmapped methods, so a
// new RPC without a mapping fails here instead of failing for plugins (or,
// worse, being reachable without a scope).
func TestEveryManagementRPCHasAScope(t *testing.T) {
	desc := pb.AIStudioManagementService_ServiceDesc
	prefix := "/" + desc.ServiceName + "/"
	var names []string
	for _, m := range desc.Methods {
		names = append(names, m.MethodName)
	}
	for _, s := range desc.Streams {
		names = append(names, s.StreamName)
	}
	if len(names) == 0 {
		t.Fatal("service descriptor lists no methods")
	}
	for _, name := range names {
		if extractScopeFromMethod(prefix+name) == "" {
			t.Errorf("%s has no scope mapping in extractScopeFromMethod", name)
		}
	}
}
