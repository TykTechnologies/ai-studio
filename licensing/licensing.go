package licensing

import (
	"fmt"
	"log"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var pubKey = `
-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA13oqkgO3RaYCMUxskU72
S5iBxTsc/KDNgcpoV3nujJuxRHC5jj3+bGaNMfpzMFCdzmtIjdkBnefLiCnqeGlT
CZCK627P1JT9ZRR9R6DGBk5Swr2ZXs0TefIR3HDJmtzBBGj63t9j6VTBYS7fnn2V
3MQG66cszXr6qPUpaN6EK61oGGs4517Ql1BzxGPdC8GJpr9teqgSLuFeeJwyqBqe
CxXxNjZ6OMjWqU2IT+lgUS97UbF1ep8iZJUdvwOmFBoWs6cY9SoTdzlzB4q90Kqs
tapRIa8HM7WWnwmI+i9uGl1QOmZfshOovOgzIZSJh1K43cdFSxgBvpO5ENyLeKai
ZwIDAQAB
-----END PUBLIC KEY-----
`

var features = map[string]interface{}{}

func FeatureSet() map[string]interface{} {
	return features
}

func Entitlement(name string) (*Feature, bool) {
	f, ok := features[name]
	if !ok {
		return nil, false
	}

	feat, err := NewFeature(f)
	if err != nil {
		slog.Error("failed to check entitlement", "name", name, "error", err)
		return nil, false
	}

	return feat, true
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

	claimsStr, ok := claims.(string)
	if !ok {
		return fmt.Errorf("invalid license format")
	}

	asArr := strings.Split(claimsStr, ",")
	claimsMap := make(map[string]interface{})
	for i, _ := range asArr {
		claimsMap[asArr[i]] = true
	}

	features = claimsMap

	// DEBUGGING LICENSE CLAIMS
	// slog.Warn("License claims", "claims", claimsMap)
	// slog.Warn("REMOVE THIS CODE BEFORE PRODUCTION")

	// features = map[string]interface{}{
	// 	"feature_chat": true,
	// }

	// END DEBUGGING LICENSE CLAIMS

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

	return claims["scope"], nil
}

func Create(ttl time.Duration, content interface{}, pKey []byte) (string, error) {
	key, err := jwt.ParseRSAPrivateKeyFromPEM(pKey)
	if err != nil {
		return "", fmt.Errorf("create: parse key: %w", err)
	}

	now := time.Now().UTC()

	claims := make(jwt.MapClaims)
	claims["scope"] = content           // Our custom data.
	claims["exp"] = now.Add(ttl).Unix() // The expiration time after which the token must be disregarded.
	claims["iat"] = now.Unix()          // The time at which the token was issued.
	claims["nbf"] = now.Unix()          // The time before which the token must be disregarded.

	token, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(key)
	if err != nil {
		return "", fmt.Errorf("create: sign token: %w", err)
	}

	return token, nil
}
