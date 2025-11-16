//go:build !enterprise
// +build !enterprise

package licensing

// communityService is a stub implementation for Community Edition
type communityService struct{}

// NewService creates a new licensing service
// CE: Returns always-valid stub
func NewService(config Config) Service {
	return &communityService{}
}

// Start is a no-op for community edition
func (s *communityService) Start() error {
	return nil
}

// Stop is a no-op for community edition
func (s *communityService) Stop() {
}

// IsValid always returns true for community edition
func (s *communityService) IsValid() bool {
	return true
}

// DaysLeft returns -1 for community edition (never expires)
func (s *communityService) DaysLeft() int {
	return -1
}
