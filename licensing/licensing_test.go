package licensing

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func OneYear() time.Duration {
	day := 24 * time.Hour
	year := 365 * day

	return year
}

func TestLicenseCreate(t *testing.T) {
	privateKey, err := os.ReadFile("./keys/signing.key")
	if err != nil {
		t.Fatal(err)
	}

	jwt, err := Create(OneYear(), LicenseData(
		"david@tyk.io",
		"Tyk Technologies",
		"123",
		"321",
	), []byte(privateKey))
	if err != nil {
		t.Fatal(err)
	}

	if jwt == "" {
		t.Fatal("jwt is empty")
	}

	_, err = Validate(jwt, []byte(pubKey))
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println(jwt)

}

func LicenseData(email, org, id, licID string) map[string]interface{} {
	return map[string]interface{}{
		"email":                   email,
		"org":                     org,
		"customer_id":             id,
		"license_id":              licID,
		"ai_portal":               true,
		"max_devs":                1,
		"max_users":               1,
		"max_workspaces":          2,
		"max_llms":                1,
		"max_embedding_clients":   1,
		"max_learning_plans":      1,
		"max_resource_expanders":  1,
		"max_prompts":             1,
		"max_bots":                1,
		"max_ai_funcs":            1,
		"max_scripts":             1,
		"allow_disable_telemetry": false,
	}
}

func HundredYears() time.Duration {
	day := 24 * time.Hour
	year := 365 * day

	return 100 * year
}
