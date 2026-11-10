package grpc

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// An edge that failed over sends one event per attempt. The hub's ProxyLog
// must keep the marker so the failed primary row and the fallback row can be
// read together; dropping it here is exactly the class of bug that made edge
// logs vanish from the LLM view before (llm_id = 0).
func TestSendAnalyticsPulse_FailoverMarkerPersisted(t *testing.T) {
	db := setupPulseTestDB(t)
	server := setupControlServer(t, db)

	now := time.Now()
	pulse := &pb.AnalyticsPulse{
		EdgeId: "edge-1", EdgeNamespace: "test", SequenceNumber: 1, TotalRecords: 2,
		AnalyticsEvents: []*pb.AnalyticsEvent{
			{RequestId: "r1", AppId: 1, LlmId: 1, ModelName: "gpt-4o", Vendor: "openai", StatusCode: 503,
				Timestamp: timestamppb.New(now)},
			{RequestId: "r2", AppId: 1, LlmId: 2, ModelName: "claude-sonnet-4", Vendor: "anthropic", StatusCode: 200,
				Timestamp: timestamppb.New(now.Add(time.Second)), FailoverFromLlmId: 1, FailoverAttempt: 1},
		},
	}

	resp, err := server.SendAnalyticsPulse(context.Background(), pulse)
	require.NoError(t, err)
	require.True(t, resp.Success)
	time.Sleep(200 * time.Millisecond) // async analytics

	var logs []models.ProxyLog
	require.NoError(t, db.Order("time_stamp asc").Find(&logs).Error)
	require.Len(t, logs, 2)

	assert.Equal(t, uint(1), logs[0].LLMID)
	assert.Nil(t, logs[0].FailoverFromLLMID)
	assert.Equal(t, 0, logs[0].FailoverAttempt)

	assert.Equal(t, uint(2), logs[1].LLMID)
	require.NotNil(t, logs[1].FailoverFromLLMID)
	assert.Equal(t, uint(1), *logs[1].FailoverFromLLMID)
	assert.Equal(t, 1, logs[1].FailoverAttempt)
}

// A rung index without a from-id (an edge whose marker lost the primary id,
// or a future producer that only counts attempts) must still be recorded as
// an attempt: the guard used to copy failover_attempt only inside the
// from-id branch, so such a row landed as attempt 0 and was counted as a
// primary request.
func TestSendAnalyticsPulse_FailoverAttemptWithoutFromID(t *testing.T) {
	db := setupPulseTestDB(t)
	server := setupControlServer(t, db)

	now := time.Now()
	pulse := &pb.AnalyticsPulse{
		EdgeId: "edge-1", EdgeNamespace: "test", SequenceNumber: 1, TotalRecords: 1,
		AnalyticsEvents: []*pb.AnalyticsEvent{
			{RequestId: "r1", AppId: 1, LlmId: 2, ModelName: "gpt-4o", Vendor: "openai", StatusCode: 200,
				Timestamp: timestamppb.New(now), FailoverAttempt: 2},
		},
	}

	resp, err := server.SendAnalyticsPulse(context.Background(), pulse)
	require.NoError(t, err)
	require.True(t, resp.Success)
	time.Sleep(200 * time.Millisecond) // async analytics

	var logs []models.ProxyLog
	require.NoError(t, db.Find(&logs).Error)
	require.Len(t, logs, 1)
	assert.Nil(t, logs[0].FailoverFromLLMID, "no from-id was sent, none is invented")
	assert.Equal(t, 2, logs[0].FailoverAttempt)
}

// --- A batch sent again after a lost acknowledgement ----------------------
//
// An edge whose control plane goes away after storing a pulse, but before
// answering, sends the pulse again to the other control plane. A pulse that
// carries a boot ID is stored at most once (features/ActiveActive.md, Phase
// 2 item 4).

// pulseReplicas returns two control servers on one database, as two replicas
// of a control plane are, with the analytics handler writing to it.
func pulseReplicas(t *testing.T, db *gorm.DB) (a, b *ControlServer) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&models.LLMChatRecord{}, &models.ProxyLog{}, &models.EdgeInstance{},
		&models.ComplianceEvent{}, &models.ToolCallRecord{}, &models.EdgeAnalyticsBatch{}))
	a = setupControlServer(t, db)
	var err error
	b, err = NewControlServer(&Config{GRPCPort: 8081, GRPCHost: "localhost", AuthToken: "test-token", NodeID: "replica-b"}, db)
	require.NoError(t, err)
	t.Cleanup(analytics.ResetHandler)
	return a, b
}

// batchPulse is a pulse with every kind of record the hub stores, and spend.
func batchPulse(bootID string, seq uint64) *pb.AnalyticsPulse {
	now := time.Now()
	return &pb.AnalyticsPulse{
		EdgeId: "edge-1", EdgeNamespace: "test", SequenceNumber: seq, BootId: bootID, TotalRecords: 5,
		AnalyticsEvents: []*pb.AnalyticsEvent{
			{RequestId: "r1", AppId: 1, LlmId: 1, ModelName: "gpt-4o", Vendor: "openai", StatusCode: 200,
				RequestTokens: 100, ResponseTokens: 50, TotalTokens: 150, Cost: 120, Timestamp: timestamppb.New(now)},
			{RequestId: "r2", AppId: 1, LlmId: 1, ModelName: "gpt-4o", Vendor: "openai", StatusCode: 200,
				RequestTokens: 10, ResponseTokens: 5, TotalTokens: 15, Cost: 30, Timestamp: timestamppb.New(now)},
			// A policy block: a request log row, no spend.
			{RequestId: "r3", AppId: 1, LlmId: 1, ModelName: "gpt-4o", Vendor: "openai", StatusCode: 403,
				NoUsage: true, Timestamp: timestamppb.New(now)},
		},
		ComplianceEvents: []*pb.ComplianceEventProto{
			{AppId: 1, LlmId: 1, FilterName: "pii", EventType: "block", Severity: "high", Timestamp: timestamppb.New(now)},
		},
		ToolCalls: []*pb.ToolCallProto{
			{ToolId: 4, OperationId: "getWeather", ExecTimeMs: 12, Timestamp: timestamppb.New(now)},
		},
	}
}

type pulseStored struct {
	logs, records, events, calls, batches int64
	// spend is the App's cost over its chat records: what its budget usage
	// is computed from.
	spend  float64
	tokens int64
}

func countPulseStored(t *testing.T, db *gorm.DB) pulseStored {
	t.Helper()
	var c pulseStored
	require.NoError(t, db.Model(&models.ProxyLog{}).Count(&c.logs).Error)
	require.NoError(t, db.Model(&models.LLMChatRecord{}).Count(&c.records).Error)
	require.NoError(t, db.Model(&models.ComplianceEvent{}).Count(&c.events).Error)
	require.NoError(t, db.Model(&models.ToolCallRecord{}).Count(&c.calls).Error)
	require.NoError(t, db.Model(&models.EdgeAnalyticsBatch{}).Count(&c.batches).Error)
	require.NoError(t, db.Model(&models.LLMChatRecord{}).Where("app_id = ?", 1).Select("COALESCE(SUM(cost), 0)").Scan(&c.spend).Error)
	require.NoError(t, db.Model(&models.LLMChatRecord{}).Where("app_id = ?", 1).Select("COALESCE(SUM(total_tokens), 0)").Scan(&c.tokens).Error)
	return c
}

var onePulseStored = pulseStored{logs: 3, records: 2, events: 1, calls: 1, batches: 1, spend: 150, tokens: 165}

// The hub stores a batch and its answer is lost; the edge sends the batch
// again to a second replica. Records, spend and token counts are stored
// once, and the second replica acknowledges the repeat so the edge stops
// sending it.
func testRepeatedBatchStoredOnce(t *testing.T, db *gorm.DB) {
	a, b := pulseReplicas(t, db)
	ctx := context.Background()

	// Replica A stores the batch. Its answer never reaches the edge.
	resp, err := a.SendAnalyticsPulse(ctx, batchPulse("boot-1", 7))
	require.NoError(t, err)
	require.True(t, resp.Success)
	// Stored when the call returns: the acknowledgement means committed.
	require.Equal(t, onePulseStored, countPulseStored(t, db))

	// The edge sends the same batch to replica B.
	again, err := b.SendAnalyticsPulse(ctx, batchPulse("boot-1", 7))
	require.NoError(t, err)
	require.True(t, again.Success, "a repeat is acknowledged, or the edge would send it for a day")
	assert.Equal(t, "Analytics pulse already processed", again.Message)
	assert.Equal(t, resp.ProcessedRecords, again.ProcessedRecords)
	assert.Equal(t, uint64(7), again.SequenceNumber)

	time.Sleep(200 * time.Millisecond) // nothing arrives late from a background writer either
	assert.Equal(t, onePulseStored, countPulseStored(t, db), "the repeated batch was stored again")

	// The edge's next batch, and the first batch of its next run (sequence
	// numbers start again after a restart; the boot ID differs), are new.
	_, err = b.SendAnalyticsPulse(ctx, batchPulse("boot-1", 8))
	require.NoError(t, err)
	_, err = a.SendAnalyticsPulse(ctx, batchPulse("boot-2", 7))
	require.NoError(t, err)
	// Another edge that happens to use the same boot ID and number.
	other := batchPulse("boot-1", 7)
	other.EdgeId = "edge-2"
	_, err = a.SendAnalyticsPulse(ctx, other)
	require.NoError(t, err)

	c := countPulseStored(t, db)
	assert.EqualValues(t, 4, c.batches)
	assert.EqualValues(t, 8, c.records)
	assert.EqualValues(t, 600, c.spend)
}

func TestSendAnalyticsPulse_RepeatedBatchStoredOnce(t *testing.T) {
	testRepeatedBatchStoredOnce(t, setupPulseTestDB(t))
}

func TestSendAnalyticsPulse_RepeatedBatchStoredOnce_Postgres(t *testing.T) {
	testRepeatedBatchStoredOnce(t, budgetSyncPostgresDB(t))
}

// The repeat arrives at the second replica while the first is still storing
// the batch: exactly one of them stores it.
func testConcurrentRepeatStoredOnce(t *testing.T, db *gorm.DB) {
	a, b := pulseReplicas(t, db)
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		server := a
		if i%2 == 1 {
			server = b
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := server.SendAnalyticsPulse(context.Background(), batchPulse("boot-1", 3))
			if assert.NoError(t, err) {
				assert.True(t, resp.Success)
			}
		}()
	}
	wg.Wait()
	assert.Equal(t, onePulseStored, countPulseStored(t, db))
}

func TestSendAnalyticsPulse_ConcurrentRepeatStoredOnce_Postgres(t *testing.T) {
	testConcurrentRepeatStoredOnce(t, budgetSyncPostgresDB(t))
}

// A pulse from an edge that predates batch identities has no boot ID. It is
// stored as it always was, and, as before, twice if it is sent twice.
func TestSendAnalyticsPulse_OldFormatWithoutBatchIDIsStored(t *testing.T) {
	db := setupPulseTestDB(t)
	a, b := pulseReplicas(t, db)
	ctx := context.Background()

	old := batchPulse("", 7)
	resp, err := a.SendAnalyticsPulse(ctx, old)
	require.NoError(t, err)
	require.True(t, resp.Success)
	assert.Equal(t, "Analytics pulse processed successfully", resp.Message)

	want := onePulseStored
	want.batches = 0 // no identity, nothing recorded for it
	require.Eventually(t, func() bool { return countPulseStored(t, db) == want }, 5*time.Second, 20*time.Millisecond,
		"an old-format pulse is stored: %+v", countPulseStored(t, db))

	// Sent again it is stored again: there is nothing to recognise it by.
	_, err = b.SendAnalyticsPulse(ctx, proto.Clone(old).(*pb.AnalyticsPulse))
	require.NoError(t, err)
	require.Eventually(t, func() bool { return countPulseStored(t, db).records == 4 }, 5*time.Second, 20*time.Millisecond)
	assert.Zero(t, countPulseStored(t, db).batches)
}

// When the batch cannot be stored the edge is told so (and sends it again);
// nothing is stored and no key is recorded, so that next attempt succeeds.
func TestSendAnalyticsPulse_StoreFailureIsReportedAndRetryable(t *testing.T) {
	db := setupPulseTestDB(t)
	a, b := pulseReplicas(t, db)

	// The hub cannot record the batch.
	require.NoError(t, db.Migrator().RenameTable("edge_analytics_batches", "edge_analytics_batches_away"))
	_, err := a.SendAnalyticsPulse(context.Background(), batchPulse("boot-1", 1))
	require.Error(t, err)
	assert.Equal(t, codes.Unavailable, status.Code(err), "a retryable error, not a refusal")
	require.NoError(t, db.Migrator().RenameTable("edge_analytics_batches_away", "edge_analytics_batches"))
	assert.Equal(t, pulseStored{}, countPulseStored(t, db), "nothing of a batch that was not stored is kept")

	resp, err := b.SendAnalyticsPulse(context.Background(), batchPulse("boot-1", 1))
	require.NoError(t, err)
	require.True(t, resp.Success)
	assert.Equal(t, "Analytics pulse processed successfully", resp.Message)
	assert.Equal(t, onePulseStored, countPulseStored(t, db))
}

// An analytics handler that cannot store a batch once (a host's own handler)
// gets the records the way it always did.
func TestSendAnalyticsPulse_HandlerWithoutBatchSupport(t *testing.T) {
	db := setupPulseTestDB(t)
	a, _ := pulseReplicas(t, db)
	rec := &pulseRecorder{}

	resp, err := a.SendAnalyticsPulse(analytics.WithHandler(context.Background(), rec), batchPulse("boot-1", 1))
	require.NoError(t, err)
	require.True(t, resp.Success)
	rec.mu.Lock()
	defer rec.mu.Unlock()
	assert.Equal(t, 3, rec.logs)
	assert.Equal(t, 2, rec.records)
	assert.Equal(t, 1, rec.events)
	assert.Equal(t, 1, rec.calls)
	assert.Zero(t, countPulseStored(t, db).batches)
}

// pulseRecorder is an AnalyticsHandler without EdgeBatchRecorder.
type pulseRecorder struct {
	mu                           sync.Mutex
	logs, records, events, calls int
}

func (r *pulseRecorder) RecordChatRecord(context.Context, *models.LLMChatRecord)     {}
func (r *pulseRecorder) RecordChatLogEntry(context.Context, *models.LLMChatLogEntry) {}
func (r *pulseRecorder) RecordProxyLog(context.Context, *models.ProxyLog)            {}
func (r *pulseRecorder) SetAsGlobalHandler()                                         {}
func (r *pulseRecorder) RecordToolCall(context.Context, string, time.Time, int, uint) {
	r.mu.Lock()
	r.calls++
	r.mu.Unlock()
}
func (r *pulseRecorder) RecordChatRecordsBatch(_ context.Context, rs []*models.LLMChatRecord) {
	r.mu.Lock()
	r.records += len(rs)
	r.mu.Unlock()
}
func (r *pulseRecorder) RecordProxyLogsBatch(_ context.Context, ls []*models.ProxyLog) {
	r.mu.Lock()
	r.logs += len(ls)
	r.mu.Unlock()
}
func (r *pulseRecorder) RecordComplianceEvents(_ context.Context, es []*models.ComplianceEvent) {
	r.mu.Lock()
	r.events += len(es)
	r.mu.Unlock()
}
