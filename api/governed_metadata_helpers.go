package api

import (
	"errors"
	"net/http"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/governed_metadata"
	"github.com/gin-gonic/gin"
)

// Helpers that connect the built-in LLM / Tool / Datasource handlers to the
// governed metadata service. Every function is a no-op in Community Edition.

// validateGovernedMetadataInput validates the optional governed_metadata attribute
// before the object is written. On create under an enforcing schema an absent
// attribute is treated as an empty map so required fields cannot be bypassed.
// Writes a 422 and returns false when the write must not proceed.
func (a *API) validateGovernedMetadataInput(c *gin.Context, objectType string, values *map[string]interface{}, isCreate bool) bool {
	if !governed_metadata.IsEnterpriseAvailable() {
		return true
	}
	svc := a.governedMetadata()
	if values == nil {
		if !isCreate {
			return true
		}
		resolved, err := svc.ResolveSchema(objectType)
		if err != nil || !resolved.IsEnforced() {
			return true
		}
		empty := map[string]interface{}{}
		values = &empty
	}
	result, err := svc.Validate(objectType, *values)
	if err != nil {
		simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
		return false
	}
	if result.Enforced && !result.Valid {
		writeMetadataValidationResponse(c, result)
		return false
	}
	return true
}

// persistGovernedMetadata stores already-validated values after the object write.
// The object exists at this point, so failures are reported in the response
// `meta` rather than as an error status. Returns nil when there is nothing to report.
func (a *API) persistGovernedMetadata(c *gin.Context, objectType, objectID string, values *map[string]interface{}) gin.H {
	if values == nil || !governed_metadata.IsEnterpriseAvailable() {
		return nil
	}
	_, _, err := a.governedMetadata().SetObjectMetadata(c.Request.Context(), objectType, objectID, *values,
		governed_metadata.SetOptions{UserID: currentUserID(c), Source: models.MetadataSourceAdmin, SkipEnforcement: true})
	if err == nil {
		return nil
	}
	var hrej *governed_metadata.HookRejectedError
	code := "metadata_write_failed"
	if errors.As(err, &hrej) {
		code = "hook_rejected"
	}
	logger.Warnf("governed metadata for %s/%s not saved: %v", objectType, objectID, err)
	return gin.H{"governed_metadata_error": gin.H{"code": code, "detail": err.Error()}}
}

// removeGovernedMetadata drops the metadata row after an object is deleted (best effort).
func (a *API) removeGovernedMetadata(c *gin.Context, objectType, objectID string) {
	if !governed_metadata.IsEnterpriseAvailable() {
		return
	}
	if err := a.governedMetadata().DeleteObjectMetadata(c.Request.Context(), objectType, objectID,
		governed_metadata.SetOptions{UserID: currentUserID(c), Source: models.MetadataSourceAdmin}); err != nil {
		logger.Warnf("governed metadata for %s/%s not removed: %v", objectType, objectID, err)
	}
}

// dataWithMeta builds the standard {data, meta?} envelope.
func dataWithMeta(data interface{}, meta gin.H) gin.H {
	resp := gin.H{"data": data}
	if meta != nil {
		resp["meta"] = meta
	}
	return resp
}

// governedMetadataFor batch-loads records for a set of object IDs (nil map in CE).
func (a *API) governedMetadataFor(objectType string, ids []string) map[string]*models.ObjectMetadata {
	if !governed_metadata.IsEnterpriseAvailable() || len(ids) == 0 {
		return nil
	}
	recs, err := a.governedMetadata().ListObjectMetadata(objectType, ids)
	if err != nil {
		logger.Warnf("governed metadata lookup for %s failed: %v", objectType, err)
		return nil
	}
	return recs
}

// adminView returns (values, status) for admin responses.
func (a *API) adminGovernedView(objectType string, rec *models.ObjectMetadata) (interface{}, string) {
	if rec == nil {
		return nil, ""
	}
	values := a.governedMetadata().VisibleValues(objectType, rec, governed_metadata.VisibilityAdmin)
	if values == nil {
		values = map[string]interface{}{}
	}
	return values, rec.ValidationStatus
}

// portalGovernedView returns the display-ready list for portal responses (nil when empty).
func (a *API) portalGovernedView(objectType string, rec *models.ObjectMetadata) interface{} {
	if rec == nil {
		return nil
	}
	display := a.governedMetadata().DisplayValues(objectType, rec)
	if len(display) == 0 {
		return nil
	}
	return display
}

// --- LLM ---

func (a *API) withLLMGovernedMetadata(items []LLMResponse, portal bool) []LLMResponse {
	ids := make([]string, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.ID)
	}
	recs := a.governedMetadataFor(models.GovernedObjectTypeLLM, ids)
	if recs == nil {
		return items
	}
	for i := range items {
		rec := recs[items[i].ID]
		if portal {
			items[i].GovernedMetadata = a.portalGovernedView(models.GovernedObjectTypeLLM, rec)
		} else {
			items[i].GovernedMetadata, items[i].GovernedMetadataStatus = a.adminGovernedView(models.GovernedObjectTypeLLM, rec)
		}
	}
	return items
}

func (a *API) withLLMGovernedMetadataOne(item LLMResponse) LLMResponse {
	return a.withLLMGovernedMetadata([]LLMResponse{item}, false)[0]
}

// --- Tool ---

func (a *API) withToolGovernedMetadata(items []ToolResponse, portal bool) []ToolResponse {
	ids := make([]string, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.ID)
	}
	recs := a.governedMetadataFor(models.GovernedObjectTypeTool, ids)
	if recs == nil {
		return items
	}
	for i := range items {
		rec := recs[items[i].ID]
		if portal {
			items[i].GovernedMetadata = a.portalGovernedView(models.GovernedObjectTypeTool, rec)
		} else {
			items[i].GovernedMetadata, items[i].GovernedMetadataStatus = a.adminGovernedView(models.GovernedObjectTypeTool, rec)
		}
	}
	return items
}

func (a *API) withToolGovernedMetadataOne(item ToolResponse) ToolResponse {
	return a.withToolGovernedMetadata([]ToolResponse{item}, false)[0]
}

// --- Datasource ---

func (a *API) withDatasourceGovernedMetadata(items []DatasourceResponse, portal bool) []DatasourceResponse {
	ids := make([]string, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.ID)
	}
	recs := a.governedMetadataFor(models.GovernedObjectTypeDatasource, ids)
	if recs == nil {
		return items
	}
	for i := range items {
		rec := recs[items[i].ID]
		if portal {
			items[i].GovernedMetadata = a.portalGovernedView(models.GovernedObjectTypeDatasource, rec)
		} else {
			items[i].GovernedMetadata, items[i].GovernedMetadataStatus = a.adminGovernedView(models.GovernedObjectTypeDatasource, rec)
		}
	}
	return items
}

func (a *API) withDatasourceGovernedMetadataOne(item DatasourceResponse) DatasourceResponse {
	return a.withDatasourceGovernedMetadata([]DatasourceResponse{item}, false)[0]
}
