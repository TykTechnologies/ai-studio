package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Privacy scores are a 0-100 scale everywhere they appear (LLMs, tools,
// data sources, submissions), and the access rule "a tool's score must not
// exceed the LLM's" only means anything if every score is on that scale.
// The submission service already validates its suggested score; the admin
// create and update routes of the three object types call this so a form
// or API client cannot store 250 or -1.

const (
	minPrivacyScore = 0
	maxPrivacyScore = 100
)

const privacyScoreRangeDetail = "privacy_score must be between 0 and 100"

// validatePrivacyScore returns false, having written a 400, when score is
// outside 0-100. A non-integer value never reaches it: JSON binding into
// the int attribute already rejects "abc" or 12.5 with a 400.
func validatePrivacyScore(c *gin.Context, score int) bool {
	if score >= minPrivacyScore && score <= maxPrivacyScore {
		return true
	}
	c.JSON(http.StatusBadRequest, ErrorResponse{
		Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Bad Request", Detail: privacyScoreRangeDetail}},
	})
	return false
}
