package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Edge-served tool calls used to reach the control plane only as an aggregate
// count with no operation attached, so tool-usage-statistics,
// tool-operations-usage-over-time and tool-calls-per-day silently under-reported
// by whatever share of traffic the edges handled.
func TestSendAnalyticsPulse_ToolCallsAreRecorded(t *testing.T) {
	db := setupPulseTestDB(t)
	server := setupControlServer(t, db)

	now := time.Now()
	pulse := &pb.AnalyticsPulse{
		EdgeId:         "test-edge-tools",
		EdgeNamespace:  "test",
		SequenceNumber: 1,
		TotalRecords:   3,
		ToolCalls: []*pb.ToolCallProto{
			{ToolId: 4, OperationId: "getExchangeRate", ExecTimeMs: 120, Timestamp: timestamppb.New(now)},
			{ToolId: 4, OperationId: "getExchangeRate", ExecTimeMs: 98, Timestamp: timestamppb.New(now.Add(time.Second))},
			{ToolId: 4, OperationId: "getExchangeRates", ExecTimeMs: 210, Timestamp: timestamppb.New(now.Add(2 * time.Second))},
		},
	}

	response, err := server.SendAnalyticsPulse(context.Background(), pulse)
	require.NoError(t, err)
	assert.True(t, response.Success)
	assert.Equal(t, uint64(3), response.ProcessedRecords)

	require.Eventually(t, func() bool {
		var count int64
		db.Model(&models.ToolCallRecord{}).Count(&count)
		return count == 3
	}, 5*time.Second, 50*time.Millisecond, "edge tool calls must reach the tool analytics table")

	var records []models.ToolCallRecord
	require.NoError(t, db.Order("exec_time asc").Find(&records).Error)
	require.Len(t, records, 3)

	// Each record has to name its operation, which is what the per-operation
	// charts group by.
	byOperation := map[string]int{}
	for _, r := range records {
		assert.Equal(t, uint(4), r.ToolID)
		byOperation[r.Name]++
	}
	assert.Equal(t, 2, byOperation["getExchangeRate"])
	assert.Equal(t, 1, byOperation["getExchangeRates"])

	assert.Equal(t, 98, records[0].ExecTime)
}

func TestSendAnalyticsPulse_ToolCallWithoutTimestamp(t *testing.T) {
	db := setupPulseTestDB(t)
	server := setupControlServer(t, db)

	before := time.Now().Add(-time.Second)
	pulse := &pb.AnalyticsPulse{
		EdgeId:         "test-edge-no-ts",
		EdgeNamespace:  "test",
		SequenceNumber: 1,
		TotalRecords:   1,
		ToolCalls: []*pb.ToolCallProto{
			{ToolId: 7, OperationId: "search", ExecTimeMs: 10},
		},
	}

	response, err := server.SendAnalyticsPulse(context.Background(), pulse)
	require.NoError(t, err)
	assert.True(t, response.Success)

	require.Eventually(t, func() bool {
		var count int64
		db.Model(&models.ToolCallRecord{}).Count(&count)
		return count == 1
	}, 5*time.Second, 50*time.Millisecond)

	var record models.ToolCallRecord
	require.NoError(t, db.First(&record).Error)
	assert.False(t, record.TimeStamp.Before(before), "a missing timestamp falls back to receipt time")
}

func TestSendAnalyticsPulse_NoToolCalls(t *testing.T) {
	db := setupPulseTestDB(t)
	server := setupControlServer(t, db)

	pulse := &pb.AnalyticsPulse{
		EdgeId:         "test-edge-empty",
		EdgeNamespace:  "test",
		SequenceNumber: 1,
		ComplianceEvents: []*pb.ComplianceEventProto{
			{AppId: 1, EventType: "unrelated", Severity: "info", Timestamp: timestamppb.Now()},
		},
	}

	response, err := server.SendAnalyticsPulse(context.Background(), pulse)
	require.NoError(t, err)
	assert.True(t, response.Success)
	assert.Equal(t, uint64(1), response.ProcessedRecords)

	time.Sleep(200 * time.Millisecond)

	var count int64
	db.Model(&models.ToolCallRecord{}).Count(&count)
	assert.Equal(t, int64(0), count)
}
