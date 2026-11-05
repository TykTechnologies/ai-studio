package entitlements

import (
	"errors"

	"github.com/TykTechnologies/midsommar/v2/helpers"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// CodeQuotaExceeded is the machine-readable code of a refusal over a limit
// or quota, in errors[0].code of the HTTP response.
const CodeQuotaExceeded = "QUOTA_EXCEEDED"

// ExceededError refuses a change that would take a limit or quota past its
// maximum. Message is the user-facing text, written by the implementation.
//
// It converts itself for both transports, also when wrapped: errors.As into
// a helpers.ErrorResponse gives 409 with code QUOTA_EXCEEDED (so handlers
// that pass helper errors through need no change), and gRPC reports it as
// ResourceExhausted.
type ExceededError struct {
	Message string
}

// NewExceededError returns an *ExceededError with the message.
func NewExceededError(message string) error {
	return &ExceededError{Message: message}
}

func (e *ExceededError) Error() string { return e.Message }

// HTTPError is the 409 response for the refusal.
func (e *ExceededError) HTTPError() helpers.ErrorResponse {
	r := helpers.NewConflictError(e.Message)
	r.Title = "Quota Exceeded"
	r.Code = CodeQuotaExceeded
	return r
}

// As lets errors.As convert the refusal into a helpers.ErrorResponse.
func (e *ExceededError) As(target any) bool {
	if he, ok := target.(*helpers.ErrorResponse); ok {
		*he = e.HTTPError()
		return true
	}
	return false
}

// GRPCStatus reports the refusal as ResourceExhausted to gRPC.
func (e *ExceededError) GRPCStatus() *status.Status {
	return status.New(codes.ResourceExhausted, e.Message)
}

// AsExceeded returns the *ExceededError in err's chain, if any.
func AsExceeded(err error) (*ExceededError, bool) {
	var ee *ExceededError
	if errors.As(err, &ee) {
		return ee, true
	}
	return nil, false
}

// GRPCError returns the ResourceExhausted status error for an err that
// carries an *ExceededError, for gRPC handlers that otherwise wrap service
// errors in their own status.
func GRPCError(err error) (error, bool) {
	if ee, ok := AsExceeded(err); ok {
		return ee.GRPCStatus().Err(), true
	}
	return nil, false
}
