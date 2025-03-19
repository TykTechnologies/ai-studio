package services

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"time"

	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/models"
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
	UserSection  = "portal"
	AdminSection = "dashboard"
	NonceLength  = 32
	NonceTTL     = 60 * time.Second
)

type NonceTokenResponse struct {
	Meta    *string `json:"Meta"`
	Status  string  `json:"Status"`
	Message string  `json:"Message"`
}

// PortalDeveloper represents a portal developer for compatibility with tib
type PortalDeveloper struct {
	Id            *string           `bson:"_id,omitempty" json:"id"`
	Email         string            `bson:"email" json:"email"`
	Password      string            `bson:"password" json:"password"`
	DateCreated   time.Time         `bson:"date_created" json:"date_created"`
	InActive      bool              `bson:"inactive" json:"inactive"`
	OrgId         string            `bson:"org_id" json:"org_id"`
	ApiKeys       map[string]string `bson:"api_keys" json:"api_keys"`
	Subscriptions map[string]string `bson:"subscriptions" json:"subscriptions"`
	Fields        map[string]string `bson:"fields" json:"fields"`
	Nonce         string            `bson:"nonce" json:"nonce"`
	SSOKey        string            `bson:"sso_key" json:"sso_key"`
}

type NonceTokenRequest struct {
	ForSection                string
	OrgID                     string
	EmailAddress              string
	GroupID                   string
	DisplayName               string `json:"DisplayName,omitempty"`
	SSOOnlyForRegisteredUsers bool
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
	s.InternalTIB.tykAPIHandler.CustomDispatcher = func(target tyk.Endpoint,
		method, _ string, body io.Reader,
	) ([]byte, int, error) {
		preparedEndpoint := string(target)

		newRequest, err := http.NewRequest(method, preparedEndpoint, body)
		if err != nil {
			slog.Error("Failed to create request in custom dispatcher", "endpoint", preparedEndpoint, "method", method, "error", err)
			return []byte{}, http.StatusInternalServerError, err
		}

		newRequest.Header.Add("Authorization", s.config.APISecret)

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
	thisIdentityProvider, thisProfile, err := providers.GetTapProfile(
		s.InternalTIB.authConfigStore,
		s.InternalTIB.kvStore,
		id,
		s.InternalTIB.tykAPIHandler)
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
	if request.ForSection != AdminSection && request.ForSection != UserSection {
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

func (s *SSOService) createUserWithTx(tx *gorm.DB, email, name, password, ssoKey string, isAdmin bool) (*models.User, error) {
	newUser := &models.User{
		Email:                email,
		Name:                 name,
		IsAdmin:              isAdmin,
		ShowChat:             true,
		ShowPortal:           isAdmin,
		EmailVerified:        true,
		NotificationsEnabled: isAdmin,
		SSOKey:               ssoKey,
	}

	if err := newUser.SetPassword(password); err != nil {
		slog.Error("Failed to set user password", "email", email, "error", err)
		return nil, helpers.NewInternalServerError("Failed to set user password")
	}

	if err := newUser.Create(tx); err != nil {
		slog.Error("Failed to create user", "email", email, "error", err)
		return nil, helpers.NewInternalServerError("Failed to create user")
	}

	return newUser, nil
}

func (s *SSOService) addUserToGroupWithTx(tx *gorm.DB, userID uint, groupID string) error {
	if groupID == "" {
		return nil
	}

	groupIDUint, err := strconv.ParseUint(groupID, 10, 64)
	if err != nil {
		slog.Error("Invalid group ID", "groupID", groupID, "userID", userID, "error", err)
		return helpers.NewBadRequestError(fmt.Sprintf("Invalid group ID: %s", groupID))
	}

	group := models.NewGroup()
	if err := group.Get(tx, uint(groupIDUint)); err != nil {
		slog.Error("Group not found", "groupID", groupID, "userID", userID, "error", err)
		return helpers.NewNotFoundError("Group not found")
	}

	user := &models.User{ID: userID}
	if err := group.AddUser(tx, user); err != nil {
		slog.Error("Failed to add user to group", "groupID", groupID, "userID", userID, "error", err)
		return helpers.NewInternalServerError("Failed to add user to group")
	}

	return nil
}

func (s *SSOService) notifyUserCreation(user *models.User, isAdmin bool) {
	if s.notificationSvc == nil {
		return
	}

	data := map[string]interface{}{
		"Name":  user.Name,
		"Email": user.Email,
	}

	var notificationID, title string
	var userFlags uint

	if isAdmin {
		// Notify super-admin (user with ID 1) about new admin
		notificationID = fmt.Sprintf("new_admin_%d_%d", user.ID, time.Now().UnixNano())
		title = "New Admin Created via SSO"
		userFlags = 1 // Super-admin ID
	} else {
		// Notify all admins about new regular user
		notificationID = fmt.Sprintf("new_user_sso_%d_%d", user.ID, time.Now().UnixNano())
		title = "New User Created via SSO"
		userFlags = models.NotifyAdmins
	}

	if err := s.notificationSvc.Notify(notificationID, title, "admin-notify.tmpl", data, userFlags); err != nil {
		slog.Error("Failed to send user creation notification", "error", err, "isAdmin", isAdmin)
	}
}

func (s *SSOService) HandleSSO(emailAddress, displayName, groupID string, ssoOnlyForRegisteredUsers bool, forSection string) (*models.User, error) {
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

			newUser, err := s.createUserWithTx(tx, emailAddress, displayName, "", "", true)
			if err != nil {
				slog.Error("Failed to create admin user", "email", emailAddress, "error", err)
				return err
			}

			existingUser = newUser
			isNewUser = true
		}

		if err := s.addUserToGroupWithTx(tx, existingUser.ID, groupID); err != nil {
			slog.Error("Failed to add user to group", "email", emailAddress, "groupID", groupID, "error", err)
			return err
		}

		user = existingUser
		return nil
	})

	if err != nil {
		slog.Error("SSO authentication failed", "email", emailAddress, "error", err)
		return nil, err
	}

	if isNewUser {
		s.notifyUserCreation(user, true)
	}

	return user, nil
}

func (s *SSOService) CreateSSOUser(email, name, password, ssoKey, groupID string) (*models.User, error) {
	var user *models.User
	var isNewUser bool

	err := s.db.Transaction(func(tx *gorm.DB) error {
		existingUser := models.NewUser()
		err := existingUser.GetBySSOKey(tx, ssoKey)
		if err == nil {
			user = existingUser
			return nil
		}

		if !errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Error("Failed to check if user exists", "email", email, "ssoKey", ssoKey, "error", err)
			return helpers.NewInternalServerError("Failed to check if user exists")
		}

		newUser, err := s.createUserWithTx(tx, email, name, password, ssoKey, false)
		if err != nil {
			slog.Error("Failed to create regular user", "email", email, "error", err)
			return err
		}

		// Always add to default group (ID 1)
		if err := s.addUserToGroupWithTx(tx, newUser.ID, "1"); err != nil {
			slog.Error("Failed to add user to default group", "email", email, "userID", newUser.ID, "error", err)
			return err
		}

		// Add to specified group if different from default
		if groupID != "" && groupID != "1" {
			if err := s.addUserToGroupWithTx(tx, newUser.ID, groupID); err != nil {
				slog.Error("Failed to add user to additional group", "email", email, "userID", newUser.ID, "groupID", groupID, "error", err)
				return err
			}
		}

		user = newUser
		isNewUser = true
		return nil
	})

	if err != nil {
		slog.Error("Failed to create SSO user", "email", email, "ssoKey", ssoKey, "error", err)
		return nil, err
	}

	if isNewUser {
		s.notifyUserCreation(user, false)
	}

	return user, nil
}

func (s *SSOService) UpdateSSOUser(ssoKey, email, password, groupID string) (*models.User, error) {
	var user *models.User

	err := s.db.Transaction(func(tx *gorm.DB) error {
		existingUser := models.NewUser()
		if err := existingUser.GetBySSOKey(tx, ssoKey); err != nil {
			slog.Error("User not found for update", "ssoKey", ssoKey, "error", err)
			return helpers.NewNotFoundError("User not found")
		}

		existingUser.Email = email
		if password != "" {
			if err := existingUser.SetPassword(password); err != nil {
				slog.Error("Failed to set user password", "ssoKey", ssoKey, "id", existingUser.ID, "error", err)
				return helpers.NewInternalServerError("Failed to set user password")
			}
		}

		if err := existingUser.Update(tx); err != nil {
			slog.Error("Failed to update user", "ssoKey", ssoKey, "id", existingUser.ID, "error", err)
			return helpers.NewInternalServerError("Failed to update user")
		}

		if err := s.addUserToGroupWithTx(tx, existingUser.ID, groupID); err != nil {
			slog.Error("Failed to add user to group during update", "ssoKey", ssoKey, "id", existingUser.ID, "groupID", groupID, "error", err)
			return err
		}

		user = existingUser
		return nil
	})

	if err != nil {
		slog.Error("Failed to update SSO user", "ssoKey", ssoKey, "error", err)
		return nil, err
	}

	return user, nil
}

func (s *SSOService) GetUserBySSOKey(ssoKey string) (*PortalDeveloper, error) {
	user := models.NewUser()
	if err := user.GetBySSOKey(s.db, ssoKey); err != nil {
		slog.Error("User not found by SSO key", "ssoKey", ssoKey, "error", err)
		return nil, helpers.NewNotFoundError("User not found")
	}

	developer := &PortalDeveloper{
		Id:            helpers.IntToObjectId(user.ID),
		Email:         user.Email,
		Password:      user.Password,
		DateCreated:   user.CreatedAt,
		InActive:      false,
		OrgId:         strconv.FormatUint(uint64(user.ID), 10),
		ApiKeys:       map[string]string{},
		Subscriptions: map[string]string{},
		Fields:        map[string]string{},
		Nonce:         "",
		SSOKey:        user.SSOKey,
	}

	return developer, nil
}
