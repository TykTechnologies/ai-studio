package services

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
)

// HostIdentity is a user as the host application Studio is embedded in has
// authenticated them.
type HostIdentity struct {
	// Subject is the host's stable identifier for the user. Required.
	Subject string
	// Email and Name are kept in step with the host. Email is required.
	Email string
	Name  string
	// Admin makes the user a Studio administrator (an Administrator role
	// binding in Enterprise). The host is authoritative: false revokes it.
	Admin bool
	// Groups names the Studio groups (teams) the user belongs to, replacing
	// their memberships; every user also keeps the Default group. Nil leaves
	// memberships to Studio's own administration. Names Studio does not know
	// are ignored.
	Groups []string
}

var (
	// ErrHostIdentityInvalid is returned for an identity without a subject
	// or email.
	ErrHostIdentityInvalid = errors.New("host identity needs a subject and an email")
	// ErrHostIdentityConflict is returned when the identity's email belongs
	// to a Studio user already linked to a different host subject.
	ErrHostIdentityConflict = errors.New("email already belongs to a user linked to another host identity")
	// ErrHostUserDisabled is returned for a user an administrator disabled
	// in Studio.
	ErrHostUserDisabled = errors.New("account is disabled")
)

// hostLoginStampInterval throttles how often a host-authenticated request
// records itself as a login: every request is authenticated by the host,
// but LastLoginAt only needs to show the user is still active there.
const hostLoginStampInterval = 15 * time.Minute

// ProvisionHostUser returns the Studio user for a host-authenticated
// identity, creating it on first sight and keeping its name, email,
// administrator status and (when given) groups in step with the host.
//
// A user is found by subject. On first sight an existing user with the same
// email and no host subject is linked (an account that predates embedding);
// otherwise a new host-origin user is created. Nothing is written when the
// identity is unchanged, apart from a throttled login stamp.
func (s *Service) ProvisionHostUser(id HostIdentity) (*models.User, error) {
	id.Subject = strings.TrimSpace(id.Subject)
	id.Email = strings.TrimSpace(id.Email)
	if id.Subject == "" || id.Email == "" {
		return nil, ErrHostIdentityInvalid
	}

	user, err := s.hostUserBySubject(id.Subject)
	if err != nil {
		return nil, err
	}
	changed := false
	if user == nil {
		if user, err = s.linkOrCreateHostUser(id); err != nil {
			return nil, err
		}
		changed = true
	} else if user.Disabled {
		return nil, ErrHostUserDisabled
	} else if changed, err = s.syncHostUser(user, id); err != nil {
		return nil, err
	}
	if changed {
		// Reload so the user carries its groups as stored, as it does when
		// found unchanged.
		if user, err = s.hostUserBySubject(id.Subject); err != nil {
			return nil, fmt.Errorf("reload host user: %w", err)
		} else if user == nil {
			return nil, errors.New("reload host user: not found")
		}
	}

	if user.LastLoginAt == nil || user.LastLoginMethod != models.LoginMethodHost ||
		time.Since(*user.LastLoginAt) > hostLoginStampInterval {
		user.StampLogin(models.LoginMethodHost)
		if err := s.DB.Model(user).Updates(map[string]interface{}{
			"last_login_at":     user.LastLoginAt,
			"last_login_method": user.LastLoginMethod,
		}).Error; err != nil {
			return nil, fmt.Errorf("stamp host login: %w", err)
		}
	}
	return user, nil
}

func (s *Service) hostUserBySubject(subject string) (*models.User, error) {
	var user models.User
	err := s.DB.Preload("Groups").Where("external_subject = ?", subject).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *Service) linkOrCreateHostUser(id HostIdentity) (*models.User, error) {
	var existing models.User
	err := s.DB.Preload("Groups").Where("email = ?", id.Email).First(&existing).Error
	switch {
	case err == nil:
		if existing.ExternalSubject != "" {
			return nil, ErrHostIdentityConflict
		}
		if existing.Disabled {
			return nil, ErrHostUserDisabled
		}
		if err := s.DB.Model(&existing).Update("external_subject", id.Subject).Error; err != nil {
			return nil, fmt.Errorf("link host identity: %w", err)
		}
		existing.ExternalSubject = id.Subject
		logger.Infof("Linked existing user %d to host identity", existing.ID)
		_, err := s.syncHostUser(&existing, id)
		return &existing, err
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return nil, err
	}

	groups, err := s.hostGroupIDs(id.Groups)
	if err != nil {
		return nil, err
	}
	// Host users sign in through the host; the password only satisfies the
	// account model and is never disclosed.
	password, err := randomPassword()
	if err != nil {
		return nil, err
	}
	user, err := s.CreateUser(UserDTO{
		Email:         id.Email,
		Name:          id.Name,
		Password:      password,
		IsAdmin:       id.Admin,
		ShowChat:      true,
		ShowPortal:    true,
		EmailVerified: true,
		Groups:        groups,
	})
	if err != nil {
		return nil, fmt.Errorf("provision host user: %w", err)
	}
	if err := s.DB.Model(user).Updates(map[string]interface{}{
		"auth_source":      models.AuthSourceHost,
		"external_subject": id.Subject,
	}).Error; err != nil {
		return nil, fmt.Errorf("provision host user: %w", err)
	}
	user.AuthSource = models.AuthSourceHost
	user.ExternalSubject = id.Subject
	logger.Infof("Provisioned user %d from host identity", user.ID)
	return user, nil
}

// syncHostUser brings user in line with the host's view of them, through
// UpdateUser so plugin hooks, role bindings and group rules apply as they
// do for an administrator's edit. It reports whether anything changed.
func (s *Service) syncHostUser(user *models.User, id HostIdentity) (bool, error) {
	groups := user.ExtractGroupIDs()
	if id.Groups != nil {
		var err error
		if groups, err = s.hostGroupIDs(id.Groups); err != nil {
			return false, err
		}
	}
	name := id.Name
	if name == "" {
		name = user.Name
	}
	if user.Email == id.Email && user.Name == name && user.IsAdmin == id.Admin &&
		models.SameIDs(user.ExtractGroupIDs(), groups) {
		return false, nil
	}

	dto := UserDTO{
		Email:                id.Email,
		Name:                 name,
		IsAdmin:              id.Admin,
		ShowChat:             user.ShowChat,
		ShowPortal:           user.ShowPortal,
		EmailVerified:        true,
		NotificationsEnabled: user.NotificationsEnabled && id.Admin,
		AccessToSSOConfig:    user.AccessToSSOConfig && id.Admin,
		Groups:               groups,
	}
	updated, err := s.UpdateUser(user, dto)
	if err != nil {
		return false, fmt.Errorf("update host user: %w", err)
	}
	*user = *updated
	return true, nil
}

// hostGroupIDs resolves group names to IDs, always including the Default
// group. Unknown names are logged and skipped.
func (s *Service) hostGroupIDs(names []string) ([]uint, error) {
	var ids []uint
	if len(names) > 0 {
		var groups []models.Group
		if err := s.DB.Where("name IN ?", names).Find(&groups).Error; err != nil {
			return nil, err
		}
		found := make([]string, 0, len(groups))
		for _, g := range groups {
			ids = append(ids, g.ID)
			found = append(found, g.Name)
		}
		for _, n := range names {
			if !slices.Contains(found, n) {
				logger.Warnf("Host identity names unknown group %q; ignoring it", n)
			}
		}
	}
	ids, err := s.applyDefaultGroup(ids)
	if err != nil {
		return nil, err
	}
	slices.Sort(ids)
	return slices.Compact(ids), nil
}

func randomPassword() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
