package services

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/sso"
	tykerrors "github.com/TykTechnologies/tyk-identity-broker/error"
	"github.com/TykTechnologies/tyk-identity-broker/initializer"
	"github.com/TykTechnologies/tyk-identity-broker/providers"
	"github.com/TykTechnologies/tyk-identity-broker/tap"
	"github.com/TykTechnologies/tyk-identity-broker/tothic"
	"github.com/TykTechnologies/tyk-identity-broker/tyk-api"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/sessions"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

const (
	DashboardSection = "dashboard"
	NonceLength      = 32
	NonceTTL         = 60 * time.Second
	defaultGroupID   = "1"
)

type NonceTokenResponse struct {
	Meta    *string `json:"Meta"`
	Status  string  `json:"Status"`
	Message string  `json:"Message"`
}
type NonceTokenRequest struct {
	ForSection                string
	OrgID                     string
	EmailAddress              string
	GroupID                   string
	GroupsIDs                 []string
	DisplayName               string
	SSOOnlyForRegisteredUsers bool
	ProfileID                 string    // identity provider profile the login came through; may be empty
	ExpiresAt                 time.Time // New field for expiry
}

type InternalTIB struct {
	authConfigStore tap.AuthRegisterBackend
	kvStore         tap.AuthRegisterBackend
	tykAPIHandler   tyk.TykAPI
}

type Config struct {
	APISecret string
	LogLevel  string
}

type SSOService struct {
	InternalTIB     *InternalTIB
	config          *Config
	router          *gin.Engine
	db              *gorm.DB
	notificationSvc *NotificationService
}

func NewSSOService(config *Config, router *gin.Engine, db *gorm.DB, notificationSvc *NotificationService) *SSOService {
	return &SSOService{
		config:          config,
		router:          router,
		db:              db,
		notificationSvc: notificationSvc,
	}
}

func (s *SSOService) InitInternalTIB() {
	backendStorage := models.NewGormAuthRegisterBackend(s.db)
	kvStore := models.NewGormKVStore(s.db)
	internalTIB := &InternalTIB{
		authConfigStore: backendStorage,
		kvStore:         kvStore,
		tykAPIHandler:   tyk.TykAPI{},
	}
	log := logrus.New()

	level, err := logrus.ParseLevel(s.config.LogLevel)
	if err != nil {
		slog.Warn("couldn't parse log level for tib logger, using default")
	}

	log.Level = level

	initializer.SetLogger(log)
	initializer.SetConfigHandler(kvStore)

	tothic.TothErrorHandler = tykerrors.HandleError
	tothic.Store = sessions.NewCookieStore([]byte(s.config.APISecret))

	s.InternalTIB = internalTIB

	s.setCustomDispatcher()
}

func (s *SSOService) setCustomDispatcher() {
	s.InternalTIB.tykAPIHandler.CustomDispatcher = s.dispatcherFor("")
}

// dispatcherFor returns the in-process dispatcher the broker uses to call
// back into Studio. When bound to a profile it tags the call with that
// profile's ID so the nonce, and therefore provisioning, knows which
// identity provider the login came through.
func (s *SSOService) dispatcherFor(profileID string) func(tyk.Endpoint, string, string, io.Reader) ([]byte, int, error) {
	return func(target tyk.Endpoint, method, _ string, body io.Reader) ([]byte, int, error) {
		preparedEndpoint := string(target)

		newRequest, err := http.NewRequest(method, preparedEndpoint, body)
		if err != nil {
			slog.Error("Failed to create request in custom dispatcher", "endpoint", preparedEndpoint, "method", method, "error", err)
			return []byte{}, http.StatusInternalServerError, err
		}

		newRequest.Header.Add("Authorization", s.config.APISecret)
		if profileID != "" {
			newRequest.Header.Set(sso.ProfileIDHeader, profileID)
		}

		recorder := httptest.NewRecorder()
		// virtual server to process the requests from tib to portal
		s.router.ServeHTTP(recorder, newRequest)

		contents, err := io.ReadAll(recorder.Body)
		if err != nil {
			slog.Error("Failed to read response body in custom dispatcher", "endpoint", preparedEndpoint, "method", method, "error", err)
			return []byte(""), recorder.Code, err
		}

		if recorder.Code > http.StatusCreated {
			slog.Error("Non-success response code in custom dispatcher", "endpoint", preparedEndpoint, "method", method, "code", recorder.Code)
			return contents, recorder.Code, errors.New("response code from dashboard was not 200")
		}

		return contents, http.StatusOK, nil
	}
}

func (s *SSOService) GetTapProfile(id string) (tap.TAProvider, *tap.Profile, error) {
	// The broker keeps a pointer to the handler it is given, so a per-call
	// copy bound to this profile is what tags the nonce request.
	handler := s.InternalTIB.tykAPIHandler
	handler.CustomDispatcher = s.dispatcherFor(id)
	thisIdentityProvider, thisProfile, err := providers.GetTapProfile(
		s.InternalTIB.authConfigStore,
		s.InternalTIB.kvStore,
		id,
		handler)
	if err != nil {
		slog.Error("Failed to get TAP profile", "id", id, "error", err.Error.Error())
		return nil, nil, helpers.NewInternalServerError(fmt.Sprintf("Failed to get TAP profile: %s", err.Error.Error()))
	}

	return thisIdentityProvider, &thisProfile, nil
}

func (s *SSOService) GenerateNonce(request NonceTokenRequest) (*string, error) {
	request.ExpiresAt = time.Now().Add(NonceTTL)
	nonceToken := helpers.GenerateRandomString(NonceLength)

	if err := s.InternalTIB.kvStore.SetKey(nonceToken, "", request); err != nil {
		slog.Error("Failed to generate nonce token", "email", request.EmailAddress, "section", request.ForSection, "error", err)
		return nil, helpers.NewInternalServerError("Failed to generate nonce token")
	}

	return &nonceToken, nil
}

func (s *SSOService) ValidateNonceRequest(request *NonceTokenRequest) error {
	if request.ForSection != DashboardSection {
		slog.Error("Invalid section in nonce request", "section", request.ForSection, "email", request.EmailAddress)
		return helpers.NewBadRequestError(fmt.Sprintf("unknown section: %s", request.ForSection))
	}

	return nil
}

func (s *SSOService) ResolveNonce(token string, consume bool) (*NonceTokenRequest, error) {
	var tokenData NonceTokenRequest

	err := s.InternalTIB.kvStore.GetKey(token, "", &tokenData)
	if err != nil {
		slog.Error("Token not found", "token", token, "error", err)
		return nil, helpers.NewNotFoundError("Token not found")
	}

	if !tokenData.ExpiresAt.IsZero() && time.Now().After(tokenData.ExpiresAt) {
		if err := s.InternalTIB.kvStore.DeleteKey(token, ""); err != nil {
			slog.Error("Failed to delete expired token", "token", token, "error", err)
		}

		slog.Warn("Token has expired", "token", token, "expiry", tokenData.ExpiresAt)
		return nil, helpers.NewBadRequestError("Token has expired")
	}

	if consume {
		if err := s.InternalTIB.kvStore.DeleteKey(token, ""); err != nil {
			slog.Error("Failed to consume token", "token", token, "error", err)
			return nil, helpers.NewInternalServerError("Failed to consume token")
		}
	}

	return &tokenData, nil
}

// provisioningProfile loads the identity provider profile a login came
// through. A missing or unknown ID yields nil, which ProvisioningDefaults
// treats as "show everything".
func (s *SSOService) provisioningProfile(tx *gorm.DB, profileID string) *models.Profile {
	if profileID == "" {
		return nil
	}
	profile := models.NewProfile()
	if err := profile.Get(tx, profileID); err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Error("Failed to load SSO profile for provisioning defaults", "profileID", profileID, "error", err)
		}
		return nil
	}
	return profile
}

func (s *SSOService) createUserWithTx(tx *gorm.DB, email, name string, profile *models.Profile) (*models.User, error) {
	showPortal, showChat := profile.ProvisioningDefaults()
	newUser := &models.User{
		Email:         email,
		Name:          name,
		EmailVerified: true,
		ShowPortal:    showPortal,
		ShowChat:      showChat,
	}

	if err := newUser.Create(tx); err != nil {
		slog.Error("Failed to create user", "email", email, "error", err)
		return nil, helpers.NewInternalServerError("Failed to create user")
	}

	return newUser, nil
}

func (s *SSOService) notifyUserCreation(user *models.User) {
	if s.notificationSvc == nil {
		return
	}

	data := map[string]interface{}{
		"Name":  user.Name,
		"Email": user.Email,
		"ID":    user.ID,
	}
	notificationID := fmt.Sprintf("new_user_sso_%d_%d", user.ID, time.Now().UnixNano())
	title := "New User Created via SSO"
	userFlags := models.NotifyAdmins

	if err := s.notificationSvc.Notify(notificationID, title, "admin-sso-notification.tmpl", data, userFlags); err != nil {
		slog.Error("Failed to send user creation notification", "error", err)
	}
}

func (s *SSOService) HandleSSO(login *NonceTokenRequest) (*models.User, error) {
	if login == nil {
		return nil, helpers.NewBadRequestError("Missing SSO login data")
	}
	emailAddress, displayName := login.EmailAddress, login.DisplayName
	groupID, groupsIDs := login.GroupID, login.GroupsIDs
	ssoOnlyForRegisteredUsers := login.SSOOnlyForRegisteredUsers

	var user *models.User
	var isNewUser bool

	err := s.db.Transaction(func(tx *gorm.DB) error {
		existingUser := models.NewUser()
		err := existingUser.GetByEmail(tx, emailAddress)

		if err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				slog.Error("Failed to check if user exists", "email", emailAddress, "error", err)
				return helpers.NewInternalServerError("Failed to check if user exists")
			}

			if ssoOnlyForRegisteredUsers {
				slog.Warn("SSO only enabled for registered users", "email", emailAddress)
				return helpers.NewForbiddenError("SSO only enabled for registered users")
			}

			newUser, err := s.createUserWithTx(tx, emailAddress, displayName, s.provisioningProfile(tx, login.ProfileID))
			if err != nil {
				slog.Error("Failed to create admin user", "email", emailAddress, "error", err)
				return err
			}

			existingUser = newUser
			isNewUser = true
		}

		if existingUser.Name != displayName || !existingUser.EmailVerified {
			existingUser.Name = displayName
			existingUser.EmailVerified = true

			if err := existingUser.Update(tx); err != nil {
				slog.Error("Failed to update user name", "email", emailAddress, "error", err)
				return helpers.NewInternalServerError("Failed to update user name")
			}
		}

		groupsToAssign := []string{defaultGroupID}
		if len(groupsIDs) > 0 {
			groupsToAssign = append(groupsToAssign, groupsIDs...)
		} else if groupID != "" && groupID != defaultGroupID {
			groupsToAssign = append(groupsToAssign, groupID)
		}

		if err := existingUser.UpdateGroupMemberships(tx, groupsToAssign...); err != nil {
			slog.Error("Failed to update user group memberships", "email", emailAddress, "error", err)
			return helpers.NewInternalServerError("Failed to update user group memberships")
		}

		user = existingUser
		return nil
	})

	if err != nil {
		return nil, err
	}

	if isNewUser {
		s.notifyUserCreation(user)
	}

	return user, nil
}
