package secrets

import (
	"fmt"
	"log"
	"os"
	"reflect"
	"strings"

	"gorm.io/gorm"
)

var dbRef *gorm.DB

func SetDBRef(db *gorm.DB) {
	dbRef = db
}

// IsSecretReference checks if a string is in the format $SECRET/NAME
func IsSecretReference(value string) bool {
	if !strings.HasPrefix(value, "$") {
		return false
	}

	parts := strings.Split(value, "/")
	return len(parts) == 2 && parts[0] == "$SECRET"
}

// GetSecretReference returns a secret reference string for a given name
func GetSecretReference(name string) string {
	return fmt.Sprintf("$SECRET/%s", name)
}

func GetValue(reference string, preserveRef bool) string {
	if !strings.HasPrefix(reference, "$") {
		return reference
	}

	// $ENV/ENV_VAR
	// $SECRET/SecretName
	// etc.
	parts := strings.Split(reference, "/")
	if len(parts) != 2 {
		return reference
	}

	loc := parts[0]
	name := parts[1]

	switch loc {
	case "$ENV":
		return os.Getenv(name)
	case "$SECRET":
		// If we're already dealing with a secret reference and preserveRef is true
		if IsSecretReference(reference) && preserveRef {
			return reference
		}

		if dbRef != nil {
			val, err := GetSecretByVarName(dbRef, name, preserveRef)
			if err != nil {
				log.Println(err)
				return reference
			}

			return val.Value
		}

		log.Println("database reference is nil!")
		return reference
	default:
		return reference
	}
}

func FilterSensitiveFields(obj interface{}) interface{} {
	var ift reflect.Type
	var ifv reflect.Value

	x := reflect.TypeOf(obj)
	if x.Kind() != reflect.Ptr {
		ift = reflect.TypeOf(obj)
		ifv = reflect.ValueOf(obj)
	} else {
		ift = reflect.TypeOf(obj).Elem()
		ifv = reflect.ValueOf(obj).Elem()
	}

	for i := 0; i < ift.NumField(); i++ {

		if ifv.Field(i).Kind() == reflect.Struct {
			// fmt.Println("Iterating Subfield: ", ifv.Field(i).Type().Name())
			FilterSensitiveFields(ifv.Field(i).Addr().Interface())
			continue
		}

		// fmt.Println("Checking field: ", ift.Field(i).Name)
		t := ift.Field(i).Tag
		if t.Get("secret") == "true" {
			vName := ift.Field(i).Name
			// fmt.Println("Filtering field: ", vName)
			reflect.ValueOf(obj).Elem().FieldByName(vName).SetString("[redacted]")
		}
	}

	return obj
}

func FilterSesitiveFieldsArr(in interface{}) interface{} {
	s := reflect.ValueOf(in)
	if s.Kind() != reflect.Slice {
		panic("given a non-slice type")
	}

	if s.Len() == 0 {
		return in
	}

	x := s.Index(0).Interface()
	if reflect.ValueOf(x).Kind() == reflect.Ptr {
		for i := 0; i < s.Len(); i++ {
			FilterSensitiveFields(s.Index(i).Interface())
		}
	} else {
		for i := 0; i < s.Len(); i++ {
			FilterSensitiveFields(s.Index(i).Addr().Interface())
		}
	}

	return in
}
