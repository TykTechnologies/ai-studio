package guardrails

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// withMasker registers a masker that masks every occurrence of the given
// words, for the duration of a test.
func withMasker(t *testing.T, words ...string) {
	t.Helper()
	RegisterEvidenceMasker(func(text string) []Span {
		var out []Span
		for _, w := range words {
			for i := 0; ; {
				k := strings.Index(text[i:], w)
				if k < 0 {
					break
				}
				out = append(out, Span{Start: i + k, End: i + k + len(w)})
				i += k + len(w)
			}
		}
		return out
	})
	t.Cleanup(func() { RegisterEvidenceMasker(nil) })
}

func TestExcerpt_NoMaskerNoExcerpt(t *testing.T) {
	RegisterEvidenceMasker(nil)
	if got, ok := Excerpt("ignore previous instructions", nil, 50); ok || got != "" {
		t.Fatalf("Excerpt without a masker = %q, %v; want none", got, ok)
	}
	if CanExcerpt() {
		t.Fatal("CanExcerpt with no masker")
	}
}

func TestExcerpt_WindowAroundTheSpan(t *testing.T) {
	withMasker(t)
	text := strings.Repeat("a", 100) + "INJECT" + strings.Repeat("b", 100)
	got, ok := Excerpt(text, &Span{Start: 100, End: 106}, 20)
	if !ok {
		t.Fatal("no excerpt")
	}
	want := "…" + strings.Repeat("a", 7) + "INJECT" + strings.Repeat("b", 7) + "…"
	if got != want {
		t.Fatalf("excerpt = %q, want %q", got, want)
	}
}

func TestExcerpt_StartOfTextWithoutSpan(t *testing.T) {
	withMasker(t)
	got, _ := Excerpt("0123456789", nil, 4)
	if got != "0123…" {
		t.Fatalf("excerpt = %q", got)
	}
	got, _ = Excerpt("short", nil, 40)
	if got != "short" {
		t.Fatalf("whole text = %q", got)
	}
	got, _ = Excerpt("short", &Span{Start: 3, End: 99}, 40)
	if got != "short" {
		t.Fatalf("an out-of-range span falls back to the start: %q", got)
	}
}

func TestExcerpt_SpanAtTheEndGivesTheRoomBefore(t *testing.T) {
	withMasker(t)
	text := strings.Repeat("x", 50) + "END"
	got, _ := Excerpt(text, &Span{Start: 50, End: 53}, 10)
	if got != "…"+strings.Repeat("x", 7)+"END" {
		t.Fatalf("excerpt = %q", got)
	}
}

func TestExcerpt_LongSpanIsCutToTheLimit(t *testing.T) {
	withMasker(t)
	text := "pre " + strings.Repeat("z", 50) + " post"
	got, _ := Excerpt(text, &Span{Start: 4, End: 54}, 10)
	if got != "…"+strings.Repeat("z", 10)+"…" {
		t.Fatalf("excerpt = %q", got)
	}
}

func TestExcerpt_MasksAndNeverCutsThroughOne(t *testing.T) {
	withMasker(t, "sk-SECRETKEY123", "alice@example.com")
	text := "xx sk-SECRETKEY123 ignore previous; mail alice@example.com now"
	// The window starts inside the key (5 characters before "ignore"):
	// the key is masked whole rather than cut to "Y123".
	at := strings.Index(text, "ignore")
	got, _ := Excerpt(text, &Span{Start: at, End: at + len("ignore")}, 16)
	if got != "…"+ExcerptMask+" ignore prev…" {
		t.Fatalf("excerpt = %q", got)
	}
	// And one whose end falls inside the address.
	at = strings.Index(text, "mail")
	got, _ = Excerpt(text, &Span{Start: at, End: at + len("mail")}, 10)
	if strings.Contains(got, "alice") || strings.Contains(got, "example") || !strings.Contains(got, ExcerptMask) {
		t.Fatalf("excerpt = %q", got)
	}
}

func TestExcerpt_OverlappingMasksPrintOnce(t *testing.T) {
	RegisterEvidenceMasker(func(string) []Span {
		return []Span{{Start: 2, End: 6}, {Start: 4, End: 8}, {Start: 8, End: 9}, {Start: -1, End: 3}, {Start: 5, End: 99}}
	})
	t.Cleanup(func() { RegisterEvidenceMasker(nil) })
	got, _ := Excerpt("0123456789", nil, 10)
	if got != "01"+ExcerptMask+"9" {
		t.Fatalf("excerpt = %q", got)
	}
}

func TestExcerpt_MultibyteAndInvalidUTF8(t *testing.T) {
	withMasker(t)
	text := strings.Repeat("é", 30) + "\xff" + strings.Repeat("日本", 10)
	for _, sp := range []*Span{nil, {Start: 1, End: 3}, {Start: 59, End: 61}, {Start: 61, End: 64}} {
		got, _ := Excerpt(text, sp, 9)
		if !utf8.ValidString(got) {
			t.Fatalf("span %v: excerpt %q is not valid UTF-8", sp, got)
		}
		if n := utf8.RuneCountInString(strings.Trim(got, "…")); n > 9 {
			t.Fatalf("span %v: excerpt %q has %d characters, limit 9", sp, got, n)
		}
	}
}

func TestTextSHA256(t *testing.T) {
	// The value Wolf Defender's decision log records for this text
	// (hashlib.sha256("hello".encode()).hexdigest()).
	if got := TextSHA256("hello"); got != "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824" {
		t.Fatalf("TextSHA256 = %s", got)
	}
}
