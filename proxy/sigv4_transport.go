package proxy

import "net/http"

// sigV4Transport signs a request just before it is sent. A SigV4 signature
// covers the request's headers, and httputil.ReverseProxy changes them after
// its Director runs: it appends to X-Forwarded-For and removes the headers
// the client listed in Connection. Signed in the Director, a Bedrock request
// carrying either then failed with 403.
type sigV4Transport struct {
	base http.RoundTripper
	sign func(*http.Request) error
}

func (t *sigV4Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	out := req.Clone(req.Context()) // a RoundTripper must not modify its request
	if err := t.sign(out); err != nil {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, err
	}
	return t.base.RoundTrip(out)
}
