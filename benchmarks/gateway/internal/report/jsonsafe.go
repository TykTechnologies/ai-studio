package report

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
)

// marshalSafe is json.MarshalIndent for values that may hold NaN, which the
// summary uses for "no data" (an arm with no successful requests has no
// percentiles). encoding/json rejects NaN, and any sentinel number would read
// as a measurement, so NaN is written as null.
func marshalSafe(v any) ([]byte, error) {
	return json.MarshalIndent(nanToNil(reflect.ValueOf(v)), "", "  ")
}

func nanToNil(v reflect.Value) any {
	if !v.IsValid() {
		return nil
	}
	if m, ok := v.Interface().(json.Marshaler); ok && v.Kind() != reflect.Pointer {
		return m
	}
	switch v.Kind() {
	case reflect.Float32, reflect.Float64:
		f := v.Float()
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return nil
		}
		return f
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			return nil
		}
		return nanToNil(v.Elem())
	case reflect.Slice, reflect.Array:
		if v.Kind() == reflect.Slice && v.IsNil() {
			return nil
		}
		out := make([]any, v.Len())
		for i := range out {
			out[i] = nanToNil(v.Index(i))
		}
		return out
	case reflect.Map:
		if v.IsNil() {
			return nil
		}
		out := make(map[string]any, v.Len())
		iter := v.MapRange()
		for iter.Next() {
			out[iter.Key().String()] = nanToNil(iter.Value())
		}
		return out
	case reflect.Struct:
		out := map[string]any{}
		t := v.Type()
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			name, opts, _ := strings.Cut(f.Tag.Get("json"), ",")
			if name == "-" {
				continue
			}
			if name == "" {
				name = f.Name
			}
			fv := v.Field(i)
			if strings.Contains(opts, "omitempty") && fv.IsZero() {
				continue
			}
			out[name] = nanToNil(fv)
		}
		return out
	}
	return v.Interface()
}
