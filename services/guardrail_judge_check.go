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
// or in the one namespace those edges are in. filterID is 0 for a new filter
// (attached to no LLM yet). A filter attached to an LLM later is not checked
// again here; an edge that cannot find the judge keeps the filter's own
// verdict.
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
	judgeNS := strings.TrimSpace(judge.Namespace)
	if judgeNS == "" || filterID == 0 {
		return nil
	}

	var attached []models.LLM
	if err := s.DB.Model(&models.LLM{}).
		Joins("JOIN llm_filters ON llm_filters.llm_id = llms.id").
		Where("llm_filters.filter_id = ?", filterID).
		Find(&attached).Error; err != nil {
		return err
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
