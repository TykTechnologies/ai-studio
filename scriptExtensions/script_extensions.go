//go:build !enterprise

package scriptExtensions

import (
	"github.com/TykTechnologies/midsommar/v2/services"
)

type SendFunc func(msg string) error
type commandFunc func(command string, data map[string]interface{}) error

// GetModules returns an empty map in CE (enterprise feature)
// This function exists for compatibility but scripting is disabled in CE
func GetModules(serviceRef services.ServiceInterface) map[string]interface{} {
	return map[string]interface{}{}
}
