package semanticrouting

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMessagesFromOpenAIBody(t *testing.T) {
	chat := `{"model":"smart/auto","messages":[
		{"role":"system","content":"be brief"},
		{"role":"user","content":[{"type":"text","text":"look at this"},{"type":"image_url","image_url":{"url":"x"}},{"type":"text","text":"and prove it"}]},
		{"role":"assistant","content":null},
		{"role":"user","content":"thanks"}]}`
	assert.Equal(t, []Message{
		{Role: "system", Content: "be brief"},
		{Role: "user", Content: "look at this\nand prove it"},
		{Role: "user", Content: "thanks"},
	}, MessagesFromOpenAIBody([]byte(chat)))

	assert.Equal(t, []Message{{Role: "user", Content: "once upon"}},
		MessagesFromOpenAIBody([]byte(`{"prompt":"once upon"}`)))
	assert.Equal(t, []Message{{Role: "user", Content: "a\nb"}},
		MessagesFromOpenAIBody([]byte(`{"prompt":["a","b"]}`)))
	assert.Nil(t, MessagesFromOpenAIBody([]byte(`not json`)))
	assert.Nil(t, MessagesFromOpenAIBody([]byte(`{"messages":[]}`)))
}
