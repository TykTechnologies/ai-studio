package api

import (
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/secrets"
)

// The admin responses of Workspace objects report the Workspace the object
// means: a stored 0 (rows an older build wrote, .41) is the Default
// (responseWorkspaceID). The base serializers stay edition- and
// database-free (portal and nested payloads use them as they are); these
// wrappers are what the object's own admin routes answer (.52).

func (a *API) filterResponseIn(f *models.Filter) FilterResponse {
	resp := toFilterResponse(f)
	resp.Attributes.WorkspaceID = a.responseWorkspaceID(f.WorkspaceID)
	return resp
}

func (a *API) filterResponsesIn(filters []models.Filter) []FilterResponse {
	out := make([]FilterResponse, len(filters))
	for i := range filters {
		out[i] = a.filterResponseIn(&filters[i])
	}
	return out
}

func (a *API) embedderIn(e *models.Embedder) map[string]interface{} {
	attrs := serializeEmbedder(e)
	if inner, ok := attrs["attributes"].(map[string]interface{}); ok {
		inner["workspace_id"] = a.responseWorkspaceID(e.WorkspaceID)
	} else {
		attrs["workspace_id"] = a.responseWorkspaceID(e.WorkspaceID)
	}
	return attrs
}

func (a *API) secretIn(resp SecretResponse, stored uint) SecretResponse {
	resp.Attributes.WorkspaceID = a.responseWorkspaceID(stored)
	return resp
}

func (a *API) secretsIn(resps []SecretResponse, list []secrets.Secret) []SecretResponse {
	for i := range resps {
		resps[i].Attributes.WorkspaceID = a.responseWorkspaceID(list[i].WorkspaceID)
	}
	return resps
}

func (a *API) datasourceIn(ds *models.Datasource) DatasourceResponse {
	resp := serializeDatasource(ds)
	resp.Attributes.WorkspaceID = a.responseWorkspaceID(ds.WorkspaceID)
	return resp
}

func (a *API) datasourcesIn(list models.Datasources) []DatasourceResponse {
	out := make([]DatasourceResponse, len(list))
	for i := range list {
		out[i] = a.datasourceIn(&list[i])
	}
	return out
}
