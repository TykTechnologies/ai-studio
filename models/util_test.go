package models

import (
	"database/sql/driver"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestJSONMapScan tests the Scan method of JSONMap
func TestJSONMapScan(t *testing.T) {
	tests := []struct {
		name    string
		input   interface{}
		want    JSONMap
		wantErr bool
	}{
		{
			name:  "valid JSON bytes",
			input: []byte(`{"key1": "value1", "key2": 123}`),
			want:  JSONMap{"key1": "value1", "key2": float64(123)},
		},
		{
			name:  "valid JSON string",
			input: `{"key1": "value1", "key2": 123}`,
			want:  JSONMap{"key1": "value1", "key2": float64(123)},
		},
		{
			name:    "invalid JSON bytes",
			input:   []byte(`{"key1": "value1", "key2": 123`), // Missing closing brace
			wantErr: true,
		},
		{
			name:    "invalid JSON string",
			input:   `{"key1": "value1", "key2": 123`, // Missing closing brace
			wantErr: true,
		},
		{
			name:    "unsupported type",
			input:   123,
			wantErr: true,
		},
		{
			name:  "empty JSON object",
			input: []byte(`{}`),
			want:  JSONMap{},
		},
		{
			name:  "complex nested structure",
			input: []byte(`{"key1": "value1", "key2": {"nested1": "nestedValue", "nested2": [1, 2, 3]}}`),
			want:  JSONMap{"key1": "value1", "key2": map[string]interface{}{"nested1": "nestedValue", "nested2": []interface{}{float64(1), float64(2), float64(3)}}},
		},
		{
			name:  "JSON with unicode",
			input: []byte(`{"key1": "值1", "key2": "值2"}`),
			want:  JSONMap{"key1": "值1", "key2": "值2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var j JSONMap
			err := j.Scan(tt.input)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, j)
			}
		})
	}
}

// TestJSONMapValue tests the Value method of JSONMap
func TestJSONMapValue(t *testing.T) {
	tests := []struct {
		name    string
		j       JSONMap
		want    driver.Value
		wantErr bool
	}{
		{
			name: "non-empty map",
			j:    JSONMap{"key1": "value1", "key2": 123},
			want: `{"key1":"value1","key2":123}`,
		},
		{
			name: "empty map",
			j:    JSONMap{},
			want: `{}`,
		},
		{
			name: "nil map",
			j:    nil,
			want: "null",
		},
		{
			name: "complex nested structure",
			j:    JSONMap{"key1": "value1", "key2": map[string]interface{}{"nested1": "nestedValue", "nested2": []interface{}{1, 2, 3}}},
			want: `{"key1":"value1","key2":{"nested1":"nestedValue","nested2":[1,2,3]}}`,
		},
		{
			name: "map with unicode",
			j:    JSONMap{"key1": "值1", "key2": "值2"},
			want: `{"key1":"值1","key2":"值2"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.j.Value()

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

// TestStringMapScan tests the Scan method of StringMap
func TestStringMapScan(t *testing.T) {
	tests := []struct {
		name    string
		input   interface{}
		want    StringMap
		wantErr bool
	}{
		{
			name:  "valid JSON bytes",
			input: []byte(`{"key1": "value1", "key2": "value2"}`),
			want:  StringMap{"key1": "value1", "key2": "value2"},
		},
		{
			name:  "valid JSON string",
			input: `{"key1": "value1", "key2": "value2"}`,
			want:  StringMap{"key1": "value1", "key2": "value2"},
		},
		{
			name:    "invalid JSON bytes",
			input:   []byte(`{"key1": "value1", "key2": "value2"`), // Missing closing brace
			wantErr: true,
		},
		{
			name:    "invalid JSON string",
			input:   `{"key1": "value1", "key2": "value2"`, // Missing closing brace
			wantErr: true,
		},
		{
			name:    "unsupported type",
			input:   123,
			wantErr: true,
		},
		{
			name:  "empty JSON object",
			input: []byte(`{}`),
			want:  StringMap{},
		},
		{
			name:    "non-string values",
			input:   []byte(`{"key1": "value1", "key2": 123}`),
			wantErr: true, // Should fail because StringMap expects string values
		},
		{
			name:  "JSON with unicode",
			input: []byte(`{"key1": "值1", "key2": "值2"}`),
			want:  StringMap{"key1": "值1", "key2": "值2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s StringMap
			err := s.Scan(tt.input)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, s)
			}
		})
	}
}

// TestStringMapValue tests the Value method of StringMap
func TestStringMapValue(t *testing.T) {
	tests := []struct {
		name    string
		s       StringMap
		want    driver.Value
		wantErr bool
	}{
		{
			name: "non-empty map",
			s:    StringMap{"key1": "value1", "key2": "value2"},
			want: `{"key1":"value1","key2":"value2"}`,
		},
		{
			name: "empty map",
			s:    StringMap{},
			want: `{}`,
		},
		{
			name: "nil map",
			s:    nil,
			want: "null",
		},
		{
			name: "map with unicode",
			s:    StringMap{"key1": "值1", "key2": "值2"},
			want: `{"key1":"值1","key2":"值2"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.s.Value()

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

// TestJSONValue tests the JSONValue helper function
func TestJSONValue(t *testing.T) {
	tests := []struct {
		name    string
		v       interface{}
		want    driver.Value
		wantErr bool
	}{
		{
			name: "nil value",
			v:    nil,
			want: nil,
		},
		{
			name: "simple map",
			v:    map[string]interface{}{"key1": "value1", "key2": 123},
			want: `{"key1":"value1","key2":123}`,
		},
		{
			name: "string map",
			v:    map[string]string{"key1": "value1", "key2": "value2"},
			want: `{"key1":"value1","key2":"value2"}`,
		},
		{
			name: "slice",
			v:    []string{"value1", "value2"},
			want: `["value1","value2"]`,
		},
		{
			name: "complex nested structure",
			v:    map[string]interface{}{"key1": "value1", "key2": map[string]interface{}{"nested1": "nestedValue", "nested2": []interface{}{1, 2, 3}}},
			want: `{"key1":"value1","key2":{"nested1":"nestedValue","nested2":[1,2,3]}}`,
		},
		{
			name: "struct",
			v: struct {
				Key1 string `json:"key1"`
				Key2 int    `json:"key2"`
			}{"value1", 123},
			want: `{"key1":"value1","key2":123}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := JSONValue(tt.v)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

// TestJSONScan tests the JSONScan helper function
func TestJSONScan(t *testing.T) {
	tests := []struct {
		name    string
		value   interface{}
		dest    interface{}
		wantErr bool
	}{
		{
			name:  "byte array to map",
			value: []byte(`{"key1":"value1","key2":123}`),
			dest:  &map[string]interface{}{},
		},
		{
			name:  "string to map",
			value: `{"key1":"value1","key2":123}`,
			dest:  &map[string]interface{}{},
		},
		{
			name:    "invalid JSON byte array",
			value:   []byte(`{"key1":"value1","key2":123`), // Missing closing brace
			dest:    &map[string]interface{}{},
			wantErr: true,
		},
		{
			name:    "invalid JSON string",
			value:   `{"key1":"value1","key2":123`, // Missing closing brace
			dest:    &map[string]interface{}{},
			wantErr: true,
		},
		{
			name:    "unsupported type",
			value:   123,
			dest:    &map[string]interface{}{},
			wantErr: true,
		},
		{
			name:  "byte array to struct",
			value: []byte(`{"Key1":"value1","Key2":123}`),
			dest: &struct {
				Key1 string
				Key2 int
			}{},
		},
		{
			name:  "string to struct",
			value: `{"Key1":"value1","Key2":123}`,
			dest: &struct {
				Key1 string
				Key2 int
			}{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := JSONScan(tt.value, tt.dest)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// Check if dest is non-nil and has values
				switch d := tt.dest.(type) {
				case *map[string]interface{}:
					assert.NotEmpty(t, *d, "JSONScan() did not populate the destination map")
				case *struct {
					Key1 string
					Key2 int
				}:
					assert.NotEmpty(t, d.Key1, "JSONScan() did not populate the destination struct field Key1")
					assert.NotZero(t, d.Key2, "JSONScan() did not populate the destination struct field Key2")
				}
			}
		})
	}
}

// Define a custom type that will fail during unmarshaling
type FailingUnmarshaler struct{}

// UnmarshalJSON implements the json.Unmarshaler interface
func (f *FailingUnmarshaler) UnmarshalJSON(data []byte) error {
	return errors.New("intentional unmarshal error")
}

// TestJSONScanWithUnmarshalError tests JSONScan with a destination that will cause an Unmarshal error
func TestJSONScanWithUnmarshalError(t *testing.T) {
	// Test with the failing unmarshaler
	var dest FailingUnmarshaler
	err := JSONScan([]byte(`{}`), &dest)

	assert.Error(t, err, "JSONScan() did not return an error with failing unmarshaler")
}

// TestPasswordFunctions tests the password hashing and validation functions
func TestPasswordFunctions(t *testing.T) {
	// Save the original functions
	originalHashPassword := HashPassword
	originalIsPasswordValid := IsPasswordValid

	// Restore the original functions after the test
	defer func() {
		HashPassword = originalHashPassword
		IsPasswordValid = originalIsPasswordValid
	}()

	// Test the actual implementations
	password := "testPassword123"

	// Test hashPassword
	hashedPassword, err := hashPassword(password)
	assert.NoError(t, err)
	assert.NotEqual(t, password, hashedPassword, "hashPassword() did not hash the password")

	// Test isPasswordValid
	assert.True(t, isPasswordValid(password, hashedPassword), "isPasswordValid() failed to validate a correct password")
	assert.False(t, isPasswordValid("wrongPassword", hashedPassword), "isPasswordValid() validated an incorrect password")
}

func TestSameIDs(t *testing.T) {
	// Test same IDs in same order
	a := []uint{1, 2, 3, 4, 5}
	b := []uint{1, 2, 3, 4, 5}
	assert.True(t, SameIDs(a, b))

	// Test same IDs in different order
	a = []uint{1, 2, 3, 4, 5}
	b = []uint{5, 4, 3, 2, 1}
	assert.True(t, SameIDs(a, b))

	// Test different lengths
	a = []uint{1, 2, 3}
	b = []uint{1, 2, 3, 4}
	assert.False(t, SameIDs(a, b))

	// Test different IDs
	a = []uint{1, 2, 3}
	b = []uint{1, 2, 4}
	assert.False(t, SameIDs(a, b))

	// Test with empty slices
	a = []uint{}
	b = []uint{}
	assert.True(t, SameIDs(a, b))

	// Test with nil slices
	assert.True(t, SameIDs(nil, nil))

	// Test with duplicate IDs
	a = []uint{1, 2, 2, 3}
	b = []uint{1, 2, 3, 3}
	assert.False(t, SameIDs(a, b))

	// Test with duplicate IDs (same duplicates)
	a = []uint{1, 2, 2, 3}
	b = []uint{3, 2, 1, 2}
	assert.True(t, SameIDs(a, b))
}
