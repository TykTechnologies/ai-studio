package api

import "time"

// UserInput represents the input for user-related operations
// @Description User input model
type UserInput struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			Email                string `json:"email"`
			Name                 string `json:"name"`
			Password             string `json:"password,omitempty"`
			IsAdmin              bool   `json:"is_admin"`
			ShowChat             bool   `json:"show_chat"`
			ShowPortal           bool   `json:"show_portal"`
			EmailVerified        bool   `json:"email_verified"`
			NotificationsEnabled bool   `json:"notifications_enabled"`
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

// GroupDataCatalogueInput represents the input for adding a data catalogue to a group
// @Description Group-DataCatalogue relationship input model
type GroupDataCatalogueInput struct {
	Data struct {
		Type string `json:"type"`
		ID   string `json:"id"`
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
		Email                string `json:"email"`
		Name                 string `json:"name"`
		IsAdmin              bool   `json:"is_admin"`
		ShowChat             bool   `json:"show_chat"`
		ShowPortal           bool   `json:"show_portal"`
		EmailVerified        bool   `json:"email_verified"`
		APIKey               string `json:"api_key"`
		NotificationsEnabled bool   `json:"notifications_enabled"`
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

// ErrorResponse represents an error response
// @Description Error response model
type ErrorResponse struct {
	Errors []struct {
		Title  string `json:"title"`
		Detail string `json:"detail"`
	} `json:"errors"`
}

// LLMInput represents the input for LLM-related operations
// @Description LLM input model
type LLMInput struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			Name             string   `json:"name"`
			APIKey           string   `json:"api_key"`
			APIEndpoint      string   `json:"api_endpoint"`
			PrivacyScore     int      `json:"privacy_score"`
			ShortDescription string   `json:"short_description"`
			LongDescription  string   `json:"long_description"`
			LogoURL          string   `json:"logo_url"`
			Vendor           string   `json:"vendor"`
			Active           bool     `json:"active"`
			Filters          []uint   `json:"filters"`
			DefaultModel     string   `json:"default_model"`
			AllowedModels    []string `json:"allowed_models"`
			MonthlyBudget    *float64 `json:"monthly_budget"`
			BudgetStartDate  *string  `json:"budget_start_date"`
		} `json:"attributes"`
	} `json:"data"`
}

// LLMResponse represents the response for LLM-related operations
// @Description LLM response model
type LLMResponse struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Attributes struct {
		Name             string           `json:"name"`
		APIKey           string           `json:"api_key"`
		APIEndpoint      string           `json:"api_endpoint"`
		PrivacyScore     int              `json:"privacy_score"`
		ShortDescription string           `json:"short_description"`
		LongDescription  string           `json:"long_description"`
		LogoURL          string           `json:"logo_url"`
		Vendor           string           `json:"vendor"`
		Active           bool             `json:"active"`
		Filters          []FilterResponse `json:"filters"`
		DefaultModel     string           `json:"default_model"`
		AllowedModels    []string         `json:"allowed_models"`
		MonthlyBudget    *float64         `json:"monthly_budget"`
		BudgetStartDate  *time.Time       `json:"budget_start_date"`
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
		Name     string   `json:"name"`
		LLMNames []string `json:"llm_names"`
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

// UserAccessibleCataloguesResponse represents the response for user-accessible catalogues
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
			Active           bool     `json:"active"`
		} `json:"attributes"`
	} `json:"data"`
}

// DatasourceResponse represents the response for datasource-related operations
// @Description Datasource response model
type DatasourceResponse struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Attributes struct {
		Name             string              `json:"name"`
		ShortDescription string              `json:"short_description"`
		LongDescription  string              `json:"long_description"`
		Icon             string              `json:"icon"`
		Url              string              `json:"url"`
		PrivacyScore     int                 `json:"privacy_score"`
		UserID           uint                `json:"user_id"`
		Tags             []TagResponse       `json:"tags"`
		DBConnString     string              `json:"db_conn_string"`
		DBSourceType     string              `json:"db_source_type"`
		DBConnAPIKey     string              `json:"db_conn_api_key"`
		DBName           string              `json:"db_name"`
		EmbedVendor      string              `json:"embed_vendor"`
		EmbedUrl         string              `json:"embed_url"`
		EmbedAPIKey      string              `json:"embed_api_key"`
		EmbedModel       string              `json:"embed_model"`
		Active           bool                `json:"active"`
		Files            []FileStoreResponse `json:"files"`
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
			KeyID  string `json:"key_id"`
			Secret string `json:"secret"`
			Active bool   `json:"active"`
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
			Name            string     `json:"name"`
			Description     string     `json:"description"`
			UserID          uint       `json:"user_id"`
			DatasourceIDs   []uint     `json:"datasource_ids"`
			LLMIDs          []uint     `json:"llm_ids"`
			MonthlyBudget   *float64   `json:"monthly_budget"`
			BudgetStartDate *time.Time `json:"budget_start_date"`
		} `json:"attributes"`
	} `json:"data"`
}

// AppResponse represents the response for app-related operations
// @Description App response model
type AppResponse struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Attributes struct {
		Name            string     `json:"name"`
		Description     string     `json:"description"`
		UserID          uint       `json:"user_id"`
		CredentialID    uint       `json:"credential_id"`
		DatasourceIDs   []uint     `json:"datasource_ids"`
		LLMIDs          []uint     `json:"llm_ids"`
		MonthlyBudget   *float64   `json:"monthly_budget"`
		BudgetStartDate *time.Time `json:"budget_start_date"`
	} `json:"attributes"`
}

// LLMSettingsInput represents the input structure for creating or updating LLM settings
// @Description LLM settings input model
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
			SystemPrompt      string                 `json:"system_prompt"`
		} `json:"attributes"`
	} `json:"data"`
}

// LLMSettingsResponse represents the response structure for LLM settings
// @Description LLM settings response model
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
		SystemPrompt      string                 `json:"system_prompt"`
	} `json:"attributes"`
}

// ChatInput represents the input for chat-related operations
// @Description Chat input model
type ChatInput struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			Name                string `json:"name"`
			Description         string `json:"description"`
			LLMSettingsID       uint   `json:"llm_settings_id"`
			LLMID               uint   `json:"llm_id"`
			GroupIDs            []uint `json:"group_ids"`
			FilterIDs           []uint `json:"filter_ids"`
			RagN                int    `json:"rag_n"`
			ToolSupport         bool   `json:"tool_support"`
			SystemPrompt        string `json:"system_prompt"`
			DefaultDataSourceID int    `json:"default_data_source_id"`
			DefaultToolIDs      []uint `json:"default_tool_ids"`
		} `json:"attributes"`
	} `json:"data"`
}

// ChatResponse represents the response for chat-related operations
// @Description Chat response model
type ChatResponse struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Attributes struct {
		Name                string                   `json:"name"`
		Description         string                   `json:"description"`
		LLMSettingsID       string                   `json:"llm_settings_id"`
		LLMID               string                   `json:"llm_id"`
		Groups              []GroupResponse          `json:"groups"`
		Filters             []FilterResponse         `json:"filters"`
		RagN                int                      `json:"rag_n"`
		ToolSupport         bool                     `json:"tool_support"`
		SystemPrompt        string                   `json:"system_prompt"`
		DefaultDataSourceID int                      `json:"default_data_source_id"`
		DefaultDataSource   DatasourceResponse       `json:"default_data_source"`
		ExtraContext        []FileStoreResponse      `json:"extra_context"`
		DefaultTools        []ToolResponse           `json:"default_tools"`
		PromptTemplates     []PromptTemplateResponse `json:"prompt_templates"`
	} `json:"attributes"`
}

// ToolInput represents the input for tool-related operations
// @Description Tool input model
type ToolInput struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			Name           string   `json:"name"`
			Description    string   `json:"description"`
			ToolType       string   `json:"tool_type"`
			OASSpec        string   `json:"oas_spec"`
			PrivacyScore   int      `json:"privacy_score"`
			AuthKey        string   `json:"auth_key"`
			AuthSchemaName string   `json:"auth_schema_name"`
			Operations     []string `json:"operations"`
		} `json:"attributes"`
	} `json:"data"`
}

// ToolResponse represents the response for tool-related operations
// @Description Tool response model
type ToolResponse struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Attributes struct {
		Name           string              `json:"name"`
		Description    string              `json:"description"`
		ToolType       string              `json:"tool_type"`
		OASSpec        string              `json:"oas_spec"`
		PrivacyScore   int                 `json:"privacy_score"`
		Operations     []string            `json:"operations"`
		AuthKey        string              `json:"auth_key"`
		AuthSchemaName string              `json:"auth_schema_name"`
		FileStores     []FileStoreResponse `json:"file_stores"`
		Filters        []FilterResponse    `json:"filters"`
		Dependencies   []ToolResponse      `json:"dependencies"`
	} `json:"attributes"`
}

// OperationsResponse represents the response for tool operations
// @Description Tool operations response model
type OperationsResponse struct {
	Operations []string `json:"operations"`
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

// ModelPriceInput represents the input for model price-related operations
// @Description Model Price input model
type ModelPriceInput struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			ModelName    string  `json:"model_name"`
			Vendor       string  `json:"vendor"`
			CPT          float64 `json:"cpt"`
			CPIT         float64 `json:"cpit"`
			CacheWritePT float64 `json:"cache_write_pt"`
			CacheReadPT  float64 `json:"cache_read_pt"`
			Currency     string  `json:"currency"`
		} `json:"attributes"`
	} `json:"data"`
}

// ModelPriceResponse represents the response for model price-related operations
// @Description Model Price response model
type ModelPriceResponse struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Attributes struct {
		ModelName    string  `json:"model_name"`
		Vendor       string  `json:"vendor"`
		CPT          float64 `json:"cpt"`
		CPIT         float64 `json:"cpit"`
		CacheWritePT float64 `json:"cache_write_pt"`
		CacheReadPT  float64 `json:"cache_read_pt"`
		Currency     string  `json:"currency"`
	} `json:"attributes"`
}

// VendorListResponse represents the response for vendor list operations
// @Description Vendor list response model
type VendorListResponse struct {
	Data []string `json:"data"`
}

// FilterInput represents the input for filter-related operations
// @Description Filter input model
type FilterInput struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Script      []byte `json:"script"`
		} `json:"attributes"`
	} `json:"data"`
}

// FilterResponse represents the response for filter-related operations
// @Description Filter response model
type FilterResponse struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Attributes struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Script      []byte `json:"script"`
	} `json:"attributes"`
}

// ChatHistoryRecordInput represents the input for chat history record-related operations
// @Description Chat History Record input model
type ChatHistoryRecordInput struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			SessionID string `json:"session_id"`
			ChatID    uint   `json:"chat_id"`
			UserID    uint   `json:"user_id"`
			Name      string `json:"name"`
		} `json:"attributes"`
	} `json:"data"`
}

// ChatHistoryRecordResponse represents the response for chat history record-related operations
// @Description Chat History Record response model
type ChatHistoryRecordResponse struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Attributes struct {
		SessionID string `json:"session_id"`
		ChatID    uint   `json:"chat_id"`
		UserID    uint   `json:"user_id"`
		Name      string `json:"name"`
	} `json:"attributes"`
}

// ChatHistoryRecordListResponse represents the response for listing chat history records
// @Description Chat History Record list response model
type ChatHistoryRecordListResponse struct {
	Data []ChatHistoryRecordResponse `json:"data"`
}

// ToolCatalogueInput represents the input for tool catalogue-related operations
// @Description Tool Catalogue input model
type ToolCatalogueInput struct {
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

// ToolCatalogueResponse represents the response for tool catalogue-related operations
// @Description Tool Catalogue response model
type ToolCatalogueResponse struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Attributes struct {
		Name             string         `json:"name"`
		ShortDescription string         `json:"short_description"`
		LongDescription  string         `json:"long_description"`
		Icon             string         `json:"icon"`
		Tools            []ToolResponse `json:"tools"`
		Tags             []TagResponse  `json:"tags"`
	} `json:"attributes"`
}

// ToolCatalogueToolInput represents the input for adding a tool to a tool catalogue
// @Description Tool Catalogue-Tool relationship input model
type ToolCatalogueToolInput struct {
	Data struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	} `json:"data"`
}

// ToolCatalogueTagInput represents the input for adding a tag to a tool catalogue
// @Description Tool Catalogue-Tag relationship input model
type ToolCatalogueTagInput struct {
	Data struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	} `json:"data"`
}

// GroupToolCatalogueInput represents the input for adding a tool catalogue to a group
// @Description Group-ToolCatalogue relationship input model
type GroupToolCatalogueInput struct {
	Data struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	} `json:"data"`
}

// LoginInput represents the input for login operations
// @Description Login input model
type LoginInput struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		} `json:"attributes"`
	} `json:"data"`
}

// RegisterInput represents the input for registration operations
// @Description Register input model
type RegisterInput struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			Email      string `json:"email"`
			Name       string `json:"name"`
			Password   string `json:"password"`
			WithPortal bool   `json:"with_portal"`
			WithChat   bool   `json:"with_chat"`
		} `json:"attributes"`
	} `json:"data"`
}

// ForgotPasswordInput represents the input for forgot password operations
// @Description Forgot Password input model
type ForgotPasswordInput struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			Email string `json:"email"`
		} `json:"attributes"`
	} `json:"data"`
}

// ResetPasswordInput represents the input for reset password operations
// @Description Reset Password input model
type ResetPasswordInput struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			Token    string `json:"token"`
			Password string `json:"password"`
		} `json:"attributes"`
	} `json:"data"`
}

// VerifyEmailInput represents the input for email verification operations
// @Description Verify Email input model
type VerifyEmailInput struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			Token string `json:"token"`
		} `json:"attributes"`
	} `json:"data"`
}

// ResendVerificationInput represents the input for resending verification email operations
// @Description Resend Verification input model
type ResendVerificationInput struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			Email string `json:"email"`
		} `json:"attributes"`
	} `json:"data"`
}

// AuthResponse represents the response for successful authentication operations
// @Description Auth response model
type AuthResponse struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Attributes struct {
		Token string `json:"token"`
		User  struct {
			ID    string `json:"id"`
			Email string `json:"email"`
			Name  string `json:"name"`
		} `json:"user"`
	} `json:"attributes"`
}

// LoginResponse represents the response for successful login
// @Description Login response model
type LoginResponse struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			Message string `json:"message"`
		} `json:"attributes"`
	} `json:"data"`
}

// RegisterResponse represents the response for successful registration
// @Description Register response model
type RegisterResponse struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			Message string `json:"message"`
		} `json:"attributes"`
	} `json:"data"`
}

// LogoutResponse represents the response for successful logout
// @Description Logout response model
type LogoutResponse struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			Message string `json:"message"`
		} `json:"attributes"`
	} `json:"data"`
}

// ForgotPasswordResponse represents the response for a successful forgot password request
// @Description ForgotPassword response model
type ForgotPasswordResponse struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			Message string `json:"message"`
		} `json:"attributes"`
	} `json:"data"`
}

// ResetPasswordResponse represents the response for a successful password reset
// @Description ResetPassword response model
type ResetPasswordResponse struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			Message string `json:"message"`
		} `json:"attributes"`
	} `json:"data"`
}

// VerifyEmailResponse represents the response for successful email verification
// @Description VerifyEmail response model
type VerifyEmailResponse struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			Message string `json:"message"`
		} `json:"attributes"`
	} `json:"data"`
}

// ResendVerificationResponse represents the response for resending verification email
// @Description ResendVerification response model
type ResendVerificationResponse struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			Message string `json:"message"`
		} `json:"attributes"`
	} `json:"data"`
}

// UserWithEntitlementsResponse represents the response for the current user with entitlements
// @Description User with entitlements response model
type UserWithEntitlementsResponse struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Attributes struct {
		Email     string `json:"email"`
		Name      string `json:"name"`
		IsAdmin   bool   `json:"is_admin"`
		UIOptions struct {
			ShowChat      bool `json:"show_chat"`
			ShowPortal    bool `json:"show_portal"`
			ShowSSOConfig bool `json:"show_sso_config"`
		} `json:"ui_options"`
		Entitlements struct {
			Catalogues     []CatalogueResponse     `json:"catalogues"`
			DataCatalogues []DataCatalogueResponse `json:"data_catalogues"`
			ToolCatalogues []ToolCatalogueResponse `json:"tool_catalogues"`
			Chats          []ChatResponse          `json:"chats"`
		} `json:"entitlements"`
	} `json:"attributes"`
}

// AppListResponse represents the response for listing apps
// @Description App list response model
type AppListResponse struct {
	Data []AppResponse `json:"data"`
	Meta struct {
		TotalCount int64 `json:"total_count"`
		TotalPages int   `json:"total_pages"`
		PageSize   int   `json:"page_size"`
		PageNumber int   `json:"page_number"`
	} `json:"meta"`
}

// AppDetailResponse represents the detailed response for app-related operations
// @Description App detail response model
type AppDetailResponse struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Attributes struct {
		Name            string           `json:"name"`
		Description     string           `json:"description"`
		UserID          uint             `json:"user_id"`
		CredentialID    uint             `json:"credential_id"`
		DatasourceIDs   []uint           `json:"datasource_ids"`
		LLMIDs          []uint           `json:"llm_ids"`
		MonthlyBudget   *float64         `json:"monthly_budget"`
		BudgetStartDate *time.Time       `json:"budget_start_date"`
		Credential      CredentialDetail `json:"credential"`
	} `json:"attributes"`
}

// CredentialDetail represents the credential details
// @Description Credential detail model
type CredentialDetail struct {
	KeyID  string `json:"key_id"`
	Secret string `json:"secret"`
	Active bool   `json:"active"`
}

// CMessageResponse represents the response structure for a CMessage
// @Description CMessage response model
type CMessageResponse struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Attributes struct {
		Session   string    `json:"session"`
		Content   any       `json:"content"`
		CreatedAt time.Time `json:"created_at"`
		ChatID    uint      `json:"chat_id"`
	} `json:"attributes"`
}

// ProxyLogResponse represents the response structure for a proxy log entry
// @Description Proxy log response model
type ProxyLogResponse struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Attributes struct {
		AppID        uint      `json:"app_id"`
		UserID       uint      `json:"user_id"`
		TimeStamp    time.Time `json:"time_stamp"`
		Vendor       string    `json:"vendor"`
		RequestBody  string    `json:"request_body"`
		ResponseBody string    `json:"response_body"`
		ResponseCode int       `json:"response_code"`
	} `json:"attributes"`
}

// PaginatedProxyLogs represents the paginated response for proxy logs
// @Description Proxy log list response model
type PaginatedProxyLogs struct {
	Data []ProxyLogResponse `json:"data"`
	Meta struct {
		TotalCount int64 `json:"total_count"`
		TotalPages int   `json:"total_pages"`
		PageSize   int   `json:"page_size"`
		PageNumber int   `json:"page_number"`
	} `json:"meta"`
}

// FrontendConfig holds front-end configuration settings
// @Description Front-end config model
type FrontendConfig struct {
	APIBaseURL        string            `json:"apiBaseURL"`
	ProxyURL          string            `json:"proxyURL"`
	DefaultSignUpMode string            `json:"defaultSignUpMode"`
	TIBEnabled        bool              `json:"tibEnabled"`
	DocsLinks         map[string]string `json:"docsLinks"`
}

// FileStoreInput represents the input for filestore-related operations
// @Description FileStore input model
type FileStoreInput struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			FileName    string `json:"file_name"`
			Description string `json:"description"`
			Content     string `json:"content"`
			Length      int    `json:"length"`
		} `json:"attributes"`
	} `json:"data"`
}

// FileStoreResponse represents the response for filestore-related operations
// @Description FileStore response model
type FileStoreResponse struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Attributes struct {
		FileName        string    `json:"file_name"`
		Description     string    `json:"description"`
		Content         string    `json:"-"`
		Length          int       `json:"length"`
		LastProcessedOn time.Time `json:"last_processed_on"`
	} `json:"attributes"`
}

// SecretInput represents the input for secret-related operations
// @Description Secret input model
type SecretInput struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			Value   string `json:"value"`
			VarName string `json:"var_name"`
		} `json:"attributes"`
	} `json:"data"`
}

// SecretResponse represents the response for secret-related operations
// @Description Secret response model
type SecretResponse struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Attributes struct {
		Value   string `json:"value"`
		VarName string `json:"var_name"`
	} `json:"attributes"`
}

// SecretListResponse represents the paginated response for listing secrets
// @Description Secret list response model
type SecretListResponse struct {
	Data []SecretResponse `json:"data"`
	Meta struct {
		TotalCount int64 `json:"total_count"`
		TotalPages int   `json:"total_pages"`
		PageSize   int   `json:"page_size"`
		PageNumber int   `json:"page_number"`
	} `json:"meta"`
}

// ProfileInput represents the input for creating/updating an SSO profile
// @Description Profile input model
type ProfileInput struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			ProfileID                 string                 `json:"profile_id"`
			Name                      string                 `json:"name"`
			OrgID                     string                 `json:"org_id"`
			ActionType                string                 `json:"action_type"`
			MatchedPolicyID           string                 `json:"matched_policy_id"`
			Type                      string                 `json:"type"`
			ProviderName              string                 `json:"provider_name"`
			CustomEmailField          string                 `json:"custom_email_field"`
			CustomUserIDField         string                 `json:"custom_user_id_field"`
			ProviderConfig            map[string]interface{} `json:"provider_config"`
			IdentityHandlerConfig     map[string]interface{} `json:"identity_handler_config"`
			ProviderConstraintsDomain string                 `json:"provider_constraints_domain"`
			ProviderConstraintsGroup  string                 `json:"provider_constraints_group"`
			ReturnURL                 string                 `json:"return_url"`
			DefaultUserGroupID        string                 `json:"default_user_group_id"`
			CustomUserGroupField      string                 `json:"custom_user_group_field"`
			UserGroupMapping          map[string]string      `json:"user_group_mapping"`
			UserGroupSeparator        string                 `json:"user_group_separator"`
			SSOOnlyForRegisteredUsers bool                   `json:"sso_only_for_registered_users"`
		} `json:"attributes"`
	} `json:"data"`
}

// ProfileResponse represents the response for an SSO profile
// @Description Profile response model
type ProfileResponse struct {
	Type       string `json:"type"`
	ID         uint   `json:"id"`
	Attributes struct {
		ProfileID                 string                 `json:"profile_id"`
		Name                      string                 `json:"name"`
		OrgID                     string                 `json:"org_id"`
		ActionType                string                 `json:"action_type"`
		MatchedPolicyID           string                 `json:"matched_policy_id"`
		Type                      string                 `json:"type"`
		ProviderName              string                 `json:"provider_name"`
		CustomEmailField          string                 `json:"custom_email_field"`
		CustomUserIDField         string                 `json:"custom_user_id_field"`
		ProviderConfig            map[string]interface{} `json:"provider_config"`
		IdentityHandlerConfig     map[string]interface{} `json:"identity_handler_config"`
		ProviderConstraintsDomain string                 `json:"provider_constraints_domain"`
		ProviderConstraintsGroup  string                 `json:"provider_constraints_group"`
		ReturnURL                 string                 `json:"return_url"`
		DefaultUserGroupID        string                 `json:"default_user_group_id"`
		CustomUserGroupField      string                 `json:"custom_user_group_field"`
		UserGroupMapping          map[string]string      `json:"user_group_mapping"`
		UserGroupSeparator        string                 `json:"user_group_separator"`
		SSOOnlyForRegisteredUsers bool                   `json:"sso_only_for_registered_users"`
		SelectedProviderType      string                 `json:"selected_provider_type"`
		LoginURL                  string                 `json:"login_url"`
		CallbackURL               string                 `json:"callback_url"`
		FailureRedirectURL        string                 `json:"failure_redirect_url"`
		UseInLoginPage            bool                   `json:"use_in_login_page"`
	} `json:"attributes"`
}

// ProfileListItem represents a simplified profile item for list responses
// @Description Profile list item model
type ProfileListItem struct {
	Type       string `json:"type"`
	ID         uint   `json:"id"`
	Attributes struct {
		Name         string    `json:"name"`
		ProfileID    string    `json:"profile_id"`
		ProfileType  string    `json:"profile_type"`
		ProviderType string    `json:"provider_type"`
		UpdatedBy    string    `json:"updated_by"`
		UpdatedAt    time.Time `json:"updated_at"`
	} `json:"attributes"`
}

// ProfilesResponse represents the paginated response for listing profiles
// @Description Profiles list response model
type ProfilesResponse struct {
	Data []ProfileListItem `json:"data"`
	Meta struct {
		TotalCount int64 `json:"total_count"`
		TotalPages int   `json:"total_pages"`
		PageSize   int   `json:"page_size"`
		PageNumber int   `json:"page_number"`
	} `json:"meta"`
}

// DependencyInput represents the input for adding or removing a dependency
// @Description Dependency input model
type DependencyInput struct {
	Data struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	} `json:"data"`
}

// DependencyListResponse represents the response for listing dependencies
// @Description Dependency list response model
type DependencyListResponse struct {
	Data []ToolResponse `json:"data"`
}

// PromptTemplateInput represents the input for prompt template-related operations
// @Description Prompt Template input model
type PromptTemplateInput struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			Name   string `json:"name"`
			Prompt string `json:"prompt"`
			ChatID uint   `json:"chat_id"`
		} `json:"attributes"`
	} `json:"data"`
}

// PromptTemplateResponse represents the response for prompt template-related operations
// @Description Prompt Template response model
type PromptTemplateResponse struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Attributes struct {
		Name   string `json:"name"`
		Prompt string `json:"prompt"`
		ChatID uint   `json:"chat_id"`
	} `json:"attributes"`
}
