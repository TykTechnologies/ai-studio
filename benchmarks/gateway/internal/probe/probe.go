// Package probe sends one LLM request and times it from the client's point of
// view: time to response headers, time to the first content token, and time to
// the end of the body. It also collects the gateway's Server-Timing breakdown
// when present.
//
// Every duration is measured from the request's *intended* start, which the
// scheduler passes in. When the load generator falls behind, the delay counts
// against the request, as it would for a real user (coordinated-omission
// correction). Lag records by how much the actual send trailed the intent.
package probe

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"strings"
	"time"
)

// Format is the request/response wire format.
type Format string

const (
	FormatOpenAI    Format = "openai"
	FormatAnthropic Format = "anthropic"
)

// Target is where one arm of a cell sends its requests.
type Target struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"-"` // may hold credentials; never serialised
}

// Request is a prepared, reusable request: the body is built once per cell.
type Request struct {
	Format Format
	Stream bool
	Body   []byte
}

// Result is one request's measurements. Durations are milliseconds.
type Result struct {
	Cell     string `json:"cell"`
	Arm      string `json:"arm"`
	Seq      int64  `json:"seq"`
	Warmup   bool   `json:"warmup,omitempty"`
	Intended int64  `json:"intended_unix_ns"`

	Lag   float64 `json:"lag_ms"`   // actual send - intended send
	TTFB  float64 `json:"ttfb_ms"`  // intended -> response headers
	TTFT  float64 `json:"ttft_ms"`  // intended -> first content token (REST: full body)
	Total float64 `json:"total_ms"` // intended -> body fully read

	Status int                `json:"status"`
	Err    string             `json:"err,omitempty"`
	Bytes  int64              `json:"bytes"`
	Tokens int                `json:"tokens"` // content deltas seen (streaming)
	Reused bool               `json:"conn_reused"`
	Timing map[string]float64 `json:"server_timing,omitempty"`
	ConnGw string             `json:"gw_conn,omitempty"` // gateway->upstream conn: reused/new
}

// OK reports whether the request succeeded end to end.
func (r *Result) OK() bool { return r.Err == "" && r.Status >= 200 && r.Status < 300 }

// Do sends req to t and fills in a Result. intended is when the scheduler
// meant to send it.
func Do(ctx context.Context, client *http.Client, t Target, req Request, intended time.Time) Result {
	var res Result
	res.Intended = intended.UnixNano()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, t.URL, bytes.NewReader(req.Body))
	if err != nil {
		res.Err = err.Error()
		return res
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if req.Stream {
		httpReq.Header.Set("Accept", "text/event-stream")
	}
	for k, v := range t.Headers {
		httpReq.Header.Set(k, v)
	}

	var headersAt time.Time
	trace := &httptrace.ClientTrace{
		GotConn:              func(i httptrace.GotConnInfo) { res.Reused = i.Reused },
		GotFirstResponseByte: func() { headersAt = time.Now() },
	}
	httpReq = httpReq.WithContext(httptrace.WithClientTrace(httpReq.Context(), trace))

	sent := time.Now()
	res.Lag = ms(sent.Sub(intended))
	resp, err := client.Do(httpReq)
	if err != nil {
		res.Err = classify(err)
		res.Total = ms(time.Since(intended))
		return res
	}
	defer resp.Body.Close()
	res.Status = resp.StatusCode
	if headersAt.IsZero() {
		headersAt = time.Now()
	}
	res.TTFB = ms(headersAt.Sub(intended))

	var firstToken time.Time
	if req.Stream && resp.StatusCode == http.StatusOK {
		firstToken, res.Tokens, res.Bytes, err = readStream(resp.Body, req.Format)
	} else {
		var n int64
		n, err = io.Copy(io.Discard, resp.Body)
		res.Bytes = n
		firstToken = time.Now()
	}
	end := time.Now()
	res.Total = ms(end.Sub(intended))
	if !firstToken.IsZero() {
		res.TTFT = ms(firstToken.Sub(intended))
	} else {
		res.TTFT = res.Total
	}
	if err != nil {
		res.Err = classify(err)
	} else if req.Stream && resp.StatusCode == http.StatusOK && res.Tokens == 0 {
		res.Err = "stream ended without content"
	}

	// The trailer (chunked responses) holds the complete breakdown; the
	// header holds what was known when headers were sent.
	st := resp.Trailer.Get("Server-Timing")
	if st == "" {
		st = resp.Header.Get("Server-Timing")
	}
	if st != "" {
		res.Timing, res.ConnGw = ParseServerTiming(st)
		// A buffered response (no trailer) has the full upstream time in its
		// header, and the body follows the header immediately, so the
		// gateway's time is what elapsed outside the upstream call.
		if _, ok := res.Timing["gw"]; !ok {
			el, hasEl := res.Timing["elapsed"]
			up, hasUp := res.Timing["upstream"]
			if hasEl && hasUp {
				res.Timing["gw"] = el - up
			}
		}
	}
	return res
}

// readStream reads an SSE body to the end and returns when the first content
// token arrived, how many content deltas there were, and the byte count.
func readStream(body io.Reader, format Format) (time.Time, int, int64, error) {
	br := bufio.NewReaderSize(body, 64*1024)
	var first time.Time
	var tokens int
	var n int64
	for {
		line, err := br.ReadSlice('\n')
		n += int64(len(line))
		if len(line) > 0 && hasContent(line, format) {
			if first.IsZero() {
				first = time.Now()
			}
			tokens++
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			// Very long line: drain the rest of it.
			for errors.Is(err, bufio.ErrBufferFull) {
				line, err = br.ReadSlice('\n')
				n += int64(len(line))
			}
		}
		if err == io.EOF {
			return first, tokens, n, nil
		}
		if err != nil {
			return first, tokens, n, err
		}
	}
}

var dataPrefix = []byte("data:")

// hasContent reports whether an SSE line carries a non-empty text delta.
// Parsing every frame as JSON would make the load generator the bottleneck at
// high rates, so this decodes only frames that could carry text.
func hasContent(line []byte, format Format) bool {
	line = bytes.TrimSpace(line)
	if !bytes.HasPrefix(line, dataPrefix) {
		return false
	}
	data := bytes.TrimSpace(line[len(dataPrefix):])
	switch format {
	case FormatAnthropic:
		if !bytes.Contains(data, []byte(`"text_delta"`)) {
			return false
		}
		var ev struct {
			Delta struct {
				Text string `json:"text"`
			} `json:"delta"`
		}
		return json.Unmarshal(data, &ev) == nil && ev.Delta.Text != ""
	default:
		if !bytes.Contains(data, []byte(`"content"`)) {
			return false
		}
		var ch struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if json.Unmarshal(data, &ch) != nil {
			return false
		}
		for _, c := range ch.Choices {
			if c.Delta.Content != "" {
				return true
			}
		}
		return false
	}
}

// ParseServerTiming parses `name;dur=1.5, name2;desc="x"` into durations, and
// returns the conn description separately.
func ParseServerTiming(v string) (map[string]float64, string) {
	out := map[string]float64{}
	conn := ""
	for _, metric := range strings.Split(v, ",") {
		fields := strings.Split(strings.TrimSpace(metric), ";")
		name := fields[0]
		if name == "" {
			continue
		}
		for _, f := range fields[1:] {
			k, val, ok := strings.Cut(strings.TrimSpace(f), "=")
			if !ok {
				continue
			}
			switch k {
			case "dur":
				var d float64
				if _, err := fmt.Sscanf(val, "%g", &d); err == nil {
					out[name] = d
				}
			case "desc":
				if name == "conn" {
					conn = strings.Trim(val, `"`)
				}
			}
		}
	}
	return out, conn
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

// classify shortens transport errors to stable categories for reporting.
func classify(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	}
	s := err.Error()
	for _, k := range []string{"connection refused", "connection reset", "broken pipe", "EOF", "no such host", "too many open files"} {
		if strings.Contains(s, k) {
			return k
		}
	}
	return s
}
