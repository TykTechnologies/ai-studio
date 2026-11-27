package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/services/compliance"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pagingCompliance records the limit and offset the handler asks for and
// returns that many of total events, as the Enterprise service does in SQL.
type pagingCompliance struct {
	compliance.Service
	total         int64
	limit, offset int
}

func (p *pagingCompliance) GetComplianceEvents(_, _ time.Time, _ *uint, _ *string, _ *string, limit, offset int) (*compliance.ComplianceEventsData, error) {
	p.limit, p.offset = limit, offset
	out := &compliance.ComplianceEventsData{Events: []compliance.ComplianceEventRecord{}, TotalCount: p.total}
	for i := offset; i < offset+limit && int64(i) < p.total; i++ {
		out.Events = append(out.Events, compliance.ComplianceEventRecord{ID: uint(i + 1)})
	}
	return out, nil
}

// o4a.9: page_size=5 returned every event; the endpoint now pages like the
// other list endpoints (page/page_size, meta) and keeps limit/offset.
func TestComplianceEventsPaging(t *testing.T) {
	gin.SetMode(gin.TestMode)
	prev := complianceService
	t.Cleanup(func() { complianceService = prev })
	fake := &pagingCompliance{total: 90}
	complianceService = fake

	api := &API{}
	r := gin.New()
	r.GET("/compliance/events", api.getComplianceEvents)
	get := func(q string) (*httptest.ResponseRecorder, compliance.ComplianceEventsData) {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/compliance/events"+q, nil))
		var body compliance.ComplianceEventsData
		if w.Code == http.StatusOK {
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		}
		return w, body
	}

	for _, tc := range []struct {
		q                   string
		limit, offset, page int
		events, totalPages  int
	}{
		{"?page_size=5", 5, 0, 1, 5, 18},
		{"?page_size=5&page=3", 5, 10, 3, 5, 18},
		{"?page=2", 100, 100, 2, 0, 1},
		{"?page_size=5000", 1000, 0, 1, 90, 1},
		{"", 100, 0, 1, 90, 1},
		{"?limit=20&offset=40", 20, 40, 3, 20, 5},
		{"?limit=0&offset=-1", 100, 0, 1, 90, 1},
		{"?page_size=10&limit=3&offset=7", 10, 0, 1, 10, 9},
	} {
		w, body := get(tc.q)
		require.Equal(t, http.StatusOK, w.Code, tc.q)
		assert.Equal(t, tc.limit, fake.limit, "limit for %q", tc.q)
		assert.Equal(t, tc.offset, fake.offset, "offset for %q", tc.q)
		assert.Len(t, body.Events, tc.events, tc.q)
		require.NotNil(t, body.Meta, tc.q)
		assert.Equal(t, compliance.PageMeta{TotalCount: 90, TotalPages: tc.totalPages, PageSize: tc.limit, PageNumber: tc.page, Offset: tc.offset}, *body.Meta, tc.q)
		assert.Equal(t, int64(90), body.TotalCount, tc.q)
	}

	for _, q := range []string{"?page_size=0", "?page_size=x", "?page=0", "?page=-2"} {
		w, _ := get(q)
		assert.Equal(t, http.StatusBadRequest, w.Code, q)
	}
}
