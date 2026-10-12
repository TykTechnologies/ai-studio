package nativeapi

import (
	"errors"
	"io"
	"strings"
	"testing"
)

// Adversarial cases added during independent verification (ut0.12).

func TestCodecRejectsCaseMalleatedMAC(t *testing.T) {
	c := mustCodec(t, "sk-ant-secret")
	s := Scope{LLMID: 1, AppID: 2}
	w, err := c.Wrap(testBatch, "msgbatch_abc", s)
	if err != nil {
		t.Fatal(err)
	}
	// Same bytes, different spelling: must not be a second valid ID.
	upper := w[:strings.LastIndex(w, ".")+1] + strings.ToUpper(w[strings.LastIndex(w, ".")+1:])
	if upper == w {
		t.Fatal("test MAC had no hex letters; pick another vendor id")
	}
	if _, err := c.Unwrap(testBatch, upper, s); !errors.Is(err, ErrMalformedID) {
		t.Fatalf("upper-case MAC: got %v, want ErrMalformedID", err)
	}
}

func TestCodecForeignKeyAndKidSpoof(t *testing.T) {
	ours := mustCodec(t, "sk-ant-ours")
	theirs := mustCodec(t, "sk-ant-theirs")
	s := Scope{LLMID: 3, AppID: 4}
	w, err := theirs.Wrap(testBatch, "msgbatch_abc", s)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ours.Unwrap(testBatch, w, s); !errors.Is(err, ErrKeyMismatch) {
		t.Fatalf("foreign key: got %v, want ErrKeyMismatch", err)
	}
	// Splice our (public) kid onto their MAC: the kid is not the credential.
	spoof := strings.Replace(w, ".t1k"+theirs.KID()+".", ".t1k"+ours.KID()+".", 1)
	if _, err := ours.Unwrap(testBatch, spoof, s); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("kid spoof: got %v, want ErrBadSignature", err)
	}
}

func TestCodecScopeFieldsNotInterchangeable(t *testing.T) {
	c := mustCodec(t, "sk-ant-secret")
	w, err := c.Wrap(testBatch, "msgbatch_abc", Scope{LLMID: 5, AppID: 9})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []Scope{{LLMID: 9, AppID: 5}, {LLMID: 0, AppID: 9}, {LLMID: 5, AppID: 0}, {}} {
		if _, err := c.Unwrap(testBatch, w, s); !errors.Is(err, ErrBadSignature) {
			t.Errorf("scope %+v: got %v, want ErrBadSignature", s, err)
		}
	}
}

func TestCodecNilAndTruncated(t *testing.T) {
	var nilCodec *Codec
	if _, err := nilCodec.Wrap(testBatch, "msgbatch_abc", Scope{}); !errors.Is(err, ErrEmptyKey) {
		t.Fatalf("nil Wrap: %v", err)
	}
	if _, err := nilCodec.Unwrap(testBatch, "msgbatch_abc.t1k00000000.00000000000000000000000000000000", Scope{}); !errors.Is(err, ErrEmptyKey) {
		t.Fatalf("nil Unwrap: %v", err)
	}
	c := mustCodec(t, "k")
	w, _ := c.Wrap(testBatch, "msgbatch_abc", Scope{LLMID: 1, AppID: 1})
	for _, bad := range []string{
		w[:len(w)-1], w + "0", w + ".", "." + w[len("msgbatch_abc"):], // truncated, extended, empty vendor id
		strings.Replace(w, ".t1k", ".t2k", 1), // unknown version
		"msgbatch_abc", "", ".t1k", strings.Repeat("a", 4096),
	} {
		if _, err := c.Unwrap(testBatch, bad, Scope{LLMID: 1, AppID: 1}); err == nil {
			t.Errorf("Unwrap(%q) accepted", bad)
		}
	}
}

// FuzzUnwrap: no panics, and the only string that verifies for a scope is the
// exact one Wrap produced (no malleable second spelling).
func FuzzUnwrap(f *testing.F) {
	c, err := NewCodec("sk-ant-fuzz")
	if err != nil {
		f.Fatal(err)
	}
	s := Scope{LLMID: 11, AppID: 22}
	w, err := c.Wrap(testBatch, "msgbatch_seed", s)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(w)
	f.Add(w + "x")
	f.Add(strings.ToUpper(w))
	f.Add("msgbatch_.t1k.t1k" + c.KID() + ".")
	f.Fuzz(func(t *testing.T, in string) {
		vid, err := c.Unwrap(testBatch, in, s)
		if err != nil {
			return
		}
		again, werr := c.Wrap(testBatch, vid, s)
		if werr != nil || again != in {
			t.Fatalf("Unwrap accepted %q (vendor id %q) but Wrap gives %q (%v)", in, vid, again, werr)
		}
	})
}

func TestLineReaderCRLFAcrossBufferBoundary(t *testing.T) {
	// The bufio buffer is 64 KiB: put the \r as the last byte of the first
	// fill and the \n in the next.
	long := strings.Repeat("a", 64<<10-1)
	in := long + "\r\n" + "second\r\n"
	got, err := readAll(t, strings.NewReader(in), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != long || got[1] != "second" {
		t.Fatalf("got %d lines; first len %d, second %q", len(got), len(got[0]), got[len(got)-1])
	}
}

func TestLineReaderExactCapBoundaries(t *testing.T) {
	const max = 10
	exact := strings.Repeat("x", max)
	for _, tc := range []struct {
		name, in string
		wantErr  bool
	}{
		{"exact LF", exact + "\n", false},
		{"exact CRLF", exact + "\r\n", false},
		{"exact EOF", exact, false},
		{"over LF", exact + "y\n", true},
		{"over CRLF", exact + "y\r\n", true},
		{"over EOF", exact + "y", true},
		{"over by CR without LF", exact + "yy\r", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lines, err := readAll(t, strings.NewReader(tc.in), max)
			if tc.wantErr {
				if !errors.Is(err, ErrLineTooLong) {
					t.Fatalf("got %v (%d lines), want ErrLineTooLong", err, len(lines))
				}
				return
			}
			if err != nil || len(lines) != 1 || lines[0] != exact {
				t.Fatalf("got %q, %v", lines, err)
			}
		})
	}
}

func TestLineReaderBlankVariantsAndInnerCR(t *testing.T) {
	in := "\r\n \t \n{\"a\":\"x\ry\"}\r\n\n\n"
	lr := NewLineReader(strings.NewReader(in), 0)
	line, err := lr.Next()
	if err != nil || string(line) != "{\"a\":\"x\ry\"}" {
		t.Fatalf("got %q, %v", line, err)
	}
	if lr.Line() != 3 {
		t.Errorf("Line() = %d, want 3 (blank lines counted)", lr.Line())
	}
	if _, err := lr.Next(); err != io.EOF {
		t.Fatalf("want io.EOF after trailing blanks, got %v", err)
	}
}

func TestMatcherRejectsNonCanonicalPaths(t *testing.T) {
	m := mustMatcher(t, batchSpec())
	// Sanity: the canonical form matches.
	if _, ok := m.Match("GET", "/v1/messages/batches"); !ok {
		t.Skip("batchSpec has no GET /v1/messages/batches list op")
	}
	for _, p := range []string{
		"/v1/messages/batches/", "//v1/messages/batches", "/v1//messages/batches",
		"v1/messages/batches", "/v1/messages/batches?x=1", "/V1/messages/batches", "",
	} {
		if mt, ok := m.Match("GET", p); ok {
			t.Errorf("Match(GET %q) = %s, want no match", p, mt.Op.Name)
		}
	}
	if _, ok := m.Match("get", "/v1/messages/batches"); ok {
		t.Error("lower-case method matched")
	}
	var nilM *Matcher
	if _, ok := nilM.Match("GET", "/v1/messages/batches"); ok || nilM.MatchesPath("/v1/messages/batches") {
		t.Error("nil matcher matched")
	}
}

func TestMatcherSameShapeAcrossMethodsAllowed(t *testing.T) {
	rt := ResourceType{Name: "r"}
	_, err := NewMatcher(Spec{
		Resources: []ResourceType{rt},
		Operations: []Operation{
			{Name: "get", Method: "GET", Path: "/x/{id}", PathResource: "r"},
			{Name: "del", Method: "DELETE", Path: "/x/{id}", PathResource: "r"},
		},
	})
	if err != nil {
		t.Fatalf("same shape, different methods: %v", err)
	}
	_, err = NewMatcher(Spec{
		Resources: []ResourceType{rt},
		Operations: []Operation{
			{Name: "a", Method: "GET", Path: "/x/{id}", PathResource: "r"},
			{Name: "b", Method: "GET", Path: "/x/{other}"},
		},
	})
	if err == nil {
		t.Fatal("ambiguous GET /x/{id} and /x/{other} accepted")
	}
}

// ut0.19 fix: IDCharset applies to the whole ID even with alternation or
// multiline flags, and dot-only IDs never pass.
func TestValidVendorIDWholeStringAdversarial(t *testing.T) {
	for _, tc := range []struct {
		src string
		id  string
		ok  bool
	}{
		{`^a|b$`, "xxb", false},
		{`^a|b$`, "axx", false},
		{`^a|b$`, "b", true},
		{`(?m)^[a-z]+$`, "abc\ndef", false},
		{`(?m)^[a-z]+$`, "abc", true},
		{`^[a-z.]+$`, "..", false},
		{`^[a-z.]+$`, ".", false},
		{`^[a-z.]+$`, "a..", true},
	} {
		rt := ResourceType{Name: "r", IDCharset: mustRe(tc.src)}
		if got := rt.ValidVendorID(tc.id); got != tc.ok {
			t.Errorf("%s ValidVendorID(%q) = %v, want %v", tc.src, tc.id, got, tc.ok)
		}
	}
	for _, id := range []string{".", "..", "-", "_", "._-"} {
		if (ResourceType{Name: "r"}).ValidVendorID(id) {
			t.Errorf("default charset accepted %q", id)
		}
	}
}

func TestValidVendorIDConcurrentCache(t *testing.T) {
	rt := ResourceType{Name: "r", IDCharset: mustRe(`^msgbatch_[A-Za-z0-9]+$`)}
	done := make(chan bool)
	for i := 0; i < 16; i++ {
		go func() { done <- rt.ValidVendorID("msgbatch_abc") && !rt.ValidVendorID("msgbatch_..") }()
	}
	for i := 0; i < 16; i++ {
		if !<-done {
			t.Fatal("inconsistent result under concurrency")
		}
	}
}

// efj: SkipLine moves past an oversized line whether the cap tripped inside
// the bufio buffer (short over-cap line, consumed whole) or across buffer
// fills (huge line, midLine), with CRLF, consecutive oversized lines and an
// oversized last line without a newline.
func TestLineReaderSkipLineVerify(t *testing.T) {
	huge := strings.Repeat("h", 200<<10) // spans several 64 KiB fills
	small := strings.Repeat("s", 50)     // over a 10-byte cap, within one fill
	in := "a\n" + small + "\n" + huge + "\r\n" + huge + "\n\n" + "b\r\n" + small + "\r\n" + "c\n" + huge
	lr := NewLineReader(strings.NewReader(in), 10)
	var got []string
	oversized := 0
	for {
		line, err := lr.Next()
		if err == io.EOF {
			break
		}
		if errors.Is(err, ErrLineTooLong) {
			oversized++
			if err := lr.SkipLine(); err != nil {
				t.Fatalf("SkipLine: %v", err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		got = append(got, string(line))
	}
	if strings.Join(got, ",") != "a,b,c" || oversized != 5 {
		t.Fatalf("got %q with %d oversized; want a,b,c with 5", got, oversized)
	}
	// SkipLine without a pending oversized line is a no-op.
	lr = NewLineReader(strings.NewReader("x\ny\n"), 10)
	if l, _ := lr.Next(); string(l) != "x" {
		t.Fatal("first line")
	}
	if err := lr.SkipLine(); err != nil {
		t.Fatal(err)
	}
	if l, _ := lr.Next(); string(l) != "y" {
		t.Fatalf("SkipLine consumed a normal line: next is %q", l)
	}
}
