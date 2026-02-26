package web

import (
	"embed"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

//go:embed all:html
var webRoot embed.FS

//go:embed all:templates
var templateFS embed.FS

var baseTemplateContent string

func init() {
	b, err := templateFS.ReadFile("templates/base.html")
	if err != nil {
		panic("failed to read base.html: " + err.Error())
	}
	baseTemplateContent = string(b)
}

// RegisterStaticFiles wires the embedded static assets into the gin engine at /static.
func RegisterStaticFiles(engine *gin.Engine) {
	sub, err := fs.Sub(webRoot, "html")
	if err != nil {
		log.Fatal().Err(err).Msg("failed to get html sub-filesystem")
	}
	engine.StaticFS("/static", http.FS(sub))
	log.Info().Msg("serving embedded static files at /static")
}

// RenderPage renders a page template into the base layout and writes to w.
// pageName is the template filename without path (e.g., "expired.html").
// data is passed to the template.
func RenderPage(w io.Writer, pageName string, data interface{}) error {
	base, err := template.New("base.html").Parse(baseTemplateContent)
	if err != nil {
		return fmt.Errorf("parse base template: %w", err)
	}
	pageContent, err := templateFS.ReadFile("templates/" + pageName)
	if err != nil {
		return fmt.Errorf("read page template %q: %w", pageName, err)
	}
	_, err = base.Parse(string(pageContent))
	if err != nil {
		return fmt.Errorf("parse page template %q: %w", pageName, err)
	}
	return base.Execute(w, data)
}
