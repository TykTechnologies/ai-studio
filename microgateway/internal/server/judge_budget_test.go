package server

import (
	"testing"

	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
)

type llmBudgetStub struct{}

func (llmBudgetStub) CheckLLMBudget(uint) error { return nil }

type adapterStub struct{ got services.LLMBudgetChecker }

func (a *adapterStub) SetLLMBudgetChecker(c services.LLMBudgetChecker) { a.got = c }

// The edge's budget service bounds guardrail judge calls when it can check
// an LLM's budget; one that enforces no budgets leaves the judge unbounded
// like everything else (midsommar-clean-9u1j.50).
func TestAttachLLMBudgetChecker(t *testing.T) {
	a := &adapterStub{}
	attachLLMBudgetChecker(a, llmBudgetStub{})
	if a.got == nil {
		t.Error("a budget service that checks LLM budgets was not attached")
	}
	b := &adapterStub{}
	attachLLMBudgetChecker(b, struct{}{})
	if b.got != nil {
		t.Error("a budget service without LLM budgets was attached")
	}
}
