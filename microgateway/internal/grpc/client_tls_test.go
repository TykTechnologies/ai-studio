package grpc

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// privateCA is a CA and a control plane certificate it signed for
// control.internal, as MDCB-style deployments issue from a private CA.
type privateCA struct {
	caPEM      []byte
	serverCert tls.Certificate
}

func newPrivateCA(t *testing.T) privateCA {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test private CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	require.NoError(t, err)
	caCert, err := x509.ParseCertificate(caDER)
	require.NoError(t, err)

	srvKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	srvTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "control.internal"},
		DNSNames:     []string{"control.internal"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	srvDER, err := x509.CreateCertificate(rand.Reader, srvTmpl, caCert, &srvKey.PublicKey, caKey)
	require.NoError(t, err)

	return privateCA{
		caPEM:      pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}),
		serverCert: tls.Certificate{Certificate: [][]byte{srvDER}, PrivateKey: srvKey},
	}
}

// serveTLS accepts TLS connections on loopback with cert and completes each
// handshake.
func serveTLS(t *testing.T, cert tls.Certificate) string {
	t.Helper()
	lis, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	require.NoError(t, err)
	t.Cleanup(func() { lis.Close() })
	go func() {
		for {
			conn, err := lis.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_ = conn.(*tls.Conn).Handshake()
			}()
		}
	}()
	return lis.Addr().String()
}

func handshake(addr string, cfg *tls.Config) error {
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 2 * time.Second}, "tcp", addr, cfg)
	if err != nil {
		return err
	}
	return conn.Close()
}

// EDGE_TLS_CA_PATH used to be checked for existence at startup and then
// ignored, so an edge could only trust a control plane certificate from a
// public CA (or skip verification).
func TestClientTLSConfig_PrivateCA(t *testing.T) {
	ca := newPrivateCA(t)
	addr := serveTLS(t, ca.serverCert)
	caPath := filepath.Join(t.TempDir(), "ca.pem")
	require.NoError(t, os.WriteFile(caPath, ca.caPEM, 0o600))

	t.Run("CA and server name verify the control plane", func(t *testing.T) {
		cfg, err := clientTLSConfig(config.HubSpokeConfig{ClientTLSCAPath: caPath, ClientTLSServerName: "control.internal"})
		require.NoError(t, err)
		assert.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion)
		assert.NoError(t, handshake(addr, cfg))
	})

	t.Run("without the CA the certificate is not trusted", func(t *testing.T) {
		cfg, err := clientTLSConfig(config.HubSpokeConfig{ClientTLSServerName: "control.internal"})
		require.NoError(t, err)
		// The error's cause differs by platform verifier (macOS reports
		// "not trusted" rather than x509.UnknownAuthorityError).
		var verifyErr *tls.CertificateVerificationError
		assert.ErrorAs(t, handshake(addr, cfg), &verifyErr)
	})

	t.Run("the name is still checked", func(t *testing.T) {
		cfg, err := clientTLSConfig(config.HubSpokeConfig{ClientTLSCAPath: caPath})
		require.NoError(t, err)
		// Dialled by IP, and the certificate names control.internal only.
		var hostErr x509.HostnameError
		assert.ErrorAs(t, handshake(addr, cfg), &hostErr)
	})

	t.Run("skip verify still skips", func(t *testing.T) {
		cfg, err := clientTLSConfig(config.HubSpokeConfig{SkipTLSVerify: true})
		require.NoError(t, err)
		assert.NoError(t, handshake(addr, cfg))
	})
}

func TestClientTLSConfig_BadCAPath(t *testing.T) {
	_, err := clientTLSConfig(config.HubSpokeConfig{ClientTLSCAPath: filepath.Join(t.TempDir(), "missing.pem")})
	assert.ErrorContains(t, err, "EDGE_TLS_CA_PATH")

	notPEM := filepath.Join(t.TempDir(), "ca.pem")
	require.NoError(t, os.WriteFile(notPEM, []byte("not a certificate"), 0o600))
	_, err = clientTLSConfig(config.HubSpokeConfig{ClientTLSCAPath: notPEM})
	assert.ErrorContains(t, err, "contains no PEM certificates")
}
