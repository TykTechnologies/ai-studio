package services

import (
	"github.com/TykTechnologies/midsommar/v2/models"
)

// CreateCredential creates a new credential
func (s *Service) CreateCredential() (*models.Credential, error) {
	credential, err := models.NewCredential()
	if err != nil {
		return nil, err
	}

	if err := credential.Create(s.DB); err != nil {
		return nil, err
	}

	return credential, nil
}

// GetCredentialByID retrieves a credential by its ID
func (s *Service) GetCredentialByID(id uint) (*models.Credential, error) {
	credential := &models.Credential{}
	if err := credential.Get(s.DB, id); err != nil {
		return nil, err
	}
	return credential, nil
}

// GetCredentialByKeyID retrieves a credential by its KeyID
func (s *Service) GetCredentialByKeyID(keyID string) (*models.Credential, error) {
	credential := &models.Credential{}
	if err := credential.GetByKeyID(s.DB, keyID); err != nil {
		return nil, err
	}
	return credential, nil
}

func (s *Service) GetCredentialBySecret(secret string) (*models.Credential, error) {
	credential := &models.Credential{}
	if err := credential.GetBySecret(s.DB, secret); err != nil {
		return nil, err
	}
	return credential, nil
}

// UpdateCredential updates an existing credential
func (s *Service) UpdateCredential(credential *models.Credential) error {
	return credential.Update(s.DB)
}

// DeleteCredential deletes a credential
func (s *Service) DeleteCredential(id uint) error {
	credential, err := s.GetCredentialByID(id)
	if err != nil {
		return err
	}
	return credential.Delete(s.DB)
}

// ActivateCredential activates a credential
func (s *Service) ActivateCredential(id uint) error {
	credential, err := s.GetCredentialByID(id)
	if err != nil {
		return err
	}
	return credential.Activate(s.DB)
}

// DeactivateCredential deactivates a credential
func (s *Service) DeactivateCredential(id uint) error {
	credential, err := s.GetCredentialByID(id)
	if err != nil {
		return err
	}
	return credential.Deactivate(s.DB)
}

// GetAllCredentials retrieves all credentials
func (s *Service) GetAllCredentials(pageSize int, pageNumber int, all bool) (models.Credentials, int64, int, error) {
	var credentials models.Credentials
	totalCount, totalPages, err := credentials.GetAll(s.DB, pageSize, pageNumber, all)
	if err != nil {
		return nil, 0, 0, err
	}
	return credentials, totalCount, totalPages, nil
}

// GetActiveCredentials retrieves all active credentials
func (s *Service) GetActiveCredentials() (models.Credentials, error) {
	var credentials models.Credentials
	if err := credentials.GetActive(s.DB); err != nil {
		return nil, err
	}
	return credentials, nil
}
