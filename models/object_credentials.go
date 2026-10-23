package models

// Tool, datasource and user credentials leave the process redacted, as LLM
// credentials do (RedactLLMCredentials): in system.tool, system.datasource
// and system.user events, which reach every subscribed plugin and the
// cluster_events table, and in every object hook. A literal value reads
// CredentialRedacted and a $SECRET/ or $ENV/ reference is kept. A
// before-hook's result gets the originals back wherever it still reads
// CredentialRedacted (RestoreObjectCredentials): the redaction means
// "unchanged", as on the API.

// redactLiteral replaces a literal credential with CredentialRedacted.
func redactLiteral(s *string) {
	if _, ok := LLMCredentialValue(*s); ok {
		*s = CredentialRedacted
	}
}

// restoreRedacted puts original back where modified still reads
// CredentialRedacted.
func restoreRedacted(modified *string, original string) {
	if *modified == CredentialRedacted {
		*modified = original
	}
}

// RedactToolCredentials is a copy of t with its auth key, and its
// dependencies' auth keys, redacted. t is not changed.
func RedactToolCredentials(t *Tool) *Tool {
	if t == nil {
		return nil
	}
	out := *t
	redactLiteral(&out.AuthKey)
	if t.Dependencies != nil {
		out.Dependencies = make([]*Tool, len(t.Dependencies))
		for i, dep := range t.Dependencies {
			out.Dependencies[i] = RedactToolCredentials(dep)
		}
	}
	return &out
}

// RedactDatasourceCredentials is a copy of d with its connection string,
// connection API key and embedder key redacted. The connection string is
// redacted whole, since it can carry a password. d is not changed.
func RedactDatasourceCredentials(d *Datasource) *Datasource {
	if d == nil {
		return nil
	}
	out := *d
	redactLiteral(&out.DBConnString)
	redactLiteral(&out.DBConnAPIKey)
	out.RedactEmbedKey()
	return &out
}

// RedactUserCredentials is a copy of u with its password hash, API key and
// session, reset and verification tokens redacted. u is not changed.
func RedactUserCredentials(u *User) *User {
	if u == nil {
		return nil
	}
	out := *u
	for _, f := range []*string{&out.Password, &out.APIKey, &out.SessionToken, &out.ResetToken, &out.VerificationToken} {
		redactLiteral(f)
	}
	return &out
}

// RedactObjectCredentials is the copy of an LLM, tool, datasource or user
// that may leave the process (events, object hooks); any other object is
// returned as it is.
func RedactObjectCredentials(object any) any {
	switch o := object.(type) {
	case *LLM:
		return RedactLLMCredentials(o)
	case LLM:
		return RedactLLMCredentials(&o)
	case *Tool:
		return RedactToolCredentials(o)
	case Tool:
		return RedactToolCredentials(&o)
	case *Datasource:
		return RedactDatasourceCredentials(o)
	case Datasource:
		return RedactDatasourceCredentials(&o)
	case *User:
		return RedactUserCredentials(o)
	case User:
		return RedactUserCredentials(&o)
	}
	return object
}

// RestoreObjectCredentials puts original's credentials back in modified (an
// object hook's result built from RedactObjectCredentials' copy) wherever it
// still reads CredentialRedacted. A datasource's embedder key needs nothing
// here: a redacted embed_api_key already keeps the stored key.
func RestoreObjectCredentials(original, modified any) {
	switch m := modified.(type) {
	case *LLM:
		if o, ok := original.(*LLM); ok {
			RestoreLLMCredentials(o, m)
		}
	case *Tool:
		if o, ok := original.(*Tool); ok && o != nil && m != nil {
			restoreRedacted(&m.AuthKey, o.AuthKey)
			deps := make(map[uint]*Tool, len(o.Dependencies))
			for _, dep := range o.Dependencies {
				if dep != nil {
					deps[dep.ID] = dep
				}
			}
			for _, dep := range m.Dependencies {
				if dep != nil && deps[dep.ID] != nil {
					RestoreObjectCredentials(deps[dep.ID], dep)
				}
			}
		}
	case *Datasource:
		if o, ok := original.(*Datasource); ok && o != nil && m != nil {
			restoreRedacted(&m.DBConnString, o.DBConnString)
			restoreRedacted(&m.DBConnAPIKey, o.DBConnAPIKey)
		}
	case *User:
		if o, ok := original.(*User); ok && o != nil && m != nil {
			restoreRedacted(&m.Password, o.Password)
			restoreRedacted(&m.APIKey, o.APIKey)
			restoreRedacted(&m.SessionToken, o.SessionToken)
			restoreRedacted(&m.ResetToken, o.ResetToken)
			restoreRedacted(&m.VerificationToken, o.VerificationToken)
		}
	}
}
