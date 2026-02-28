// Package aws provides an AWS Secrets Manager-backed implementation of secrets.SecretStore.
// It registers itself under the name "aws" via init().
//
// TODO: implement AWS Secrets Manager integration.
package aws

import (
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/secrets"
)

func init() {
	secrets.RegisterStore("aws", func(_ interface{}, _ string) (secrets.SecretStore, error) {
		return nil, fmt.Errorf("aws secret store is not yet implemented")
	})
}
