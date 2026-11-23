package bedrockVendor

import "strings"

// geoProfilePrefixes are the geography prefixes of Bedrock's system-defined
// (cross-region) inference profiles: us.amazon.nova-lite-v1:0 routes
// amazon.nova-lite-v1:0 within the US regions.
var geoProfilePrefixes = []string{"us-gov.", "us.", "eu.", "apac.", "jp.", "au.", "ca.", "global."}

// PricingModelIDs returns the other names a Bedrock model identifier can be
// priced under, most specific first. It never includes modelID itself, and
// returns nil for an identifier that names no model.
//
//   - A system inference-profile ARN
//     (arn:aws:bedrock:<region>:<account>:inference-profile/apac.amazon.nova-lite-v1:0)
//     names its profile id (apac.amazon.nova-lite-v1:0), then the profile's
//     model (amazon.nova-lite-v1:0).
//   - A foundation-model ARN names its model id.
//   - A system inference-profile id names its model.
//
// An application inference profile, a provisioned, custom or imported model
// and any other ARN carry an opaque id: the model behind it cannot be read
// from the name, so nothing is returned and the call stays unpriced unless
// the identifier itself has a price. The mapping is for pricing only; the
// allowed-models check runs on the identifier as the client sent it.
func PricingModelIDs(modelID string) []string {
	id := modelID
	if isARN(id) {
		resource, ok := bedrockARNResource(id)
		if !ok {
			return nil
		}
		kind, name, ok := strings.Cut(resource, "/")
		if !ok || name == "" || strings.Contains(name, "/") {
			return nil
		}
		switch kind {
		case "inference-profile":
			out := []string{name}
			if base, ok := profileModelID(name); ok {
				out = append(out, base)
			}
			return out
		case "foundation-model":
			if !looksLikeModelID(name) {
				return nil
			}
			return []string{name}
		default:
			return nil
		}
	}
	if base, ok := profileModelID(id); ok {
		return []string{base}
	}
	return nil
}

func isARN(s string) bool {
	return len(s) >= 4 && strings.EqualFold(s[:4], "arn:")
}

// bedrockARNResource returns the resource part of a Bedrock ARN
// (arn:<partition>:bedrock:<region>:<account>:<resource>), in any partition.
func bedrockARNResource(arn string) (string, bool) {
	parts := strings.SplitN(arn, ":", 6)
	if len(parts) != 6 || parts[1] == "" || !strings.EqualFold(parts[2], "bedrock") || parts[5] == "" {
		return "", false
	}
	return parts[5], true
}

// profileModelID returns the model id of a system inference-profile id: the
// id without its geography prefix, when what remains is a model id.
func profileModelID(id string) (string, bool) {
	for _, prefix := range geoProfilePrefixes {
		if strings.HasPrefix(id, prefix) {
			base := id[len(prefix):]
			if looksLikeModelID(base) {
				return base, true
			}
			return "", false
		}
	}
	return "", false
}

// looksLikeModelID reports whether s has the provider.model shape of a
// Bedrock foundation-model id (amazon.nova-lite-v1:0), so a geography prefix
// is only stripped from a profile id, never from an arbitrary name that
// happens to start with "us.".
func looksLikeModelID(s string) bool {
	provider, model, ok := strings.Cut(s, ".")
	if !ok || provider == "" || model == "" {
		return false
	}
	for _, c := range provider {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
			return false
		}
	}
	return !strings.ContainsAny(s, "/ ")
}
