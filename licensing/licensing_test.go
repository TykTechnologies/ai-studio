package licensing

import (
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
	t.Skip()
	privateKey, err := os.ReadFile("./keys/license_key.key")
	if err != nil {
		t.Fatal(err)
	}

	allLic, _ := Create(OneYear(),
		"feature_portal,feature_chat,feature_gateway",
		[]byte(privateKey))
	portal_gateway, _ := Create(OneYear(), "feature_portal,feature_gateway", []byte(privateKey))
	chat_only, _ := Create(OneYear(), "feature_chat", []byte(privateKey))
	os.WriteFile("keys/all.lic", []byte(allLic), 0644)
	os.WriteFile("keys/portal_gateway.lic", []byte(portal_gateway), 0644)
	os.WriteFile("keys/chat_only.lic", []byte(chat_only), 0644)
}

func TestLIcenseValidateFeatures(t *testing.T) {
	t.Skip()
	licenseData := map[string]interface{}{
		"feature_portal":  true,
		"feature_gateway": true,
		"num_seats":       5,
	}

	privateKey, err := os.ReadFile("./keys/signing.key")
	if err != nil {
		t.Fatal(err)
	}

	lic, err := Create(OneYear(), licenseData, []byte(privateKey))
	if err != nil {
		t.Fatal(err)
	}

	os.Setenv("TYK_AI_LICENSE", string(lic))

	err = IsLicensed()
	if err != nil {
		t.Fatal(err)
	}

	// test some features
	portal, ok := Entitlement("feature_portal")
	if !ok {
		t.Fatal("feature_portal not found")
	}

	if !portal.Bool() {
		t.Fatal("feature_portal should be true")
	}

	gateway, ok := Entitlement("feature_gateway")
	if !ok {
		t.Fatal("feature_gateway not found")
	}

	if !gateway.Bool() {
		t.Fatal("feature_gateway should be true")
	}

	seats, ok := Entitlement("num_seats")
	if !ok {
		t.Fatal("num_seats not found")
	}

	if seats.Int() != 5 {
		t.Fatal("num_seats should be 5")
	}

}

func LicenseData(email, org, id, licID string) map[string]interface{} {
	return map[string]interface{}{
		"email":           email,
		"org":             org,
		"customer_id":     id,
		"license_id":      licID,
		"feature_portal":  true,
		"feature_gateway": true,
		"num_seats":       5,
	}
}

func HundredYears() time.Duration {
	day := 24 * time.Hour
	year := 365 * day

	return 100 * year
}
