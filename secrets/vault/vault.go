// Package vault provides a HashiCorp Vault-backed implementation of secrets.SecretStore.
// It registers itself under the name "vault" via init().
//
// TODO: implement Vault integration.
package vault

import (
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/secrets"
)

func init() {
	secrets.RegisterStore("vault", func(_ interface{}, _ string) (secrets.SecretStore, error) {
		return nil, fmt.Errorf("vault secret store is not yet implemented")
	})
}
