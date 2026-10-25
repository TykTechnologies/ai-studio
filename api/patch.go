package api

import (
	"encoding/json"
	"errors"
	"reflect"

	"github.com/gin-gonic/gin"
)

// PATCH changes only the attributes its body holds (96e, as dg5 for Apps
// and mergeLLMPatch for LLMs). Handlers bind the body onto inputs of plain
// values, where an omitted attribute is indistinguishable from its zero
// value, so each PATCH handler builds the input of the stored record
// (base) and lays the body's attributes over it with patchOnto: an omitted
// attribute keeps the stored value, a present one, an explicit empty list,
// 0, false, "" or null included, replaces it.

// patchOnto sets *dst to base with every key of present replaced by the
// body's value for it. dst is a pointer to a value of base's type; keys
// are the JSON names of its fields.
func patchOnto(dst any, base any, present map[string]json.RawMessage) error {
	b, err := json.Marshal(base)
	if err != nil {
		return err
	}
	merged := map[string]json.RawMessage{}
	if err := json.Unmarshal(b, &merged); err != nil {
		return err
	}
	for k, v := range present {
		merged[k] = v
	}
	out, err := json.Marshal(merged)
	if err != nil {
		return err
	}
	v := reflect.ValueOf(dst)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return errors.New("patchOnto: dst must be a non-nil pointer")
	}
	v.Elem().Set(reflect.Zero(v.Elem().Type()))
	return json.Unmarshal(out, dst)
}

// boundBodyKeys returns the top-level keys of a body bound with
// ShouldBindBodyWith, for inputs that are not JSON:API documents. Like
// boundAttributeKeys, it fails rather than report "nothing present".
func boundBodyKeys(c *gin.Context) (map[string]json.RawMessage, error) {
	raw, _ := c.Get(gin.BodyBytesKey)
	body, ok := raw.([]byte)
	if !ok {
		return nil, errors.New("request body is unavailable")
	}
	var present map[string]json.RawMessage
	if err := json.Unmarshal(body, &present); err != nil || present == nil {
		return nil, errors.New("request body is not a JSON object")
	}
	return present, nil
}
