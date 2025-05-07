package licensing

import "fmt"

func NewFeature(value interface{}) (*Feature, error) {
	switch value.(type) {
	case string:
		return processValue(value.(string))
	case int:
		return &Feature{tp: "int", valInt: value.(int)}, nil
	case float64:
		return &Feature{tp: "int", valInt: int(value.(float64))}, nil
	case float32:
		return &Feature{tp: "int", valInt: int(value.(float32))}, nil
	case bool:
		return &Feature{tp: "bool", valBool: value.(bool)}, nil
	default:
		return nil, fmt.Errorf("unsupported type (%T)", value)
	}
}

func processValue(value string) (*Feature, error) {
	switch value {
	case "true":
		return &Feature{tp: "bool", valBool: true}, nil
	case "false":
		return &Feature{tp: "bool", valBool: false}, nil
	default:
		return &Feature{tp: "string", valString: value}, nil
	}
}

func (f *Feature) Bool() bool {
	return f.valBool
}

func (f *Feature) String() string {
	return f.valString
}

func (f *Feature) Int() int {
	return f.valInt
}
