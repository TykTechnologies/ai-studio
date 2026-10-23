package ociplugins

import "context"

type authConfigKey struct{}

// WithAuthConfig returns a context carrying the registry auth config name
// (OCIPluginParams.AuthConfig) for a signature verifier to use.
func WithAuthConfig(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, authConfigKey{}, name)
}

// AuthConfigFrom returns the registry auth config name set by
// WithAuthConfig, or "" if none was set.
func AuthConfigFrom(ctx context.Context) string {
	name, _ := ctx.Value(authConfigKey{}).(string)
	return name
}
