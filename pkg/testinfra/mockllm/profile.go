package mockllm

import (
	"fmt"
	"math"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"
)

// ProfileHeader lets a single request override the server's default Profile,
// so one running mock can serve every benchmark scenario. The value is a
// comma-separated list of key=value pairs using the same keys as ParseProfile.
const ProfileHeader = "X-Mock-Profile"

// Profile shapes how the benchmark mock answers: how long until the first
// token, how fast tokens arrive afterwards, and how many there are. The zero
// value answers immediately with a single token.
type Profile struct {
	// TTFT is the median delay before the first byte of the response body.
	TTFT time.Duration
	// TTFTP99 turns TTFT into a lognormal distribution whose median is TTFT
	// and whose 99th percentile is TTFTP99. Zero (or <= TTFT) means fixed.
	TTFTP99 time.Duration
	// TokensPerSecond paces streamed tokens after the first. Zero sends them
	// back to back. For non-streaming responses the whole generation time
	// (TTFT + tokens/rate) elapses before the body is written, as it would
	// with a real vendor.
	TokensPerSecond float64
	// OutputTokens is the number of completion tokens produced.
	OutputTokens int
	// TokenText is the text of each streamed token. Its length times
	// OutputTokens sets the response size.
	TokenText string
	// FailureRate is the probability (0-1) of answering 500 instead.
	FailureRate float64
	// Model is echoed when the request does not name one.
	Model string
}

// DefaultProfile answers immediately with 16 short tokens.
func DefaultProfile() Profile {
	return Profile{OutputTokens: 16, TokenText: "tok ", Model: "mock-model"}
}

// ParseProfile applies "key=value,key=value" overrides onto base. Keys:
// ttft, ttft_p99 (durations), tps (tokens/s), tokens, token_text,
// failure_rate, model.
func ParseProfile(base Profile, spec string) (Profile, error) {
	p := base
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			return base, fmt.Errorf("profile entry %q is not key=value", part)
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		var err error
		switch k {
		case "ttft":
			p.TTFT, err = time.ParseDuration(v)
		case "ttft_p99":
			p.TTFTP99, err = time.ParseDuration(v)
		case "tps":
			p.TokensPerSecond, err = strconv.ParseFloat(v, 64)
		case "tokens":
			p.OutputTokens, err = strconv.Atoi(v)
		case "token_text":
			p.TokenText = v
		case "failure_rate":
			p.FailureRate, err = strconv.ParseFloat(v, 64)
		case "model":
			p.Model = v
		default:
			return base, fmt.Errorf("unknown profile key %q", k)
		}
		if err != nil {
			return base, fmt.Errorf("profile key %q: %w", k, err)
		}
	}
	if p.OutputTokens < 1 {
		p.OutputTokens = 1
	}
	if p.TokenText == "" {
		p.TokenText = "tok "
	}
	return p, nil
}

// sampleTTFT draws one first-token delay from the profile's distribution.
func (p Profile) sampleTTFT() time.Duration {
	if p.TTFT <= 0 {
		return 0
	}
	if p.TTFTP99 <= p.TTFT {
		return p.TTFT
	}
	// Lognormal with median m and p99 q: sigma = ln(q/m) / z(0.99).
	const z99 = 2.3263478740
	sigma := math.Log(float64(p.TTFTP99)/float64(p.TTFT)) / z99
	return time.Duration(float64(p.TTFT) * math.Exp(sigma*rand.NormFloat64()))
}

// tokenInterval is the gap between consecutive streamed tokens.
func (p Profile) tokenInterval() time.Duration {
	if p.TokensPerSecond <= 0 {
		return 0
	}
	return time.Duration(float64(time.Second) / p.TokensPerSecond)
}
