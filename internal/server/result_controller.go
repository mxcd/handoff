package server

import (
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mxcd/handoff/internal/model"
	"github.com/mxcd/handoff/internal/util"
	"github.com/rs/zerolog/log"
)

// getResultHandler returns the result polling handler.
// GET /api/v1/sessions/:id/result
//
// Returns:
//   - 200 with result items when session is completed
//   - 202 with current status when session is pending/opened/action_started
//   - 404 when session does not exist
//   - 410 when session has expired
func (s *Server) getResultHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")

		session, err := s.Store.GetSession(id)
		if err != nil {
			log.Error().Err(err).Str("session_id", id).Msg("result: failed to get session")
			jsonError(c, http.StatusInternalServerError, "internal error")
			return
		}
		if session == nil {
			jsonError(c, http.StatusNotFound, "session not found")
			return
		}
		if session.Status == model.SessionStatusExpired {
			jsonError(c, http.StatusGone, "session expired")
			return
		}
		if session.Status == model.SessionStatusCompleted {
			resp := gin.H{
				"status":       "completed",
				"completed_at": session.CompletedAt,
				"items":        session.Result,
			}
			if session.ActionType == model.ActionTypeScan && session.ScanResult != nil {
				resp["scan_result"] = session.ScanResult
			}
			c.JSON(http.StatusOK, resp)
			return
		}

		// pending / opened / action_started
		c.JSON(http.StatusAccepted, gin.H{
			"status": string(session.Status),
		})
	}
}

type submitResultItem struct {
	ContentType string `json:"content_type" binding:"required"`
	Filename    string `json:"filename" binding:"required"`
	Data        string `json:"data" binding:"required"` // base64 encoded
}

type submitResultRequest struct {
	Items []submitResultItem `json:"items" binding:"required"`
}

// submitResultHandler returns the result submission handler used by the phone UI.
// POST /s/:id/result  (public — no API key required)
//
// Returns:
//   - 200 with result items on success
//   - 400 on invalid request body or bad base64 data
//   - 404 when session does not exist
//   - 409 when session is already completed or has not yet been opened
//   - 410 when session has expired
func (s *Server) submitResultHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")

		session, err := s.Store.GetSession(id)
		if err != nil {
			log.Error().Err(err).Str("session_id", id).Msg("submit: failed to get session")
			jsonError(c, http.StatusInternalServerError, "internal error")
			return
		}
		if session == nil {
			jsonError(c, http.StatusNotFound, "session not found")
			return
		}
		if session.Status == model.SessionStatusExpired {
			jsonError(c, http.StatusGone, "session expired")
			return
		}
		if session.Status == model.SessionStatusCompleted {
			jsonError(c, http.StatusConflict, "session already completed")
			return
		}
		if !session.Opened {
			jsonError(c, http.StatusConflict, "session not yet opened")
			return
		}

		var req submitResultRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			jsonError(c, http.StatusBadRequest, "invalid request body")
			return
		}

		resultItems := make([]model.ResultItem, 0, len(req.Items))
		for _, item := range req.Items {
			downloadID := model.NewSessionID()

			decoded, err := base64.StdEncoding.DecodeString(item.Data)
			if err != nil {
				// Try URL-safe base64 as a fallback.
				decoded, err = base64.URLEncoding.DecodeString(item.Data)
				if err != nil {
					jsonError(c, http.StatusBadRequest, "invalid base64 data")
					return
				}
			}

			fileData := decoded
			fileContentType := item.ContentType
			fileFilename := item.Filename

			// Convert to PDF if the session's output format requires it
			if session.OutputFormat == model.OutputFormatPDF {
				if strings.HasPrefix(item.ContentType, "image/svg") {
					pdfBytes, pdfErr := util.SVGToPDF(decoded)
					if pdfErr != nil {
						log.Error().Err(pdfErr).Str("session_id", id).Msg("submit: SVG to PDF conversion failed")
						jsonError(c, http.StatusInternalServerError, "PDF conversion failed")
						return
					}
					fileData = pdfBytes
					fileContentType = "application/pdf"
					fileFilename = strings.TrimSuffix(item.Filename, ".svg") + ".pdf"
				} else if strings.HasPrefix(item.ContentType, "image/") {
					pdfBytes, pdfErr := util.ImageToPDF(decoded, item.ContentType)
					if pdfErr != nil {
						log.Error().Err(pdfErr).Str("session_id", id).Msg("submit: image to PDF conversion failed")
						jsonError(c, http.StatusInternalServerError, "PDF conversion failed")
						return
					}
					fileData = pdfBytes
					fileContentType = "application/pdf"
					// Replace image extension with .pdf
					if idx := strings.LastIndex(item.Filename, "."); idx >= 0 {
						fileFilename = item.Filename[:idx] + ".pdf"
					} else {
						fileFilename = item.Filename + ".pdf"
					}
				}
			}

			if storeErr := s.Store.StoreFile(downloadID, fileData, fileContentType, session.ResultTTL); storeErr != nil {
				log.Error().Err(storeErr).Str("session_id", id).Str("download_id", downloadID).Msg("submit: failed to store file")
				jsonError(c, http.StatusInternalServerError, "internal error")
				return
			}

			resultItems = append(resultItems, model.ResultItem{
				DownloadID:  downloadID,
				ContentType: fileContentType,
				Filename:    fileFilename,
			})
		}

		if err := s.Store.MarkSessionCompleted(id, resultItems); err != nil {
			log.Error().Err(err).Str("session_id", id).Msg("submit: failed to mark session completed")
			jsonError(c, http.StatusInternalServerError, "internal error")
			return
		}

		// Notify all WebSocket subscribers that the session is complete.
		s.Hub.BroadcastCompletion(id, string(model.SessionStatusCompleted), resultItems)

		log.Info().Str("session_id", id).Int("items", len(resultItems)).Msg("submit: session completed")
		c.JSON(http.StatusOK, gin.H{"items": resultItems})
	}
}
