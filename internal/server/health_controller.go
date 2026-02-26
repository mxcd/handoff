package server

import (
	"github.com/gin-gonic/gin"
)

func (s *Server) registerHealthRoute() {
	s.Engine.GET(apiBasePath+"/health", s.getHealthHandler())
}

func (s *Server) getHealthHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "ok",
		})
	}
}
