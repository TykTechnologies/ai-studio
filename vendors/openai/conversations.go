package openaiVendor

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"

	"github.com/TykTechnologies/midsommar/v2/pkg/nativeapi"
)

// OpenAI Conversations on the native routes. A conversation stores items the
// Responses API reads as context (POST /v1/responses with "conversation"),
// so its ID is owned like a response ID: signed for the App and LLM when the
// gateway hands it out, verified wherever it comes back. The items a client
// stores are screened by the request filters as Responses input, since they
// reach the model through every response that names the conversation.
//
// Item IDs (msg_..., fc_..., rs_...) are not wrapped. They are random,
// returned only to the App whose response or conversation holds them, and an
// item_reference to one in another App's stored data needs that ID; see
// features/NativeAPIExtensions.md.

// ResourceConversation is the native resource type of a conversation.
const ResourceConversation = "openai.conversation"

var conversationIDCharset = regexp.MustCompile(`^conv_[A-Za-z0-9_-]{1,200}$`)

var conversationResource = nativeapi.ResourceType{Name: ResourceConversation, IDCharset: conversationIDCharset}

// conversationRef is a request's or response's conversation: "conv_..." or
// {"id": "conv_..."}.
var conversationRef = nativeapi.BodyRef{Path: "conversation", Resource: ResourceConversation, ObjectID: true}

var conversationIDs = []nativeapi.BodyRef{{Path: "id", Resource: ResourceConversation}}

var conversationItemsAdmit = &nativeapi.AdmitSpec{Split: splitConversationItems, Join: joinConversationItems}

var conversationOperations = []nativeapi.Operation{
	{
		Name:        "openai.conversations.create",
		Method:      http.MethodPost,
		Path:        "/v1/conversations",
		Ledger:      nativeapi.LedgerNone,
		Admit:       conversationItemsAdmit,
		ResponseIDs: conversationIDs,
		Upstream:    nativeapi.UpstreamSpec{HeaderAllow: nativeHeaderAllow},
		ErrorShape:  NativeError,
	},
	{
		Name:         "openai.conversations.retrieve",
		Method:       http.MethodGet,
		Path:         "/v1/conversations/{id}",
		PathResource: ResourceConversation,
		Ledger:       nativeapi.LedgerNone,
		ResponseIDs:  conversationIDs,
		Upstream:     nativeapi.UpstreamSpec{HeaderAllow: nativeHeaderAllow},
		ErrorShape:   NativeError,
	},
	{
		// Updates the metadata.
		Name:         "openai.conversations.update",
		Method:       http.MethodPost,
		Path:         "/v1/conversations/{id}",
		PathResource: ResourceConversation,
		Ledger:       nativeapi.LedgerNone,
		ResponseIDs:  conversationIDs,
		Upstream:     nativeapi.UpstreamSpec{HeaderAllow: nativeHeaderAllow},
		ErrorShape:   NativeError,
	},
	{
		Name:         "openai.conversations.delete",
		Method:       http.MethodDelete,
		Path:         "/v1/conversations/{id}",
		PathResource: ResourceConversation,
		Ledger:       nativeapi.LedgerNone,
		ResponseIDs:  conversationIDs,
		Upstream:     nativeapi.UpstreamSpec{HeaderAllow: nativeHeaderAllow},
		ErrorShape:   NativeError,
	},
	{
		Name:         "openai.conversations.items.create",
		Method:       http.MethodPost,
		Path:         "/v1/conversations/{id}/items",
		PathResource: ResourceConversation,
		Ledger:       nativeapi.LedgerNone,
		Admit:        conversationItemsAdmit,
		Upstream:     nativeapi.UpstreamSpec{HeaderAllow: nativeHeaderAllow, QueryAllow: []string{"include", "include[]"}},
		ErrorShape:   NativeError,
	},
	{
		Name:         "openai.conversations.items.list",
		Method:       http.MethodGet,
		Path:         "/v1/conversations/{id}/items",
		PathResource: ResourceConversation,
		Ledger:       nativeapi.LedgerNone,
		Upstream: nativeapi.UpstreamSpec{
			HeaderAllow: nativeHeaderAllow,
			QueryAllow:  []string{"after", "limit", "order", "include", "include[]"},
		},
		ErrorShape: NativeError,
	},
	{
		Name:         "openai.conversations.items.retrieve",
		Method:       http.MethodGet,
		Path:         "/v1/conversations/{id}/items/{item_id}",
		PathResource: ResourceConversation,
		Ledger:       nativeapi.LedgerNone,
		Upstream:     nativeapi.UpstreamSpec{HeaderAllow: nativeHeaderAllow, QueryAllow: []string{"include", "include[]"}},
		ErrorShape:   NativeError,
	},
	{
		// Answers with the conversation.
		Name:         "openai.conversations.items.delete",
		Method:       http.MethodDelete,
		Path:         "/v1/conversations/{id}/items/{item_id}",
		PathResource: ResourceConversation,
		Ledger:       nativeapi.LedgerNone,
		ResponseIDs:  conversationIDs,
		Upstream:     nativeapi.UpstreamSpec{HeaderAllow: nativeHeaderAllow},
		ErrorShape:   NativeError,
	},
}

// errConversationBody is a conversation body the gateway does not accept.
var errConversationBody = errors.New("invalid conversation body")

// splitConversationItems admits the items of a conversation create or items
// create body as one Responses-shaped item ({"input": items}), so the
// filters screen them as they would the same items sent as a response's
// input. It invokes no model.
func splitConversationItems(body []byte) ([]nativeapi.Item, error) {
	// The items are read as maps (exact keys); only the body is a struct.
	if err := nativeapi.CheckJSONKeys(body, ""); err != nil {
		return nil, fmt.Errorf("%w: %v", errConversationBody, err)
	}
	var b map[string]json.RawMessage
	if err := json.Unmarshal(body, &b); err != nil || b == nil {
		return nil, fmt.Errorf("%w: must be a JSON object", errConversationBody)
	}
	items, ok := b["items"]
	if !ok || bytes.Equal(bytes.TrimSpace(items), []byte("null")) {
		return nil, nil
	}
	if t := bytes.TrimSpace(items); len(t) == 0 || t[0] != '[' {
		return nil, fmt.Errorf("%w: items must be an array", errConversationBody)
	}
	view, err := json.Marshal(map[string]json.RawMessage{"input": items})
	if err != nil {
		return nil, err
	}
	return []nativeapi.Item{{Format: ResponsesFilterFormat, Body: view}}, nil
}

// joinConversationItems writes the filtered items back into the body.
func joinConversationItems(original []byte, items []nativeapi.Item) ([]byte, error) {
	if len(items) != 1 {
		return nil, fmt.Errorf("%w: %d items", errConversationBody, len(items))
	}
	var view struct {
		Input json.RawMessage `json:"input"`
	}
	if err := json.Unmarshal(items[0].Body, &view); err != nil || len(view.Input) == 0 {
		return nil, fmt.Errorf("%w: filtered items unreadable", errConversationBody)
	}
	dec := json.NewDecoder(bytes.NewReader(original))
	dec.UseNumber()
	var b map[string]any
	if err := dec.Decode(&b); err != nil {
		return nil, err
	}
	b["items"] = view.Input
	return json.Marshal(b)
}
