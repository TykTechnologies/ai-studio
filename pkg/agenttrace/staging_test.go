package agenttrace

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"reflect"
	"testing"
)

// gcmSealer is a test Sealer: AES-256-GCM with a random nonce prefix.
type gcmSealer struct{ aead cipher.AEAD }

func newGCMSealer(t *testing.T) gcmSealer {
	t.Helper()
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	return gcmSealer{aead: aead}
}

func (g gcmSealer) Seal(plain, aad []byte) ([]byte, error) {
	nonce := make([]byte, g.aead.NonceSize())
	_, _ = rand.Read(nonce)
	return g.aead.Seal(nonce, nonce, plain, aad), nil
}

func (g gcmSealer) Open(sealed, aad []byte) ([]byte, error) {
	n := g.aead.NonceSize()
	if len(sealed) < n {
		return nil, errors.New("short")
	}
	return g.aead.Open(nil, sealed[:n], sealed[n:], aad)
}

var secretContent = []byte("TOP-SECRET-PROMPT-CONTENT")

func stagedEvent() *Event {
	e := fullEvent()
	e.Parts[0].Data = append([]byte(nil), secretContent...)
	e.Parts[0].ContentState = StateFull
	e.SetIdempotencyKey(SourceEmbedded)
	return e
}

func TestStagedSealedRoundTrip(t *testing.T) {
	SetSealer(newGCMSealer(t))
	t.Cleanup(func() { SetSealer(nil) })

	in := stagedEvent()
	blob, stripped, err := EncodeStaged(in)
	if err != nil || stripped {
		t.Fatalf("encode: stripped=%v err=%v", stripped, err)
	}
	if bytes.Contains(blob, secretContent) {
		t.Fatal("content appears in clear in the staged blob")
	}
	out, err := DecodeStaged(blob, in.IdempotencyKey)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Fatalf("round trip mismatch:\n in=%+v\nout=%+v", in, out)
	}
	if _, err := DecodeStaged(blob, "embedded:other:0"); err == nil {
		t.Fatal("a blob must not open under another row's idempotency key")
	}

	SetSealer(nil)
	if _, err := DecodeStaged(blob, in.IdempotencyKey); !errors.Is(err, ErrStagedSealed) {
		t.Fatalf("want ErrStagedSealed, got %v", err)
	}
}

func TestStagedWithoutSealerFailsClosed(t *testing.T) {
	SetSealer(nil)
	in := stagedEvent()
	blob, stripped, err := EncodeStaged(in)
	if err != nil || !stripped {
		t.Fatalf("encode: stripped=%v err=%v", stripped, err)
	}
	if bytes.Contains(blob, secretContent) {
		t.Fatal("content staged in plaintext without a sealer")
	}
	out, err := DecodeStaged(blob, in.IdempotencyKey)
	if err != nil {
		t.Fatal(err)
	}
	if out.Parts[0].Data != nil || out.Parts[0].ContentState != StateDropped || out.Parts[0].Fingerprint == nil {
		t.Fatalf("part not stripped to metadata: %+v", out.Parts[0])
	}
}

func TestEncodeStagedNeedsKey(t *testing.T) {
	if _, _, err := EncodeStaged(&Event{Kind: KindToolCall}); err == nil {
		t.Fatal("missing idempotency key accepted")
	}
	if _, err := DecodeStaged(nil, "k"); err == nil {
		t.Fatal("empty blob accepted")
	}
	if _, err := DecodeStaged([]byte{9, 1}, "k"); err == nil {
		t.Fatal("unknown format accepted")
	}
}

func TestStripHelpers(t *testing.T) {
	e := &Event{
		Parts: []PayloadPart{
			{Part: PartRequest, ContentState: StateHashed, Fingerprint: Fingerprint{1, 2}},
			{Part: PartResponse, ContentState: StateFull, Data: []byte("x")},
		},
		GatewayEvents: []GatewayEvent{{Type: GatewayEventBudget, Detail: "over by 2"}},
	}
	if !StripPayloadData(e) || StripPayloadData(e) {
		t.Fatal("StripPayloadData should report one strip")
	}
	if e.Parts[0].ContentState != StateHashed || e.Parts[1].ContentState != StateDropped {
		t.Fatalf("states: %+v", e.Parts)
	}
	if !StripGatewayEventDetail(e) || e.GatewayEvents[0].Detail != "" || StripGatewayEventDetail(e) {
		t.Fatal("StripGatewayEventDetail")
	}
}
