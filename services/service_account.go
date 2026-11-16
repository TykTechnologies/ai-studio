package services

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gosimple/slug"
)

// ServiceAccountCode is the machine-readable code ("errors[0].code") of
// every refusal that exists because the account is a service account.
const ServiceAccountCode = "service_account"

// serviceAccountRefusal is a 422 for a request a service account cannot
// take (an email, a password, Portal access, a team).
func serviceAccountRefusal(msg string) error {
	return helpers.NewUnprocessableEntityError(msg, ServiceAccountCode)
}

// serviceAccountTeamsRefusal is the 422 form of models.ErrServiceAccountTeams.
func serviceAccountTeamsRefusal() error {
	return serviceAccountRefusal(models.ErrServiceAccountTeams.Error())
}

// validAccountType refuses an unknown account type.
func validAccountType(t string) error {
	switch t {
	case models.AccountTypeUser, models.AccountTypeService:
		return nil
	}
	return helpers.NewUnprocessableEntityError(fmt.Sprintf("account_type must be %q or omitted", models.AccountTypeService), "invalid_account_type")
}

// ServiceAccountEmail generates the address a service account is created
// with: svc-<name slug>-<random>@service-account.invalid. It is never
// deliverable and never matches an identity provider's user.
func ServiceAccountEmail(name string) (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	s := slug.Make(name)
	if len(s) > 40 {
		s = s[:40]
	}
	if s == "" {
		s = "account"
	}
	return fmt.Sprintf("svc-%s-%s@%s", s, hex.EncodeToString(b), models.ServiceAccountEmailDomain), nil
}

// checkServiceAccountDTO refuses what a service account cannot have. For an
// update, existing is the stored account and the DTO carries the full
// desired state (the API fills omitted attributes from the stored user).
func checkServiceAccountDTO(dto UserDTO, existing *models.User) error {
	if existing == nil && dto.Email != "" {
		return serviceAccountRefusal("a service account's email is generated; do not send one")
	}
	if existing != nil && dto.Email != existing.Email {
		return serviceAccountRefusal("a service account's email cannot be changed")
	}
	if dto.Password != "" {
		return serviceAccountRefusal("a service account has no password; it authenticates with its API key")
	}
	switch {
	case dto.ShowPortal:
		return serviceAccountRefusal("a service account has no Portal access")
	case dto.ShowChat:
		return serviceAccountRefusal("a service account has no Chat access")
	case dto.NotificationsEnabled:
		return serviceAccountRefusal("a service account receives no notifications")
	case dto.AccessToSSOConfig:
		return serviceAccountRefusal("a service account has no identity provider access")
	case len(dto.Groups) > 0:
		return serviceAccountTeamsRefusal()
	}
	return nil
}

// keepServiceAccountInvariants puts back, after an object hook, what no
// one may change on a service account (and the account type of any user):
// a plugin cannot turn a user into a service account or back, nor give a
// service account a password, a deliverable address, Portal, Chat or
// notifications.
func keepServiceAccountInvariants(user *models.User, accountType, email string) {
	user.AccountType = accountType
	if accountType != models.AccountTypeService {
		return
	}
	user.Email = email
	user.Password = ""
	user.EmailVerified = true
	user.ShowPortal = false
	user.ShowChat = false
	user.NotificationsEnabled = false
	user.AccessToSSOConfig = false
	user.Groups = nil
}
