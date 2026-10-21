package bedrockVendor

import (
	"io"
	"net/http"

	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
)

// sdkHTTPClient is the HTTP client of the Bedrock SDK clients: the SDK's
// default, with request bodies that do not offer io.WriterTo.
//
// smithy-go hands net/http a request body that, once closed, answers WriteTo
// with io.EOF, and closes it as soon as Do returns, which is when the
// response headers arrive. net/http may still be writing the request then:
// after the body's ContentLength bytes it drains the rest with
// io.Copy(io.Discard, body), and io.Copy prefers WriteTo, so the drain fails
// with io.EOF, the transport's write loop closes the connection, and the
// response being read on it breaks ("use of closed network connection",
// midsommar-clean-q0z.26). It needs the server to answer before that drain
// runs, so it shows under load. Without WriteTo the drain reads, and a
// closed body's io.EOF on Read is the normal end of the copy.
//
// It wraps the client the SDK built, so whatever configured that client
// (AWS_CA_BUNDLE, proxy settings, the defaults mode's timeouts) still
// applies; LoadDefaultConfig refuses a custom CA bundle for any client but
// its own.
type sdkHTTPClient struct {
	base bedrockruntime.HTTPClient
}

// newSDKHTTPClient wraps base, or the SDK's default client when base is nil.
func newSDKHTTPClient(base bedrockruntime.HTTPClient) *sdkHTTPClient {
	if base == nil {
		base = awshttp.NewBuildableClient()
	}
	return &sdkHTTPClient{base: base}
}

// Do sends r with its body hidden behind a plain io.ReadCloser.
func (c *sdkHTTPClient) Do(r *http.Request) (*http.Response, error) {
	if r.Body != nil && r.Body != http.NoBody {
		r.Body = readOnlyBody{r.Body}
	}
	return c.base.Do(r)
}

// readOnlyBody is a request body with only Read and Close.
type readOnlyBody struct{ rc io.ReadCloser }

func (b readOnlyBody) Read(p []byte) (int, error) { return b.rc.Read(p) }
func (b readOnlyBody) Close() error               { return b.rc.Close() }
