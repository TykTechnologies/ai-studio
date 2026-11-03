package services

import (
	"errors"
	"fmt"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/switches"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// checkJudge refuses a guardrail filter whose second opinion (judge) could
// not be asked where the filter runs: the judge LLM must exist, be active,
// serve chat, and reach every edge that runs the filter. An edge runs a
// filter through the LLMs it is attached to, so the judge LLM must be global
// or in the one namespace those edges are in; and its privacy score must be
// at least that of every LLM the filter is attached to. filterID is 0 for a
// new filter (attached to no LLM yet). A filter attached to an LLM later is
// not checked again here: at run time an edge that cannot find the judge,
// or a judge scored below the request's LLM, keeps the filter's own verdict.
func (s *Service) checkJudge(filterID uint, filterNamespace string, config models.JSONMap) error {
	if config == nil {
		return nil
	}
	cfg, err := guardrails.ParseConfig(config)
	if err != nil || !cfg.Judge.Enabled() {
		return nil
	}
	judge := &models.LLM{}
	if err := s.DB.First(judge, cfg.Judge.LLMID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w: judge LLM %d does not exist", ErrInvalidFilterSpec, cfg.Judge.LLMID)
		}
		return err
	}
	if !judge.Active {
		return fmt.Errorf("%w: judge LLM %q is not active", ErrInvalidFilterSpec, judge.Name)
	}
	if _, known := switches.VendorMap[judge.Vendor]; !known || !switches.SupportsChat(judge.Vendor) {
		return fmt.Errorf("%w: judge LLM %q does not serve chat completions", ErrInvalidFilterSpec, judge.Name)
	}
	if filterID == 0 {
		return nil
	}

	var attached []models.LLM
	if err := s.DB.Model(&models.LLM{}).
		Joins("JOIN llm_filters ON llm_filters.llm_id = llms.id").
		Where("llm_filters.filter_id = ?", filterID).
		Find(&attached).Error; err != nil {
		return err
	}
	// The judge reads the traffic of every LLM the filter guards, so it must
	// be trusted with it: a privacy score at least theirs (the platform's
	// rule that data never flows to a provider scored below its source).
	for _, llm := range attached {
		if judge.PrivacyScore < llm.PrivacyScore {
			return fmt.Errorf("%w: judge LLM %q has privacy score %d, below the %d of LLM %q whose traffic it would read; choose a judge LLM with a privacy score of at least %d", ErrInvalidFilterSpec, judge.Name, judge.PrivacyScore, llm.PrivacyScore, llm.Name, llm.PrivacyScore)
		}
	}
	judgeNS := strings.TrimSpace(judge.Namespace)
	if judgeNS == "" {
		return nil
	}
	filterNS := strings.TrimSpace(filterNamespace)
	for _, llm := range attached {
		edges, runs := edgeNamespacesFor(strings.TrimSpace(llm.Namespace), filterNS)
		if !runs {
			continue
		}
		if edges == "" {
			return fmt.Errorf("%w: judge LLM %q is in namespace %q, but this filter runs on every edge through LLM %q; choose a global judge LLM", ErrInvalidFilterSpec, judge.Name, judgeNS, llm.Name)
		}
		if edges != judgeNS {
			return fmt.Errorf("%w: judge LLM %q is in namespace %q, but this filter runs on edges in namespace %q through LLM %q; choose a judge LLM those edges receive", ErrInvalidFilterSpec, judge.Name, judgeNS, edges, llm.Name)
		}
	}
	return nil
}

// edgeNamespacesFor is where a filter in filterNS attached to an LLM in
// llmNS runs: on every edge (""), on the edges of one namespace, or on none
// (runs false: the edges that get the LLM do not get the filter). An edge gets
// global objects and those of its own namespace (grpc getConfigurationSnapshot).
func edgeNamespacesFor(llmNS, filterNS string) (edges string, runs bool) {
	switch {
	case llmNS == "" && filterNS == "":
		return "", true
	case llmNS == "":
		return filterNS, true
	case filterNS == "" || filterNS == llmNS:
		return llmNS, true
	}
	return "", false
}

// CheckLLMBudget reports whether llm's own monthly budget allows another
// call, for calls the gateway makes on its own account rather than an App's
// (a guardrail's judge): the LLM half of the proxy's budget check. No budget
// service (Community Edition) enforces nothing.
func (s *Service) CheckLLMBudget(llm *models.LLM) error {
	if s.Budget == nil || llm == nil {
		return nil
	}
	_, _, err := s.Budget.CheckBudget(nil, llm)
	return err
}
