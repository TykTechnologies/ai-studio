package bedrockVendor

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrorBody(t *testing.T) {
	for body, want := range map[string]string{
		`{"error":{"message":"blocked","type":"content_policy_violation"}}`: `{"message":"blocked"}`,
		`{"error":"rate limit exceeded"}`:                                   `{"message":"rate limit exceeded"}`,
		`{"message":"already AWS"}`:                                         `{"message":"already AWS"}`,
		`{"Message":"upper"}`:                                               `{"message":"upper"}`,
		"plain text refusal\n":                                              `{"message":"plain text refusal"}`,
		"":                                                                  `{"message":"the request was refused"}`,
	} {
		assert.JSONEq(t, want, string(ErrorBody([]byte(body))), body)
	}
}

// An AWS SDK reads the shaped refusal as the typed Bedrock error with the
// plugin's message (8xf.30): a firewall's 403 is AccessDeniedException, a
// rate limiter's 429 ThrottlingException.
func TestErrorShapeReadsAsTheSDKError(t *testing.T) {
	for status, check := range map[int]func(t *testing.T, err error){
		http.StatusForbidden: func(t *testing.T, err error) {
			var denied *types.AccessDeniedException
			require.True(t, errors.As(err, &denied), "%v", err)
			assert.Equal(t, "prompt injection detected", denied.ErrorMessage())
		},
		http.StatusTooManyRequests: func(t *testing.T, err error) {
			var throttled *types.ThrottlingException
			require.True(t, errors.As(err, &throttled), "%v", err)
			assert.Equal(t, "prompt injection detected", throttled.ErrorMessage())
		},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set(ErrorTypeHeader, ErrorType(status))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = w.Write(ErrorBody([]byte(`{"error":{"message":"prompt injection detected"}}`)))
		}))
		client := bedrockruntime.New(bedrockruntime.Options{
			BaseEndpoint: aws.String(srv.URL), Region: "us-east-1",
			Credentials: aws.AnonymousCredentials{}, Retryer: aws.NopRetryer{},
		}, func(o *bedrockruntime.Options) { o.AuthSchemePreference = []string{"noAuth"} })
		_, err := client.Converse(context.Background(), &bedrockruntime.ConverseInput{ModelId: aws.String("m")})
		check(t, err)
		srv.Close()
	}
}

func TestErrorType(t *testing.T) {
	for status, want := range map[int]string{
		400: "ValidationException", 401: "AccessDeniedException", 403: "AccessDeniedException",
		404: "ResourceNotFoundException", 408: "ModelTimeoutException", 413: "ValidationException",
		429: "ThrottlingException", 500: "InternalServerException", 502: "InternalServerException",
		503: "ServiceUnavailableException",
	} {
		assert.Equal(t, want, ErrorType(status), status)
	}
}
