package data_session

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/netguard"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/postgres"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/vectorstores/pgvector"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
	pineconeSDK "github.com/pinecone-io/go-pinecone/pinecone"
	redis "github.com/redis/go-redis/v9"
	weaviateSDK "github.com/weaviate/weaviate-go-client/v5/weaviate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/resolver"
)

// Every connection to a datasource's vector store goes through netguard
// (w4e, 5hw): internal addresses are refused, as for any destination taken
// from stored configuration (a datasource can be a community submission),
// unless an administrator switched "Allow internal host" on for the
// datasource, which exempts its own host only.

// exemption is the datasource's exemption for the host of its connection
// string: a URL, or a Postgres DSN (URL or key=value form).
func exemption(d *models.Datasource) netguard.Exemption {
	if !d.AllowInternalHost {
		return netguard.Exemption{}
	}
	if s := d.DBConnString; !strings.Contains(s, "://") || isPostgresURL(s) {
		if cfg, err := pgconn.ParseConfig(s); err == nil {
			hosts := []string{cfg.Host}
			for _, fb := range cfg.Fallbacks {
				hosts = append(hosts, fb.Host)
			}
			return netguard.ExemptionFor(true, hosts...)
		}
	}
	return netguard.ExemptionFor(true, d.DBConnString)
}

func isPostgresURL(s string) bool {
	return strings.HasPrefix(s, "postgres://") || strings.HasPrefix(s, "postgresql://")
}

// httpClient is the client of an HTTP vector store (Chroma, Pinecone's REST
// API, Qdrant, Weaviate). No timeout of its own: calls carry their context.
func httpClient(d *models.Datasource) *http.Client {
	return exemption(d).HTTPClient(0)
}

// grpcDialOptions are the options of Pinecone's index (gRPC) connection:
// the datasource's dialer, and a resolver for the "dns" scheme the
// connection's target gets. gRPC resolves a host before dialling, so an
// exempt host goes through passthrough instead: the dial sees the host
// named in the datasource, which the exemption matches, and resolves it.
// Every other host resolves as before and its addresses get the dial-time
// check.
func grpcDialOptions(d *models.Datasource) []grpc.DialOption {
	ex := exemption(d)
	return []grpc.DialOption{
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			return ex.DialContext(ctx, "tcp", addr)
		}),
		grpc.WithResolvers(exemptResolver{ex}),
	}
}

// exemptResolver builds the "dns" resolver of one datasource's connection.
type exemptResolver struct{ ex netguard.Exemption }

func (exemptResolver) Scheme() string { return "dns" }

func (r exemptResolver) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	host, _, err := net.SplitHostPort(target.Endpoint())
	if err != nil {
		host = target.Endpoint()
	}
	scheme := "dns"
	if r.ex.Has(host) {
		scheme = "passthrough"
	}
	b := resolver.Get(scheme)
	if b == nil {
		return nil, fmt.Errorf("grpc resolver %q not registered", scheme)
	}
	return b.Build(target, cc, opts)
}

// pineconeClient is a Pinecone client for the datasource; index connections
// take grpcDialOptions(d).
func pineconeClient(d *models.Datasource) (*pineconeSDK.Client, error) {
	return pineconeSDK.NewClient(pineconeSDK.NewClientParams{ApiKey: d.DBConnAPIKey, RestClient: httpClient(d)})
}

// openPostgres opens the datasource's pgvector database through the
// datasource's dialer.
func openPostgres(d *models.Datasource) (*gorm.DB, error) {
	cfg, err := pgx.ParseConfig(d.DBConnString)
	if err != nil {
		return nil, err
	}
	cfg.DialFunc = exemption(d).DialContext
	cfg.LookupFunc = pgLookup(d)
	return gorm.Open(postgres.New(postgres.Config{Conn: stdlib.OpenDB(*cfg)}), &gorm.Config{})
}

// pgLookup is pgx's host resolver for the datasource. pgx resolves a host
// itself and dials the addresses, so an exempt host is returned unresolved:
// the dial then sees the host named in the connection string, which the
// exemption matches, and resolves it. Every other host resolves as pgx's
// default does, and its addresses get the dial-time check.
func pgLookup(d *models.Datasource) pgconn.LookupFunc {
	ex := exemption(d)
	return func(ctx context.Context, host string) ([]string, error) {
		if ex.Has(host) {
			return []string{host}, nil
		}
		return net.DefaultResolver.LookupHost(ctx, host)
	}
}

// pgvectorDial is the pgvector store's connection option: the datasource's
// dialer and host resolver, as openPostgres uses.
func pgvectorDial(d *models.Datasource) pgvector.Option {
	dial, lookup := pgvector.WithDialFunc(exemption(d).DialContext), pgvector.WithLookupFunc(pgLookup(d))
	return func(s *pgvector.Store) {
		dial(s)
		lookup(s)
	}
}

// redisDialer sets opt's dialer to the datasource's, applying opt's TLS
// configuration as go-redis's own dialer would.
func redisDialer(d *models.Datasource, opt *redis.Options) {
	ex := exemption(d)
	tlsConfig := opt.TLSConfig
	opt.Dialer = func(ctx context.Context, network, addr string) (net.Conn, error) {
		conn, err := ex.DialContext(ctx, network, addr)
		if err != nil || tlsConfig == nil {
			return conn, err
		}
		return tls.Client(conn, tlsConfig), nil
	}
}

// rueidisDial is the Redis vector store's dialer (rueidis DialFn).
func rueidisDial(d *models.Datasource) func(string, *net.Dialer, *tls.Config) (net.Conn, error) {
	ex := exemption(d)
	return func(addr string, dialer *net.Dialer, tlsConfig *tls.Config) (net.Conn, error) {
		ctx := context.Background()
		if dialer != nil && dialer.Timeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, dialer.Timeout)
			defer cancel()
		}
		conn, err := ex.DialContext(ctx, "tcp", addr)
		if err != nil || tlsConfig == nil {
			return conn, err
		}
		return tls.Client(conn, tlsConfig), nil
	}
}

// weaviateConfig is the Weaviate client configuration for the datasource:
// its HTTP client, with the API key as a header (the SDK takes either an
// auth config or a client, and an API key is only a header).
func weaviateConfig(d *models.Datasource, u *url.URL) weaviateSDK.Config {
	cfg := weaviateSDK.Config{Host: u.Host, Scheme: u.Scheme, ConnectionClient: httpClient(d)}
	if d.DBConnAPIKey != "" {
		cfg.Headers = map[string]string{"authorization": "Bearer " + d.DBConnAPIKey}
		// As the SDK does for an API key on Weaviate's own cloud.
		if h := strings.ToLower(u.Host); strings.Contains(h, "weaviate.io") || strings.Contains(h, "semi.technology") || strings.Contains(h, "weaviate.cloud") {
			cfg.Headers["X-Weaviate-Api-Key"] = d.DBConnAPIKey
		}
	}
	return cfg
}
