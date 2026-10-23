package data_session

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/netguard/netguardtest"
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
