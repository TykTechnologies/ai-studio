package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/guardrails/templates"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// Trusted prompt templates (features/TrustedPromptTemplates.md): an app's own
// prompt text that request guardrails do not score once an administrator has
// approved its exact bytes. Anyone who may write the app proposes; only an
// administrator approves (the API enforces that).

var (
	// ErrInvalidPromptTemplate is a template the service refuses to store.
	ErrInvalidPromptTemplate = errors.New("invalid prompt template")
	// ErrPromptTemplateConflict is an action the template's state does not
	// allow: approving one that is not proposed, or past the app's cap.
	ErrPromptTemplateConflict = errors.New("prompt template conflict")
)

// promptTemplateName keeps names usable as name@version in events.
var promptTemplateName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// PromptTemplateSpec is what a template is created or changed with. On an
// update nil fields are left as they are.
type PromptTemplateSpec struct {
	Name             string
	Placement        *templates.Placement
	Parts            []templates.Part
	ExpiresAfterDays *int
}

func invalidPromptTemplate(err error) error {
	return fmt.Errorf("%w: %v", ErrInvalidPromptTemplate, err)
}

// ListAppPromptTemplates lists an app's template versions; status filters
// when not empty.
func (s *Service) ListAppPromptTemplates(appID uint, status string) ([]models.AppPromptTemplate, error) {
	return models.ListAppPromptTemplates(s.DB, appID, status)
}

// GetAppPromptTemplate loads one template version of an app.
func (s *Service) GetAppPromptTemplate(appID, id uint) (*models.AppPromptTemplate, error) {
	return models.GetAppPromptTemplate(s.DB, appID, id)
}

// CreateAppPromptTemplate proposes a template: version 1 of a new name, or
// the next version of an existing one.
func (s *Service) CreateAppPromptTemplate(appID, userID uint, spec PromptTemplateSpec) (*models.AppPromptTemplate, error) {
	if !promptTemplateName.MatchString(spec.Name) {
		return nil, invalidPromptTemplate(errors.New("name must be 1-128 letters, digits, '.', '_' or '-', starting with a letter or digit"))
	}
	if spec.Placement == nil {
		return nil, invalidPromptTemplate(errors.New("placement is required"))
	}
	t := &models.AppPromptTemplate{AppID: appID, Name: spec.Name, CreatedBy: userID, Status: templates.StatusProposed, ExpiresAfterDays: models.DefaultPromptTemplateExpiryDays}
	t.SetPlacement(spec.Placement.Normalize())
	t.Parts = spec.Parts
	if spec.ExpiresAfterDays != nil {
		t.ExpiresAfterDays = *spec.ExpiresAfterDays
	}
	if err := validatePromptTemplate(t); err != nil {
		return nil, err
	}
	if _, err := s.GetAppByID(appID); err != nil {
		return nil, err
	}
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		latest, err := models.LatestAppPromptTemplateVersion(tx, appID, spec.Name)
		if err != nil {
			return err
		}
		t.Version = latest + 1
		return t.Create(tx)
	})
	if err != nil {
		return nil, err
	}
	s.promptTemplatesChanged(appID)
	return t, nil
}

// UpdateAppPromptTemplate changes a template. A proposed version changes in
// place. A change to the parts or placement of an approved or retired one
// creates the next version, proposed, and leaves the old one as it was: its
// bytes were what was approved. Other fields change in place.
func (s *Service) UpdateAppPromptTemplate(appID, id, userID uint, spec PromptTemplateSpec) (*models.AppPromptTemplate, error) {
	t, err := models.GetAppPromptTemplate(s.DB, appID, id)
	if err != nil {
		return nil, err
	}
	if spec.Name != "" && spec.Name != t.Name {
		return nil, invalidPromptTemplate(errors.New("a template cannot be renamed; create one under the new name"))
	}
	next := *t
	if spec.Placement != nil {
		next.SetPlacement(spec.Placement.Normalize())
	}
	if spec.Parts != nil {
		next.Parts = spec.Parts
	}
	if spec.ExpiresAfterDays != nil {
		next.ExpiresAfterDays = *spec.ExpiresAfterDays
	}
	if err := validatePromptTemplate(&next); err != nil {
		return nil, err
	}
	trusted := next.Placement() != t.Placement() || !slices.Equal(next.Parts, t.Parts)
	if trusted && t.Status != templates.StatusProposed {
		return s.CreateAppPromptTemplate(appID, userID, PromptTemplateSpec{Name: t.Name, Placement: new(next.Placement()), Parts: next.Parts, ExpiresAfterDays: &next.ExpiresAfterDays})
	}
	if err := next.Update(s.DB); err != nil {
		return nil, err
	}
	s.promptTemplatesChanged(appID)
	return &next, nil
}

// DeleteAppPromptTemplate removes a template version, whatever its status:
// the guardrails then score the text it covered.
func (s *Service) DeleteAppPromptTemplate(appID, id uint) error {
	t, err := models.GetAppPromptTemplate(s.DB, appID, id)
	if err != nil {
		return err
	}
	if err := t.Delete(s.DB); err != nil {
		return err
	}
	s.promptTemplatesChanged(appID)
	return nil
}

// ApproveAppPromptTemplate approves a proposed version. The other approved
// versions of the same name are retired: one version of a name is trusted
// at a time. An app has at most templates.MaxApprovedPerApp approved.
func (s *Service) ApproveAppPromptTemplate(appID, id, adminID uint) (*models.AppPromptTemplate, error) {
	var out *models.AppPromptTemplate
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		t, err := models.GetAppPromptTemplate(tx, appID, id)
		if err != nil {
			return err
		}
		if t.Status != templates.StatusProposed {
			return fmt.Errorf("%w: only a proposed template can be approved (this one is %s)", ErrPromptTemplateConflict, t.Status)
		}
		if err := validatePromptTemplate(t); err != nil {
			return err
		}
		approved, err := models.ListAppPromptTemplates(tx, appID, templates.StatusApproved)
		if err != nil {
			return err
		}
		others := 0
		for i := range approved {
			if approved[i].Name == t.Name {
				if err := retire(tx, &approved[i]); err != nil {
					return err
				}
				continue
			}
			others++
		}
		if others >= templates.MaxApprovedPerApp {
			return fmt.Errorf("%w: an app has at most %d approved templates; retire one first", ErrPromptTemplateConflict, templates.MaxApprovedPerApp)
		}
		now := time.Now()
		t.Status, t.ApprovedBy, t.ApprovedAt = templates.StatusApproved, &adminID, &now
		if err := t.Update(tx); err != nil {
			return err
		}
		out = t
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.promptTemplatesChanged(appID)
	return out, nil
}

// RetireAppPromptTemplate retires a version: it is kept for the record and
// never matched again.
func (s *Service) RetireAppPromptTemplate(appID, id uint) (*models.AppPromptTemplate, error) {
	t, err := models.GetAppPromptTemplate(s.DB, appID, id)
	if err != nil {
		return nil, err
	}
	if t.Status == templates.StatusRetired {
		return t, nil
	}
	if err := retire(s.DB, t); err != nil {
		return nil, err
	}
	s.promptTemplatesChanged(appID)
	return t, nil
}

func retire(db *gorm.DB, t *models.AppPromptTemplate) error {
	t.Status = templates.StatusRetired
	return t.Update(db)
}

func validatePromptTemplate(t *models.AppPromptTemplate) error {
	if t.ExpiresAfterDays < 1 || t.ExpiresAfterDays > 3650 {
		return invalidPromptTemplate(errors.New("expires_after_days must be 1-3650"))
	}
	if err := templates.Validate(t.Placement(), t.Parts); err != nil {
		return invalidPromptTemplate(err)
	}
	return nil
}

// ApprovedAppPromptTemplates returns the templates the guardrails of an app's
// requests match against. An approved template that has gone its
// expires_after_days without a match drops back to proposed here, and is
// left out.
func (s *Service) ApprovedAppPromptTemplates(appID uint) ([]*templates.Template, error) {
	rows, err := models.ListAppPromptTemplates(s.DB, appID, templates.StatusApproved)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	out := make([]*templates.Template, 0, len(rows))
	for i := range rows {
		t := &rows[i]
		if t.Lapsed(now) {
			res := s.DB.Model(&models.AppPromptTemplate{}).
				Where("id = ? AND status = ?", t.ID, templates.StatusApproved).
				Updates(map[string]any{"status": templates.StatusProposed, "approved_by": nil, "approved_at": nil})
			if res.Error != nil {
				slog.Warn("could not lapse an expired prompt template", "app_id", appID, "template", t.Matcher().Ref(), "error", res.Error)
			}
			continue
		}
		out = append(out, t.Matcher())
	}
	return out, nil
}

// promptTemplatesChanged drops the source's cached templates of the app on
// this replica (other replicas see the change within promptTemplateTTL) and
// announces the change: approved templates are in the edge snapshot, so
// edges become pending until the next push.
func (s *Service) promptTemplatesChanged(appID uint) {
	s.PromptTemplateSource().forget(appID)
	if s.SystemEvents != nil {
		s.SystemEvents.EmitAppPromptTemplatesChanged(appID)
	}
}

// promptTemplateTTL is how long the guardrails use an app's approved
// templates before reading them again.
const promptTemplateTTL = 15 * time.Second

// promptTemplateTouchEvery bounds how often a template's last_matched_at is
// written: often enough for expiry, which counts in days.
const promptTemplateTouchEvery = 10 * time.Minute

// PromptTemplateSource serves the approved templates of an app to the
// request guardrails (scripting.SetPromptTemplateSource), cached per app,
// and records their matches.
type PromptTemplateSource struct {
	svc *Service

	mu      sync.Mutex
	cache   map[uint]promptTemplateEntry
	touched map[uint]time.Time
}

type promptTemplateEntry struct {
	at        time.Time
	templates []*templates.Template
}

// PromptTemplateSource returns the service's template source.
func (s *Service) PromptTemplateSource() *PromptTemplateSource {
	s.promptTemplatesOnce.Do(func() {
		s.promptTemplates = &PromptTemplateSource{svc: s, cache: map[uint]promptTemplateEntry{}, touched: map[uint]time.Time{}}
	})
	return s.promptTemplates
}

// ApprovedTemplates returns the app's approved templates, nil when it has
// none or they cannot be read (the guardrails then score everything).
func (p *PromptTemplateSource) ApprovedTemplates(ctx context.Context, appID uint) []*templates.Template {
	if appID == 0 {
		return nil
	}
	p.mu.Lock()
	e, ok := p.cache[appID]
	p.mu.Unlock()
	if ok && time.Since(e.at) < promptTemplateTTL {
		return e.templates
	}
	list, err := p.svc.ApprovedAppPromptTemplates(appID)
	if err != nil {
		slog.Warn("could not read the app's prompt templates; scoring its requests in full", "app_id", appID, "error", err)
		return nil
	}
	if len(list) == 0 {
		list = nil
	}
	p.mu.Lock()
	p.cache[appID] = promptTemplateEntry{at: time.Now(), templates: list}
	p.mu.Unlock()
	return list
}

// Matched records that a template matched a request, at most once per
// promptTemplateTouchEvery per template, off the request path.
func (p *PromptTemplateSource) Matched(t *templates.Template) {
	if t == nil || t.RecordID == 0 {
		return
	}
	now := time.Now()
	p.mu.Lock()
	last, ok := p.touched[t.RecordID]
	if ok && now.Sub(last) < promptTemplateTouchEvery {
		p.mu.Unlock()
		return
	}
	p.touched[t.RecordID] = now
	p.mu.Unlock()
	go func() {
		if err := models.TouchAppPromptTemplate(p.svc.DB, t.RecordID, now); err != nil {
			slog.Debug("could not record a prompt template match", "template", t.Ref(), "error", err)
		}
	}()
}

func (p *PromptTemplateSource) forget(appID uint) {
	p.mu.Lock()
	delete(p.cache, appID)
	p.mu.Unlock()
}
