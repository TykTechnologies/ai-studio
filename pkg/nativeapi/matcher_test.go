package nativeapi

import (
	"net/http"
	"strings"
	"testing"
)

func batchSpec() Spec {
	return Spec{
		Resources: []ResourceType{testBatch},
		Operations: []Operation{
			{Name: "create", Method: http.MethodPost, Path: "/v1/messages/batches", Ledger: LedgerCreate},
			{Name: "list", Method: http.MethodGet, Path: "/v1/messages/batches", Ledger: LedgerList},
			{Name: "retrieve", Method: http.MethodGet, Path: "/v1/messages/batches/{id}", PathResource: testBatch.Name, Ledger: LedgerRead},
			{Name: "delete", Method: http.MethodDelete, Path: "/v1/messages/batches/{id}", PathResource: testBatch.Name, Ledger: LedgerDelete},
			{Name: "cancel", Method: http.MethodPost, Path: "/v1/messages/batches/{id}/cancel", PathResource: testBatch.Name, Ledger: LedgerMutate},
			{Name: "results", Method: http.MethodGet, Path: "/v1/messages/batches/{id}/results", PathResource: testBatch.Name, Ledger: LedgerRead},
			{Name: "special", Method: http.MethodGet, Path: "/v1/messages/batches/stats"},
		},
	}
}

func mustMatcher(t *testing.T, s Spec) *Matcher {
	t.Helper()
	m, err := NewMatcher(s)
	if err != nil {
		t.Fatalf("NewMatcher: %v", err)
	}
	return m
}

func TestMatcherExact(t *testing.T) {
	m := mustMatcher(t, batchSpec())
	cases := []struct {
		method, path string
		want         string // op name, "" = no match
		id           string
	}{
		{"POST", "/v1/messages/batches", "create", ""},
		{"GET", "/v1/messages/batches", "list", ""},
		{"GET", "/v1/messages/batches/msgbatch_1.t1kab", "retrieve", "msgbatch_1.t1kab"},
		{"DELETE", "/v1/messages/batches/x", "delete", "x"},
		{"POST", "/v1/messages/batches/x/cancel", "cancel", "x"},
		{"GET", "/v1/messages/batches/x/results", "results", "x"},
		// Literal segments beat parameters.
		{"GET", "/v1/messages/batches/stats", "special", ""},

		// The sync chain is never captured.
		{"POST", "/v1/messages", "", ""},
		{"POST", "/v1/messages/count_tokens", "", ""},
		// Method mismatch.
		{"PUT", "/v1/messages/batches", "", ""},
		{"POST", "/v1/messages/batches/x", "", ""},
		{"get", "/v1/messages/batches", "", ""},
		// No prefix, suffix, trailing-slash or empty-segment tolerance.
		{"GET", "/v1/messages/batches/", "", ""},
		{"GET", "/v1/messages/batches//results", "", ""},
		{"GET", "/v1/messages/batches/x/results/extra", "", ""},
		{"GET", "/api/v1/messages/batches", "", ""},
		{"GET", "v1/messages/batches", "", ""},
		{"GET", "", "", ""},
	}
	for _, tc := range cases {
		got, ok := m.Match(tc.method, tc.path)
		if tc.want == "" {
			if ok {
				t.Errorf("%s %s matched %q, want no match", tc.method, tc.path, got.Op.Name)
			}
			continue
		}
		if !ok || got.Op.Name != tc.want {
			t.Errorf("%s %s = %v %v, want %q", tc.method, tc.path, ok, got.Op, tc.want)
			continue
		}
		if got.ID() != tc.id {
			t.Errorf("%s %s id = %q, want %q", tc.method, tc.path, got.ID(), tc.id)
		}
	}
}

func TestMatcherMatchesPath(t *testing.T) {
	m := mustMatcher(t, batchSpec())
	if !m.MatchesPath("/v1/messages/batches/x") {
		t.Error("native path not recognised")
	}
	if m.MatchesPath("/v1/messages") {
		t.Error("sync path recognised as native")
	}
	var nilM *Matcher
	if _, ok := nilM.Match("GET", "/x"); ok || nilM.MatchesPath("/x") {
		t.Error("nil matcher matched")
	}
}

func TestMatcherReturnsSpecOperation(t *testing.T) {
	m := mustMatcher(t, batchSpec())
	got, _ := m.Match("GET", "/v1/messages/batches/x/results")
	if got.Op != &m.Spec().Operations[5] {
		t.Error("Match does not point into the matcher's spec")
	}
}

func TestSpecValidate(t *testing.T) {
	bad := []struct {
		name string
		mut  func(*Spec)
		want string
	}{
		{"dup resource", func(s *Spec) { s.Resources = append(s.Resources, testBatch) }, "duplicate resource"},
		{"unanchored charset", func(s *Spec) { s.Resources[0].IDCharset = mustRe("[a-z]+") }, "anchored"},
		{"no name", func(s *Spec) { s.Operations[0].Name = "" }, "no name"},
		{"dup name", func(s *Spec) { s.Operations[1].Name = "create" }, "duplicate operation"},
		{"bad method", func(s *Spec) { s.Operations[0].Method = "post" }, "invalid method"},
		{"relative path", func(s *Spec) { s.Operations[0].Path = "v1/x" }, "must start"},
		{"empty segment", func(s *Spec) { s.Operations[0].Path = "/v1//x" }, "empty segment"},
		{"trailing slash", func(s *Spec) { s.Operations[0].Path = "/v1/x/" }, "empty segment"},
		{"partial param", func(s *Spec) { s.Operations[0].Path = "/v1/x{id}" }, "partial"},
		{"id without resource", func(s *Spec) { s.Operations[2].PathResource = "" }, "without PathResource"},
		{"resource without id", func(s *Spec) { s.Operations[0].PathResource = testBatch.Name }, "without {id}"},
		{"unknown resource", func(s *Spec) { s.Operations[2].PathResource = "nope" }, "unknown PathResource"},
		{"bad ref", func(s *Spec) { s.Operations[0].ResponseIDs = []BodyRef{{Path: "id", Resource: "nope"}} }, "body ref"},
		{"admit incomplete", func(s *Spec) { s.Operations[0].Admit = &AdmitSpec{} }, "Admit"},
		{"results incomplete", func(s *Spec) { s.Operations[5].Results = &ResultsSpec{} }, "Results"},
		{"list incomplete", func(s *Spec) { s.Operations[1].List = &ListSpec{} }, "List"},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			s := batchSpec()
			s.Resources = append([]ResourceType{}, s.Resources...)
			tc.mut(&s)
			_, err := NewMatcher(s)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want containing %q", err, tc.want)
			}
		})
	}

	t.Run("ambiguous shapes", func(t *testing.T) {
		s := batchSpec()
		s.Resources = append(s.Resources, ResourceType{Name: "other"})
		s.Operations = append(s.Operations, Operation{
			Name: "retrieve2", Method: http.MethodGet, Path: "/v1/messages/batches/{id}", PathResource: "other",
		})
		if _, err := NewMatcher(s); err == nil || !strings.Contains(err.Error(), "same requests") {
			t.Fatalf("err = %v", err)
		}
	})
}
