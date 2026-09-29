package pushes

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/cluster"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// StreamOpened is called when an edge opens a stream on this replica: any
// push waiting for it is delivered now.
func (c *Coordinator) StreamOpened(edgeID string) { c.poke() }

// StreamClosed is called when an edge's stream on this replica ends. A
// command in flight on it will never be answered there, so it goes back to
// pending at once (the edge may reconnect anywhere) instead of waiting for
// the answer timeout.
func (c *Coordinator) StreamClosed(edgeID, session string) {
	if session == "" {
		return
	}
	var cmds []models.EdgePushCommand
	if err := c.db.Where("edge_id = ? AND stream_session_id = ? AND status IN ?", edgeID, session, models.PushCommandInFlight).Find(&cmds).Error; err != nil {
		logger.Warnf("Edge pushes: could not look up pushes in flight to %s after its stream closed; the janitor retries them after %s: %v", edgeID, c.opts.AnswerTimeout, err)
		return
	}
	for _, cmd := range cmds {
		c.retry(cmd.ID, fmt.Sprintf("the edge's connection to replica %s closed before it answered", c.node), func(cur models.EdgePushCommand) bool {
			return inFlight(cur.Status) && cur.StreamSessionID == session
		})
	}
}

// errNotApplicable: the change no longer applies to the command as it is now.
var errNotApplicable = errors.New("change no longer applies")

// change applies a change to command id: it reads the command, lets build
// check that the change still applies and describe it, and writes it only
// if no other change was made in between; otherwise it reads the command
// again and repeats. It reports whether the change was made, and the
// command as it was read for it.
func (c *Coordinator) change(id int64, build func(cur models.EdgePushCommand) (map[string]interface{}, error)) (bool, models.EdgePushCommand, error) {
	for i := 0; i < 10; i++ {
		var cur models.EdgePushCommand
		if err := c.db.Where("id = ?", id).First(&cur).Error; err != nil {
			return false, cur, err
		}
		updates, err := build(cur)
		if errors.Is(err, errNotApplicable) {
			return false, cur, nil
		}
		if err != nil {
			return false, cur, err
		}
		updates["version"] = cur.Version + 1
		res := c.db.Model(&models.EdgePushCommand{}).Where("id = ? AND version = ?", id, cur.Version).Updates(updates)
		if res.Error != nil {
			return false, cur, res.Error
		}
		if res.RowsAffected == 1 {
			return true, cur, nil
		}
	}
	return false, models.EdgePushCommand{}, fmt.Errorf("push command %d kept changing under this replica", id)
}

// dispatch claims and sends the pending commands for edges whose stream
// this replica holds.
func (c *Coordinator) dispatch() {
	local := c.streams.LocalStreams()
	if len(local) == 0 {
		return
	}
	ids := make([]string, 0, len(local))
	for id := range local {
		ids = append(ids, id)
	}
	now := c.now()
	var cmds []models.EdgePushCommand
	if err := c.db.Where("status = ? AND edge_id IN ? AND attempts < max_attempts AND deadline_at > ?", models.PushCommandPending, ids, now).
		Order("id").Find(&cmds).Error; err != nil {
		logger.Warnf("Edge pushes: looking for pushes to deliver failed; retrying: %v", err)
		return
	}
	for _, cmd := range cmds {
		c.deliver(cmd.ID, local[cmd.EdgeID], now)
	}
}

// deliver claims command id for this replica and sends it on the edge's
// stream. The claim is a conditional write, so of two replicas that both
// see the edge (it reconnected and the old stream has not closed yet),
// only one sends.
func (c *Coordinator) deliver(id int64, session string, now time.Time) {
	claimUntil := now.Add(c.opts.ClaimTimeout)
	claimed, cmd, err := c.change(id, func(cur models.EdgePushCommand) (map[string]interface{}, error) {
		if cur.Status != models.PushCommandPending || cur.Attempts >= cur.MaxAttempts || !cur.DeadlineAt.After(now) {
			return nil, errNotApplicable // another replica claimed it first, or it was settled
		}
		return map[string]interface{}{
			"status":            models.PushCommandClaimed,
			"claimed_by":        c.node,
			"claim_expires_at":  claimUntil,
			"stream_session_id": session,
			"attempts":          cur.Attempts + 1,
			"updated_at":        now,
		}, nil
	})
	if err != nil {
		logger.Warnf("Edge pushes: claiming push command %d failed; retrying: %v", id, err)
		return
	}
	if !claimed {
		return
	}
	attempt := cmd.Attempts + 1
	// Ours while it is claimed by this replica for this stream and attempt.
	ours := func(cur models.EdgePushCommand) bool {
		return cur.Status == models.PushCommandClaimed && cur.ClaimedBy == c.node && cur.StreamSessionID == session && cur.Attempts == attempt
	}

	remaining := int64(time.Until(cmd.DeadlineAt).Seconds())
	if remaining < 1 {
		remaining = 1
	}
	var op models.PushOperation
	_ = c.db.Where("operation_id = ?", cmd.OperationID).First(&op).Error
	req := &pb.ConfigurationReloadRequest{
		OperationId:     cmd.OperationID,
		TargetNamespace: cmd.Namespace,
		TargetEdges:     []string{cmd.EdgeID},
		InitiatedBy:     op.InitiatedBy,
		TimeoutSeconds:  remaining,
		InitiatedAt:     timestamppb.New(op.CreatedAt),
	}
	if err := c.streams.SendReload(cmd.EdgeID, session, req); err != nil {
		if errors.Is(err, ErrNoStream) {
			c.unclaim(id, attempt, ours, err)
			return
		}
		c.retry(id, fmt.Sprintf("sending on replica %s failed: %v", c.node, err), ours)
		return
	}
	sentAt := c.now()
	marked, _, err := c.change(id, func(cur models.EdgePushCommand) (map[string]interface{}, error) {
		if !ours(cur) {
			// Requeued meanwhile (its stream closed, or the claim lapsed):
			// whoever did that recorded why. If the edge still answers
			// this send, the answer settles the command.
			return nil, errNotApplicable
		}
		return map[string]interface{}{
			"status":     models.PushCommandSent,
			"sent_at":    sentAt,
			"history":    models.PushAttemptsJSON(append(cur.History, models.PushAttempt{Attempt: attempt, Node: c.node, At: sentAt, Outcome: "sent"})),
			"updated_at": sentAt,
		}, nil
	})
	if err != nil {
		// The request is on the wire; if the edge answers, the answer
		// settles the command whatever its status says.
		logger.Warnf("Edge pushes: recording that the push %s was sent to %s failed: %v", cmd.OperationID, cmd.EdgeID, err)
	}
	if marked {
		logger.Infof("Edge push %s sent to %s on replica %s (attempt %d of %d)", cmd.OperationID, cmd.EdgeID, c.node, attempt, cmd.MaxAttempts)
	}
}

// unclaim hands back a claim made for a stream that closed before the
// push could be sent on it (this replica's view of its streams is a moment
// old). Nothing reached the edge, so the attempt is not counted.
func (c *Coordinator) unclaim(id int64, attempt int, ours func(models.EdgePushCommand) bool, cause error) {
	applied, cmd, err := c.change(id, func(cur models.EdgePushCommand) (map[string]interface{}, error) {
		if !ours(cur) {
			return nil, errNotApplicable
		}
		return map[string]interface{}{
			"status":            models.PushCommandPending,
			"attempts":          attempt - 1,
			"claimed_by":        "",
			"claim_expires_at":  nil,
			"stream_session_id": "",
			"updated_at":        c.now(),
		}, nil
	})
	if err != nil {
		logger.Warnf("Edge pushes: releasing push command %d failed; the janitor releases it after %s: %v", id, c.opts.ClaimTimeout, err)
		return
	}
	if applied {
		logger.Debugf("Edge push %s to %s not sent on replica %s: %v; waiting for its current stream", cmd.OperationID, cmd.EdgeID, c.node, cause)
		c.notify()
	}
}

// retry returns an in-flight command to pending so any replica can deliver
// it again, or fails it when it has used all its attempts. It acts only if
// still holds for the command as it is when the change is made.
func (c *Coordinator) retry(id int64, reason string, still func(models.EdgePushCommand) bool) {
	var failed bool
	applied, cmd, err := c.change(id, func(cur models.EdgePushCommand) (map[string]interface{}, error) {
		if !inFlight(cur.Status) || !still(cur) {
			return nil, errNotApplicable // settled or moved on meanwhile
		}
		now := c.now()
		updates := map[string]interface{}{
			"claimed_by":        "",
			"claim_expires_at":  nil,
			"stream_session_id": "",
			"message":           reason,
			"history":           models.PushAttemptsJSON(append(cur.History, models.PushAttempt{Attempt: cur.Attempts, Node: cur.ClaimedBy, At: now, Outcome: reason})),
			"updated_at":        now,
		}
		failed = cur.Attempts >= cur.MaxAttempts
		if failed {
			updates["status"] = models.PushCommandFailed
			updates["message"] = fmt.Sprintf("%s; gave up after %d attempts", reason, cur.Attempts)
			updates["completed_at"] = now
		} else {
			updates["status"] = models.PushCommandPending
		}
		return updates, nil
	})
	if err != nil {
		logger.Warnf("Edge pushes: returning push command %d to pending failed; the janitor retries it: %v", id, err)
		return
	}
	if !applied {
		return
	}
	outcome := "will retry"
	if failed {
		outcome = "failed"
	}
	logger.Warnf("Edge push %s to %s: %s (attempt %d of %d, %s)", cmd.OperationID, cmd.EdgeID, reason, cmd.Attempts, cmd.MaxAttempts, outcome)
	if failed {
		c.settle(cmd.OperationID)
	} else {
		c.notify()
	}
}

// HandleReloadResponse records an edge's report on a push. READY is checked
// against the namespace's expected checksum; FAILED is final (an edge that
// cannot apply a configuration would fail the same way again). Reports on
// settled commands, and on pushes this database does not know, are ignored.
func (c *Coordinator) HandleReloadResponse(resp *pb.ConfigurationReloadResponse) {
	if resp == nil {
		return
	}
	var cmd models.EdgePushCommand
	err := c.db.Where("operation_id = ? AND edge_id = ?", resp.OperationId, resp.EdgeId).First(&cmd).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		logger.Warnf("Edge %s reported on push %s, which is not one of its pushes; ignored", resp.EdgeId, resp.OperationId)
		return
	}
	if err != nil {
		logger.Warnf("Edge pushes: looking up push %s for %s failed; its report is lost, the answer timeout will retry it: %v", resp.OperationId, resp.EdgeId, err)
		return
	}
	if isTerminal(cmd.Status) {
		logger.Debugf("Edge %s reported %s on push %s, already %s; ignored", resp.EdgeId, resp.Phase, resp.OperationId, cmd.Status)
		return
	}

	phase := resp.Phase.String()
	switch resp.Phase {
	case pb.ReloadPhase_READY:
		status, warning, expected, loaded := c.verify(cmd)
		c.finish(cmd.ID, func(cur models.EdgePushCommand, now time.Time) map[string]interface{} {
			return map[string]interface{}{
				"status":            status,
				"phase":             phase,
				"phase_at":          now,
				"message":           resp.Message,
				"warning":           warning,
				"expected_checksum": expected,
				"loaded_checksum":   loaded,
				"history":           models.PushAttemptsJSON(append(cur.History, models.PushAttempt{Attempt: cur.Attempts, Node: c.node, At: now, Outcome: status})),
				"completed_at":      now,
				"updated_at":        now,
			}
		})
	case pb.ReloadPhase_FAILED:
		msg := resp.Message
		if msg == "" {
			msg = "the edge reported that applying the configuration failed"
		}
		c.finish(cmd.ID, func(cur models.EdgePushCommand, now time.Time) map[string]interface{} {
			return map[string]interface{}{
				"status":       models.PushCommandFailed,
				"phase":        phase,
				"phase_at":     now,
				"message":      msg,
				"history":      models.PushAttemptsJSON(append(cur.History, models.PushAttempt{Attempt: cur.Attempts, Node: c.node, At: now, Outcome: "edge reported failure"})),
				"completed_at": now,
				"updated_at":   now,
			}
		})
	default:
		// Progress: record it, which also resets the answer timeout.
		now := c.now()
		if _, _, err := c.change(cmd.ID, func(cur models.EdgePushCommand) (map[string]interface{}, error) {
			if !inFlight(cur.Status) {
				return nil, errNotApplicable
			}
			return map[string]interface{}{"phase": phase, "phase_at": now, "message": resp.Message, "updated_at": now}, nil
		}); err != nil {
			logger.Warnf("Edge pushes: recording %s's progress on %s failed: %v", resp.EdgeId, resp.OperationId, err)
		}
	}
}

// finish settles command id, unless it has been settled meanwhile.
func (c *Coordinator) finish(id int64, build func(cur models.EdgePushCommand, now time.Time) map[string]interface{}) {
	c.finishIf(id, func(models.EdgePushCommand) bool { return true }, build)
}

// finishIf settles command id if it is open and still holds.
func (c *Coordinator) finishIf(id int64, still func(models.EdgePushCommand) bool, build func(cur models.EdgePushCommand, now time.Time) map[string]interface{}) {
	var updates map[string]interface{}
	applied, cmd, err := c.change(id, func(cur models.EdgePushCommand) (map[string]interface{}, error) {
		if isTerminal(cur.Status) || !still(cur) {
			return nil, errNotApplicable
		}
		updates = build(cur, c.now())
		return updates, nil
	})
	if err != nil {
		logger.Warnf("Edge pushes: settling push command %d failed; it is retried: %v", id, err)
		return
	}
	if applied {
		logger.Infof("Edge push %s to %s: %v", cmd.OperationID, cmd.EdgeID, updates["status"])
		c.settle(cmd.OperationID)
	}
}

// verify compares what the edge loaded with what its namespace expects.
func (c *Coordinator) verify(cmd models.EdgePushCommand) (status, warning, expected, loaded string) {
	var edge models.EdgeInstance
	if err := c.db.Where("edge_id = ?", cmd.EdgeID).First(&edge).Error; err == nil {
		loaded = edge.LoadedChecksum
	}
	var ns models.NamespaceSyncStatus
	if err := ns.GetByNamespace(c.db, models.CanonicalNamespace(cmd.Namespace)); err == nil {
		expected = ns.ExpectedChecksum
	}
	switch {
	case expected == "" || loaded == expected:
		return models.PushCommandSucceeded, "", expected, loaded
	case loaded == "":
		return models.PushCommandSucceededWarning, "the edge reported ready but has not reported which configuration it loaded", expected, loaded
	default:
		return models.PushCommandSucceededWarning, fmt.Sprintf(
			"the edge reported ready with configuration %s, but its namespace now expects %s: the configuration changed during the push. The edge shows as Pending until it is pushed again.",
			short(loaded), short(expected)), expected, loaded
	}
}

// janitor is the same on every replica and only makes changes that are
// re-checked against the command as it is when made, so replicas running
// it at once agree.
func (c *Coordinator) janitor() {
	now := c.now()

	// Replicas that stopped (crashed, or lost the database) while holding
	// commands: their commands go back to pending.
	live, err := cluster.LiveNodes(c.db)
	if err != nil {
		logger.Warnf("Edge pushes: janitor could not list live replicas: %v", err)
	} else {
		liveSet := map[string]bool{}
		ids := make([]string, 0, len(live))
		for _, n := range live {
			ids = append(ids, n.NodeID)
			liveSet[n.NodeID] = true
		}
		q := c.db.Where("status IN ?", models.PushCommandInFlight)
		if len(ids) > 0 {
			q = q.Where("claimed_by NOT IN ?", ids)
		}
		c.eachCommand(q, func(cmd models.EdgePushCommand) {
			dead := cmd.ClaimedBy
			c.retry(cmd.ID, fmt.Sprintf("replica %s stopped before the edge answered", dead), func(cur models.EdgePushCommand) bool {
				return cur.ClaimedBy == dead && !liveSet[dead]
			})
		})
	}

	// Claims that were never sent.
	c.eachCommand(c.db.Where("status = ? AND claim_expires_at < ?", models.PushCommandClaimed, now), func(cmd models.EdgePushCommand) {
		c.retry(cmd.ID, fmt.Sprintf("replica %s did not send it within %s", cmd.ClaimedBy, c.opts.ClaimTimeout), func(cur models.EdgePushCommand) bool {
			return cur.Status == models.PushCommandClaimed && cur.ClaimExpiresAt != nil && cur.ClaimExpiresAt.Before(now)
		})
	})

	// Sent, and silent for too long (any phase report counts as word).
	silentSince := now.Add(-c.opts.AnswerTimeout)
	silent := func(cur models.EdgePushCommand) bool {
		last := cur.PhaseAt
		if last == nil {
			last = cur.SentAt
		}
		return cur.Status == models.PushCommandSent && last != nil && last.Before(silentSince)
	}
	c.eachCommand(c.db.Where("status = ? AND ((phase_at IS NULL AND sent_at < ?) OR phase_at < ?)", models.PushCommandSent, silentSince, silentSince), func(cmd models.EdgePushCommand) {
		c.retry(cmd.ID, fmt.Sprintf("the edge did not answer within %s", c.opts.AnswerTimeout), silent)
	})

	// Past the deadline.
	c.eachCommand(c.db.Where("status NOT IN ? AND deadline_at < ?", models.PushCommandTerminal, now), func(cmd models.EdgePushCommand) {
		c.finishIf(cmd.ID, func(cur models.EdgePushCommand) bool { return cur.DeadlineAt.Before(now) }, func(cur models.EdgePushCommand, at time.Time) map[string]interface{} {
			return map[string]interface{}{
				"status":       models.PushCommandExpired,
				"message":      expiryReason(cur),
				"completed_at": at,
				"updated_at":   at,
			}
		})
	})

	// Pending with no attempts left (a retry that raced its last attempt).
	c.eachCommand(c.db.Where("status = ? AND attempts >= max_attempts", models.PushCommandPending), func(cmd models.EdgePushCommand) {
		c.finishIf(cmd.ID, func(cur models.EdgePushCommand) bool {
			return cur.Status == models.PushCommandPending && cur.Attempts >= cur.MaxAttempts
		}, func(cur models.EdgePushCommand, at time.Time) map[string]interface{} {
			return map[string]interface{}{
				"status":       models.PushCommandFailed,
				"message":      fmt.Sprintf("%s; gave up after %d attempts", cur.Message, cur.Attempts),
				"completed_at": at,
				"updated_at":   at,
			}
		})
	})

	// Operations whose commands are all settled.
	var open []string
	if err := c.db.Model(&models.PushOperation{}).Where("status = ?", models.PushOperationInProgress).Pluck("operation_id", &open).Error; err == nil {
		for _, id := range open {
			c.settle(id)
		}
	}
}

func (c *Coordinator) eachCommand(q *gorm.DB, fn func(models.EdgePushCommand)) {
	var cmds []models.EdgePushCommand
	if err := q.Order("id").Limit(500).Find(&cmds).Error; err != nil {
		logger.Warnf("Edge pushes: janitor query failed: %v", err)
		return
	}
	for _, cmd := range cmds {
		fn(cmd)
	}
}

func expiryReason(cmd models.EdgePushCommand) string {
	switch {
	case cmd.Attempts == 0:
		return "the edge was not connected to any control-plane replica before the deadline"
	case cmd.Status == models.PushCommandPending:
		return fmt.Sprintf("the edge disconnected (%s) and did not reconnect before the deadline", cmd.Message)
	default:
		return "the edge did not finish reloading before the deadline"
	}
}

func inFlight(status string) bool {
	return status == models.PushCommandClaimed || status == models.PushCommandSent
}

// settle sets the operation's final status once all its commands are
// settled. It is safe to call any time, from any replica.
func (c *Coordinator) settle(operationID string) {
	counts, err := c.commandCounts(operationID)
	if err != nil {
		logger.Warnf("Edge pushes: counting results of %s failed: %v", operationID, err)
		return
	}
	status := operationStatus(counts)
	if status == models.PushOperationInProgress {
		return
	}
	now := c.now()
	res := c.db.Model(&models.PushOperation{}).Where("operation_id = ? AND status = ?", operationID, models.PushOperationInProgress).
		Updates(map[string]interface{}{"status": status, "completed_at": now})
	if res.Error == nil && res.RowsAffected > 0 {
		logger.Infof("Edge push %s finished: %s (%s)", operationID, status, summary(counts))
	}
}

func (c *Coordinator) commandCounts(operationID string) (map[string]int, error) {
	var rows []struct {
		Status string
		N      int
	}
	if err := c.db.Model(&models.EdgePushCommand{}).Select("status, count(*) AS n").Where("operation_id = ?", operationID).Group("status").Scan(&rows).Error; err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, r := range rows {
		counts[r.Status] = r.N
	}
	return counts, nil
}

// operationStatus derives an operation's status from its commands'.
func operationStatus(counts map[string]int) string {
	total, open := 0, 0
	for s, n := range counts {
		total += n
		if !isTerminal(s) {
			open += n
		}
	}
	if total == 0 {
		return models.PushOperationFailed
	}
	if open > 0 {
		return models.PushOperationInProgress
	}
	ok := counts[models.PushCommandSucceeded] + counts[models.PushCommandSucceededWarning]
	switch {
	case ok == total && counts[models.PushCommandSucceededWarning] > 0:
		return models.PushOperationSucceededWarning
	case ok == total:
		return models.PushOperationSucceeded
	case ok == 0 && counts[models.PushCommandExpired] == total:
		return models.PushOperationExpired
	case ok == 0:
		return models.PushOperationFailed
	default:
		return models.PushOperationPartiallyFailed
	}
}

// OperationStatus is a push with its per-edge results.
type OperationStatus struct {
	Operation models.PushOperation     `json:"operation"`
	Commands  []models.EdgePushCommand `json:"commands"`
	Counts    map[string]int           `json:"counts"`
	Progress  int                      `json:"progress"` // percent of commands settled
	Message   string                   `json:"message"`
	Targets   []Target                 `json:"targets"` // current reachability of the edges still waiting
}

// ErrOperationNotFound is returned by Status for an unknown operation.
var ErrOperationNotFound = errors.New("push operation not found")

// Status reports a push, from any replica.
func (c *Coordinator) Status(ctx context.Context, operationID string) (*OperationStatus, error) {
	db := c.db.WithContext(ctx)
	var op models.PushOperation
	if err := db.Where("operation_id = ?", operationID).First(&op).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: %s", ErrOperationNotFound, operationID)
		}
		return nil, err
	}
	st := &OperationStatus{Operation: op, Counts: map[string]int{}}
	if err := db.Where("operation_id = ?", operationID).Order("edge_id").Find(&st.Commands).Error; err != nil {
		return nil, err
	}
	settled := 0
	var waiting []string
	for _, cmd := range st.Commands {
		st.Counts[cmd.Status]++
		if isTerminal(cmd.Status) {
			settled++
		} else if cmd.Status == models.PushCommandPending {
			waiting = append(waiting, cmd.EdgeID)
		}
	}
	if len(st.Commands) > 0 {
		st.Progress = settled * 100 / len(st.Commands)
	}
	if len(waiting) > 0 {
		var edges []models.EdgeInstance
		if err := db.Where("edge_id IN ?", waiting).Find(&edges).Error; err == nil {
			for _, e := range edges {
				st.Targets = append(st.Targets, c.target(e))
			}
		}
	}
	st.Message = statusMessage(op, st.Counts, len(st.Commands))
	return st, nil
}

// OperationSummary is a push with its outcome counts, for listings.
type OperationSummary struct {
	Operation models.PushOperation `json:"operation"`
	Counts    map[string]int       `json:"counts"`
	Progress  int                  `json:"progress"`
	Message   string               `json:"message"`
}

// Recent lists pushes created since the given time, newest first, with
// their outcome counts.
func (c *Coordinator) Recent(ctx context.Context, since time.Time, limit int) ([]OperationSummary, error) {
	db := c.db.WithContext(ctx)
	var ops []models.PushOperation
	if err := db.Where("created_at > ?", since.UTC()).Order("created_at DESC").Limit(limit).Find(&ops).Error; err != nil {
		return nil, err
	}
	if len(ops) == 0 {
		return []OperationSummary{}, nil
	}
	ids := make([]string, len(ops))
	for i, op := range ops {
		ids[i] = op.OperationID
	}
	var rows []struct {
		OperationID string
		Status      string
		N           int
	}
	if err := db.Model(&models.EdgePushCommand{}).Select("operation_id, status, COUNT(*) AS n").
		Where("operation_id IN ?", ids).Group("operation_id, status").Scan(&rows).Error; err != nil {
		return nil, err
	}
	counts := map[string]map[string]int{}
	for _, r := range rows {
		if counts[r.OperationID] == nil {
			counts[r.OperationID] = map[string]int{}
		}
		counts[r.OperationID][r.Status] = r.N
	}
	out := make([]OperationSummary, len(ops))
	for i, op := range ops {
		cs := counts[op.OperationID]
		if cs == nil {
			cs = map[string]int{}
		}
		total, settled := 0, 0
		for status, n := range cs {
			total += n
			if isTerminal(status) {
				settled += n
			}
		}
		sum := OperationSummary{Operation: op, Counts: cs, Message: statusMessage(op, cs, total)}
		if total > 0 {
			sum.Progress = settled * 100 / total
		}
		out[i] = sum
	}
	return out, nil
}

func statusMessage(op models.PushOperation, counts map[string]int, total int) string {
	switch op.Status {
	case models.PushOperationInProgress:
		return fmt.Sprintf("Pushing to %d edge(s): %s", total, summary(counts))
	case models.PushOperationSucceeded:
		return fmt.Sprintf("All %d edge(s) loaded the configuration.", total)
	case models.PushOperationSucceededWarning:
		return fmt.Sprintf("All %d edge(s) reloaded, with warnings: %s", total, summary(counts))
	default:
		return fmt.Sprintf("Push finished: %s", summary(counts))
	}
}

func summary(counts map[string]int) string {
	order := []struct{ status, label string }{
		{models.PushCommandSucceeded, "updated"},
		{models.PushCommandSucceededWarning, "updated with a warning"},
		{models.PushCommandFailed, "failed"},
		{models.PushCommandExpired, "timed out"},
		{models.PushCommandSent, "reloading"},
		{models.PushCommandClaimed, "being sent"},
		{models.PushCommandPending, "waiting for connection"},
	}
	out := ""
	for _, o := range order {
		if n := counts[o.status]; n > 0 {
			if out != "" {
				out += ", "
			}
			out += fmt.Sprintf("%d %s", n, o.label)
		}
	}
	if out == "" {
		return "no edges"
	}
	return out
}

func isTerminal(status string) bool {
	for _, s := range models.PushCommandTerminal {
		if s == status {
			return true
		}
	}
	return false
}

func short(checksum string) string {
	if len(checksum) > 12 {
		return checksum[:12]
	}
	return checksum
}
