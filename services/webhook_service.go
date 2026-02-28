package services

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	neturl "net/url"
	"os"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"gorm.io/gorm"
)

type WebhookServiceConfig struct {
	Workers              int
	QueueSize            int
	DefaultMaxAttempts   int
	DefaultBackoff       []time.Duration
	DefaultHTTPTimeout   time.Duration
	MaxResponseBodyBytes int
	PollInterval         time.Duration
	AllowInternalNetwork bool // Disable SSRF protection (e.g. for testing or internal deployments)
}

func DefaultWebhookServiceConfig() WebhookServiceConfig {
	return WebhookServiceConfig{
		Workers:    4,
		QueueSize:  512,
		DefaultMaxAttempts: 5,
		DefaultBackoff: []time.Duration{
			10 * time.Second,
			30 * time.Second,
			2 * time.Minute,
			10 * time.Minute,
			30 * time.Minute,
		},
		DefaultHTTPTimeout:   15 * time.Second,
		MaxResponseBodyBytes: 4 * 1024,
		PollInterval:         5 * time.Second,
	}
}

func WebhookServiceConfigFromValues(workers, queueSize, maxRetries, httpTimeoutSecs, maxRespBody int, backoffSecs []int) WebhookServiceConfig {
	cfg := DefaultWebhookServiceConfig()
	if workers > 0 {
		cfg.Workers = workers
	}
	if queueSize > 0 {
		cfg.QueueSize = queueSize
	}
	if maxRetries > 0 {
		cfg.DefaultMaxAttempts = maxRetries
	}
	if httpTimeoutSecs > 0 {
		cfg.DefaultHTTPTimeout = time.Duration(httpTimeoutSecs) * time.Second
	}
	if maxRespBody > 0 {
		cfg.MaxResponseBodyBytes = maxRespBody
	}
	if len(backoffSecs) > 0 {
		cfg.DefaultBackoff = make([]time.Duration, len(backoffSecs))
		for i, s := range backoffSecs {
			cfg.DefaultBackoff[i] = time.Duration(s) * time.Second
		}
	}
	cfg.AllowInternalNetwork = os.Getenv("ALLOW_INTERNAL_NETWORK_ACCESS") == "true"
	return cfg
}

type resolvedPolicy struct {
	maxAttempts int
	backoff     []time.Duration
	httpTimeout time.Duration
}

type WebhookService struct {
	db     *gorm.DB
	cfg    WebhookServiceConfig
	queue  chan uint // WebhookEvent IDs
	stopCh chan struct{}
}

func NewWebhookService(db *gorm.DB, cfg WebhookServiceConfig) *WebhookService {
	return &WebhookService{
		db:     db,
		cfg:    cfg,
		queue:  make(chan uint, cfg.QueueSize),
		stopCh: make(chan struct{}),
	}
}

func (s *WebhookService) Start(ctx context.Context) {
	// Reset any events left in_flight from a previous crash back to pending.
	s.db.Model(&models.WebhookEvent{}).
		Where("status = ?", "in_flight").
		Updates(map[string]interface{}{"status": models.DeliveryStatusPending})

	for i := 0; i < s.cfg.Workers; i++ {
		go s.worker(ctx)
	}
	go s.poller(ctx)
}

func (s *WebhookService) Stop() {
	close(s.stopCh)
}

// poller periodically scans WebhookEvent rows whose next_run_at is due and
// pushes their IDs to the worker queue. It also recovers any events that were
// left in-flight if the process previously crashed (status=pending but
// next_run_at is old enough to be considered stale — beyond the longest
// possible delivery timeout).
func (s *WebhookService) poller(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.pollDueEvents()
		case <-s.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (s *WebhookService) pollDueEvents() {
	var events []models.WebhookEvent
	now := time.Now()
	if err := s.db.
		Where("status = ? AND next_run_at <= ?", models.DeliveryStatusPending, now).
		Order("next_run_at ASC").
		Limit(s.cfg.QueueSize).
		Find(&events).Error; err != nil {
		logger.Warnf("webhook: poll error: %v", err)
		return
	}
	for _, ev := range events {
		select {
		case s.queue <- ev.ID:
		default:
			// Queue full; will pick up on next poll cycle
		}
	}
}

func (s *WebhookService) worker(ctx context.Context) {
	for {
		select {
		case eventID := <-s.queue:
			s.processEvent(ctx, eventID)
		case <-s.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (s *WebhookService) processEvent(ctx context.Context, eventID uint) {
	// Atomically claim the event: only proceed if status is still "pending".
	// This prevents two workers from processing the same event concurrently.
	result := s.db.Model(&models.WebhookEvent{}).
		Where("id = ? AND status = ?", eventID, models.DeliveryStatusPending).
		Update("status", "in_flight")
	if result.Error != nil {
		logger.Errorf("webhook: failed to claim event %d: %v", eventID, result.Error)
		return
	}
	if result.RowsAffected == 0 {
		// Another worker claimed it first
		return
	}

	var ev models.WebhookEvent
	if err := s.db.First(&ev, eventID).Error; err != nil {
		logger.Errorf("webhook: event %d not found after claim: %v", eventID, err)
		return
	}

	var sub models.WebhookSubscription
	if err := s.db.First(&sub, ev.SubscriptionID).Error; err != nil {
		logger.Errorf("webhook: subscription %d not found for event %d: %v", ev.SubscriptionID, eventID, err)
		s.db.Delete(&ev)
		return
	}

	policy := s.resolveRetryPolicy(sub)

	body := []byte(ev.Payload)

	// SSRF check at delivery time (defence-in-depth; URL also checked at write time)
	if err := validateWebhookURLInternal(sub.URL, s.cfg.AllowInternalNetwork); err != nil {
		log := models.WebhookDeliveryLog{
			SubscriptionID: sub.ID,
			EventTopic:     ev.EventTopic,
			EventID:        ev.EventID,
			Payload:        string(body),
			AttemptNumber:  ev.AttemptNumber,
			Status:         models.DeliveryStatusFailed,
			ErrorMessage:   "SSRF: " + err.Error(),
			AttemptedAt:    time.Now(),
		}
		s.db.Create(&log)
		s.db.Model(&ev).Update("status", "exhausted")
		return
	}

	deliveryTime := time.Now()
	sig := computeHMAC(body, sub.Secret, deliveryTime)

	client, err := buildHTTPClient(sub.TransportConfig, policy.httpTimeout)
	if err != nil {
		log := models.WebhookDeliveryLog{
			SubscriptionID: sub.ID,
			EventTopic:     ev.EventTopic,
			EventID:        ev.EventID,
			Payload:        string(body),
			AttemptNumber:  ev.AttemptNumber,
			Status:         models.DeliveryStatusFailed,
			ErrorMessage:   err.Error(),
			AttemptedAt:    deliveryTime,
		}
		s.db.Create(&log)
		s.db.Model(&ev).Update("status", "exhausted")
		return
	}

	deliveryCtx, cancel := context.WithTimeout(ctx, policy.httpTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(deliveryCtx, http.MethodPost, sub.URL, strings.NewReader(string(body)))
	if err != nil {
		logger.Errorf("webhook: failed to build request for sub %d: %v", sub.ID, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tyk-Timestamp", fmt.Sprintf("%d", deliveryTime.Unix()))
	if sig != "" {
		req.Header.Set("X-Tyk-Signature", "sha256="+sig)
	}
	req.Header.Set("X-Tyk-Event-Topic", ev.EventTopic)
	req.Header.Set("X-Tyk-Event-ID", ev.EventID)
	req.Header.Set("X-Tyk-Delivery-Attempt", fmt.Sprintf("%d", ev.AttemptNumber))

	maxRespBody := s.cfg.MaxResponseBodyBytes
	if dbCfg, err := models.GetWebhookConfig(s.db); err == nil && dbCfg.MaxResponseBodyBytes > 0 {
		maxRespBody = dbCfg.MaxResponseBodyBytes
	}

	now := time.Now()
	resp, httpErr := client.Do(req)

	logEntry := models.WebhookDeliveryLog{
		SubscriptionID: sub.ID,
		EventTopic:     ev.EventTopic,
		EventID:        ev.EventID,
		Payload:        string(body),
		AttemptNumber:  ev.AttemptNumber,
		AttemptedAt:    now,
	}

	success := false
	if httpErr != nil {
		logEntry.Status = models.DeliveryStatusFailed
		logEntry.ErrorMessage = httpErr.Error()
	} else {
		defer resp.Body.Close()
		respBytes, _ := io.ReadAll(io.LimitReader(resp.Body, int64(maxRespBody)))
		logEntry.HTTPStatusCode = resp.StatusCode
		logEntry.ResponseBody = string(respBytes)
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			logEntry.Status = models.DeliveryStatusSuccess
			success = true
		} else {
			logEntry.Status = models.DeliveryStatusFailed
			logEntry.ErrorMessage = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
	}

	if err := s.db.Create(&logEntry).Error; err != nil {
		logger.Errorf("webhook: failed to persist delivery log for sub %d: %v", sub.ID, err)
	}

	if success {
		s.db.Model(&ev).Update("status", "delivered")
		return
	}

	nextAttempt := ev.AttemptNumber + 1
	if nextAttempt > policy.maxAttempts {
		s.db.Model(&ev).Update("status", "exhausted")
		return
	}

	backoffIdx := ev.AttemptNumber - 1
	var delay time.Duration
	if backoffIdx < len(policy.backoff) {
		delay = policy.backoff[backoffIdx]
	} else if len(policy.backoff) > 0 {
		delay = policy.backoff[len(policy.backoff)-1]
	}

	nextRun := now.Add(delay)
	logEntry.NextRetryAt = &nextRun
	s.db.Model(&logEntry).Update("next_retry_at", &nextRun)

	s.db.Model(&ev).Updates(map[string]interface{}{
		"status":         models.DeliveryStatusPending,
		"attempt_number": nextAttempt,
		"next_run_at":    nextRun,
	})
}

func (s *WebhookService) resolveRetryPolicy(sub models.WebhookSubscription) resolvedPolicy {
	policy := resolvedPolicy{
		maxAttempts: s.cfg.DefaultMaxAttempts,
		backoff:     s.cfg.DefaultBackoff,
		httpTimeout: s.cfg.DefaultHTTPTimeout,
	}

	if dbCfg, err := models.GetWebhookConfig(s.db); err == nil {
		if dbCfg.DefaultRetryPolicy.MaxAttempts > 0 {
			policy.maxAttempts = dbCfg.DefaultRetryPolicy.MaxAttempts
		}
		if len(dbCfg.DefaultRetryPolicy.BackoffSeconds) > 0 {
			policy.backoff = secondsToDurations(dbCfg.DefaultRetryPolicy.BackoffSeconds)
		}
		if dbCfg.DefaultRetryPolicy.TimeoutSeconds > 0 {
			policy.httpTimeout = time.Duration(dbCfg.DefaultRetryPolicy.TimeoutSeconds) * time.Second
		}
	}

	if sub.RetryPolicy.MaxAttempts > 0 {
		policy.maxAttempts = sub.RetryPolicy.MaxAttempts
	}
	if len(sub.RetryPolicy.BackoffSeconds) > 0 {
		policy.backoff = secondsToDurations(sub.RetryPolicy.BackoffSeconds)
	}
	if sub.RetryPolicy.TimeoutSeconds > 0 {
		policy.httpTimeout = time.Duration(sub.RetryPolicy.TimeoutSeconds) * time.Second
	}

	return policy
}

func secondsToDurations(secs []int) []time.Duration {
	out := make([]time.Duration, len(secs))
	for i, s := range secs {
		out[i] = time.Duration(s) * time.Second
	}
	return out
}

// HandleEvent is called by the event bus. It persists a WebhookEvent row for
// each matching subscription so delivery survives process restarts.
func (s *WebhookService) HandleEvent(ev eventbridge.Event) {
	subs, err := s.findMatchingSubscriptions(ev.Topic)
	if err != nil {
		logger.Warnf("webhook: failed to query subscriptions for topic %s: %v", ev.Topic, err)
		return
	}

	body, err := json.Marshal(ev)
	if err != nil {
		logger.Errorf("webhook: failed to marshal event %s: %v", ev.ID, err)
		return
	}

	for _, sub := range subs {
		row := models.WebhookEvent{
			SubscriptionID: sub.ID,
			EventTopic:     ev.Topic,
			EventID:        ev.ID,
			Payload:        string(body),
			AttemptNumber:  1,
			Status:         models.DeliveryStatusPending,
			NextRunAt:      time.Now(),
		}
		if err := s.db.Create(&row).Error; err != nil {
			logger.Errorf("webhook: failed to persist event for sub %d: %v", sub.ID, err)
			continue
		}
		select {
		case s.queue <- row.ID:
		default:
			// Poller will pick it up
		}
	}
}

func (s *WebhookService) findMatchingSubscriptions(topic string) ([]models.WebhookSubscription, error) {
	var subs []models.WebhookSubscription
	err := s.db.
		Joins("JOIN webhook_topics ON webhook_topics.subscription_id = webhook_subscriptions.id").
		Where("webhook_subscriptions.enabled = ? AND webhook_topics.topic = ?", true, topic).
		Preload("Topics").
		Find(&subs).Error
	return subs, err
}

// RetryDelivery re-enqueues the original payload from a delivery log as a new
// WebhookEvent with attempt=1 for immediate delivery.
func (s *WebhookService) RetryDelivery(logID uint) error {
	var deliveryLog models.WebhookDeliveryLog
	if err := s.db.First(&deliveryLog, logID).Error; err != nil {
		return fmt.Errorf("delivery log not found: %w", err)
	}
	sub, err := s.GetWebhook(deliveryLog.SubscriptionID)
	if err != nil {
		return fmt.Errorf("subscription not found: %w", err)
	}

	row := models.WebhookEvent{
		SubscriptionID: sub.ID,
		EventTopic:     deliveryLog.EventTopic,
		EventID:        deliveryLog.EventID,
		Payload:        deliveryLog.Payload,
		AttemptNumber:  1,
		Status:         models.DeliveryStatusPending,
		NextRunAt:      time.Now(),
	}
	if err := s.db.Create(&row).Error; err != nil {
		return fmt.Errorf("failed to enqueue retry: %w", err)
	}

	select {
	case s.queue <- row.ID:
	default:
		// Poller will pick it up
	}
	return nil
}

func validateWebhookURLInternal(rawURL string, allowInternal bool) error {
	if rawURL == "" {
		return fmt.Errorf("webhook URL must not be empty")
	}
	if allowInternal {
		return nil
	}

	parsed, err := neturl.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("URL scheme must be http or https")
	}

	hostname := parsed.Hostname()
	ips, err := net.LookupIP(hostname)
	if err != nil {
		return fmt.Errorf("cannot resolve hostname %q: %w", hostname, err)
	}
	for _, ip := range ips {
		if isWebhookPrivateIP(ip) {
			return fmt.Errorf("URL %q resolves to private/internal address %s", rawURL, ip)
		}
	}
	return nil
}

var webhookPrivateCIDRs = func() []*net.IPNet {
	ranges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"::1/128",
		"fc00::/7",
	}
	nets := make([]*net.IPNet, 0, len(ranges))
	for _, cidr := range ranges {
		_, n, _ := net.ParseCIDR(cidr)
		nets = append(nets, n)
	}
	return nets
}()

func isWebhookPrivateIP(ip net.IP) bool {
	for _, n := range webhookPrivateCIDRs {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// computeHMAC signs "timestamp.body" with the subscription secret so the
// signature is bound to a specific delivery time. This prevents replay attacks:
// even if an attacker captures a valid (timestamp, body, sig) tuple, re-sending
// it after the receiver's tolerance window (typically 5 minutes) will be rejected
// because the timestamp embedded in the signed material will be stale.
//
// The signature covers: strconv.FormatInt(ts.Unix(), 10) + "." + string(body)
//
// Receivers verify by:
//  1. Parsing X-Tyk-Timestamp as a Unix epoch integer
//  2. Rejecting if abs(now - ts) > tolerance
//  3. Recomputing HMAC-SHA256 over "ts.body" with their copy of the secret
//  4. Comparing with X-Tyk-Signature using constant-time equality
func computeHMAC(body []byte, secret string, ts time.Time) string {
	if secret == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(fmt.Sprintf("%d.%s", ts.Unix(), body)))
	return hex.EncodeToString(mac.Sum(nil))
}

// buildHTTPClient constructs a per-subscription http.Client that applies the
// subscription's TransportConfig (proxy, TLS settings) on top of the service's
// default timeout. Returns an error only if the TLS configuration is invalid.
func buildHTTPClient(tc models.WebhookTransportConfig, timeout time.Duration) (*http.Client, error) {
	tlsCfg := &tls.Config{
		InsecureSkipVerify: tc.InsecureSkipVerify, //nolint:gosec // user-controlled per subscription
	}

	if tc.TLSCACert != "" {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(tc.TLSCACert)) {
			return nil, fmt.Errorf("webhook: invalid TLS CA certificate")
		}
		tlsCfg.RootCAs = pool
	}

	if tc.TLSClientCert != "" || tc.TLSClientKey != "" {
		cert, err := tls.X509KeyPair([]byte(tc.TLSClientCert), []byte(tc.TLSClientKey))
		if err != nil {
			return nil, fmt.Errorf("webhook: invalid TLS client cert/key: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}

	transport := &http.Transport{
		TLSClientConfig: tlsCfg,
	}

	if tc.ProxyURL != "" {
		proxyURL, err := neturl.Parse(tc.ProxyURL)
		if err != nil {
			return nil, fmt.Errorf("webhook: invalid proxy URL: %w", err)
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}

	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}, nil
}

// ValidateURL checks whether rawURL is safe to use as a webhook target,
// respecting the service's AllowInternalNetwork setting.
func (s *WebhookService) ValidateURL(rawURL string) error {
	return validateWebhookURLInternal(rawURL, s.cfg.AllowInternalNetwork)
}

// ValidateTopics returns an error if any topic in the list is not a known subscribable topic.
func ValidateTopics(topics []string) error {
	known := make(map[string]struct{}, len(KnownWebhookTopics))
	for _, t := range KnownWebhookTopics {
		known[t] = struct{}{}
	}
	for _, t := range topics {
		if _, ok := known[t]; !ok {
			return fmt.Errorf("unknown event topic %q", t)
		}
	}
	return nil
}

func (s *WebhookService) CreateWebhook(sub *models.WebhookSubscription) error {
	return s.db.Create(sub).Error
}

func (s *WebhookService) GetWebhook(id uint) (*models.WebhookSubscription, error) {
	var sub models.WebhookSubscription
	if err := s.db.Preload("Topics").First(&sub, id).Error; err != nil {
		return nil, err
	}
	return &sub, nil
}

func (s *WebhookService) ListWebhooks() ([]models.WebhookSubscription, error) {
	var subs []models.WebhookSubscription
	if err := s.db.Preload("Topics").Find(&subs).Error; err != nil {
		return nil, err
	}
	return subs, nil
}

func (s *WebhookService) UpdateWebhook(sub *models.WebhookSubscription) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(sub).Error; err != nil {
			return err
		}
		if err := tx.Where("subscription_id = ?", sub.ID).Delete(&models.WebhookTopic{}).Error; err != nil {
			return err
		}
		for i := range sub.Topics {
			sub.Topics[i].SubscriptionID = sub.ID
			sub.Topics[i].ID = 0
		}
		if len(sub.Topics) > 0 {
			return tx.Create(&sub.Topics).Error
		}
		return nil
	})
}

func (s *WebhookService) DeleteWebhook(id uint) error {
	return s.db.Delete(&models.WebhookSubscription{}, id).Error
}

func (s *WebhookService) ListDeliveryLogs(subscriptionID uint, limit int) ([]models.WebhookDeliveryLog, error) {
	if limit <= 0 {
		limit = 50
	}
	var logs []models.WebhookDeliveryLog
	if err := s.db.Where("subscription_id = ?", subscriptionID).
		Order("created_at DESC").
		Limit(limit).
		Find(&logs).Error; err != nil {
		return nil, err
	}
	return logs, nil
}

func (s *WebhookService) GetWebhookConfig() (*models.WebhookConfig, error) {
	return models.GetWebhookConfig(s.db)
}

func (s *WebhookService) UpdateWebhookConfig(cfg *models.WebhookConfig) error {
	return models.UpdateWebhookConfig(s.db, cfg)
}

// TestWebhook fires a synchronous test delivery. No delivery log or queue row is persisted.
func (s *WebhookService) TestWebhook(sub *models.WebhookSubscription) error {
	payload, _ := json.Marshal(map[string]string{
		"message": "This is a test webhook delivery from Tyk AI Studio.",
	})

	event := eventbridge.Event{
		ID:      fmt.Sprintf("test-%d", time.Now().UnixNano()),
		Topic:   "system.webhook.test",
		Origin:  "control",
		Dir:     eventbridge.DirLocal,
		Payload: payload,
	}

	body, _ := json.Marshal(event)
	deliveryTime := time.Now()
	sig := computeHMAC(body, sub.Secret, deliveryTime)

	policy := s.resolveRetryPolicy(*sub)

	client, err := buildHTTPClient(sub.TransportConfig, policy.httpTimeout)
	if err != nil {
		return fmt.Errorf("failed to build HTTP client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), policy.httpTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sub.URL, strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("failed to build test request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tyk-Timestamp", fmt.Sprintf("%d", deliveryTime.Unix()))
	if sig != "" {
		req.Header.Set("X-Tyk-Signature", "sha256="+sig)
	}
	req.Header.Set("X-Tyk-Event-Topic", event.Topic)
	req.Header.Set("X-Tyk-Event-ID", event.ID)
	req.Header.Set("X-Tyk-Delivery-Attempt", "1")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("test webhook delivery failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("test webhook returned HTTP %d", resp.StatusCode)
	}
	return nil
}
