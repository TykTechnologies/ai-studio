package services

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupWebhookDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&models.WebhookSubscription{},
		&models.WebhookTopic{},
		&models.WebhookEvent{},
		&models.WebhookDeliveryLog{},
		&models.WebhookConfig{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func setupService(t *testing.T) *WebhookService {
	t.Helper()
	cfg := DefaultWebhookServiceConfig()
	cfg.AllowInternalNetwork = true // httptest servers use 127.0.0.1
	return NewWebhookService(setupWebhookDB(t), cfg)
}

// setupFastService creates a service with short backoff for retry tests.
// backoffMs is the delay between attempts in milliseconds.
func setupFastService(t *testing.T, maxAttempts, backoffMs int) *WebhookService {
	t.Helper()
	backoff := make([]time.Duration, maxAttempts)
	for i := range backoff {
		backoff[i] = time.Duration(backoffMs) * time.Millisecond
	}
	cfg := WebhookServiceConfig{
		Workers:              2,
		QueueSize:            64,
		DefaultMaxAttempts:   maxAttempts,
		DefaultBackoff:       backoff,
		DefaultHTTPTimeout:   5 * time.Second,
		MaxResponseBodyBytes: 4096,
		PollInterval:         20 * time.Millisecond,
		AllowInternalNetwork: true, // httptest servers use 127.0.0.1
	}
	return NewWebhookService(setupWebhookDB(t), cfg)
}

func createSub(t *testing.T, svc *WebhookService, url string, topics []string) *models.WebhookSubscription {
	t.Helper()
	sub := &models.WebhookSubscription{
		Name:    "test",
		URL:     url,
		Enabled: true,
		Topics:  make([]models.WebhookTopic, len(topics)),
	}
	for i, t := range topics {
		sub.Topics[i] = models.WebhookTopic{Topic: t}
	}
	if err := svc.CreateWebhook(sub); err != nil {
		t.Fatalf("create webhook: %v", err)
	}
	return sub
}

func waitForEvent(t *testing.T, svc *WebhookService, subID uint, status string, timeout time.Duration) models.WebhookEvent {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var ev models.WebhookEvent
		err := svc.db.Where("subscription_id = ? AND status = ?", subID, status).First(&ev).Error
		if err == nil {
			return ev
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for event status=%s for sub %d", status, subID)
	return models.WebhookEvent{}
}

func TestValidateTopics_Known(t *testing.T) {
	if err := ValidateTopics([]string{TopicLLMCreated, TopicAppApproved}); err != nil {
		t.Fatalf("known topics should be valid: %v", err)
	}
}

func TestValidateTopics_Unknown(t *testing.T) {
	if err := ValidateTopics([]string{"custom.made.up.topic"}); err == nil {
		t.Fatal("unknown topic should fail validation")
	}
}

func TestValidateTopics_Empty(t *testing.T) {
	if err := ValidateTopics([]string{}); err != nil {
		t.Fatalf("empty topics should be valid: %v", err)
	}
}

func TestTopicMatching_Exact(t *testing.T) {
	svc := setupService(t)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ts.Close()

	createSub(t, svc, ts.URL, []string{TopicLLMCreated})

	matched, err := svc.findMatchingSubscriptions(TopicLLMCreated)
	if err != nil || len(matched) != 1 {
		t.Fatalf("expected 1 match for exact topic, got %d (err=%v)", len(matched), err)
	}

	notMatched, err := svc.findMatchingSubscriptions(TopicLLMDeleted)
	if err != nil || len(notMatched) != 0 {
		t.Fatalf("expected 0 matches for different topic, got %d (err=%v)", len(notMatched), err)
	}
}

func TestComputeHMAC(t *testing.T) {
	body := []byte(`{"hello":"world"}`)
	ts := time.Unix(1700000000, 0)
	sig := computeHMAC(body, "secret", ts)
	if sig == "" {
		t.Fatal("expected non-empty signature")
	}
	// Same inputs must produce the same output.
	if computeHMAC(body, "secret", ts) != sig {
		t.Fatal("HMAC must be deterministic for same inputs")
	}
	// Different timestamp must produce a different signature.
	if computeHMAC(body, "secret", ts.Add(time.Second)) == sig {
		t.Fatal("HMAC must differ when timestamp differs")
	}
}

func TestComputeHMAC_EmptySecret(t *testing.T) {
	if computeHMAC([]byte("data"), "", time.Now()) != "" {
		t.Fatal("empty secret should return empty string")
	}
}

func TestDefaultBackoffDuration(t *testing.T) {
	cfg := DefaultWebhookServiceConfig()
	expected := []time.Duration{10 * time.Second, 30 * time.Second, 2 * time.Minute, 10 * time.Minute, 30 * time.Minute}
	if len(cfg.DefaultBackoff) != len(expected) {
		t.Fatalf("expected %d backoff entries, got %d", len(expected), len(cfg.DefaultBackoff))
	}
	for i, d := range expected {
		if cfg.DefaultBackoff[i] != d {
			t.Errorf("backoff[%d]: want %v, got %v", i, d, cfg.DefaultBackoff[i])
		}
	}
}

func setupSSRFService(t *testing.T) *WebhookService {
	t.Helper()
	cfg := DefaultWebhookServiceConfig()
	cfg.AllowInternalNetwork = false
	return NewWebhookService(setupWebhookDB(t), cfg)
}

func TestValidateWebhookURL_Empty(t *testing.T) {
	svc := setupSSRFService(t)
	if err := svc.ValidateURL(""); err == nil {
		t.Fatal("empty URL should fail")
	}
}

func TestValidateWebhookURL_BadScheme(t *testing.T) {
	svc := setupSSRFService(t)
	if err := svc.ValidateURL("ftp://example.com/hook"); err == nil {
		t.Fatal("ftp scheme should fail")
	}
}

func TestValidateWebhookURL_PrivateLoopback(t *testing.T) {
	svc := setupSSRFService(t)
	if err := svc.ValidateURL("http://127.0.0.1/hook"); err == nil {
		t.Fatal("loopback URL should fail SSRF check")
	}
}

func TestValidateWebhookURL_AllowInternal(t *testing.T) {
	cfg := DefaultWebhookServiceConfig()
	cfg.AllowInternalNetwork = true
	svc := NewWebhookService(setupWebhookDB(t), cfg)
	if err := svc.ValidateURL("http://127.0.0.1/hook"); err != nil {
		t.Fatalf("should be allowed with AllowInternalNetwork=true: %v", err)
	}
}

func TestHandleEvent_PersistsQueue(t *testing.T) {
	svc := setupFastService(t, 1, 50)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	createSub(t, svc, ts.URL, []string{"system.llm.created"})

	ev := eventbridge.Event{ID: "evt-1", Topic: "system.llm.created", Origin: "control"}
	svc.HandleEvent(ev)

	var events []models.WebhookEvent
	if err := svc.db.Find(&events).Error; err != nil {
		t.Fatalf("db find: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 WebhookEvent row, got %d", len(events))
	}
	if events[0].Status != models.DeliveryStatusPending && events[0].Status != "delivered" {
		t.Errorf("unexpected initial status: %s", events[0].Status)
	}
}

func TestDelivery_Success(t *testing.T) {
	svc := setupFastService(t, 3, 30)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	sub := createSub(t, svc, ts.URL, []string{"system.llm.created"})

	ctx := t.Context()
	svc.Start(ctx)
	defer svc.Stop()

	ev := eventbridge.Event{ID: "evt-ok", Topic: "system.llm.created", Origin: "control"}
	svc.HandleEvent(ev)

	delivered := waitForEvent(t, svc, sub.ID, "delivered", 10*time.Second)
	if delivered.Status != "delivered" {
		t.Errorf("expected delivered, got %s", delivered.Status)
	}

	var logs []models.WebhookDeliveryLog
	svc.db.Where("subscription_id = ?", sub.ID).Find(&logs)
	if len(logs) != 1 {
		t.Fatalf("expected 1 delivery log, got %d", len(logs))
	}
	if logs[0].Status != models.DeliveryStatusSuccess {
		t.Errorf("log status: want success, got %s", logs[0].Status)
	}
}

func TestDelivery_Failure(t *testing.T) {
	svc := setupFastService(t, 1, 30)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	sub := createSub(t, svc, ts.URL, []string{"system.llm.created"})

	ctx := t.Context()
	svc.Start(ctx)
	defer svc.Stop()

	ev := eventbridge.Event{ID: "evt-fail", Topic: "system.llm.created", Origin: "control"}
	svc.HandleEvent(ev)

	waitForEvent(t, svc, sub.ID, "exhausted", 2*time.Second)

	var logs []models.WebhookDeliveryLog
	svc.db.Where("subscription_id = ?", sub.ID).Find(&logs)
	if len(logs) != 1 {
		t.Fatalf("expected 1 delivery log, got %d", len(logs))
	}
	if logs[0].Status != models.DeliveryStatusFailed {
		t.Errorf("log status: want failed, got %s", logs[0].Status)
	}
}

func TestDelivery_Retry(t *testing.T) {
	var callCount int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		if n == 1 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer ts.Close()

	svc := setupFastService(t, 3, 30)
	sub := createSub(t, svc, ts.URL, []string{"system.llm.created"})

	ctx := t.Context()
	svc.Start(ctx)
	defer svc.Stop()

	ev := eventbridge.Event{ID: "evt-retry", Topic: "system.llm.created", Origin: "control"}
	svc.HandleEvent(ev)

	waitForEvent(t, svc, sub.ID, "delivered", 5*time.Second)

	var logs []models.WebhookDeliveryLog
	svc.db.Where("subscription_id = ?", sub.ID).Find(&logs)
	if len(logs) < 2 {
		t.Fatalf("expected at least 2 delivery log entries, got %d", len(logs))
	}
	last := logs[len(logs)-1]
	if last.Status != models.DeliveryStatusSuccess {
		t.Errorf("last log status: want success, got %s", last.Status)
	}
}

func TestDelivery_MaxRetries(t *testing.T) {
	maxAttempts := 3
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	svc := setupFastService(t, maxAttempts, 30)
	sub := createSub(t, svc, ts.URL, []string{"system.llm.created"})

	ctx := t.Context()
	svc.Start(ctx)
	defer svc.Stop()

	ev := eventbridge.Event{ID: "evt-max", Topic: "system.llm.created", Origin: "control"}
	svc.HandleEvent(ev)

	waitForEvent(t, svc, sub.ID, "exhausted", 5*time.Second)

	var logs []models.WebhookDeliveryLog
	svc.db.Where("subscription_id = ?", sub.ID).Find(&logs)
	if len(logs) != maxAttempts {
		t.Fatalf("expected %d delivery log entries, got %d", maxAttempts, len(logs))
	}
	for _, l := range logs {
		if l.Status != models.DeliveryStatusFailed {
			t.Errorf("log status: want failed, got %s", l.Status)
		}
	}
}

func TestTestWebhook_Success(t *testing.T) {
	svc := setupService(t)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	sub := createSub(t, svc, ts.URL, []string{"system.llm.created"})
	if err := svc.TestWebhook(sub); err != nil {
		t.Fatalf("TestWebhook: %v", err)
	}
}

func TestTestWebhook_Failure(t *testing.T) {
	svc := setupService(t)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer ts.Close()

	sub := createSub(t, svc, ts.URL, []string{"system.llm.created"})
	if err := svc.TestWebhook(sub); err == nil {
		t.Fatal("expected error for non-2xx response")
	}
}

func TestWebhookCRUD(t *testing.T) {
	svc := setupService(t)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ts.Close()

	sub := &models.WebhookSubscription{
		Name:    "crud-test",
		URL:     ts.URL,
		Topics:  []models.WebhookTopic{{Topic: "system.llm.created"}},
		Enabled: true,
	}
	if err := svc.CreateWebhook(sub); err != nil {
		t.Fatalf("create: %v", err)
	}
	if sub.ID == 0 {
		t.Fatal("expected non-zero ID after create")
	}

	got, err := svc.GetWebhook(sub.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "crud-test" {
		t.Errorf("name: want crud-test, got %s", got.Name)
	}

	got.Name = "updated"
	if err := svc.UpdateWebhook(got); err != nil {
		t.Fatalf("update: %v", err)
	}
	got2, _ := svc.GetWebhook(sub.ID)
	if got2.Name != "updated" {
		t.Errorf("after update name: want updated, got %s", got2.Name)
	}

	all, err := svc.ListWebhooks()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 webhook, got %d", len(all))
	}

	if err := svc.DeleteWebhook(sub.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.GetWebhook(sub.ID); err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestRetryPolicy_Resolution_StaticDefaults(t *testing.T) {
	svc := setupService(t)
	sub := models.WebhookSubscription{}
	policy := svc.resolveRetryPolicy(sub)
	if policy.maxAttempts != svc.cfg.DefaultMaxAttempts {
		t.Errorf("maxAttempts: want %d, got %d", svc.cfg.DefaultMaxAttempts, policy.maxAttempts)
	}
	if policy.httpTimeout != svc.cfg.DefaultHTTPTimeout {
		t.Errorf("httpTimeout: want %v, got %v", svc.cfg.DefaultHTTPTimeout, policy.httpTimeout)
	}
}

func TestRetryPolicy_Resolution_DBGlobal(t *testing.T) {
	svc := setupService(t)

	cfg := &models.WebhookConfig{
		ID: models.WebhookConfigSingletonID,
		DefaultRetryPolicy: models.WebhookRetryPolicy{
			MaxAttempts:    7,
			TimeoutSeconds: 30,
		},
	}
	if err := svc.UpdateWebhookConfig(cfg); err != nil {
		t.Fatalf("update config: %v", err)
	}

	sub := models.WebhookSubscription{}
	policy := svc.resolveRetryPolicy(sub)
	if policy.maxAttempts != 7 {
		t.Errorf("maxAttempts: want 7, got %d", policy.maxAttempts)
	}
	if policy.httpTimeout != 30*time.Second {
		t.Errorf("httpTimeout: want 30s, got %v", policy.httpTimeout)
	}
}

func TestRetryPolicy_Resolution_PerSubscriptionOverride(t *testing.T) {
	svc := setupService(t)

	cfg := &models.WebhookConfig{
		ID: models.WebhookConfigSingletonID,
		DefaultRetryPolicy: models.WebhookRetryPolicy{
			MaxAttempts: 7,
		},
	}
	svc.UpdateWebhookConfig(cfg)

	sub := models.WebhookSubscription{
		RetryPolicy: models.WebhookRetryPolicy{
			MaxAttempts:    2,
			TimeoutSeconds: 5,
		},
	}
	policy := svc.resolveRetryPolicy(sub)
	if policy.maxAttempts != 2 {
		t.Errorf("maxAttempts: want 2 (sub override), got %d", policy.maxAttempts)
	}
	if policy.httpTimeout != 5*time.Second {
		t.Errorf("httpTimeout: want 5s, got %v", policy.httpTimeout)
	}
}

func TestRetryDelivery(t *testing.T) {
	svc := setupFastService(t, 3, 30)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	sub := createSub(t, svc, ts.URL, []string{"system.llm.created"})

	log := &models.WebhookDeliveryLog{
		SubscriptionID: sub.ID,
		EventTopic:     "system.llm.created",
		EventID:        "evt-retry-manual",
		Payload:        `{"topic":"system.llm.created"}`,
		AttemptNumber:  1,
		Status:         models.DeliveryStatusFailed,
		AttemptedAt:    time.Now(),
	}
	if err := svc.db.Create(log).Error; err != nil {
		t.Fatalf("create log: %v", err)
	}

	ctx := t.Context()
	svc.Start(ctx)
	defer svc.Stop()

	if err := svc.RetryDelivery(log.ID, 0); err != nil {
		t.Fatalf("RetryDelivery: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var ev models.WebhookEvent
		if svc.db.Where("subscription_id = ? AND event_id = ? AND status = ?", sub.ID, "evt-retry-manual", "delivered").First(&ev).Error == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timeout waiting for retried delivery to succeed")
}

func TestRetryDelivery_NotFound(t *testing.T) {
	svc := setupService(t)
	if err := svc.RetryDelivery(99999, 0); err == nil {
		t.Fatal("expected error for unknown log ID")
	}
}

func TestWebhookConfig_DefaultsAndUpdate(t *testing.T) {
	svc := setupService(t)

	cfg, err := svc.GetWebhookConfig()
	if err != nil {
		t.Fatalf("get config: %v", err)
	}
	if cfg.ID != models.WebhookConfigSingletonID {
		t.Errorf("singleton ID: want %d, got %d", models.WebhookConfigSingletonID, cfg.ID)
	}

	cfg.DefaultRetryPolicy = models.WebhookRetryPolicy{
		MaxAttempts:    3,
		BackoffSeconds: []int{5, 15, 60},
	}
	if err := svc.UpdateWebhookConfig(cfg); err != nil {
		t.Fatalf("update config: %v", err)
	}

	got, err := svc.GetWebhookConfig()
	if err != nil {
		t.Fatalf("get config after update: %v", err)
	}
	if got.DefaultRetryPolicy.MaxAttempts != 3 {
		t.Errorf("max_attempts: want 3, got %d", got.DefaultRetryPolicy.MaxAttempts)
	}
	if fmt.Sprint(got.DefaultRetryPolicy.BackoffSeconds) != "[5 15 60]" {
		t.Errorf("backoff: want [5 15 60], got %v", got.DefaultRetryPolicy.BackoffSeconds)
	}
}

func TestListWebhooks(t *testing.T) {
	svc := setupService(t)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ts.Close()

	svc.CreateWebhook(&models.WebhookSubscription{Name: "sub-a", URL: ts.URL, Topics: []models.WebhookTopic{{Topic: "system.llm.created"}}, Enabled: true})
	svc.CreateWebhook(&models.WebhookSubscription{Name: "sub-b", URL: ts.URL, Topics: []models.WebhookTopic{{Topic: "system.llm.created"}}, Enabled: true})

	all, err := svc.ListWebhooks()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("want 2 webhooks, got %d", len(all))
	}
}

func TestWebhookTransportConfig_Proxy(t *testing.T) {
	svc := setupService(t)

	// Use an invalid proxy URL — buildHTTPClient should reject it.
	sub := &models.WebhookSubscription{
		Name:    "proxy-test",
		URL:     "http://example.com",
		Enabled: true,
		TransportConfig: models.WebhookTransportConfig{
			ProxyURL: "://bad-proxy",
		},
	}
	_, err := buildHTTPClient(sub.TransportConfig, 5*time.Second)
	if err == nil {
		t.Fatal("expected error for invalid proxy URL")
	}

	// Valid proxy URL should succeed.
	sub.TransportConfig.ProxyURL = "http://proxy.example.com:3128"
	client, err := buildHTTPClient(sub.TransportConfig, 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error for valid proxy: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	_ = svc
}

func TestWebhookTransportConfig_InsecureSkipVerify(t *testing.T) {
	tc := models.WebhookTransportConfig{InsecureSkipVerify: true}
	client, err := buildHTTPClient(tc, 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok || !transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("expected InsecureSkipVerify=true on transport")
	}
}

func TestWebhookTransportConfig_InvalidCACert(t *testing.T) {
	tc := models.WebhookTransportConfig{TLSCACert: "not-a-pem-cert"}
	_, err := buildHTTPClient(tc, 5*time.Second)
	if err == nil {
		t.Fatal("expected error for invalid CA cert")
	}
}
