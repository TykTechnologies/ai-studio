package grpc

import (
	"sort"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/secrets"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// refuseNewReferences refuses a plugin's write that adds or changes a
// $SECRET/ or $ENV/ reference in a credential field (old is nil for a new
// object). AI Studio resolves such a reference and sends its value to an
// endpoint the writer chooses; plugins act for no user who could be checked
// for secrets read, so they may keep a stored reference but not set one.
func refuseNewReferences(old, updated map[string]string) error {
	keys := secrets.NewReferences(old, updated)
	if len(keys) == 0 {
		return nil
	}
	sort.Strings(keys)
	return status.Errorf(codes.InvalidArgument, "plugins may not set $SECRET/ or $ENV/ references (fields: %s)", strings.Join(keys, ", "))
}
