package web

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

//go:embed all:html
var webRoot embed.FS

// RegisterStaticFiles wires the embedded static assets into the gin engine at /static.
func RegisterStaticFiles(engine *gin.Engine) {
	sub, err := fs.Sub(webRoot, "html")
	if err != nil {
		log.Fatal().Err(err).Msg("failed to get html sub-filesystem")
	}
	engine.StaticFS("/static", http.FS(sub))
	log.Info().Msg("serving embedded static files at /static")
}
