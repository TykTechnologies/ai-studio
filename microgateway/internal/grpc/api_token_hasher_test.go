package grpc

import "github.com/TykTechnologies/midsommar/microgateway/internal/database"

// API tokens are stored and looked up as hashes; a gateway sets the hasher
// from ENCRYPTION_KEY when it starts (services.prepareAPITokenHashing). The
// tests of this package build their databases directly, so they set it here.
func init() {
	if err := database.SetAPITokenHasherFromKey("0123456789abcdef0123456789abcdef"); err != nil {
		panic(err)
	}
}
