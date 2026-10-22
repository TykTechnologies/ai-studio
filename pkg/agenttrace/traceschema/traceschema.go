// Package traceschema holds the JSON Schema of the agent-trace export format
// (application/vnd.tyk.agent-trace+json; v=1, spec §7). The serializer is
// Enterprise; the schema is public so clients and fixture authors can
// validate against it. docs/site/public/schemas/agent-trace-v1.json is a
// published copy, kept identical by a test.
package traceschema

import (
	_ "embed"
	"fmt"

	"github.com/xeipuuv/gojsonschema"
)

// MediaTypeV1 is the media type of a version 1 document.
const MediaTypeV1 = "application/vnd.tyk.agent-trace+json; v=1"

// V1 is the JSON Schema (draft-07) of a version 1 document.
//
//go:embed agent-trace-v1.json
var V1 []byte

// ValidateV1 validates doc against the version 1 schema and returns an error
// listing every violation.
func ValidateV1(doc []byte) error {
	res, err := gojsonschema.Validate(gojsonschema.NewBytesLoader(V1), gojsonschema.NewBytesLoader(doc))
	if err != nil {
		return fmt.Errorf("agent-trace schema: %w", err)
	}
	if res.Valid() {
		return nil
	}
	msg := "agent-trace document does not match schema v1:"
	for _, e := range res.Errors() {
		msg += "\n  - " + e.String()
	}
	return fmt.Errorf("%s", msg)
}
