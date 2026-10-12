package nativeapi

import (
	"errors"
	"regexp"
	"strings"
	"testing"
)

var testBatch = ResourceType{
	Name:      "anthropic.message_batch",
	IDCharset: regexp.MustCompile(`^msgbatch_[A-Za-z0-9_.-]{1,200}$`),
}

func mustCodec(t *testing.T, cred string) *Codec {
	t.Helper()
	c, err := NewCodec(cred)
	if err != nil {
		t.Fatalf("NewCodec: %v", err)
	}
	return c
}

func TestCodecRoundTrip(t *testing.T) {
	c := mustCodec(t, "sk-ant-secret")
	scope := Scope{LLMID: 7, AppID: 42}
	for _, vid := range []string{
		"msgbatch_01HkcTjaV5uDC8jWR4ZsDV8d",
		"msgbatch_with.dots.inside",
		"msgbatch_under_scores__x",
		"msgbatch_x.t1kdeadbeef.0123", // looks like a wrapped suffix
	} {
		w, err := c.Wrap(testBatch, vid, scope)
		if err != nil {
			t.Fatalf("Wrap(%q): %v", vid, err)
		}
		if !strings.HasPrefix(w, vid+".t1k"+c.KID()+".") {
			t.Fatalf("unexpected format %q", w)
		}
		got, err := c.Unwrap(testBatch, w, scope)
		if err != nil || got != vid {
			t.Fatalf("Unwrap(%q) = %q, %v; want %q", w, got, err, vid)
		}
	}
}

func TestCodecDeterministicAcrossNodes(t *testing.T) {
	// Hub, replicas and edges derive the same key from the same credential.
	a, b := mustCodec(t, "k"), mustCodec(t, "k")
	wa, _ := a.Wrap(testBatch, "msgbatch_1", Scope{1, 2})
	wb, _ := b.Wrap(testBatch, "msgbatch_1", Scope{1, 2})
	if wa != wb {
		t.Fatalf("not deterministic: %q vs %q", wa, wb)
	}
}

func TestCodecRejects(t *testing.T) {
	c := mustCodec(t, "sk-ant-secret")
	scope := Scope{LLMID: 7, AppID: 42}
	w, err := c.Wrap(testBatch, "msgbatch_abc", scope)
	if err != nil {
		t.Fatal(err)
	}
	i := strings.LastIndex(w, ".")
	mac := w[i+1:]
	flip := func(s string, pos int) string {
		b := []byte(s)
		if b[pos] == '0' {
			b[pos] = '1'
		} else {
			b[pos] = '0'
		}
		return string(b)
	}
	otherType := ResourceType{Name: "anthropic.file", IDCharset: testBatch.IDCharset}

	cases := []struct {
		name    string
		rt      ResourceType
		id      string
		scope   Scope
		codec   *Codec
		wantErr error
	}{
		{"tampered mac", testBatch, w[:i+1] + flip(mac, 0), scope, c, ErrBadSignature},
		{"tampered vendor id", testBatch, strings.Replace(w, "msgbatch_abc", "msgbatch_abd", 1), scope, c, ErrBadSignature},
		{"tampered kid", testBatch, strings.Replace(w, ".t1k"+c.KID(), ".t1k"+flip(c.KID(), 0), 1), scope, c, ErrKeyMismatch},
		{"wrong app", testBatch, w, Scope{LLMID: 7, AppID: 43}, c, ErrBadSignature},
		{"wrong llm", testBatch, w, Scope{LLMID: 8, AppID: 42}, c, ErrBadSignature},
		{"wrong type", otherType, w, scope, c, ErrBadSignature},
		{"other key", testBatch, w, scope, mustCodec(t, "rotated"), ErrKeyMismatch},
		{"raw id", testBatch, "msgbatch_abc", scope, c, ErrMalformedID},
		{"empty", testBatch, "", scope, c, ErrMalformedID},
		{"no vendor id", testBatch, w[len("msgbatch_abc"):], scope, c, ErrMalformedID},
		{"uppercase mac", testBatch, w[:i+1] + strings.ToUpper(mac), scope, c, ErrMalformedID},
		{"short mac", testBatch, w[:len(w)-1], scope, c, ErrMalformedID},
		{"long mac", testBatch, w + "0", scope, c, ErrMalformedID},
		{"charset", testBatch, strings.Replace(w, "msgbatch_abc", "msgbatch_a/c", 1), scope, c, ErrMalformedID},
		{"wrong prefix for charset", testBatch, strings.Replace(w, "msgbatch_abc", "file_abcdefg", 1), scope, c, ErrMalformedID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.codec.Unwrap(tc.rt, tc.id, tc.scope)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Unwrap(%q) err = %v, want %v", tc.id, err, tc.wantErr)
			}
			if got != "" {
				t.Fatalf("Unwrap returned vendor id %q on failure", got)
			}
		})
	}
}

func TestCodecEmptyKey(t *testing.T) {
	if _, err := NewCodec(""); !errors.Is(err, ErrEmptyKey) {
		t.Fatalf("NewCodec(\"\") err = %v", err)
	}
	var c *Codec
	if _, err := c.Wrap(testBatch, "msgbatch_1", Scope{}); !errors.Is(err, ErrEmptyKey) {
		t.Fatalf("nil Wrap err = %v", err)
	}
	if _, err := c.Unwrap(testBatch, "msgbatch_1.t1k00000000.00000000000000000000000000000000", Scope{}); !errors.Is(err, ErrEmptyKey) {
		t.Fatalf("nil Unwrap err = %v", err)
	}
}

func TestCodecWrapRejectsInvalidVendorID(t *testing.T) {
	c := mustCodec(t, "k")
	for _, vid := range []string{"", "file_1", "msgbatch_a b", "msgbatch_é", "msgbatch_" + strings.Repeat("x", 201)} {
		if _, err := c.Wrap(testBatch, vid, Scope{}); !errors.Is(err, ErrMalformedID) {
			t.Fatalf("Wrap(%q) err = %v, want ErrMalformedID", vid, err)
		}
	}
	if _, err := c.Wrap(ResourceType{}, "x", Scope{}); !errors.Is(err, ErrMalformedID) {
		t.Fatalf("Wrap with unnamed type err = %v", err)
	}
	// Default charset applies when none is declared.
	rt := ResourceType{Name: "x.thing"}
	if _, err := c.Wrap(rt, "a:b", Scope{}); !errors.Is(err, ErrMalformedID) {
		t.Fatalf("default charset accepted ':' (err %v)", err)
	}
	if _, err := c.Wrap(rt, "a.b_c-d", Scope{}); err != nil {
		t.Fatalf("default charset rejected a.b_c-d: %v", err)
	}
}

func TestParseIDForLedgerFallback(t *testing.T) {
	old := mustCodec(t, "old-key")
	w, _ := old.Wrap(testBatch, "msgbatch_rot", Scope{1, 2})
	p, err := ParseID(testBatch, w)
	if err != nil || p.VendorID != "msgbatch_rot" || p.KID != old.KID() {
		t.Fatalf("ParseID = %+v, %v", p, err)
	}
	if _, err := mustCodec(t, "new-key").Unwrap(testBatch, w, Scope{1, 2}); !errors.Is(err, ErrKeyMismatch) {
		t.Fatalf("rotated key err = %v", err)
	}
}

func TestCodecFieldsAreLengthPrefixed(t *testing.T) {
	// Without length prefixes, (type "a.b", vendor "c") and (type "a",
	// vendor "b.c")-style shifts could collide. Scope digits shifting between
	// LLM and App must not collide either.
	c := mustCodec(t, "k")
	rt := ResourceType{Name: "t"}
	w1, _ := c.Wrap(rt, "x", Scope{LLMID: 1, AppID: 23})
	if _, err := c.Unwrap(rt, w1, Scope{LLMID: 12, AppID: 3}); err == nil {
		t.Fatal("scope shift verified")
	}
}

func TestValidVendorIDWholeStringAndDots(t *testing.T) {
	def := ResourceType{Name: "r"}
	for _, id := range []string{".", "..", "...", "-", "_", ".-_"} {
		if def.ValidVendorID(id) {
			t.Errorf("default charset accepted %q", id)
		}
	}
	if !def.ValidVendorID("a.b") || !def.ValidVendorID("..a") {
		t.Error("default charset rejected an ID with letters")
	}

	// A top-level alternation passes the anchoring check but must still be
	// applied to the whole ID.
	alt := ResourceType{Name: "alt", IDCharset: mustRe(`^a|b$`)}
	if err := (Spec{Resources: []ResourceType{alt}}).Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	for id, want := range map[string]bool{"a": true, "b": true, "xxb": false, "axx": false, "ab": false} {
		if got := alt.ValidVendorID(id); got != want {
			t.Errorf("ValidVendorID(%q) = %v, want %v", id, got, want)
		}
	}
	if _, err := mustCodec(t, "k").Wrap(alt, "xxb", Scope{}); !errors.Is(err, ErrMalformedID) {
		t.Errorf("Wrap accepted substring match: %v", err)
	}
}
