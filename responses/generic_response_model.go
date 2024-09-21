package responses

type GenericResponse struct {
	Model            string
	Choices          int
	ToolCalls        int
	PromptTokens     int
	CompletionTokens int
}

func (o *GenericResponse) GetPromptTokens() int {
	return o.PromptTokens
}

func (o *GenericResponse) GetResponseTokens() int {
	return o.CompletionTokens
}

func (o *GenericResponse) GetChoiceCount() int {
	return o.Choices
}

func (o *GenericResponse) GetToolCount() int {
	return o.ToolCalls
}

func (o *GenericResponse) GetModel() string {
	return o.Model
}
