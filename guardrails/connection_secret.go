package guardrails

// IsSecretConnectionField reports whether a provider's connection field
// holds a secret: the provider marks it Secret, or it is a field (or a
// provider) the catalog does not know. What leaves the process with a
// guardrail's configuration (system events, object hooks) redacts these.
func IsSecretConnectionField(provider, field string) bool {
	return isSecretConnectionField(provider, field)
}
