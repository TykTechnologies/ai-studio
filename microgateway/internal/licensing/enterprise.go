//go:build enterprise
// +build enterprise

package licensing

import (
	"crypto/rsa"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/TykTechnologies/ai-studio-enterprise/v2/licensing/verify"
)

// enterpriseService implements licensing for microgateway enterprise edition
type enterpriseService struct {
	config    Config
	publicKey *rsa.PublicKey
	expiresAt time.Time
	maxLLMs   verify.Limit
	lock      sync.RWMutex
	done      chan bool
}

// NewService creates a new enterprise licensing service
func NewService(config Config) Service {
	service := &enterpriseService{
		config: config,
		done:   make(chan bool),
	}

	pubKey, err := verify.DefaultPublicKey()
	if err != nil {
		log.Fatalf("Failed to parse RSA public key: %v", err)
	}
	service.publicKey = pubKey

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

// MaxLLMs returns the licence's max_llms; a licence without it allows 0
func (s *enterpriseService) MaxLLMs() (int64, bool) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	return s.maxLLMs.Max, s.maxLLMs.Unlimited
}

// validateLicense validates the JWT license token
func (s *enterpriseService) validateLicense() error {
	if s.config.LicenseKey == "" {
		return fmt.Errorf("no license key provided")
	}

	// The shared verifier checks the signature, expiry and the AI Studio
	// SKU. AI Studio enforces the limits; a standalone gateway enforces
	// max_llms on its own LLMs.
	claims, err := verify.Verify(s.config.LicenseKey, s.publicKey)
	if err != nil {
		return err
	}

	// Store expiration time and the LLM limit
	s.lock.Lock()
	s.expiresAt = claims.ExpiresAt
	s.maxLLMs = claims.Limit(verify.LimitLLMs)
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
