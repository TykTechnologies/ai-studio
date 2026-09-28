package analytics

import (
	"context"
	"time"
)

// requestStartKey carries the time the gateway received a proxied request, so
// the analytics recorded for it can say how long it took (total_time_ms).
type requestStartKey struct{}

// WithRequestStart records when the gateway received the request.
func WithRequestStart(ctx context.Context, start time.Time) context.Context {
	return context.WithValue(ctx, requestStartKey{}, start)
}

// RequestLatencyMS is the time from the request's start to end, in
// milliseconds. end is normally the moment the response completed (a record's
// TimeStamp); when it is not after the start (a timestamp taken at the start
// of the request) the time elapsed until now is used instead. ok is false when
// the context carries no start.
func RequestLatencyMS(ctx context.Context, end time.Time) (int, bool) {
	start, ok := ctx.Value(requestStartKey{}).(time.Time)
	if !ok || start.IsZero() {
		return 0, false
	}
	if !end.After(start) {
		end = time.Now()
	}
	return int(end.Sub(start).Milliseconds()), true
}
