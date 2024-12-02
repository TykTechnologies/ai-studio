package proxy

type Hate struct {
	Filtered bool   `json:"filtered"`
	Severity string `json:"severity,omitempty"`
}
type SelfHarm struct {
	Filtered bool   `json:"filtered"`
	Severity string `json:"severity,omitempty"`
}
type Sexual struct {
	Filtered bool   `json:"filtered"`
	Severity string `json:"severity,omitempty"`
}
type Violence struct {
	Filtered bool   `json:"filtered"`
	Severity string `json:"severity,omitempty"`
}

type ContentFilterResults struct {
	Hate     Hate     `json:"hate,omitempty"`
	SelfHarm SelfHarm `json:"self_harm,omitempty"`
	Sexual   Sexual   `json:"sexual,omitempty"`
	Violence Violence `json:"violence,omitempty"`
}

type InnerError struct {
	Code                 string               `json:"code,omitempty"`
	ContentFilterResults ContentFilterResults `json:"content_filter_result,omitempty"`
}

type APIError struct {
	Code           any         `json:"code,omitempty"`
	Message        string      `json:"message"`
	Param          *string     `json:"param,omitempty"`
	Type           string      `json:"type"`
	HTTPStatus     string      `json:"-"`
	HTTPStatusCode int         `json:"-"`
	InnerError     *InnerError `json:"innererror,omitempty"`
}

type OAIErrorResponse struct {
	Error *APIError `json:"error,omitempty"`
}
