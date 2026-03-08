package authz

import "context"

// NoopAuthorizer is used when OpenFGA is disabled. It always allows access,
// deferring all authorization decisions to the legacy auth system.
type NoopAuthorizer struct{}

var _ Authorizer = (*NoopAuthorizer)(nil)

func (n *NoopAuthorizer) Enabled() bool { return false }

func (n *NoopAuthorizer) Check(context.Context, uint, string, string, uint) (bool, error) {
	return true, nil
}

func (n *NoopAuthorizer) CheckStr(context.Context, uint, string, string, string) (bool, error) {
	return true, nil
}

func (n *NoopAuthorizer) ListObjects(context.Context, uint, string, string) ([]uint, error) {
	return nil, nil
}

func (n *NoopAuthorizer) ListObjectsStr(context.Context, uint, string, string) ([]string, error) {
	return nil, nil
}

func (n *NoopAuthorizer) WriteTuples(context.Context, []Tuple) error   { return nil }
func (n *NoopAuthorizer) DeleteTuples(context.Context, []Tuple) error  { return nil }
func (n *NoopAuthorizer) WriteTuplesAndDelete(context.Context, []Tuple, []Tuple) error {
	return nil
}
func (n *NoopAuthorizer) Close() {}
