package authz

import "context"

// NoopAuthorizer is used when fine-grained authorization is disabled. It always allows access,
// deferring all authorization decisions to the legacy auth system.
type NoopAuthorizer struct{}

var _ Authorizer = (*NoopAuthorizer)(nil)

func (n *NoopAuthorizer) Enabled() bool { return false }

func (n *NoopAuthorizer) Check(context.Context, uint, string, string, uint) (bool, error) {
	return true, nil
}

func (n *NoopAuthorizer) CheckByName(context.Context, uint, string, string, string) (bool, error) {
	return true, nil
}

func (n *NoopAuthorizer) ListResources(context.Context, uint, string, string) ([]uint, error) {
	return nil, nil
}

func (n *NoopAuthorizer) ListResourcesByName(context.Context, uint, string, string) ([]string, error) {
	return nil, nil
}

func (n *NoopAuthorizer) ListResourcesPage(context.Context, uint, string, string, int, string) ([]uint, string, error) {
	return nil, "", nil
}

func (n *NoopAuthorizer) ListResourcesByNamePage(context.Context, uint, string, string, int, string) ([]string, string, error) {
	return nil, "", nil
}

func (n *NoopAuthorizer) Grant(context.Context, []Relationship) error   { return nil }
func (n *NoopAuthorizer) Revoke(context.Context, []Relationship) error  { return nil }
func (n *NoopAuthorizer) GrantAndRevoke(context.Context, []Relationship, []Relationship) error {
	return nil
}
func (n *NoopAuthorizer) Close() {}
