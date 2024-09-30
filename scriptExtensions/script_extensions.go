package scriptExtensions

import (
	"github.com/TykTechnologies/midsommar/v2/scriptExtensions/httpcaller"
	"github.com/d5/tengo/v2"
)

type SendFunc func(msg string) error
type commandFunc func(command string, data map[string]interface{}) error

func GetModules() map[string]tengo.Object {
	return map[string]tengo.Object{
		"makeHTTPRequest": &tengo.UserFunction{
			Name: "makeHTTPRequest", Value: httpcaller.NewHttpCaller().Call},
	}
}
