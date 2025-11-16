//go:build enterprise
// +build enterprise

package licensing

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

// RSA public key for JWT verification (same as main service)
const publicKey = `
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

// enterpriseService implements licensing for microgateway enterprise edition
type enterpriseService struct {
	config    Config
	publicKey *rsa.PublicKey
	expiresAt time.Time
	lock      sync.RWMutex
	done      chan bool
}

// NewService creates a new enterprise licensing service
func NewService(config Config) Service {
	service := &enterpriseService{
		config: config,
		done:   make(chan bool),
	}

	// Parse RSA public key
	block, _ := pem.Decode([]byte(publicKey))
	if block == nil {
		log.Fatalf("Failed to parse PEM block containing the public key")
	}

	pubKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		log.Fatalf("Failed to parse RSA public key: %v", err)
	}

	rsaPubKey, ok := pubKey.(*rsa.PublicKey)
	if !ok {
		log.Fatalf("Public key is not RSA")
	}
	service.publicKey = rsaPubKey

	return service
}

// Start validates license and starts periodic checks
func (s *enterpriseService) Start() error {
	// Validate license at boot
	if err := s.validateLicense(); err != nil {
		log.Fatalf("Microgateway license validation failed at boot: %v", err)
		return err
	}

	log.Printf("Microgateway license validated successfully. Expires: %s", s.expiresAt.Format(time.RFC3339))

	// Start periodic validation
	go s.periodicValidation()

	return nil
}

// Stop gracefully stops background processes
func (s *enterpriseService) Stop() {
	close(s.done)
}

// IsValid returns whether the license is currently valid
func (s *enterpriseService) IsValid() bool {
	s.lock.RLock()
	defer s.lock.RUnlock()

	return time.Now().Before(s.expiresAt)
}

// DaysLeft returns the number of days until license expiry
func (s *enterpriseService) DaysLeft() int {
	s.lock.RLock()
	defer s.lock.RUnlock()

	duration := time.Until(s.expiresAt)
	days := int(duration.Hours() / 24)
	if days < 0 {
		return 0
	}
	return days
}

// validateLicense validates the JWT license token
func (s *enterpriseService) validateLicense() error {
	if s.config.LicenseKey == "" {
		return fmt.Errorf("no license key provided")
	}

	// Parse JWT token
	token, err := jwt.Parse(s.config.LicenseKey, func(token *jwt.Token) (interface{}, error) {
		// Verify signing method
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.publicKey, nil
	})

	if err != nil {
		return fmt.Errorf("invalid license signature: %v", err)
	}

	if !token.Valid {
		return fmt.Errorf("invalid license")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return fmt.Errorf("invalid license claims")
	}

	// Parse expiration
	exp, ok := claims["exp"].(float64)
	if !ok {
		return fmt.Errorf("missing or invalid exp claim")
	}
	expiresAt := time.Unix(int64(exp), 0)

	// Check if expired
	if time.Now().After(expiresAt) {
		return fmt.Errorf("license expired")
	}

	// Store expiration time
	s.lock.Lock()
	s.expiresAt = expiresAt
	s.lock.Unlock()

	return nil
}

// periodicValidation re-validates the license periodically
func (s *enterpriseService) periodicValidation() {
	ticker := time.NewTicker(s.config.ValidityCheckPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := s.validateLicense(); err != nil {
				log.Fatalf("Microgateway license validation failed during periodic check: %v", err)
			}
			log.Printf("Microgateway license re-validated successfully. Days left: %d", s.DaysLeft())
		case <-s.done:
			return
		}
	}
}
