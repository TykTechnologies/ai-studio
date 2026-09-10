package services

import (
	"context"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/governed_metadata"
)

// metadataHookRunner adapts HookManager to governed_metadata.HookRunner so the
// enterprise service can run plugin object hooks on metadata records without
// importing the services package.
type metadataHookRunner struct {
	hm *HookManager
}

// newMetadataHookRunner returns nil (no hooks) when no hook manager is available,
// so callers can pass the result straight into governed_metadata.Deps.
func newMetadataHookRunner(hm *HookManager) governed_metadata.HookRunner {
	if hm == nil {
		return nil
	}
	return &metadataHookRunner{hm: hm}
}

func (r *metadataHookRunner) RunMetadataHook(ctx context.Context, hookType string, rec *models.ObjectMetadata, userID uint) (*governed_metadata.HookOutcome, error) {
	result, err := r.hm.ExecuteHooks(ctx, ObjectTypeGovernedMetadata, HookType(hookType), rec, uint32(userID))
	if err != nil {
		return nil, err
	}
	return hookOutcomeFromResult(r.hm, result, rec), nil
}

// hookOutcomeFromResult maps a HookExecutionResult onto the governed metadata
// outcome. Plugin-supplied metadata (map[string]string) is merged into a copy
// of the record's values so the caller's record is never mutated.
func hookOutcomeFromResult(hm *HookManager, result *HookExecutionResult, rec *models.ObjectMetadata) *governed_metadata.HookOutcome {
	if result == nil {
		return &governed_metadata.HookOutcome{Allowed: true}
	}
	out := &governed_metadata.HookOutcome{
		Allowed:         result.Allowed,
		RejectionReason: result.RejectionReason,
		Executed:        result.Executed,
	}
	// ModifiedObject is pre-seeded with the original; only report a change when
	// a plugin actually returned a different record.
	if modified, ok := result.ModifiedObject.(*models.ObjectMetadata); ok && modified != nil && modified != rec {
		out.Modified = modified
	}
	// Plugin metadata (map[string]string) merges into Values, matching the other object types.
	if len(result.Metadata) > 0 {
		target := out.Modified
		if target == nil {
			copyRec := *rec
			copyRec.Values = make(models.JSONMap, len(rec.Values)+len(result.Metadata))
			for k, v := range rec.Values {
				copyRec.Values[k] = v
			}
			target = &copyRec
			out.Modified = target
		}
		_ = hm.MergeMetadata(target, result.Metadata)
	}
	return out
}
