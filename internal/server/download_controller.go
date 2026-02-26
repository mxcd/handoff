package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// downloadHandler returns the file download handler.
// GET /api/v1/downloads/:download_id
//
// Returns:
//   - 200 with binary file data and correct Content-Type header
//   - 404 when the file has expired or does not exist
func (s *Server) downloadHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		downloadID := c.Param("download_id")

		storedFile, err := s.Store.GetFile(downloadID)
		if err != nil {
			log.Error().Err(err).Str("download_id", downloadID).Msg("download: failed to retrieve file")
			jsonError(c, http.StatusInternalServerError, "internal error")
			return
		}
		if storedFile == nil {
			jsonError(c, http.StatusNotFound, "file not found or expired")
			return
		}

		c.Header("Content-Disposition", "attachment")
		c.Data(http.StatusOK, storedFile.ContentType, storedFile.Data)
	}
}
