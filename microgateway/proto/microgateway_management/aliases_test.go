package microgateway_management_test

import (
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	old "github.com/TykTechnologies/midsommar/microgateway/proto/microgateway_management"
	moved "github.com/TykTechnologies/midsommar/v2/proto/microgateway_management"
)

// The old path must forward to the root-module package, not hold a second
// copy: a second copy would register the same proto names twice and panic.
func TestOldPathForwardsToMovedPackage(t *testing.T) {
	var req *old.StoreAppRequest = &moved.StoreAppRequest{}
	if req.ProtoReflect().Descriptor().FullName() != "microgateway_management.StoreAppRequest" {
		t.Fatalf("full name = %s", req.ProtoReflect().Descriptor().FullName())
	}
	if old.File_microgateway_proto_microgateway_management_service_proto != moved.File_proto_microgateway_management_proto {
		t.Fatal("old file descriptor name does not point at the moved descriptor")
	}
	if old.MicrogatewayManagementService_ServiceDesc.ServiceName != "microgateway_management.MicrogatewayManagementService" {
		t.Fatalf("service name = %s", old.MicrogatewayManagementService_ServiceDesc.ServiceName)
	}
	n := 0
	protoregistry.GlobalFiles.RangeFilesByPackage("microgateway_management", func(protoreflect.FileDescriptor) bool {
		n++
		return true
	})
	if n != 1 {
		t.Fatalf("microgateway_management registered by %d files, want 1", n)
	}
}
