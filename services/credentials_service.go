package services

import (
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
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
	// An edge may hold a refusal for this key: it drops it.
	s.credentialChanged(credential)

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

// UpdateCredential updates an existing credential. A regenerated secret
// replaces the old key at once: edges are told about the old one (as
// deleted) as well as the new one. The old secret is read and replaced in
// one transaction, with the row locked on PostgreSQL, so the old key the
// event names is the one this write replaced.
func (s *Service) UpdateCredential(credential *models.Credential) error {
	var replaced *eventbridge.CredentialChanged
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		old := &models.Credential{}
		if err := lockCredential(tx).First(old, credential.ID).Error; err == nil && old.Secret != credential.Secret {
			if change, ok := s.credentialChange(tx, old, nil, true); ok {
				replaced = &change
			}
		}
		return credential.Update(tx)
	})
	if err != nil {
		return err
	}
	if replaced != nil {
		s.publishCredentialChanged(*replaced)
	}
	s.credentialChanged(credential)
	return nil
}

// DeleteCredential deletes a credential. The edges' hash is of the secret:
// it is taken from the row in the transaction that deletes it.
func (s *Service) DeleteCredential(id uint) error {
	var change eventbridge.CredentialChanged
	var ok bool
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		credential := &models.Credential{}
		if err := lockCredential(tx).First(credential, id).Error; err != nil {
			return err
		}
		change, ok = s.credentialChange(tx, credential, nil, true)
		return credential.Delete(tx)
	})
	if err != nil {
		return err
	}
	if ok {
		s.publishCredentialChanged(change)
	}
	return nil
}

// ActivateCredential activates a credential
func (s *Service) ActivateCredential(id uint) error {
	credential, err := s.GetCredentialByID(id)
	if err != nil {
		return err
	}
	if err := credential.Activate(s.DB); err != nil {
		return err
	}
	s.credentialChanged(credential)
	return nil
}

// DeactivateCredential deactivates a credential
func (s *Service) DeactivateCredential(id uint) error {
	credential, err := s.GetCredentialByID(id)
	if err != nil {
		return err
	}
	if err := credential.Deactivate(s.DB); err != nil {
		return err
	}
	s.credentialChanged(credential)
	return nil
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
