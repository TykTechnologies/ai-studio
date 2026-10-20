// Package pluginlimits sets the largest gRPC message plugins and plugin hosts
// exchange, and recognises a plugin that refused a message for its size.
//
// A gRPC server accepts 4 MiB per message unless told otherwise, and
// go-plugin's DefaultGRPCServer does not tell it. A request body over that
// size (a long context, a few base64 screenshots, one PDF) then failed in
// every post_auth plugin. The plugin SDKs serve with GRPCServer instead, and
// the hosts' brokered servers add ServerOptions, so both sides accept
// MaxMsgBytes. go-plugin's client side already allows math.MaxInt32.
//
// A plugin built on an older SDK keeps the 4 MiB limit. Hosts wrap its refusal
// with AsTooLarge so the caller can answer 413 naming the plugin.
package pluginlimits

import (
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// EnvMaxMsgBytes ("PLUGIN_GRPC_MAX_MSG_BYTES") overrides DefaultMaxMsgBytes,
// in bytes. The hosts pass it to plugin processes (see pkg/pluginenv), so one
// setting covers both sides.
const EnvMaxMsgBytes = "PLUGIN_GRPC_MAX_MSG_BYTES"

// DefaultMaxMsgBytes is 64 MiB, above the LLM vendors' request limits
// (Anthropic's is 32 MB).
const DefaultMaxMsgBytes = 64 << 20

// LegacyMaxMsgBytes is gRPC's default receive limit, which plugins built
// before AI Studio 2.3 still have.
const LegacyMaxMsgBytes = 4 << 20

// MaxMsgBytes returns EnvMaxMsgBytes when it holds a positive whole number,
// capped at math.MaxInt32 (gRPC's own ceiling), and DefaultMaxMsgBytes
// otherwise.
func MaxMsgBytes() int {
	v := strings.TrimSpace(os.Getenv(EnvMaxMsgBytes))
	if v == "" {
		return DefaultMaxMsgBytes
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return DefaultMaxMsgBytes
	}
	if n > math.MaxInt32 {
		return math.MaxInt32
	}
	return int(n)
}

// ServerOptions returns the receive and send limits for a gRPC server that
// talks to plugins or plugin hosts.
func ServerOptions() []grpc.ServerOption {
	n := MaxMsgBytes()
	return []grpc.ServerOption{grpc.MaxRecvMsgSize(n), grpc.MaxSendMsgSize(n)}
}

// GRPCServer is a drop-in for go-plugin's DefaultGRPCServer (the ServeConfig
// GRPCServer field) that adds ServerOptions.
func GRPCServer(opts []grpc.ServerOption) *grpc.Server {
	return grpc.NewServer(append(opts, ServerOptions()...)...)
}

// IsTooLarge reports whether err is gRPC refusing a message for its size, on
// either end: "received message larger than max" or "trying to send message
// larger than max". A plugin's own ResourceExhausted (a quota, say) is not.
func IsTooLarge(err error) bool {
	if err == nil {
		return false
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.ResourceExhausted {
		return false
	}
	return strings.Contains(st.Message(), "larger than max")
}

// TooLargeError is a plugin call that failed because the message was over the
// plugin's gRPC limit.
type TooLargeError struct {
	Plugin string // plugin name
	Hook   string // hook or RPC, e.g. "post_auth"
	Bytes  int    // size of the message the host sent
	Err    error
}

func (e *TooLargeError) Error() string {
	return fmt.Sprintf("plugin %s refused a %d-byte %s message as too large; plugins built before AI Studio 2.3 accept at most %d bytes, so rebuild it on the current SDK (%s sets the limit): %v",
		e.Plugin, e.Bytes, e.Hook, LegacyMaxMsgBytes, EnvMaxMsgBytes, e.Err)
}

func (e *TooLargeError) Unwrap() error { return e.Err }

// AsTooLarge wraps err in a *TooLargeError when IsTooLarge(err), and returns
// it unchanged otherwise.
func AsTooLarge(err error, plugin, hook string, bytes int) error {
	if !IsTooLarge(err) {
		return err
	}
	return &TooLargeError{Plugin: plugin, Hook: hook, Bytes: bytes, Err: err}
}

// AsTooLargeError returns the *TooLargeError in err's chain, if any.
func AsTooLargeError(err error) (*TooLargeError, bool) {
	var tl *TooLargeError
	if errors.As(err, &tl) {
		return tl, true
	}
	return nil, false
}
