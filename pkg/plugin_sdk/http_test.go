package plugin_sdk

import (
	"net/http"
	"testing"
)

func TestDefaultHTTPClient(t *testing.T) {
	client := DefaultHTTPClient()

	if client == nil {
		t.Fatal("DefaultHTTPClient returned nil")
	}

	if client.Timeout == 0 {
		t.Error("Expected non-zero timeout")
	}

	if client.Timeout.Seconds() != 30 {
		t.Errorf("Expected 30s timeout, got %v", client.Timeout)
	}

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("Expected *http.Transport")
	}

	if transport.MaxIdleConns != 100 {
		t.Errorf("Expected 100 max idle conns, got %d", transport.MaxIdleConns)
	}

	if transport.MaxIdleConnsPerHost != 10 {
		t.Errorf("Expected 10 max idle conns per host, got %d", transport.MaxIdleConnsPerHost)
	}

	if transport.TLSHandshakeTimeout.Seconds() != 10 {
		t.Errorf("Expected 10s TLS handshake timeout, got %v", transport.TLSHandshakeTimeout)
	}
}

func TestDefaultHTTPClient_ReturnsNewInstance(t *testing.T) {
	client1 := DefaultHTTPClient()
	client2 := DefaultHTTPClient()

	if client1 == client2 {
		t.Error("Expected different instances from each call")
	}
}
