package grpc

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
)

// clientTLSConfig is the TLS configuration an edge dials its control plane
// with: the control plane's certificate is checked against EDGE_TLS_CA_PATH
// when set (a private CA) or the system roots, under the name in
// EDGE_TLS_SERVER_NAME when set (a load balancer or IP address in
// EDGE_CONTROL_ENDPOINT), and an optional client certificate is presented
// for mutual TLS.
func clientTLSConfig(hs config.HubSpokeConfig) (*tls.Config, error) {
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}

	if hs.SkipTLSVerify {
		tlsConfig.InsecureSkipVerify = true
	} else if hs.ClientTLSCAPath != "" {
		pem, err := os.ReadFile(hs.ClientTLSCAPath)
		if err != nil {
			return nil, fmt.Errorf("read EDGE_TLS_CA_PATH: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("EDGE_TLS_CA_PATH %s contains no PEM certificates", hs.ClientTLSCAPath)
		}
		tlsConfig.RootCAs = pool
	}
	tlsConfig.ServerName = hs.ClientTLSServerName

	if hs.ClientTLSCertPath != "" && hs.ClientTLSKeyPath != "" {
		cert, err := tls.LoadX509KeyPair(hs.ClientTLSCertPath, hs.ClientTLSKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificates: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}
	return tlsConfig, nil
}
