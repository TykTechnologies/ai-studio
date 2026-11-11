package database

// API tokens are stored and looked up as hashes; a gateway sets the hasher
// from ENCRYPTION_KEY when it starts. The tests of this package set it here.
func init() {
	if err := SetAPITokenHasherFromKey(testAPITokenKey); err != nil {
		panic(err)
	}
}

const testAPITokenKey = "0123456789abcdef0123456789abcdef"
