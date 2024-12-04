package licensing

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var pubKey = `
-----BEGIN PUBLIC KEY-----
MIICIjANBgkqhkiG9w0BAQEFAAOCAg8AMIICCgKCAgEArjpnqyvJBEBRwRowSy7X
LYHKhRbsmt+CJ1OdVwtRf5BrnBp0DZG3Mg/b4CscKk9x3YXj3QmL6VDSHgZ5Idkn
FuWNNKLfi5iqa5MPasSe0UM0falX/6gthD7wfsi6Ed2JimL/CAYUa5gnkVPgYy6U
2Oj1Qx7qXCqCqpGtxxSuowzBoudHUH4Usnm+Br8SasovLpKx5lM6JDHLVmblcxkS
S1XSHogLC2WlEy7lVnzf7gBWgoq3Zq/NJqrM2oIYlUA6kc5k2/vchnbn26jqYiqi
Qm+Q3Q+pUT1VykhyfEpDQJFAJWw57EexsNLD0Gp9DnsNtfrM5gjPZkY6gCiTBVu7
h5o0EfSPyEx+jlEmBJBAxhZIGZwX9ngc0xRpsI3IXciwyTPQeusTQqp33+I/P9mW
in3MfPysa7PWYHOZB6INB/t6QV2Q13s6i2S/J6zQpETfuQoejh2tKO7Kfqcn4Rmy
gJlunHGbnVwLeFv7ktlm9+wUPaFY6W+KGm9QDAGCEU49Gi87a9AYrZzlo3HXYfeC
eCmtq7LdYwgZzVWLfj1BPGMJftjPzu5HowHdL9hLaaYmt63VNo1sMInLMfIbRofX
FwowtmuBnKlLAH81cpStJoiOTyz6XGPM+0vtyS9YeQ4vBs6NOr7+zpdmYc6f3VH0
sEx3hBKw7U8P9Pn4nMaallsCAwEAAQ==
-----END PUBLIC KEY-----
`

var features = map[string]interface{}{}

func FeatureSet() map[string]interface{} {
	return features
}

func LicenseService() {
	for {
		if err := IsLicensed(); err != nil {
			log.Fatalf("License is not valid: %v", err)
		}
		time.Sleep(10 * time.Minute)
	}

}

func IsLicensed() error {
	licenseStr := os.Getenv("TYK_AI_LICENSE")
	if licenseStr == "" {
		return fmt.Errorf("No TYK_AI_LICENSE env var found")
	}

	claims, err := Validate(licenseStr, []byte(pubKey))
	if err != nil {
		return err
	}

	claimsMap, ok := claims.(map[string]interface{})
	if !ok {
		return fmt.Errorf("Invalid license format")
	}

	features = claimsMap

	return nil
}

func Validate(token string, pub []byte) (interface{}, error) {
	key, err := jwt.ParseRSAPublicKeyFromPEM([]byte(pubKey))
	if err != nil {
		return "", fmt.Errorf("validate: parse key: %w", err)
	}

	tok, err := jwt.Parse(token, func(jwtToken *jwt.Token) (interface{}, error) {
		if _, ok := jwtToken.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected method: %s", jwtToken.Header["alg"])
		}

		return key, nil
	})
	if err != nil {
		return nil, fmt.Errorf("validate: %w", err)
	}

	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok || !tok.Valid {
		return nil, fmt.Errorf("validate: invalid")
	}

	return claims["dat"], nil
}

func Create(ttl time.Duration, content interface{}, pKey []byte) (string, error) {
	key, err := jwt.ParseRSAPrivateKeyFromPEM(pKey)
	if err != nil {
		return "", fmt.Errorf("create: parse key: %w", err)
	}

	now := time.Now().UTC()

	claims := make(jwt.MapClaims)
	claims["dat"] = content             // Our custom data.
	claims["exp"] = now.Add(ttl).Unix() // The expiration time after which the token must be disregarded.
	claims["iat"] = now.Unix()          // The time at which the token was issued.
	claims["nbf"] = now.Unix()          // The time before which the token must be disregarded.

	token, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(key)
	if err != nil {
		return "", fmt.Errorf("create: sign token: %w", err)
	}

	return token, nil
}
