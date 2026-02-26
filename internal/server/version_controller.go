package server

import (
	"github.com/gin-gonic/gin"
	"github.com/mxcd/handoff/internal/util"
)

func (s *Server) registerVersionRoute() {
	s.Engine.GET(apiBasePath+"/version", s.getVersionHandler())
}

func (s *Server) getVersionHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(200, gin.H{
			"version": util.Version,
			"commit":  util.Commit,
		})
	}
}
