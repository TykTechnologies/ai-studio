package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func dateRangeContext(query string) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/analytics/tool-usage-statistics?"+query, nil)
	return c
}

func TestGetDateRange(t *testing.T) {
	t.Run("valid range", func(t *testing.T) {
		start, end, err := getDateRange(dateRangeContext("start_date=2026-08-01&end_date=2026-08-28"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if start.Format("2006-01-02") != "2026-08-01" {
			t.Errorf("unexpected start: %v", start)
		}
		// The end date is widened to the end of the day.
		if end.Format("2006-01-02 15:04:05") != "2026-08-28 23:59:59" {
			t.Errorf("unexpected end: %v", end)
		}
	})

	// A missing parameter used to surface time.Parse's raw Go format string:
	// `parsing time "" as "2006-01-02": cannot parse "" as "2006"`, which names
	// neither the parameter nor what it wanted.
	t.Run("missing start_date names the parameter", func(t *testing.T) {
		_, _, err := getDateRange(dateRangeContext("end_date=2026-08-28"))
		if err == nil {
			t.Fatal("expected an error")
		}
		assertMentions(t, err.Error(), "start_date", "YYYY-MM-DD")
		assertOmits(t, err.Error(), "2006")
	})

	t.Run("missing end_date names the parameter", func(t *testing.T) {
		_, _, err := getDateRange(dateRangeContext("start_date=2026-08-01"))
		if err == nil {
			t.Fatal("expected an error")
		}
		assertMentions(t, err.Error(), "end_date", "YYYY-MM-DD")
	})

	t.Run("both missing", func(t *testing.T) {
		_, _, err := getDateRange(dateRangeContext(""))
		if err == nil {
			t.Fatal("expected an error")
		}
		assertMentions(t, err.Error(), "start_date")
	})

	t.Run("malformed date echoes what was sent", func(t *testing.T) {
		_, _, err := getDateRange(dateRangeContext("start_date=28-08-2026&end_date=2026-08-28"))
		if err == nil {
			t.Fatal("expected an error")
		}
		assertMentions(t, err.Error(), "start_date", "28-08-2026")
		assertOmits(t, err.Error(), "2006")
	})
}

func assertMentions(t *testing.T, got string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Errorf("error %q should mention %q", got, want)
		}
	}
}

func assertOmits(t *testing.T, got string, unwanted ...string) {
	t.Helper()
	for _, bad := range unwanted {
		if strings.Contains(got, bad) {
			t.Errorf("error %q should not leak the Go layout string %q", got, bad)
		}
	}
}
