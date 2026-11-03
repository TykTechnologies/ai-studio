package services

import (
	"errors"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// budgetStub is a budget service whose check returns err for every LLM.
type budgetStub struct {
	BudgetService
	err     error
	gotApp  *models.App
	checked bool
}

func (b *budgetStub) CheckBudget(app *models.App, llm *models.LLM) (float64, float64, error) {
	b.gotApp, b.checked = app, true
	return 0, 0, b.err
}

// The hub's judge budget check is the LLM half of the proxy's: no App, the
// budget service's verdict (midsommar-clean-9u1j.50).
func TestService_CheckLLMBudget(t *testing.T) {
	llm := &models.LLM{ID: 4}
	if err := (&Service{}).CheckLLMBudget(llm); err != nil {
		t.Errorf("no budget service: %v", err)
	}
	stub := &budgetStub{err: errors.New("LLM monthly budget exceeded")}
	s := &Service{Budget: stub}
	if err := s.CheckLLMBudget(llm); err == nil || !stub.checked || stub.gotApp != nil {
		t.Errorf("err %v checked %v app %v", err, stub.checked, stub.gotApp)
	}
	stub.err = nil
	if err := s.CheckLLMBudget(llm); err != nil {
		t.Errorf("under budget: %v", err)
	}
}
