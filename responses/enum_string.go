package responses

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// EnumString is a protobuf enum field that a vendor may serialize either as its
// name ("STOP") or as its ordinal (1).
//
// Gemini does both, depending on which transport served the request. Typing
// these fields as a plain string made every affected response fail to decode
// with "cannot unmarshal number into Go struct field ... of type string", and
// because analytics decoding is best-effort, the whole record was silently
// dropped - so calls that worked perfectly for the caller produced no usage or
// cost data at all.
type EnumString string

// String returns the value as received: the enum name where the vendor sent
// one, the decimal ordinal where it sent a number.
func (e EnumString) String() string { return string(e) }

func (e *EnumString) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		*e = ""
		return nil
	}

	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*e = EnumString(s)
		return nil
	}

	var n json.Number
	if err := json.Unmarshal(data, &n); err == nil {
		*e = EnumString(n.String())
		return nil
	}

	// A bool or an object here means the field is not an enum at all, which is
	// worth surfacing rather than silently blanking.
	return fmt.Errorf("enum field is neither a string nor a number: %s", strconv.Quote(string(data)))
}

func (e EnumString) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(e))
}
