package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mxcd/handoff/internal/store"
	"github.com/mxcd/handoff/internal/web"
	"github.com/mxcd/handoff/internal/ws"
	"github.com/rs/zerolog/log"
)

const apiBasePath = "/api/v1"

type ServerOptions struct {
	DevMode bool
	Port    int
	Store   *store.Store
}

type Server struct {
	Options      *ServerOptions
	Engine       *gin.Engine
	HttpServer   *http.Server
	ProtectedAPI *gin.RouterGroup
	Store        *store.Store
	Hub          *ws.Hub
}

func NewServer(options *ServerOptions) (*Server, error) {
	if options == nil {
		return nil, fmt.Errorf("server options cannot be nil")
	}
	if options.Store == nil {
		return nil, fmt.Errorf("server options Store cannot be nil")
	}

	server := &Server{
		Options: options,
		Store:   options.Store,
		Hub:     ws.NewHub(),
	}

	if !server.Options.DevMode {
		log.Info().Msg("Running Gin in production mode")
		gin.SetMode(gin.ReleaseMode)
	} else {
		log.Info().Msg("Running Gin in development mode")
	}

	engine := gin.New()
	server.Engine = engine
	server.Engine.Use(gin.Recovery())

	server.HttpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", options.Port),
		Handler: engine,
	}

	return server, nil
}

func (s *Server) RegisterRoutes() error {
	// Public routes — no authentication required
	s.Engine.GET(apiBasePath+"/health", s.getHealthHandler())
	s.Engine.GET(apiBasePath+"/version", s.getVersionHandler())

	// Protected API group — all routes here require a valid X-API-Key header
	protected := s.Engine.Group(apiBasePath)
	protected.Use(apiKeyAuth())
	s.ProtectedAPI = protected

	// Session management routes (protected)
	s.ProtectedAPI.POST("/sessions", s.createSessionHandler())
	s.ProtectedAPI.GET("/sessions/:id", s.getSessionHandler())

	// Result polling and download routes (protected — caller uses API key)
	s.ProtectedAPI.GET("/sessions/:id/result", s.getResultHandler())
	s.ProtectedAPI.GET("/downloads/:download_id", s.downloadHandler())

	// WebSocket endpoint for real-time session updates (auth handled in handler)
	s.Engine.GET(apiBasePath+"/sessions/:id/ws", s.wsHandler())

	// Phone-facing session pages (public — session UUID is the auth)
	s.Engine.GET("/s/:id", s.sessionPageHandler())
	s.Engine.GET("/s/:id/action", s.sessionActionHandler())

	// Session action routes (public — phone UI submits results via session UUID)
	s.Engine.POST("/s/:id/result", s.submitResultHandler())

	// Scan session routes (public — session UUID is the auth)
	s.Engine.POST("/s/:id/scan/upload", s.scanUploadHandler())
	s.Engine.POST("/s/:id/scan/finalize", s.scanFinalizeHandler())

	// Static files are public (no middleware)
	web.RegisterStaticFiles(s.Engine)
	return nil
}

func (s *Server) Run() error {
	if err := s.HttpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) {
	s.HttpServer.Shutdown(ctx)
}
