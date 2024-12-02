package secrets

import (
	"fmt"
	"testing"
)

type PointToMe struct{}

func TestFilter(t *testing.T) {
	type MyEmbeddedStructWithSecrets struct {
		SubField1 int
		SubField2 *PointToMe
		SubField3 string `secret:"true"`
	}

	type MyStructWithSecrets struct {
		Field1 int
		Field2 *PointToMe
		Field3 string `secret:"true"`
		MyEmbeddedStructWithSecrets
	}

	x := MyStructWithSecrets{
		Field1: 1,
		Field2: &PointToMe{},
		Field3: "foo",
		MyEmbeddedStructWithSecrets: MyEmbeddedStructWithSecrets{
			SubField1: 2,
			SubField2: &PointToMe{},
			SubField3: "bar",
		},
	}

	FilterSensitiveFields(&x)

	if x.Field3 == "foo" {
		t.Errorf("Field3 should have been filtered")
	}

	if x.SubField3 == "bar" {
		t.Errorf("SubField3 should have been filtered")
	}

	fmt.Println(x)
}

func TestArrFilter(t *testing.T) {
	type MyStructWithSecrets struct {
		Field1 int
		Field2 *PointToMe
		Field3 string `secret:"true"`
	}

	x := MyStructWithSecrets{
		Field1: 1,
		Field2: &PointToMe{},
		Field3: "foo",
	}
	x1 := MyStructWithSecrets{
		Field1: 2,
		Field2: &PointToMe{},
		Field3: "bar",
	}

	arr := []MyStructWithSecrets{x, x1}

	fmt.Println("Test with pointer")
	FilterSesitiveFieldsArr(arr)
	for _, v := range arr {
		fmt.Printf("%v: %v\n", v.Field1, v.Field3)
	}

	fmt.Println("Test with non-pointer")
	x2 := MyStructWithSecrets{
		Field1: 1,
		Field2: &PointToMe{},
		Field3: "foo",
	}
	x3 := MyStructWithSecrets{
		Field1: 2,
		Field2: &PointToMe{},
		Field3: "bar",
	}
	arr2 := []MyStructWithSecrets{x2, x3}

	FilterSesitiveFieldsArr(arr2)
	for _, v := range arr2 {
		fmt.Printf("%v: %v\n", v.Field1, v.Field3)
	}

	fmt.Println(x.Field1)
	fmt.Println(x.Field3)
}
