package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const bearerPrefix = "Bearer "

// requireAdminToken guards the write API with the shared admin token that
// blog-manager sends as "Authorization: Bearer <token>".
//
// It fails closed: when no token is configured the write endpoints reject
// every request rather than staying open. Reads are unaffected, so a missing
// token degrades the site to read-only instead of exposing it.
func requireAdminToken(token string) gin.HandlerFunc {
	configured := token != ""
	// Compare digests rather than the raw strings so the comparison is both
	// constant-time and independent of token length.
	want := sha256.Sum256([]byte(token))

	return func(c *gin.Context) {
		if !configured {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, Response{
				Code:    "WRITE_DISABLED",
				Message: "write API disabled: no admin token configured",
			})
			return
		}

		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, bearerPrefix) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, Response{
				Code:    "UNAUTHORIZED",
				Message: "missing bearer token",
			})
			return
		}

		got := sha256.Sum256([]byte(strings.TrimPrefix(header, bearerPrefix)))
		if subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, Response{
				Code:    "UNAUTHORIZED",
				Message: "invalid admin token",
			})
			return
		}

		c.Next()
	}
}
