package models

import (
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// purgeChunk bounds the span ids in one IN list.
const purgeChunk = 500

// PurgeAgentTraceSession deletes one session of one App and everything
// captured for it (spec §9.4 right to erasure): spans, links, payloads
// (content and previews), staged capture events of its spans, correlation
// keys, agents, the filter join rows, traces and the session row. It returns
// the number of rows deleted.
//
// The deletes run one after another, children first and the session row
// last, so a purge that fails part-way leaves the session visible and can
// simply be run again. No transaction: a long one would hold locks on the
// trace tables, and every step is idempotent.
func PurgeAgentTraceSession(db *gorm.DB, appID uint, sessionID string) (int64, error) {
	var spanIDs [][]byte
	if err := db.Model(&AgentTraceSpan{}).Where("app_id = ? AND session_id = ?", appID, sessionID).
		Pluck("span_id", &spanIDs).Error; err != nil {
		return 0, err
	}
	var total int64
	del := func(q *gorm.DB, model any) error {
		res := q.Delete(model)
		total += res.RowsAffected
		return res.Error
	}
	for start := 0; start < len(spanIDs); start += purgeChunk {
		ids := spanIDs[start:min(start+purgeChunk, len(spanIDs))]
		steps := []struct {
			q     *gorm.DB
			model any
		}{
			{db.Where("app_id = ? AND (span_id IN ? OR linked_span_id IN ?)", appID, ids, ids), &AgentTraceSpanLink{}},
			{db.Where("app_id = ? AND span_id IN ?", appID, ids), &AgentTracePayload{}},
			{db.Where("app_id = ? AND span_id IN ?", appID, ids), &AgentTraceCaptureEvent{}},
			{db.Where("app_id = ? AND span_id IN ?", appID, ids), &AgentTraceCorrelationKey{}},
		}
		for _, s := range steps {
			if err := del(s.q, s.model); err != nil {
				return total, err
			}
		}
	}
	rest := []struct {
		q     *gorm.DB
		model any
	}{
		{db.Where("app_id = ? AND session_id = ?", appID, sessionID), &AgentTraceCorrelationKey{}},
		{db.Where("session_id = ?", sessionID), &AgentTraceAgent{}},
		{db.Where("session_id = ?", sessionID), &AgentTraceSessionModel{}},
		{db.Where("session_id = ?", sessionID), &AgentTraceSessionTool{}},
		{db.Where("session_id = ?", sessionID), &AgentTraceSessionAgent{}},
		{db.Where("app_id = ? AND session_id = ?", appID, sessionID), &AgentTraceSpan{}},
		{db.Where("app_id = ? AND session_id = ?", appID, sessionID), &AgentTraceTrace{}},
		{db.Where("app_id = ? AND id = ?", appID, sessionID), &AgentTraceSession{}},
	}
	for _, s := range rest {
		if err := del(s.q, s.model); err != nil {
			return total, err
		}
	}
	return total, nil
}

// AgentTraceSessionsOfEndUser returns the sessions in scope whose declared
// end user is endUser (exact match), for a right-to-erasure purge.
func AgentTraceSessionsOfEndUser(db *gorm.DB, scope AgentTraceAppScope, endUser string) ([]AgentTraceSession, error) {
	var out []AgentTraceSession
	err := scope.apply(db.Model(&AgentTraceSession{}), "app_id").Select("id", "app_id").
		Where("end_user = ?", endUser).Find(&out).Error
	return out, err
}
