package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// Guardrail exceptions (features/GuardrailExceptions.md): an exception
// softens the action a guardrail filter takes on a verdict for one app,
// filter, detector and channel, bounded by the floor for the app's guardrail
// capability. Anyone who may write the app proposes; a full administrator
// approves (the API enforces that), and a full administrator's own exception
// is approved as it is created. The exception API is never exposed to
// plugins or agents.

var (
	// ErrInvalidGuardrailException is an exception the service refuses to
	// store: a bad field, or an action below the app's floor.
	ErrInvalidGuardrailException = errors.New("invalid guardrail exception")
	// ErrGuardrailExceptionConflict is an action the exception's state does
	// not allow: approving one that is not proposed, a duplicate scope, or
	// past the app's cap.
	ErrGuardrailExceptionConflict = errors.New("guardrail exception conflict")
	// ErrGuardrailExceptionChanged is an approval of an exception that
	// changed since the administrator reviewed it. It is also an
	// ErrGuardrailExceptionConflict.
	ErrGuardrailExceptionChanged = fmt.Errorf("%w: the exception changed since it was reviewed", ErrGuardrailExceptionConflict)
)

func invalidGuardrailException(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidGuardrailException, fmt.Sprintf(format, args...))
}

// GuardrailExceptionSpec is what an exception is created or changed with.
// On an update nil fields are left as they are; the filter and the source
// event cannot change.
type GuardrailExceptionSpec struct {
	FilterID         *uint
	Detector         *string
	Channel          *string
	ToolName         *string
	Action           *string
	Justification    *string
	ExpiresAfterDays *int
	SourceEvent      *uint
}

// GuardrailExceptionActor is who creates or changes an exception. A full
// administrator's change is approved as it is made.
type GuardrailExceptionActor struct {
	UserID    uint
	FullAdmin bool
}

// ListGuardrailExceptions lists an app's exceptions; status filters when
// not empty.
func (s *Service) ListGuardrailExceptions(appID uint, status string) ([]models.GuardrailException, error) {
	if _, err := s.GetAppByID(appID); err != nil {
		return nil, err
	}
	return models.ListGuardrailExceptions(s.DB, appID, status)
}

// ListFilterGuardrailExceptions lists a filter's exceptions across apps;
// status filters when not empty.
func (s *Service) ListFilterGuardrailExceptions(filterID uint, status string) ([]models.GuardrailException, error) {
	if _, err := s.GetFilterByID(filterID); err != nil {
		return nil, err
	}
	return models.ListGuardrailExceptionsForFilter(s.DB, filterID, status)
}

// GetGuardrailException loads one exception of an app.
func (s *Service) GetGuardrailException(appID, id uint) (*models.GuardrailException, error) {
	return models.GetGuardrailException(s.DB, appID, id)
}

// CreateGuardrailException proposes an exception for an app, or creates it
// approved when the actor is a full administrator (recorded as both proposer
// and approver). A proposal notifies the administrators.
func (s *Service) CreateGuardrailException(appID uint, actor GuardrailExceptionActor, spec GuardrailExceptionSpec) (*models.GuardrailException, error) {
	if spec.FilterID == nil || *spec.FilterID == 0 {
		return nil, invalidGuardrailException("filter_id is required")
	}
	e := &models.GuardrailException{
		AppID:            appID,
		FilterID:         *spec.FilterID,
		Status:           models.GuardrailExceptionProposed,
		ProposedBy:       actor.UserID,
		ExpiresAfterDays: models.DefaultGuardrailExceptionExpiryDays,
	}
	applyGuardrailExceptionSpec(e, spec)
	app, err := s.GetAppByID(appID)
	if err != nil {
		return nil, err
	}
	if spec.SourceEvent != nil && *spec.SourceEvent != 0 {
		if err := s.checkExceptionSourceEvent(appID, *spec.SourceEvent, e.FilterID); err != nil {
			return nil, err
		}
		e.SourceEvent = spec.SourceEvent
	}
	if err := s.validateGuardrailException(s.DB, app, e); err != nil {
		return nil, err
	}
	err = s.DB.Transaction(func(tx *gorm.DB) error {
		if err := checkExceptionScopeFree(tx, e); err != nil {
			return err
		}
		if actor.FullAdmin {
			if err := checkApprovedExceptionCap(tx, appID, 0); err != nil {
				return err
			}
			now := time.Now()
			e.Status, e.ApprovedBy, e.ApprovedAt = models.GuardrailExceptionApproved, &actor.UserID, &now
		}
		return tx.Create(e).Error
	})
	if err != nil {
		return nil, err
	}
	s.guardrailExceptionsChanged(appID)
	if e.Status == models.GuardrailExceptionProposed {
		s.notifyGuardrailExceptionProposed(app, e, actor.UserID)
	}
	return e, nil
}

// UpdateGuardrailException changes an exception. A retired one cannot
// change. A change to an approved exception returns it to proposed, unless
// the actor is a full administrator, whose change stays approved under their
// name.
func (s *Service) UpdateGuardrailException(appID, id uint, actor GuardrailExceptionActor, spec GuardrailExceptionSpec) (*models.GuardrailException, error) {
	if spec.FilterID != nil {
		return nil, invalidGuardrailException("an exception's filter cannot change; create one for the other filter")
	}
	if spec.SourceEvent != nil {
		return nil, invalidGuardrailException("an exception's source event cannot change")
	}
	app, err := s.GetAppByID(appID)
	if err != nil {
		return nil, err
	}
	var out *models.GuardrailException
	proposed := false
	err = s.DB.Transaction(func(tx *gorm.DB) error {
		e, err := models.GetGuardrailExceptionForUpdate(tx, appID, id)
		if err != nil {
			return err
		}
		if e.Status == models.GuardrailExceptionRetired {
			return fmt.Errorf("%w: a retired exception cannot change; create a new one", ErrGuardrailExceptionConflict)
		}
		next := *e
		applyGuardrailExceptionSpec(&next, spec)
		if next.ContentHash() == e.ContentHash() {
			out = e
			return nil
		}
		if err := s.validateGuardrailException(tx, app, &next); err != nil {
			return err
		}
		if !next.SameScope(e) {
			if err := checkExceptionScopeFree(tx, &next); err != nil {
				return err
			}
		}
		if e.Status == models.GuardrailExceptionApproved {
			if actor.FullAdmin {
				now := time.Now()
				next.ApprovedBy, next.ApprovedAt = &actor.UserID, &now
			} else {
				next.Status, next.ApprovedBy, next.ApprovedAt = models.GuardrailExceptionProposed, nil, nil
				proposed = true
			}
		}
		if err := tx.Save(&next).Error; err != nil {
			return err
		}
		out = &next
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.guardrailExceptionsChanged(appID)
	if proposed {
		s.notifyGuardrailExceptionProposed(app, out, actor.UserID)
	}
	return out, nil
}

// ApproveGuardrailException approves a proposed exception. contentHash is
// the models.GuardrailException.ContentHash of the exception the
// administrator reviewed: an approval of anything else is refused
// (ErrGuardrailExceptionChanged), and one without it is invalid. The floor
// is checked again against the app's capability now. An app has at most
// models.MaxApprovedGuardrailExceptionsPerApp approved.
func (s *Service) ApproveGuardrailException(appID, id, adminID uint, contentHash string) (*models.GuardrailException, error) {
	if contentHash == "" {
		return nil, invalidGuardrailException("content_hash of the reviewed exception is required")
	}
	app, err := s.GetAppByID(appID)
	if err != nil {
		return nil, err
	}
	var out *models.GuardrailException
	err = s.DB.Transaction(func(tx *gorm.DB) error {
		e, err := models.GetGuardrailExceptionForUpdate(tx, appID, id)
		if err != nil {
			return err
		}
		if e.Status != models.GuardrailExceptionProposed {
			return fmt.Errorf("%w: only a proposed exception can be approved (this one is %s)", ErrGuardrailExceptionConflict, e.Status)
		}
		if e.ContentHash() != contentHash {
			return fmt.Errorf("%w; reload it and review it again", ErrGuardrailExceptionChanged)
		}
		if err := s.validateGuardrailException(tx, app, e); err != nil {
			return err
		}
		if err := checkApprovedExceptionCap(tx, appID, e.ID); err != nil {
			return err
		}
		now := time.Now()
		e.Status, e.ApprovedBy, e.ApprovedAt = models.GuardrailExceptionApproved, &adminID, &now
		if err := tx.Save(e).Error; err != nil {
			return err
		}
		out = e
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.guardrailExceptionsChanged(appID)
	return out, nil
}

// RetireGuardrailException retires an exception: it is kept for the record
// and never applies again.
func (s *Service) RetireGuardrailException(appID, id uint) (*models.GuardrailException, error) {
	e, err := models.GetGuardrailException(s.DB, appID, id)
	if err != nil {
		return nil, err
	}
	if e.Status == models.GuardrailExceptionRetired {
		return e, nil
	}
	e.Status = models.GuardrailExceptionRetired
	if err := s.DB.Model(e).Update("status", models.GuardrailExceptionRetired).Error; err != nil {
		return nil, err
	}
	s.guardrailExceptionsChanged(appID)
	return e, nil
}

// DeleteGuardrailException removes an exception, whatever its status.
func (s *Service) DeleteGuardrailException(appID, id uint) error {
	e, err := models.GetGuardrailException(s.DB, appID, id)
	if err != nil {
		return err
	}
	if err := s.DB.Delete(e).Error; err != nil {
		return err
	}
	s.guardrailExceptionsChanged(appID)
	return nil
}

// ApprovedGuardrailExceptions returns the exceptions that apply to an app's
// requests now. An approved exception that has gone its expires_after_days
// without a hit drops back to proposed here and is left out; one outside the
// floor for the app's current capability is left out (and stays approved,
// so attesting the capability back restores it).
func (s *Service) ApprovedGuardrailExceptions(appID uint) ([]models.GuardrailException, error) {
	app := &models.App{}
	if err := s.DB.Select("id", "guardrail_capability").First(app, appID).Error; err != nil {
		return nil, err
	}
	rows, err := models.ListGuardrailExceptions(s.DB, appID, models.GuardrailExceptionApproved)
	if err != nil {
		return nil, err
	}
	capability := guardrails.Capability(app.GuardrailCapability)
	now := time.Now()
	out := make([]models.GuardrailException, 0, len(rows))
	lapsed := false
	for i := range rows {
		e := &rows[i]
		if e.Lapsed(now) {
			changed, err := models.LapseGuardrailException(s.DB, e.ID)
			if err != nil {
				slog.Warn("could not lapse an expired guardrail exception", "app_id", appID, "exception_id", e.ID, "error", err)
			}
			lapsed = lapsed || changed
			continue
		}
		if !guardrails.Allowed(capability, guardrails.Channel(e.Channel), e.Action) {
			continue
		}
		out = append(out, *e)
	}
	if lapsed && s.SystemEvents != nil {
		s.SystemEvents.EmitAppGuardrailExceptionsChanged(appID)
	}
	return out, nil
}

// applyGuardrailExceptionSpec writes the spec's set fields into e,
// trimmed.
func applyGuardrailExceptionSpec(e *models.GuardrailException, spec GuardrailExceptionSpec) {
	if spec.Detector != nil {
		e.Detector = strings.TrimSpace(*spec.Detector)
	}
	if spec.Channel != nil {
		e.Channel = strings.TrimSpace(*spec.Channel)
	}
	if spec.ToolName != nil {
		e.ToolName = strings.TrimSpace(*spec.ToolName)
	}
	if spec.Action != nil {
		e.Action = strings.TrimSpace(*spec.Action)
	}
	if spec.Justification != nil {
		e.Justification = strings.TrimSpace(*spec.Justification)
	}
	if spec.ExpiresAfterDays != nil {
		e.ExpiresAfterDays = *spec.ExpiresAfterDays
	}
}

// printableLine reports text with no control characters (so no line
// breaks) within max bytes.
func printableLine(s string, max int) bool {
	if len(s) > max || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return false
		}
	}
	return true
}

// validateGuardrailException checks the fields of e, that its filter is a
// guardrail filter, and that its action is within the floor for the app's
// capability.
func (s *Service) validateGuardrailException(db *gorm.DB, app *models.App, e *models.GuardrailException) error {
	var filter models.Filter
	if err := db.First(&filter, e.FilterID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return invalidGuardrailException("filter %d does not exist", e.FilterID)
		}
		return err
	}
	if !filter.IsGuardrail() {
		return invalidGuardrailException("filter %q is a script filter; exceptions apply to guardrail filters only", filter.Name)
	}
	if !printableLine(e.Detector, 128) {
		return invalidGuardrailException("detector must be at most 128 bytes of printable text on one line")
	}
	ch, err := guardrails.ParseChannel(e.Channel)
	if err != nil {
		return invalidGuardrailException("%v", err)
	}
	if e.ToolName != "" && !ch.IsTool() {
		return invalidGuardrailException("tool_name only applies to the tool channels (tool_result, tool_call, tool_definition)")
	}
	if !printableLine(e.ToolName, 256) {
		return invalidGuardrailException("tool_name must be at most 256 bytes of printable text on one line")
	}
	if e.Justification == "" {
		return invalidGuardrailException("justification is required")
	}
	if !printableLine(e.Justification, 500) {
		return invalidGuardrailException("justification must be one line of at most 500 bytes")
	}
	if e.ExpiresAfterDays < 1 || e.ExpiresAfterDays > 3650 {
		return invalidGuardrailException("expires_after_days must be 1-3650")
	}
	if err := guardrails.CheckException(guardrails.Capability(app.GuardrailCapability), ch, e.Action); err != nil {
		return invalidGuardrailException("%v", err)
	}
	return nil
}

// checkExceptionScopeFree refuses a second proposed or approved exception
// with the same scope as e.
func checkExceptionScopeFree(tx *gorm.DB, e *models.GuardrailException) error {
	var n int64
	q := tx.Model(&models.GuardrailException{}).
		Where("app_id = ? AND filter_id = ? AND detector = ? AND channel = ? AND tool_name = ? AND status IN ?",
			e.AppID, e.FilterID, e.Detector, e.Channel, e.ToolName,
			[]string{models.GuardrailExceptionProposed, models.GuardrailExceptionApproved})
	if e.ID != 0 {
		q = q.Where("id <> ?", e.ID)
	}
	if err := q.Count(&n).Error; err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("%w: the app already has an exception for this filter, detector, channel and tool; change that one", ErrGuardrailExceptionConflict)
	}
	return nil
}

// checkApprovedExceptionCap refuses approving one more exception (other
// than except) for an app at the cap.
func checkApprovedExceptionCap(tx *gorm.DB, appID, except uint) error {
	var n int64
	q := tx.Model(&models.GuardrailException{}).Where("app_id = ? AND status = ?", appID, models.GuardrailExceptionApproved)
	if except != 0 {
		q = q.Where("id <> ?", except)
	}
	if err := q.Count(&n).Error; err != nil {
		return err
	}
	if n >= models.MaxApprovedGuardrailExceptionsPerApp {
		return fmt.Errorf("%w: an app has at most %d approved guardrail exceptions; retire one first", ErrGuardrailExceptionConflict, models.MaxApprovedGuardrailExceptionsPerApp)
	}
	return nil
}

// checkExceptionSourceEvent checks that the compliance event an exception
// is created from is a guardrail event of this app recorded by this filter:
// the stored claim of where the exception came from must hold. A guardrail
// event names its filter twice, by name (filter_name) and by id
// (metadata.filter_id); both must be the exception's filter.
func (s *Service) checkExceptionSourceEvent(appID, eventID, filterID uint) error {
	var ev models.ComplianceEvent
	if err := s.DB.Select("id", "app_id", "event_type", "filter_name", "metadata").First(&ev, eventID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return invalidGuardrailException("compliance event %d does not exist", eventID)
		}
		return err
	}
	if ev.AppID != appID {
		return invalidGuardrailException("compliance event %d is not one of this app's", eventID)
	}
	if !strings.HasPrefix(ev.EventType, "guardrail.") {
		return invalidGuardrailException("compliance event %d is not a guardrail event", eventID)
	}
	var meta struct {
		FilterID *json.Number `json:"filter_id"`
	}
	dec := json.NewDecoder(strings.NewReader(ev.Metadata))
	dec.UseNumber()
	if ev.Metadata == "" || dec.Decode(&meta) != nil || meta.FilterID == nil || meta.FilterID.String() != strconv.FormatUint(uint64(filterID), 10) {
		return invalidGuardrailException("compliance event %d was not recorded by filter %d", eventID, filterID)
	}
	var filter models.Filter
	if err := s.DB.Select("id", "name").First(&filter, filterID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return invalidGuardrailException("filter %d does not exist", filterID)
		}
		return err
	}
	if ev.FilterName != filter.Name {
		return invalidGuardrailException("compliance event %d was not recorded by filter %d", eventID, filterID)
	}
	return nil
}

// notifyGuardrailExceptionProposed tells the administrators an exception
// waits for approval.
func (s *Service) notifyGuardrailExceptionProposed(app *models.App, e *models.GuardrailException, actorID uint) {
	if s.NotificationService == nil {
		return
	}
	link := fmt.Sprintf("/admin/apps/%d", app.ID)
	title := fmt.Sprintf("Guardrail exception proposed for %s", app.Name)
	detector := e.Detector
	if detector == "" {
		detector = "any detector"
	}
	content := fmt.Sprintf("A guardrail exception for the app **%s** is waiting for approval.\n\n"+
		"- **Filter:** %d\n- **Detector:** %s\n- **Channel:** %s\n- **Soften to:** %s\n- **Justification:** %s\n\n[Review it on the app page](%s)",
		app.Name, e.FilterID, detector, e.Channel, e.Action, e.Justification, link)
	if err := s.NotificationService.NotifyWithOptions(
		fmt.Sprintf("guardrail_exception_proposed_%d_%d", e.ID, time.Now().Unix()),
		title, content, models.NotifyAdmins,
		NotifyOptions{Type: "guardrail_exception", Link: link, ActorID: actorID},
	); err != nil {
		slog.Warn("could not notify administrators of a proposed guardrail exception", "app_id", app.ID, "exception_id", e.ID, "error", err)
	}
}

// guardrailExceptionsChanged announces a change to an app's exceptions:
// approved ones travel in the edge snapshot, so edges become pending.
func (s *Service) guardrailExceptionsChanged(appID uint) {
	if s.SystemEvents != nil {
		s.SystemEvents.EmitAppGuardrailExceptionsChanged(appID)
	}
}

// guardrailExceptionHitFlushEvery bounds how often an exception's hit count
// and last hit are written: often enough for expiry, which counts in days,
// and for the hit count shown on the app page.
const guardrailExceptionHitFlushEvery = time.Minute

// GuardrailExceptionHits counts the hits of exceptions and writes them in
// batches, off the request path: a hit is counted in memory and an
// exception's pending count is written at most once per
// guardrailExceptionHitFlushEvery (with the next hit after that, or by
// Flush).
type GuardrailExceptionHits struct {
	db *gorm.DB

	mu      sync.Mutex
	pending map[uint]int64
	flushed map[uint]time.Time
	// write is the database write; tests replace it.
	write func(id uint, n int64, at time.Time)
}

// GuardrailExceptionHits returns the service's hit recorder.
func (s *Service) GuardrailExceptionHits() *GuardrailExceptionHits {
	s.guardrailExceptionHitsOnce.Do(func() {
		s.guardrailExceptionHits = NewGuardrailExceptionHits(s.DB)
	})
	return s.guardrailExceptionHits
}

// NewGuardrailExceptionHits builds a recorder writing to db.
func NewGuardrailExceptionHits(db *gorm.DB) *GuardrailExceptionHits {
	h := &GuardrailExceptionHits{db: db, pending: map[uint]int64{}, flushed: map[uint]time.Time{}}
	h.write = func(id uint, n int64, at time.Time) {
		if err := models.AddGuardrailExceptionHits(h.db, id, n, at); err != nil {
			slog.Debug("could not record guardrail exception hits", "exception_id", id, "error", err)
		}
	}
	return h
}

// Hit records n hits of the exception (edges report counts in batches).
func (h *GuardrailExceptionHits) Hit(id uint, n int64) {
	if id == 0 || n <= 0 {
		return
	}
	now := time.Now()
	h.mu.Lock()
	h.pending[id] += n
	if last, ok := h.flushed[id]; ok && now.Sub(last) < guardrailExceptionHitFlushEvery {
		h.mu.Unlock()
		return
	}
	count := h.pending[id]
	delete(h.pending, id)
	h.flushed[id] = now
	h.mu.Unlock()
	go h.write(id, count, now)
}

// Flush writes every pending count now (at shutdown, or from a periodic
// sweep, so trailing hits are not held until the next one).
func (h *GuardrailExceptionHits) Flush() {
	now := time.Now()
	h.mu.Lock()
	pending := h.pending
	h.pending = map[uint]int64{}
	for id := range pending {
		h.flushed[id] = now
	}
	h.mu.Unlock()
	for id, n := range pending {
		h.write(id, n, now)
	}
}
