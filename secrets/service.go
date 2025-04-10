package secrets

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// SecretService provides functionality for managing secrets
type SecretService struct {
	db *gorm.DB
}

// NewSecretService creates a new SecretService
func NewSecretService(db *gorm.DB) *SecretService {
	return &SecretService{
		db: db,
	}
}

// ResolveSecret resolves a secret reference to its actual value
func (s *SecretService) ResolveSecret(reference string) (string, error) {
	if !IsSecretReference(reference) {
		return reference, nil
	}

	parts := strings.Split(reference, "/")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid secret reference format: %s", reference)
	}

	name := parts[1]
	secret, err := GetSecretByVarName(s.db, name, false)
	if err != nil {
		return "", err
	}

	return secret.Value, nil
}

// CreateSecret creates a new secret
func (s *SecretService) CreateSecret(name, value string) (*Secret, error) {
	secret := &Secret{
		VarName: name,
		Value:   value,
	}

	err := CreateSecret(s.db, secret)
	if err != nil {
		return nil, err
	}

	return secret, nil
}

// GetSecret gets a secret by ID
func (s *SecretService) GetSecret(id uint) (*Secret, error) {
	var secret Secret
	err := s.db.First(&secret, id).Error
	if err != nil {
		return nil, err
	}

	return &secret, nil
}

// GetSecretByName gets a secret by name
func (s *SecretService) GetSecretByName(name string) (*Secret, error) {
	// Use the existing GetSecretByVarName function which uses the correct field name
	return GetSecretByVarName(s.db, name, false)
}

// UpdateSecret updates a secret
func (s *SecretService) UpdateSecret(id uint, value string) (*Secret, error) {
	secret, err := s.GetSecret(id)
	if err != nil {
		return nil, err
	}

	secret.Value = value
	err = s.db.Save(secret).Error
	if err != nil {
		return nil, err
	}

	return secret, nil
}

// DeleteSecret deletes a secret
func (s *SecretService) DeleteSecret(id uint) error {
	secret, err := s.GetSecret(id)
	if err != nil {
		return err
	}

	return s.db.Delete(secret).Error
}

// ListSecrets lists all secrets
func (s *SecretService) ListSecrets() ([]Secret, error) {
	var secrets []Secret
	err := s.db.Find(&secrets).Error
	if err != nil {
		return nil, err
	}

	return secrets, nil
}
