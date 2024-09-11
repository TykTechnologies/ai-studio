package api

// UserInput represents the input for user-related operations
// @Description User input model
type UserInput struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			Email    string `json:"email"`
			Name     string `json:"name"`
			Password string `json:"password,omitempty"`
		} `json:"attributes"`
	} `json:"data"`
}

// GroupInput represents the input for group-related operations
// @Description Group input model
type GroupInput struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			Name string `json:"name"`
		} `json:"attributes"`
	} `json:"data"`
}

// UserGroupInput represents the input for adding a user to a group
// @Description User-group relationship input model
type UserGroupInput struct {
	Data struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	} `json:"data"`
}

// UserResponse represents the response for user-related operations
// @Description User response model
type UserResponse struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Attributes struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	} `json:"attributes"`
}

// GroupResponse represents the response for group-related operations
// @Description Group response model
type GroupResponse struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Attributes struct {
		Name string `json:"name"`
	} `json:"attributes"`
}

// ErrorResponse represents the structure of an error response
// @Description Error response model
type ErrorResponse struct {
	Errors []struct {
		Title  string `json:"title"`
		Detail string `json:"detail"`
	} `json:"errors"`
}

// @Description LLM input model
type LLMInput struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			Name              string `json:"name"`
			APIKey            string `json:"api_key"`
			APIEndpoint       string `json:"api_endpoint"`
			StreamingEndpoint string `json:"streaming_endpoint"`
			PrivacyScore      int    `json:"privacy_score"`
			ShortDescription  string `json:"short_description"`
			LongDescription   string `json:"long_description"`
			ExternalURL       string `json:"external_url"`
			LogoURL           string `json:"logo_url"`
			Vendor            string `json:"vendor"`
		} `json:"attributes"`
	} `json:"data"`
}

// LLMResponse represents the response for LLM-related operations
// @Description LLM response model
type LLMResponse struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Attributes struct {
		Name              string `json:"name"`
		APIKey            string `json:"api_key"`
		APIEndpoint       string `json:"api_endpoint"`
		StreamingEndpoint string `json:"streaming_endpoint"`
		PrivacyScore      int    `json:"privacy_score"`
		ShortDescription  string `json:"short_description"`
		LongDescription   string `json:"long_description"`
		ExternalURL       string `json:"external_url"`
		LogoURL           string `json:"logo_url"`
		Vendor            string `json:"vendor"`
	} `json:"attributes"`
}

// CatalogueInput represents the input for catalogue-related operations
// @Description Catalogue input model
type CatalogueInput struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			Name string `json:"name"`
		} `json:"attributes"`
	} `json:"data"`
}

// CatalogueResponse represents the response for catalogue-related operations
// @Description Catalogue response model
type CatalogueResponse struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Attributes struct {
		Name string `json:"name"`
	} `json:"attributes"`
}

// CatalogueLLMInput represents the input for adding an LLM to a catalogue
// @Description Catalogue-LLM relationship input model
type CatalogueLLMInput struct {
	Data struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	} `json:"data"`
}

// GroupCatalogueInput represents the input for adding a catalogue to a group
// @Description Group-Catalogue relationship input model
type GroupCatalogueInput struct {
	Data struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	} `json:"data"`
}

// UserAccessibleCataloguesResponse represents the response for user accessible catalogues
// @Description User accessible catalogues response model
type UserAccessibleCataloguesResponse struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Attributes struct {
		Catalogues []CatalogueResponse `json:"catalogues"`
	} `json:"attributes"`
}

// TagInput represents the input for tag-related operations
// @Description Tag input model
type TagInput struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			Name string `json:"name"`
		} `json:"attributes"`
	} `json:"data"`
}

// TagResponse represents the response for tag-related operations
// @Description Tag response model
type TagResponse struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Attributes struct {
		Name string `json:"name"`
	} `json:"attributes"`
}

// DatasourceInput represents the input for datasource-related operations
// @Description Datasource input model
type DatasourceInput struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			Name             string   `json:"name"`
			ShortDescription string   `json:"short_description"`
			LongDescription  string   `json:"long_description"`
			Icon             string   `json:"icon"`
			Url              string   `json:"url"`
			PrivacyScore     int      `json:"privacy_score"`
			UserID           uint     `json:"user_id"`
			Tags             []string `json:"tags"`
			DBConnString     string   `json:"db_conn_string"`
			DBSourceType     string   `json:"db_source_type"`
			DBConnAPIKey     string   `json:"db_conn_api_key"`
			DBName           string   `json:"db_name"`
			EmbedVendor      string   `json:"embed_vendor"`
			EmbedUrl         string   `json:"embed_url"`
			EmbedAPIKey      string   `json:"embed_api_key"`
			EmbedModel       string   `json:"embed_model"`
		} `json:"attributes"`
	} `json:"data"`
}

// DatasourceResponse represents the response for datasource-related operations
// @Description Datasource response model
type DatasourceResponse struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Attributes struct {
		Name             string        `json:"name"`
		ShortDescription string        `json:"short_description"`
		LongDescription  string        `json:"long_description"`
		Icon             string        `json:"icon"`
		Url              string        `json:"url"`
		PrivacyScore     int           `json:"privacy_score"`
		UserID           uint          `json:"user_id"`
		Tags             []TagResponse `json:"tags"`
		DBConnString     string        `json:"db_conn_string"`
		DBSourceType     string        `json:"db_source_type"`
		DBConnAPIKey     string        `json:"db_conn_api_key"`
		DBName           string        `json:"db_name"`
		EmbedVendor      string        `json:"embed_vendor"`
		EmbedUrl         string        `json:"embed_url"`
		EmbedAPIKey      string        `json:"embed_api_key"`
		EmbedModel       string        `json:"embed_model"`
	} `json:"attributes"`
}

// DataCatalogueInput represents the input for data catalogue-related operations
// @Description Data Catalogue input model
type DataCatalogueInput struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			Name             string `json:"name"`
			ShortDescription string `json:"short_description"`
			LongDescription  string `json:"long_description"`
			Icon             string `json:"icon"`
		} `json:"attributes"`
	} `json:"data"`
}

// DataCatalogueResponse represents the response for data catalogue-related operations
// @Description Data Catalogue response model
type DataCatalogueResponse struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Attributes struct {
		Name             string               `json:"name"`
		ShortDescription string               `json:"short_description"`
		LongDescription  string               `json:"long_description"`
		Icon             string               `json:"icon"`
		Datasources      []DatasourceResponse `json:"datasources"`
		Tags             []TagResponse        `json:"tags"`
	} `json:"attributes"`
}

// DataCatalogueTagInput represents the input for adding a tag to a data catalogue
// @Description Data Catalogue-Tag relationship input model
type DataCatalogueTagInput struct {
	Data struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	} `json:"data"`
}

// DataCatalogueDatasourceInput represents the input for adding a datasource to a data catalogue
// @Description Data Catalogue-Datasource relationship input model
type DataCatalogueDatasourceInput struct {
	Data struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	} `json:"data"`
}

// CredentialInput represents the input for credential-related operations
// @Description Credential input model
type CredentialInput struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			Active bool `json:"active"`
		} `json:"attributes"`
	} `json:"data"`
}

// CredentialResponse represents the response for credential-related operations
// @Description Credential response model
type CredentialResponse struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Attributes struct {
		KeyID  string `json:"key_id"`
		Secret string `json:"secret"`
		Active bool   `json:"active"`
	} `json:"attributes"`
}

// AppInput represents the input for app-related operations
// @Description App input model
type AppInput struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			Name          string `json:"name"`
			Description   string `json:"description"`
			UserID        uint   `json:"user_id"`
			DatasourceIDs []uint `json:"datasource_ids"`
			LLMIDs        []uint `json:"llm_ids"`
		} `json:"attributes"`
	} `json:"data"`
}

// AppResponse represents the response for app-related operations
// @Description App response model
type AppResponse struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Attributes struct {
		Name          string `json:"name"`
		Description   string `json:"description"`
		UserID        uint   `json:"user_id"`
		CredentialID  uint   `json:"credential_id"`
		DatasourceIDs []uint `json:"datasource_ids"`
		LLMIDs        []uint `json:"llm_ids"`
	} `json:"attributes"`
}

// LLMSettingsInput represents the input structure for creating or updating LLM settings
type LLMSettingsInput struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			ModelName         string                 `json:"model_name"`
			MaxLength         int                    `json:"max_length"`
			MaxTokens         int                    `json:"max_tokens"`
			Metadata          map[string]interface{} `json:"metadata"`
			MinLength         int                    `json:"min_length"`
			RepetitionPenalty float64                `json:"repetition_penalty"`
			Seed              int                    `json:"seed"`
			StopWords         []string               `json:"stop_words"`
			Temperature       float64                `json:"temperature"`
			TopK              int                    `json:"top_k"`
			TopP              float64                `json:"top_p"`
		} `json:"attributes"`
	} `json:"data"`
}

// LLMSettingsResponse represents the response structure for LLM settings
type LLMSettingsResponse struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Attributes struct {
		ModelName         string                 `json:"model_name"`
		MaxLength         int                    `json:"max_length"`
		MaxTokens         int                    `json:"max_tokens"`
		Metadata          map[string]interface{} `json:"metadata"`
		MinLength         int                    `json:"min_length"`
		RepetitionPenalty float64                `json:"repetition_penalty"`
		Seed              int                    `json:"seed"`
		StopWords         []string               `json:"stop_words"`
		Temperature       float64                `json:"temperature"`
		TopK              int                    `json:"top_k"`
		TopP              float64                `json:"top_p"`
	} `json:"attributes"`
}

// ChatInput represents the input for chat-related operations
// @Description Chat input model
type ChatInput struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			Name          string `json:"name"`
			LLMSettingsID uint   `json:"llm_settings_id"`
			LLMID         uint   `json:"llm_id"`
			GroupIDs      []uint `json:"group_ids"`
		} `json:"attributes"`
	} `json:"data"`
}

// ChatResponse represents the response for chat-related operations
// @Description Chat response model
type ChatResponse struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Attributes struct {
		Name          string          `json:"name"`
		LLMSettingsID uint            `json:"llm_settings_id"`
		LLMID         uint            `json:"llm_id"`
		Groups        []GroupResponse `json:"groups"`
	} `json:"attributes"`
}

// ToolInput represents the input for tool-related operations
// @Description Tool input model
type ToolInput struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			Name         string  `json:"name"`
			Description  string  `json:"description"`
			ToolType     string  `json:"tool_type"`
			OASSpec      []byte  `json:"oas_spec"`
			PrivacyScore float64 `json:"privacy_score"`
		} `json:"attributes"`
	} `json:"data"`
}

// ToolResponse represents the response for tool-related operations
// @Description Tool response model
type ToolResponse struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Attributes struct {
		Name         string   `json:"name"`
		Description  string   `json:"description"`
		ToolType     string   `json:"tool_type"`
		OASSpec      []byte   `json:"oas_spec"`
		PrivacyScore float64  `json:"privacy_score"`
		Operations   []string `json:"operations"`
	} `json:"attributes"`
}

// OperationInput represents the input for adding or removing operations from a tool
// @Description Operation input model
type OperationInput struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			Operation string `json:"operation"`
		} `json:"attributes"`
	} `json:"data"`
}
