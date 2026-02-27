package server

import (
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mxcd/go-config/config"
	"github.com/mxcd/handoff/internal/model"
	"github.com/mxcd/handoff/internal/store"
	"github.com/mxcd/handoff/internal/util"
	"github.com/rs/zerolog/log"
)

// scanUploadHandler accepts a single page upload for a scan session.
// POST /s/:id/scan/upload (public — session UUID is the auth)
//
// Expects multipart/form-data with:
//   - file: the image file
//   - document_index: integer (optional, defaults 0; forced 0 for single-document sessions)
//   - page_index: integer (optional, defaults 0)
func (s *Server) scanUploadHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")

		session, err := s.Store.GetSession(id)
		if err != nil {
			log.Error().Err(err).Str("session_id", id).Msg("scan_upload: failed to get session")
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
		if session.ActionType != model.ActionTypeScan {
			jsonError(c, http.StatusBadRequest, "session is not a scan session")
			return
		}

		// Enforce body size limit before parsing multipart.
		maxBytes := int64(config.Get().Int("SCAN_UPLOAD_MAX_BYTES"))
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)

		if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
			if strings.Contains(err.Error(), "request body too large") ||
				strings.Contains(err.Error(), "http: request body too large") {
				jsonError(c, http.StatusRequestEntityTooLarge, "request body too large")
				return
			}
			jsonError(c, http.StatusBadRequest, "failed to parse multipart form: "+err.Error())
			return
		}

		file, fileHeader, err := c.Request.FormFile("file")
		if err != nil {
			jsonError(c, http.StatusBadRequest, "missing file field")
			return
		}
		defer file.Close()

		// Parse document_index and page_index; default to 0 on missing/invalid.
		documentIndex := 0
		if v := c.Request.FormValue("document_index"); v != "" {
			if n, parseErr := strconv.Atoi(v); parseErr == nil && n >= 0 {
				documentIndex = n
			}
		}

		pageIndex := 0
		if v := c.Request.FormValue("page_index"); v != "" {
			if n, parseErr := strconv.Atoi(v); parseErr == nil && n >= 0 {
				pageIndex = n
			}
		}

		// Single-document mode forces document_index to 0.
		if session.ScanDocumentMode == model.ScanDocumentModeSingle {
			documentIndex = 0
		}

		// Check page count limit before reading data.
		maxPages := config.Get().Int("SCAN_MAX_PAGES")
		currentCount := s.Store.GetScanPageCount(id)
		if currentCount >= maxPages {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "page limit exceeded",
				"limit":   maxPages,
				"current": currentCount,
			})
			return
		}

		data, err := io.ReadAll(file)
		if err != nil {
			log.Error().Err(err).Str("session_id", id).Msg("scan_upload: failed to read file data")
			jsonError(c, http.StatusInternalServerError, "failed to read file data")
			return
		}

		contentType := fileHeader.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "application/octet-stream"
		}

		// Calculate remaining session TTL for the scan page cache entry.
		remainingTTL := time.Until(session.CreatedAt.Add(session.SessionTTL))

		if err := s.Store.AddScanPage(id, store.ScanPageData{
			DocumentIndex: documentIndex,
			PageIndex:     pageIndex,
			Data:          data,
			ContentType:   contentType,
		}, remainingTTL); err != nil {
			log.Error().Err(err).Str("session_id", id).Msg("scan_upload: failed to store scan page")
			jsonError(c, http.StatusInternalServerError, "failed to store page")
			return
		}

		log.Info().
			Str("session_id", id).
			Int("document_index", documentIndex).
			Int("page_index", pageIndex).
			Int("bytes", len(data)).
			Msg("scan_upload: page accepted")

		c.JSON(http.StatusOK, gin.H{
			"status":         "page accepted",
			"document_index": documentIndex,
			"page_index":     pageIndex,
		})
	}
}

// scanFinalizeHandler assembles all uploaded pages into the final scan result.
// POST /s/:id/scan/finalize (public — session UUID is the auth)
//
// For pdf output_format: each document group is assembled into a multi-page PDF.
// For images output_format: each page is stored individually with its original content type.
func (s *Server) scanFinalizeHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")

		session, err := s.Store.GetSession(id)
		if err != nil {
			log.Error().Err(err).Str("session_id", id).Msg("scan_finalize: failed to get session")
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
		if session.ActionType != model.ActionTypeScan {
			jsonError(c, http.StatusBadRequest, "session is not a scan session")
			return
		}

		pages, err := s.Store.GetScanPages(id)
		if err != nil {
			log.Error().Err(err).Str("session_id", id).Msg("scan_finalize: failed to get scan pages")
			jsonError(c, http.StatusInternalServerError, "internal error")
			return
		}
		if len(pages) == 0 {
			jsonError(c, http.StatusBadRequest, "no pages uploaded")
			return
		}

		// Group pages by DocumentIndex.
		docMap := make(map[int][]store.ScanPageData)
		for _, p := range pages {
			docMap[p.DocumentIndex] = append(docMap[p.DocumentIndex], p)
		}

		// Sort document indexes.
		docIndexes := make([]int, 0, len(docMap))
		for idx := range docMap {
			docIndexes = append(docIndexes, idx)
		}
		sort.Ints(docIndexes)

		scanResult := model.ScanResult{
			Documents: make([]model.ScanDocument, 0, len(docIndexes)),
		}

		for _, docIdx := range docIndexes {
			docPages := docMap[docIdx]
			// Sort pages within each document by PageIndex.
			sort.Slice(docPages, func(i, j int) bool {
				return docPages[i].PageIndex < docPages[j].PageIndex
			})

			if session.ScanOutputFormat == model.ScanOutputFormatPDF {
				// Assemble all pages of this document into a single PDF.
				pageData := make([][]byte, len(docPages))
				pageContentTypes := make([]string, len(docPages))
				for i, p := range docPages {
					pageData[i] = p.Data
					pageContentTypes[i] = p.ContentType
				}

				pdfBytes, err := util.ImagesToPDF(pageData, pageContentTypes)
				if err != nil {
					log.Error().Err(err).
						Str("session_id", id).
						Int("document_index", docIdx).
						Msg("scan_finalize: PDF assembly failed")
					jsonError(c, http.StatusInternalServerError, "PDF assembly failed")
					return
				}

				dlID := model.NewSessionID()
				if err := s.Store.StoreFile(dlID, pdfBytes, "application/pdf", session.ResultTTL); err != nil {
					log.Error().Err(err).Str("session_id", id).Msg("scan_finalize: failed to store PDF")
					jsonError(c, http.StatusInternalServerError, "failed to store PDF")
					return
				}

				scanResult.Documents = append(scanResult.Documents, model.ScanDocument{
					PDFURL: "/api/v1/downloads/" + dlID,
				})
			} else {
				// Store each page image individually.
				docPageResults := make([]model.ScanPage, 0, len(docPages))
				for _, p := range docPages {
					dlID := model.NewSessionID()
					if err := s.Store.StoreFile(dlID, p.Data, p.ContentType, session.ResultTTL); err != nil {
						log.Error().Err(err).Str("session_id", id).Msg("scan_finalize: failed to store page image")
						jsonError(c, http.StatusInternalServerError, "failed to store page image")
						return
					}
					docPageResults = append(docPageResults, model.ScanPage{
						URL:         "/api/v1/downloads/" + dlID,
						ContentType: p.ContentType,
					})
				}
				scanResult.Documents = append(scanResult.Documents, model.ScanDocument{
					Pages: docPageResults,
				})
			}
		}

		if err := s.Store.MarkScanSessionCompleted(id, &scanResult); err != nil {
			log.Error().Err(err).Str("session_id", id).Msg("scan_finalize: failed to mark session completed")
			jsonError(c, http.StatusInternalServerError, "internal error")
			return
		}

		// Notify WebSocket subscribers that the session is complete.
		s.Hub.BroadcastCompletion(id, string(model.SessionStatusCompleted), scanResult)

		// Clear raw page data from the store — no longer needed after finalization.
		s.Store.ClearScanPages(id)

		log.Info().
			Str("session_id", id).
			Int("documents", len(scanResult.Documents)).
			Msg("scan_finalize: session completed")

		c.JSON(http.StatusOK, gin.H{"status": "completed"})
	}
}
