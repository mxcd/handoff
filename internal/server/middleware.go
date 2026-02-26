package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mxcd/go-config/config"
)

// apiKeyAuth returns a Gin middleware that validates the X-API-Key header
// against the configured API_KEYS list.
func apiKeyAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.GetHeader("X-API-Key")
		if key == "" {
			jsonError(c, http.StatusUnauthorized, "missing API key")
			c.Abort()
			return
		}

		validKeys := config.Get().StringArray("API_KEYS")
		for _, valid := range validKeys {
			if key == valid {
				c.Next()
				return
			}
		}

		jsonError(c, http.StatusUnauthorized, "invalid API key")
		c.Abort()
	}
}
