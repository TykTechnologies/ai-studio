package data_session

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/netguard/netguardtest"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/vectorstores/pgvector"
	pineconeSDK "github.com/pinecone-io/go-pinecone/pinecone"
	redis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// w4e (5hw): the exemption covers the host of the datasource's own
// connection string, in each of its forms.
func TestExemptionHosts(t *testing.T) {
	for conn, host := range map[string]string{
		"postgres://u:p@pg.internal:5432/vec":          "pg.internal",
		"host=pg.internal port=5432 user=u dbname=vec": "pg.internal",
		"redis://cache.internal:6379/0":                "cache.internal",
		"http://chroma.internal:8000":                  "chroma.internal",
		"index-abc.svc.pinecone.io":                    "index-abc.svc.pinecone.io",
	} {
		ex := exemption(&models.Datasource{DBConnString: conn, AllowInternalHost: true})
		assert.Equal(t, []string{host}, ex.Hosts(), conn)
		assert.Empty(t, exemption(&models.Datasource{DBConnString: conn}).Hosts(), "switch off: %s", conn)
	}
}

// Every client kind refuses an internal address unless the datasource's
// switch is on: HTTP stores, Postgres (pgvector) and Redis.
func TestDatasourceClientsRefuseInternalUnlessAllowed(t *testing.T) {
	netguardtest.BlockInternal(t)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	t.Cleanup(api.Close)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	addr := ln.Addr().String()

	for _, allow := range []bool{false, true} {
		check := func(what string, err error) {
			if allow {
				if err != nil {
					assert.NotContains(t, err.Error(), "internal address", "%s with the switch on", what)
				}
			} else {
				require.Error(t, err, what)
				assert.Contains(t, err.Error(), "internal address", what)
			}
		}

		d := &models.Datasource{DBConnString: api.URL, AllowInternalHost: allow}
		resp, err := httpClient(d).Get(api.URL)
		if err == nil {
			resp.Body.Close()
		}
		check("HTTP", err)

		pg := &models.Datasource{DBConnString: "postgres://u:p@" + addr + "/vec?sslmode=disable&connect_timeout=2", AllowInternalHost: allow}
		_, err = openPostgresPing(pg)
		check("Postgres", err)

		rd := &models.Datasource{DBConnString: "redis://" + addr, AllowInternalHost: allow}
		opt, err := redis.ParseURL(rd.DBConnString)
		require.NoError(t, err)
		redisDialer(rd, opt)
		conn, err := opt.Dialer(context.Background(), "tcp", opt.Addr)
		if err == nil {
			conn.Close()
		}
		check("Redis", err)

		conn, err = rueidisDial(rd)(addr, &net.Dialer{Timeout: 2 * time.Second}, nil)
		if err == nil {
			conn.Close()
		}
		check("Redis vector store", err)
	}
}

// The switch is keyed by the host named in the connection string, and pgx
// and gRPC resolve a host themselves before dialling: a hostname that
// resolves to an internal address (pgvector at "pgvec" in a compose
// network) must connect with the switch on and be refused with it off,
// through openPostgres, the pgvector store and Pinecone's index connection.
// An internal host the switch does not name stays refused. Redis dials the
// hostname; checked too.
func TestHostnameExemption(t *testing.T) {
	netguardtest.BlockInternal(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	var accepted atomic.Int64
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			accepted.Add(1)
			_ = c.Close()
		}
	}()
	_, port, err := net.SplitHostPort(ln.Addr().String())
	require.NoError(t, err)

	// Each client fails either way (the listener closes every connection):
	// with the switch on it must have reached the listener, with it off it
	// must have been refused before dialling.
	check := func(t *testing.T, what string, allow bool, dial func() error) {
		before := accepted.Load()
		err := dial()
		if allow {
			if err != nil {
				assert.NotContains(t, err.Error(), "internal address", "%s with the switch on", what)
			}
			assert.Eventually(t, func() bool { return accepted.Load() > before }, 2*time.Second, 10*time.Millisecond, "%s with the switch on must connect", what)
		} else {
			require.Error(t, err, what)
			assert.Contains(t, err.Error(), "internal address", "%s with the switch off", what)
			assert.Equal(t, before, accepted.Load(), "%s with the switch off must not connect", what)
		}
	}

	pgConn := "postgres://u:p@localhost:" + port + "/vec?sslmode=disable&connect_timeout=2"
	for _, allow := range []bool{false, true} {
		pg := &models.Datasource{DBConnString: pgConn, AllowInternalHost: allow}
		check(t, "openPostgres", allow, func() error {
			_, err := openPostgresPing(pg)
			return err
		})
		check(t, "pgvector store", allow, func() error {
			_, err := pgvector.New(context.Background(),
				pgvector.WithConnectionURL(pgConn),
				pgvector.WithEmbedder(stubEmbedder{}),
				pgvectorDial(pg),
			)
			return err
		})

		pc := &models.Datasource{DBConnString: "http://localhost:" + port, DBConnAPIKey: "k", AllowInternalHost: allow}
		check(t, "Pinecone index", allow, func() error {
			client, err := pineconeClient(pc)
			require.NoError(t, err)
			idx, err := client.Index(pineconeSDK.NewIndexConnParams{Host: pc.DBConnString}, grpcDialOptions(pc)...)
			require.NoError(t, err)
			defer idx.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_, err = idx.DescribeIndexStats(ctx)
			return err
		})

		rd := &models.Datasource{DBConnString: "redis://localhost:" + port, AllowInternalHost: allow}
		opt, err := redis.ParseURL(rd.DBConnString)
		require.NoError(t, err)
		redisDialer(rd, opt)
		check(t, "Redis", allow, func() error {
			c, err := opt.Dialer(context.Background(), "tcp", opt.Addr)
			if err == nil {
				c.Close()
			}
			return err
		})
		check(t, "Redis vector store", allow, func() error {
			c, err := rueidisDial(rd)("localhost:"+port, &net.Dialer{Timeout: 2 * time.Second}, nil)
			if err == nil {
				c.Close()
			}
			return err
		})
	}

	// The switch on for another host leaves this one refused: pgx and gRPC
	// resolve it and their dials get the check.
	other := &models.Datasource{DBConnString: "postgres://u:p@pg.example:5432/vec", AllowInternalHost: true}
	ips, err := pgLookup(other)(context.Background(), "localhost")
	require.NoError(t, err)
	assert.NotContains(t, ips, "localhost", "a host the switch does not name is resolved")
	_, err = exemption(other).DialContext(context.Background(), "tcp", net.JoinHostPort(ips[0], port))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "internal address")
}

type stubEmbedder struct{}

func (stubEmbedder) EmbedDocuments(context.Context, []string) ([][]float32, error) { return nil, nil }
func (stubEmbedder) EmbedQuery(context.Context, string) ([]float32, error)         { return nil, nil }

// openPostgresPing opens the database and pings it, as the stores do.
func openPostgresPing(d *models.Datasource) (any, error) {
	db, err := openPostgres(d)
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	defer sqlDB.Close()
	return nil, sqlDB.Ping()
}
