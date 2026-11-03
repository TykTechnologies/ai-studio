package services

import (
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/testinfra/studiodb"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type exceptionFixture struct {
	svc      *Service
	db       *gorm.DB
	app      *models.App
	filter   *models.Filter
	script   *models.Filter
	admin    *models.User
	writer   *models.User
	asAdmin  GuardrailExceptionActor
	asWriter GuardrailExceptionActor
}

func newExceptionFixture(t *testing.T, capability string) *exceptionFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, studiodb.Migrate(db))
	require.NoError(t, db.AutoMigrate(&models.ComplianceEvent{}))
	svc := NewService(db)
	svc.NotificationService = NewTestNotificationService(db)

	admin := &models.User{Email: "admin@test.com", Name: "Admin", IsAdmin: true, NotificationsEnabled: true, EmailVerified: true}
	require.NoError(t, admin.Create(db))
	writer := &models.User{Email: "dev@test.com", Name: "Dev", NotificationsEnabled: true, EmailVerified: true}
	require.NoError(t, writer.Create(db))

	app := &models.App{Name: "Visor", UserID: writer.ID, GuardrailCapability: capability}
	require.NoError(t, app.Create(db))
	filter := &models.Filter{Name: "Wolf Defender", Kind: models.FilterKindGuardrail, Config: models.JSONMap{"provider": "http", "on_detect": "block"}}
	require.NoError(t, db.Create(filter).Error)
	script := &models.Filter{Name: "Script", Kind: models.FilterKindScript}
	require.NoError(t, db.Create(script).Error)

	return &exceptionFixture{
		svc: svc, db: db, app: app, filter: filter, script: script, admin: admin, writer: writer,
		asAdmin:  GuardrailExceptionActor{UserID: admin.ID, FullAdmin: true},
		asWriter: GuardrailExceptionActor{UserID: writer.ID},
	}
}

func ptr[T any](v T) *T { return &v }

func (f *exceptionFixture) spec(channel, action string) GuardrailExceptionSpec {
	return GuardrailExceptionSpec{
		FilterID:      &f.filter.ID,
		Detector:      ptr("prompt_injection"),
		Channel:       ptr(channel),
		Action:        ptr(action),
		Justification: ptr("PR diffs carry injection test vectors"),
	}
}

func TestGuardrailException_CreateByRole(t *testing.T) {
	f := newExceptionFixture(t, "read_only")

	t.Run("a full admin's own exception is approved at once", func(t *testing.T) {
		e, err := f.svc.CreateGuardrailException(f.app.ID, f.asAdmin, f.spec("text_user", "log"))
		require.NoError(t, err)
		assert.Equal(t, models.GuardrailExceptionApproved, e.Status)
		require.NotNil(t, e.ApprovedBy)
		assert.Equal(t, f.admin.ID, *e.ApprovedBy)
		assert.Equal(t, f.admin.ID, e.ProposedBy, "proposer = approver")
		assert.NotNil(t, e.ApprovedAt)
		assert.Equal(t, 30, e.ExpiresAfterDays)
	})

	t.Run("an apps writer proposes and the admins are notified", func(t *testing.T) {
		e, err := f.svc.CreateGuardrailException(f.app.ID, f.asWriter, f.spec("tool_result", "log"))
		require.NoError(t, err)
		assert.Equal(t, models.GuardrailExceptionProposed, e.Status)
		assert.Nil(t, e.ApprovedBy)

		var n []models.Notification
		require.NoError(t, f.db.Where("notification_id LIKE ?", "guardrail_exception_proposed_%").Find(&n).Error)
		require.Len(t, n, 1)
		assert.Equal(t, f.admin.ID, n[0].UserID)
		assert.Equal(t, "/admin/apps/"+itoa(f.app.ID), n[0].Link)
		assert.Contains(t, n[0].Title, "Visor")
	})

	t.Run("a second exception with the same scope is a conflict", func(t *testing.T) {
		_, err := f.svc.CreateGuardrailException(f.app.ID, f.asWriter, f.spec("text_user", "log"))
		assert.ErrorIs(t, err, ErrGuardrailExceptionConflict)
	})
}

func itoa(n uint) string { return strconv.FormatUint(uint64(n), 10) }

func TestGuardrailException_Validation(t *testing.T) {
	f := newExceptionFixture(t, "")

	bad := func(name string, mutate func(*GuardrailExceptionSpec), want string) {
		t.Run(name, func(t *testing.T) {
			s := f.spec("text_user", "quarantine")
			mutate(&s)
			_, err := f.svc.CreateGuardrailException(f.app.ID, f.asAdmin, s)
			require.ErrorIs(t, err, ErrInvalidGuardrailException)
			assert.Contains(t, err.Error(), want)
		})
	}
	bad("below the floor of an unattested app", func(s *GuardrailExceptionSpec) { s.Action = ptr("log") }, "not attested (treated as acting)")
	bad("tool results of an acting app", func(s *GuardrailExceptionSpec) { s.Channel = ptr("tool_result") }, "cannot soften")
	bad("no filter", func(s *GuardrailExceptionSpec) { s.FilterID = nil }, "filter_id is required")
	bad("unknown filter", func(s *GuardrailExceptionSpec) { s.FilterID = ptr(uint(999)) }, "does not exist")
	bad("script filter", func(s *GuardrailExceptionSpec) { s.FilterID = &f.script.ID }, "script filter")
	bad("unknown channel", func(s *GuardrailExceptionSpec) { s.Channel = ptr("assistant") }, "unknown channel")
	bad("tool name on a text channel", func(s *GuardrailExceptionSpec) { s.ToolName = ptr("read_file") }, "tool_name only applies")
	bad("no justification", func(s *GuardrailExceptionSpec) { s.Justification = ptr("  ") }, "justification is required")
	bad("multi-line justification", func(s *GuardrailExceptionSpec) { s.Justification = ptr("one\ntwo") }, "one line")
	bad("expiry out of range", func(s *GuardrailExceptionSpec) { s.ExpiresAfterDays = ptr(0) }, "expires_after_days")
	bad("redact is not an exception action", func(s *GuardrailExceptionSpec) { s.Action = ptr("redact") }, "can soften to")

	t.Run("within the floor is accepted", func(t *testing.T) {
		_, err := f.svc.CreateGuardrailException(f.app.ID, f.asAdmin, f.spec("text_user", "quarantine"))
		require.NoError(t, err)
	})

	t.Run("a source event must be this filter's guardrail event for the app", func(t *testing.T) {
		event := func(appID uint, eventType, filterName, metadata string) *uint {
			ev := &models.ComplianceEvent{AppID: appID, EventType: eventType, FilterName: filterName, Metadata: metadata}
			require.NoError(t, f.db.Create(ev).Error)
			return &ev.ID
		}
		mine := fmt.Sprintf(`{"filter_id":%d,"detector":"prompt_injection"}`, f.filter.ID)
		other := &models.App{Name: "Other", UserID: f.writer.ID}
		require.NoError(t, other.Create(f.db))
		otherFilter := &models.Filter{Name: "Other guardrail", Kind: models.FilterKindGuardrail}
		require.NoError(t, f.db.Create(otherFilter).Error)

		refused := map[string]*uint{
			"another app's":           event(other.ID, "guardrail.prompt_injection", f.filter.Name, mine),
			"another filter's":        event(f.app.ID, "guardrail.prompt_injection", otherFilter.Name, fmt.Sprintf(`{"filter_id":%d}`, otherFilter.ID)),
			"filter id of another":    event(f.app.ID, "guardrail.prompt_injection", f.filter.Name, fmt.Sprintf(`{"filter_id":%d}`, otherFilter.ID)),
			"name of another":         event(f.app.ID, "guardrail.prompt_injection", otherFilter.Name, mine),
			"a script filter's event": event(f.app.ID, "pii_redacted", f.script.Name, fmt.Sprintf(`{"filter_id":%d}`, f.script.ID)),
			"not a guardrail event":   event(f.app.ID, "pii_redacted", f.filter.Name, mine),
			"no filter id":            event(f.app.ID, "guardrail.prompt_injection", f.filter.Name, `{"detector":"x"}`),
			"no metadata":             event(f.app.ID, "guardrail.prompt_injection", f.filter.Name, ""),
		}
		for name, id := range refused {
			s := f.spec("text_system", "quarantine")
			s.SourceEvent = id
			_, err := f.svc.CreateGuardrailException(f.app.ID, f.asAdmin, s)
			assert.ErrorIs(t, err, ErrInvalidGuardrailException, name)
		}

		s := f.spec("text_system", "quarantine")
		s.SourceEvent = event(f.app.ID, "guardrail.prompt_injection", f.filter.Name, mine)
		e, err := f.svc.CreateGuardrailException(f.app.ID, f.asAdmin, s)
		require.NoError(t, err)
		assert.Equal(t, *s.SourceEvent, *e.SourceEvent)
	})
}

func TestGuardrailException_Approve(t *testing.T) {
	f := newExceptionFixture(t, "read_only")
	e, err := f.svc.CreateGuardrailException(f.app.ID, f.asWriter, f.spec("text_user", "log"))
	require.NoError(t, err)
	reviewed := e.ContentHash()

	_, err = f.svc.ApproveGuardrailException(f.app.ID, e.ID, f.admin.ID, "")
	assert.ErrorIs(t, err, ErrInvalidGuardrailException, "the reviewed hash is required")

	// The writer changes it after the admin's review.
	_, err = f.svc.UpdateGuardrailException(f.app.ID, e.ID, f.asWriter, GuardrailExceptionSpec{Detector: ptr("")})
	require.NoError(t, err)
	_, err = f.svc.ApproveGuardrailException(f.app.ID, e.ID, f.admin.ID, reviewed)
	assert.ErrorIs(t, err, ErrGuardrailExceptionChanged)

	cur, err := f.svc.GetGuardrailException(f.app.ID, e.ID)
	require.NoError(t, err)
	approved, err := f.svc.ApproveGuardrailException(f.app.ID, e.ID, f.admin.ID, cur.ContentHash())
	require.NoError(t, err)
	assert.Equal(t, models.GuardrailExceptionApproved, approved.Status)
	assert.Equal(t, f.admin.ID, *approved.ApprovedBy)

	_, err = f.svc.ApproveGuardrailException(f.app.ID, e.ID, f.admin.ID, approved.ContentHash())
	assert.ErrorIs(t, err, ErrGuardrailExceptionConflict, "only proposed exceptions are approved")

	t.Run("the floor is checked again at approval", func(t *testing.T) {
		p, err := f.svc.CreateGuardrailException(f.app.ID, f.asWriter, f.spec("tool_result", "log"))
		require.NoError(t, err)
		_, err = f.svc.SetAppGuardrailCapability(f.app.ID, "read_tools", f.admin.ID)
		require.NoError(t, err)
		_, err = f.svc.ApproveGuardrailException(f.app.ID, p.ID, f.admin.ID, p.ContentHash())
		assert.ErrorIs(t, err, ErrInvalidGuardrailException)
	})
}

func TestGuardrailException_UpdateOfApproved(t *testing.T) {
	f := newExceptionFixture(t, "read_only")
	e, err := f.svc.CreateGuardrailException(f.app.ID, f.asAdmin, f.spec("text_user", "log"))
	require.NoError(t, err)

	// A writer's change returns it to proposed.
	changed, err := f.svc.UpdateGuardrailException(f.app.ID, e.ID, f.asWriter, GuardrailExceptionSpec{Action: ptr("quarantine")})
	require.NoError(t, err)
	assert.Equal(t, models.GuardrailExceptionProposed, changed.Status)
	assert.Nil(t, changed.ApprovedBy)

	// An admin's change stays approved.
	approved, err := f.svc.ApproveGuardrailException(f.app.ID, e.ID, f.admin.ID, changed.ContentHash())
	require.NoError(t, err)
	again, err := f.svc.UpdateGuardrailException(f.app.ID, approved.ID, f.asAdmin, GuardrailExceptionSpec{ExpiresAfterDays: ptr(7)})
	require.NoError(t, err)
	assert.Equal(t, models.GuardrailExceptionApproved, again.Status)
	assert.Equal(t, 7, again.ExpiresAfterDays)

	_, err = f.svc.UpdateGuardrailException(f.app.ID, e.ID, f.asAdmin, GuardrailExceptionSpec{FilterID: &f.filter.ID})
	assert.ErrorIs(t, err, ErrInvalidGuardrailException, "the filter cannot change")

	retired, err := f.svc.RetireGuardrailException(f.app.ID, e.ID)
	require.NoError(t, err)
	assert.Equal(t, models.GuardrailExceptionRetired, retired.Status)
	_, err = f.svc.UpdateGuardrailException(f.app.ID, e.ID, f.asAdmin, GuardrailExceptionSpec{ExpiresAfterDays: ptr(8)})
	assert.ErrorIs(t, err, ErrGuardrailExceptionConflict)

	// A retired exception frees its scope.
	_, err = f.svc.CreateGuardrailException(f.app.ID, f.asAdmin, f.spec("text_user", "log"))
	require.NoError(t, err)
}

func TestGuardrailException_CapPerApp(t *testing.T) {
	f := newExceptionFixture(t, "read_only")
	for i := 0; i < models.MaxApprovedGuardrailExceptionsPerApp; i++ {
		s := f.spec("text_user", "log")
		s.Detector = ptr(itoa(uint(i)))
		_, err := f.svc.CreateGuardrailException(f.app.ID, f.asAdmin, s)
		require.NoError(t, err)
	}
	s := f.spec("text_user", "log")
	s.Detector = ptr("21st")
	_, err := f.svc.CreateGuardrailException(f.app.ID, f.asAdmin, s)
	assert.ErrorIs(t, err, ErrGuardrailExceptionConflict, "a full admin's 21st is refused")

	p, err := f.svc.CreateGuardrailException(f.app.ID, f.asWriter, s)
	require.NoError(t, err, "proposing past the cap is fine")
	_, err = f.svc.ApproveGuardrailException(f.app.ID, p.ID, f.admin.ID, p.ContentHash())
	assert.ErrorIs(t, err, ErrGuardrailExceptionConflict, "approving the 21st is refused")
}

func TestApprovedGuardrailExceptions_LapseAndFloor(t *testing.T) {
	f := newExceptionFixture(t, "read_only")
	fresh, err := f.svc.CreateGuardrailException(f.app.ID, f.asAdmin, f.spec("text_user", "log"))
	require.NoError(t, err)
	stale, err := f.svc.CreateGuardrailException(f.app.ID, f.asAdmin, f.spec("text_system", "log"))
	require.NoError(t, err)
	toolRes, err := f.svc.CreateGuardrailException(f.app.ID, f.asAdmin, f.spec("tool_result", "log"))
	require.NoError(t, err)

	// stale was approved 31 days ago and has not applied since.
	old := time.Now().Add(-31 * 24 * time.Hour)
	require.NoError(t, f.db.Model(&models.GuardrailException{}).Where("id = ?", stale.ID).Update("approved_at", old).Error)
	// fresh was approved as long ago, but hit yesterday.
	yesterday := time.Now().Add(-24 * time.Hour)
	require.NoError(t, f.db.Model(&models.GuardrailException{}).Where("id = ?", fresh.ID).
		Updates(map[string]any{"approved_at": old, "last_hit_at": yesterday}).Error)

	got, err := f.svc.ApprovedGuardrailExceptions(f.app.ID)
	require.NoError(t, err)
	ids := []uint{}
	for _, e := range got {
		ids = append(ids, e.ID)
	}
	assert.ElementsMatch(t, []uint{fresh.ID, toolRes.ID}, ids)

	lapsed, err := f.svc.GetGuardrailException(f.app.ID, stale.ID)
	require.NoError(t, err)
	assert.Equal(t, models.GuardrailExceptionProposed, lapsed.Status, "a lapsed exception returns to proposed")
	assert.Nil(t, lapsed.ApprovedBy)

	// The capability tightens: the tool-result exception no longer applies,
	// but stays approved so attesting back restores it.
	_, err = f.svc.SetAppGuardrailCapability(f.app.ID, "read_tools", f.admin.ID)
	require.NoError(t, err)
	got, err = f.svc.ApprovedGuardrailExceptions(f.app.ID)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, fresh.ID, got[0].ID)
	still, err := f.svc.GetGuardrailException(f.app.ID, toolRes.ID)
	require.NoError(t, err)
	assert.Equal(t, models.GuardrailExceptionApproved, still.Status)
}

func TestGuardrailExceptionHits(t *testing.T) {
	f := newExceptionFixture(t, "read_only")
	e, err := f.svc.CreateGuardrailException(f.app.ID, f.asAdmin, f.spec("text_user", "log"))
	require.NoError(t, err)

	h := NewGuardrailExceptionHits(f.db)
	var mu sync.Mutex
	writes := map[uint]int64{}
	var wg sync.WaitGroup
	write := h.write
	h.write = func(id uint, n int64, at time.Time) {
		defer wg.Done()
		write(id, n, at)
		mu.Lock()
		writes[id] += n
		mu.Unlock()
	}

	wg.Add(1)
	h.Hit(e.ID, 1) // written at once
	h.Hit(e.ID, 2) // held: within the flush interval
	h.Hit(e.ID, 3)
	wg.Wait()
	mu.Lock()
	assert.Equal(t, int64(1), writes[e.ID])
	mu.Unlock()

	wg.Add(1)
	h.Flush()
	wg.Wait()
	got, err := f.svc.GetGuardrailException(f.app.ID, e.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(6), got.HitCount)
	assert.NotNil(t, got.LastHitAt)
}
