package licensing

import (
	"errors"
	"fmt"
	"log"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/golang-jwt/jwt/v5"
)

var pubKey = `
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

type Licenser struct {
	license            *LicenseInfo
	config             LicenseConfig
	telemetryClient    *Client
	done               chan bool
	lock               sync.RWMutex
	featuresInit       chan struct{}
	initialized        bool
	telemetrySemaphore chan struct{}
}

func NewLicenser(config LicenseConfig) *Licenser {
	if config.TelemetryPeriod == 0 {
		config.TelemetryPeriod = DefaultTelemetryPeriod
	}

	if config.ValidityCheckPeriod == 0 {
		config.ValidityCheckPeriod = DefaultValidityCheckPeriod
	}

	if config.TelemetryURL == "" {
		config.TelemetryURL = telemetryAPIURL
	}

	featuresInit := make(chan struct{})

	return &Licenser{
		config:             config,
		telemetryClient:    NewClient(config.TelemetryURL),
		done:               make(chan bool),
		featuresInit:       featuresInit,
		telemetrySemaphore: make(chan struct{}, MaxConcurrentTelemetryRequests),
	}
}

func (l *Licenser) Start() {
	if err := l.isLicensed(); err != nil {
		log.Fatalf("License is not valid: %v", err)
	}

	l.initialized = true
	close(l.featuresInit)

	if !l.config.DisableTelemetry {
		go l.SendTelemetry()
	}

	licenseCheckTicker := time.NewTicker(l.config.ValidityCheckPeriod)
	telemetryTicker := time.NewTicker(l.config.TelemetryPeriod)

	go func() {
		for {
			select {
			case <-l.done:
				licenseCheckTicker.Stop()
				telemetryTicker.Stop()
				return
			case <-licenseCheckTicker.C:
				if err := l.isLicensed(); err != nil {
					log.Fatalf("License is not valid: %v", err)
				}
			case <-telemetryTicker.C:
				if !l.config.DisableTelemetry {
					l.SendTelemetry()
				}
			}
		}
	}()
}

func (l *Licenser) Stop() {
	l.done <- true
}

func (l *Licenser) FeatureSet() map[string]*Feature {
	l.lock.RLock()
	initialized := l.initialized
	l.lock.RUnlock()

	if !initialized {
		<-l.featuresInit
	}

	l.lock.RLock()
	defer l.lock.RUnlock()

	if l.license == nil {
		return nil
	}

	return l.license.Features
}

func (l *Licenser) Entitlement(name string) (*Feature, bool) {
	l.lock.RLock()
	initialized := l.initialized
	l.lock.RUnlock()

	if !initialized {
		<-l.featuresInit
	}

	l.lock.RLock()
	defer l.lock.RUnlock()

	if l.license == nil || l.license.Features == nil {
		return nil, false
	}

	f, ok := l.license.Features[name]
	if !ok {
		return nil, false
	}

	return f, true
}

func (l *Licenser) isLicensed() error {
	licenseStr := l.config.LicenseKey
	if licenseStr == "" {
		return errors.New("no TYK_AI_LICENSE env var found")
	}

	claims, err := l.validate(licenseStr, []byte(pubKey))
	if err != nil {
		return err
	}

	licenseInfo := &LicenseInfo{
		Key:      licenseStr,
		IsValid:  true,
		Features: make(map[string]*Feature),
		claims:   claims,
	}

	licenseInfo.setup()

	l.lock.Lock()
	l.license = licenseInfo
	l.lock.Unlock()

	return nil
}

func (l *Licenser) validate(token string, pubKey []byte) (jwt.MapClaims, error) {
	key, err := jwt.ParseRSAPublicKeyFromPEM([]byte(pubKey))
	if err != nil {
		return nil, fmt.Errorf("validate: parse key: %w", err)
	}

	tok, err := jwt.Parse(token, func(jwtToken *jwt.Token) (interface{}, error) {
		if _, ok := jwtToken.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected method: %s", jwtToken.Header["alg"])
		}

		return key, nil
	})
	if err != nil {
		return nil, fmt.Errorf("validate: %w", err)
	}

	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok || !tok.Valid {
		return nil, fmt.Errorf("validate: invalid")
	}

	return claims, nil
}

func (l *LicenseInfo) setup() {
	if !l.IsValid {
		return
	}

	l.setVersion()
	l.setLicenseExpire()
	l.setFeatures()
}

func (l *LicenseInfo) getClaim(name string) (claim interface{}, found bool) {
	claim, found = l.claims[name]
	return
}

func (l *LicenseInfo) setVersion() {
	licenseVersion, found := l.getClaim(claimVersion)
	if !found {
		return
	}

	version, ok := licenseVersion.(string)
	if ok {
		l.Version = version
	}
}

func (l *LicenseInfo) setLicenseExpire() {
	asTime, found := l.getClaim(claimExp)
	if !found {
		return
	}

	expFloat, ok := asTime.(float64)
	if ok {
		l.ExpiresAt = time.Unix(int64(expFloat), 0)
	}
}

func (l *LicenseInfo) setFeatures() {
	capabilities, found := l.getClaim(claimScope)
	if !found {
		return
	}

	capStr, ok := capabilities.(string)
	if !ok {
		return
	}

	featureMap := make(map[string]*Feature)

	for _, c := range strings.Split(capStr, ",") {
		feature, err := NewFeature(true)
		if err != nil {
			log.Printf("Warning: failed to create feature %s: %v", c, err)
			continue
		}

		featureMap[c] = feature
	}

	l.Features = featureMap
}

func (l *Licenser) collectLLMStats() {
	stats, err := l.config.TelemetryService.GetLLMStats()
	if err != nil {
		slog.Error("Failed to collect LLM stats", "error", err)
		return
	}

	err = l.sendTelemetryReport("llm_report", stats)
	if err != nil {
		slog.Error("Failed to send LLM telemetry report", "error", err)
	}
}

func (l *Licenser) collectAppStats() {
	stats, err := l.config.TelemetryService.GetAppStats()
	if err != nil {
		slog.Error("Failed to collect App stats", "error", err)
		return
	}

	err = l.sendTelemetryReport("app_report", stats)
	if err != nil {
		slog.Error("Failed to send App telemetry report", "error", err)
	}
}

func (l *Licenser) collectUserStats() {
	stats, err := l.config.TelemetryService.GetUserStats()
	if err != nil {
		slog.Error("Failed to collect User stats", "error", err)
		return
	}

	err = l.sendTelemetryReport("user_report", stats)
	if err != nil {
		slog.Error("Failed to send User telemetry report", "error", err)
	}
}

func (l *Licenser) collectChatStats() {
	stats, err := l.config.TelemetryService.GetChatStats()
	if err != nil {
		slog.Error("Failed to collect Chat stats", "error", err)
		return
	}

	err = l.sendTelemetryReport("chat_report", stats)
	if err != nil {
		slog.Error("Failed to send Chat telemetry report", "error", err)
	}
}

func (l *Licenser) sendTelemetryReport(reportName string, stats map[string]interface{}) error {
	l.lock.RLock()
	license := l.license
	l.lock.RUnlock()

	if license == nil {
		return fmt.Errorf("no license available")
	}

	licenseHash := helpers.HashString(license.Key)

	properties := map[string]interface{}{
		"midsommar_version": l.config.Version,
		"component":         l.config.Component,
	}

	for k := range license.Features {
		properties[k] = true
	}

	for k, v := range stats {
		properties[k] = v
	}

	return l.telemetryClient.Track(licenseHash, reportName, properties)
}

func (l *Licenser) SendTelemetry() {
	if l.config.TelemetryService == nil || !l.TelemetryEnabled() {
		return
	}

	l.collectLLMStats()
	l.collectAppStats()
	l.collectUserStats()
	l.collectChatStats()
}

func (l *Licenser) TelemetryEnabled() bool {
	if l.config.DisableTelemetry {
		return false
	}

	feature, ok := l.Entitlement(TrackLicenseUsage)
	return ok && feature.Bool()
}

func (l *Licenser) SendHTTPTelemetry(action string, status int, accessType string) {
	if !l.TelemetryEnabled() {
		return
	}

	telemetryRecord := map[string]interface{}{
		"action":      action,
		"status":      status,
		"access_type": accessType,
	}

	go func() {
		l.telemetrySemaphore <- struct{}{}
		defer func() { <-l.telemetrySemaphore }()

		l.sendTelemetryReport("http_interaction", telemetryRecord)
	}()
}

func (l *Licenser) License() *LicenseInfo {
	l.lock.RLock()
	defer l.lock.RUnlock()

	return l.license
}

func (l *Licenser) InitializeForTests(testFeatures map[string]interface{}) {
	featureMap := make(map[string]*Feature)
	for k, v := range testFeatures {
		feature, err := NewFeature(v)
		if err == nil {
			featureMap[k] = feature
		}
	}

	l.lock.Lock()

	l.license = &LicenseInfo{
		Key:      "test-license",
		IsValid:  true,
		Features: featureMap,
	}

	wasInitialized := l.initialized
	l.initialized = true

	if !wasInitialized {
		close(l.featuresInit)
	}

	l.lock.Unlock()
}

func Create(ttl time.Duration, content interface{}, pKey []byte) (string, error) {
	key, err := jwt.ParseRSAPrivateKeyFromPEM(pKey)
	if err != nil {
		return "", fmt.Errorf("create: parse key: %w", err)
	}

	now := time.Now().UTC()

	claims := make(jwt.MapClaims)
	claims["scope"] = content
	claims["exp"] = now.Add(ttl).Unix()
	claims["iat"] = now.Unix()
	claims["nbf"] = now.Unix()

	token, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(key)
	if err != nil {
		return "", fmt.Errorf("create: sign token: %w", err)
	}

	return token, nil
}
