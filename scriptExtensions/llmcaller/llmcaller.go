package llmcaller

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/switches"
	"github.com/d5/tengo/v2"
	"github.com/d5/tengo/v2/token"
	"github.com/tmc/langchaingo/llms"
)

type LLMCaller struct {
	service services.ServiceInterface
}

func NewLLMCaller(serviceRef services.ServiceInterface) *LLMCaller {
	return &LLMCaller{
		service: serviceRef,
	}
}

func (*LLMCaller) TypeName() string {
	return "LLMCaller"
}
func (*LLMCaller) String() string {
	return "LLMCaller"
}
func (*LLMCaller) BinaryOp(op token.Token, rhs tengo.Object) (tengo.Object, error) {
	panic("not implemented")
}

func (*LLMCaller) IsFalsy() bool {
	return false
}
func (*LLMCaller) Equals(another tengo.Object) bool {
	return false
}

func (*LLMCaller) IndexGet(index tengo.Object) (value tengo.Object, err error) {
	panic("not implemented")
}

func (*LLMCaller) IndexSet(index, value tengo.Object) error {
	panic("not implemented")
}

func (*LLMCaller) Iterate() tengo.Iterator {
	panic("not implemented")
}

func (*LLMCaller) CanIterate() bool {
	return false
}

func (*LLMCaller) CanCall() bool {
	return true
}

func (a *LLMCaller) makeLLMCall(llmID int, llmSettingsID int, prompt string) (string, error) {
	slog.Info("running LLMCaller", "llmID", llmID, "llmSettingsID", llmSettingsID)
	llmDetail, err := a.service.GetLLMByID(uint(llmID))
	if err != nil {
		slog.Error("error getting LLM by ID", "llmID", llmID, "error", err)
		return "", err
	}

	conf, err := a.service.GetLLMSettingsByID(uint(llmSettingsID))
	if err != nil {
		slog.Error("error getting LLM settings by ID", "llmSettingsID", llmSettingsID, "error", err)
		return "", err
	}

	llm, err := switches.FetchDriver(llmDetail, conf, nil, nil)
	if err != nil {
		slog.Error("error fetching LLM driver", "error", err)
		return "", err
	}

	options := conf.GenerateOptionsFromSettings([]llms.Tool{}, "message", nil)

	mc := llms.TextParts(llms.ChatMessageTypeHuman, prompt)
	response, err := llm.GenerateContent(context.Background(), []llms.MessageContent{mc}, options...)
	if err != nil {
		slog.Error("error generating content", "error", err)
		return "", err
	}

	if len(response.Choices) < 1 {
		slog.Error("no responses returned")
		return "", fmt.Errorf("no responses returned")
	}

	return response.Choices[0].Content, nil
}

func (a *LLMCaller) cleanString(str string) string {
	return strings.Trim(str, "\"")
}

func (a *LLMCaller) Call(args ...tengo.Object) (ret tengo.Object, err error) {
	if len(args) != 3 {
		return nil, fmt.Errorf("expected 3 arguments, got %d", len(args))
	}

	llmIDObj := args[0].(*tengo.Int)
	confIDObj := args[1].(*tengo.Int)
	prompt := a.cleanString(args[2].String())

	out, err := a.makeLLMCall(int(llmIDObj.Value), int(confIDObj.Value), prompt)
	if err != nil {
		return nil, err
	}

	return &tengo.String{
		Value: out,
	}, nil
}

func (*LLMCaller) Copy() tengo.Object {
	return &LLMCaller{}
}
