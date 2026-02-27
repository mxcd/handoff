package store

import (
	"fmt"
	"time"

	"github.com/mxcd/handoff/internal/model"
	cache "github.com/patrickmn/go-cache"
	"github.com/rs/zerolog/log"
)

const (
	tombstoneTTL    = 24 * time.Hour
	sessionKeyFmt   = "session:%s"
	tombstoneKeyFmt = "tombstone:%s"
	fileKeyFmt      = "file:%s"
	scanPagesKeyFmt = "scanpages:%s"
	defaultExpiry   = cache.NoExpiration
	cleanupInterval = time.Minute
)

// Store is an in-memory session and file store backed by patrickmn/go-cache.
// Sessions expire according to their SessionTTL. Tombstones linger for 24 hours
// after session expiry so callers can distinguish "expired" from "never existed".
// Result files expire independently according to the session's ResultTTL.
// Scan pages accumulate until finalization or expiry.
type Store struct {
	sessions  *cache.Cache // keyed by "session:{id}"
	files     *cache.Cache // keyed by "file:{downloadID}"
	scanPages *cache.Cache // keyed by "scanpages:{sessionID}", stores []ScanPageData
}

// ScanPageData holds raw uploaded page data before finalization.
type ScanPageData struct {
	DocumentIndex int
	PageIndex     int
	Data          []byte
	ContentType   string
}

// NewStore creates a new Store with separate caches for sessions, files, and scan pages.
// The session cache retains tombstone entries for up to 24 hours.
// The file cache uses a 5-minute default expiry.
func NewStore() *Store {
	return &Store{
		// 24-hour default expiry covers the tombstone lifetime; cleanup runs every minute.
		sessions: cache.New(tombstoneTTL, cleanupInterval),
		// 5-minute default expiry for files; cleanup every minute.
		files: cache.New(5*time.Minute, cleanupInterval),
		// scan pages live as long as the session; cleanup every minute.
		scanPages: cache.New(tombstoneTTL, cleanupInterval),
	}
}

// sessionKey returns the cache key for the given session ID.
func sessionKey(id string) string {
	return fmt.Sprintf(sessionKeyFmt, id)
}

// tombstoneKey returns the cache key for the expired-session tombstone of id.
func tombstoneKey(id string) string {
	return fmt.Sprintf(tombstoneKeyFmt, id)
}

// fileKey returns the cache key for the given download ID.
func fileKey(downloadID string) string {
	return fmt.Sprintf(fileKeyFmt, downloadID)
}

// scanPagesKey returns the cache key for the accumulated scan pages of a session.
func scanPagesKey(sessionID string) string {
	return fmt.Sprintf(scanPagesKeyFmt, sessionID)
}

// CreateSession stores a new session in the cache with its SessionTTL.
// It also stores a tombstone entry that persists for 24 hours so expired sessions
// can be distinguished from sessions that never existed.
func (s *Store) CreateSession(session *model.Session) error {
	key := sessionKey(session.ID)
	tombstone := tombstoneKey(session.ID)

	log.Debug().Str("session_id", session.ID).Dur("ttl", session.SessionTTL).Msg("store: creating session")

	// Store the live session entry with the session's own TTL.
	s.sessions.Set(key, session, session.SessionTTL)

	// Pre-store a tombstone that outlives the session.
	// The tombstone is a minimal Session snapshot indicating expiry.
	expired := &model.Session{
		ID:     session.ID,
		Status: model.SessionStatusExpired,
	}
	s.sessions.Set(tombstone, expired, tombstoneTTL)

	return nil
}

// GetSession retrieves a session by ID.
//
// Return semantics:
//   - (session, nil)  — live session found
//   - (expiredSession, nil)  — session expired but tombstone present; Status == expired
//   - (nil, nil)  — never existed or tombstone also gone; caller should 404
func (s *Store) GetSession(id string) (*model.Session, error) {
	// Try the live session entry first.
	if v, found := s.sessions.Get(sessionKey(id)); found {
		sess := v.(*model.Session)
		log.Debug().Str("session_id", id).Str("status", string(sess.Status)).Msg("store: session found")
		return sess, nil
	}

	// Not found — check for a tombstone (expired session).
	if v, found := s.sessions.Get(tombstoneKey(id)); found {
		expired := v.(*model.Session)
		log.Debug().Str("session_id", id).Msg("store: tombstone found — session expired")
		return expired, nil
	}

	log.Debug().Str("session_id", id).Msg("store: session not found")
	return nil, nil
}

// UpdateSession replaces a session in the cache, preserving a proportional TTL
// calculated from CreatedAt + SessionTTL - now.
func (s *Store) UpdateSession(session *model.Session) error {
	remaining := time.Until(session.CreatedAt.Add(session.SessionTTL))
	if remaining <= 0 {
		// Session has already expired; nothing to update.
		log.Debug().Str("session_id", session.ID).Msg("store: update skipped — session already expired")
		return nil
	}

	log.Debug().Str("session_id", session.ID).Dur("remaining_ttl", remaining).Msg("store: updating session")
	s.sessions.Set(sessionKey(session.ID), session, remaining)
	return nil
}

// DeleteSession removes a session (and its tombstone) from the store.
func (s *Store) DeleteSession(id string) error {
	log.Debug().Str("session_id", id).Msg("store: deleting session")
	s.sessions.Delete(sessionKey(id))
	s.sessions.Delete(tombstoneKey(id))
	return nil
}

// MarkSessionOpened sets the session status to "opened" and marks the Opened flag.
func (s *Store) MarkSessionOpened(id string) error {
	sess, err := s.GetSession(id)
	if err != nil {
		return err
	}
	if sess == nil {
		return fmt.Errorf("session %q not found", id)
	}

	sess.Opened = true
	sess.Status = model.SessionStatusOpened

	log.Debug().Str("session_id", id).Msg("store: marking session opened")
	return s.UpdateSession(sess)
}

// MarkSessionCompleted sets the session status to "completed", records the
// completion time and result items, and stores each result file with the
// session's ResultTTL.
func (s *Store) MarkSessionCompleted(id string, result []model.ResultItem) error {
	sess, err := s.GetSession(id)
	if err != nil {
		return err
	}
	if sess == nil {
		return fmt.Errorf("session %q not found", id)
	}

	now := time.Now()
	sess.Status = model.SessionStatusCompleted
	sess.CompletedAt = &now
	sess.Result = result

	log.Debug().Str("session_id", id).Int("result_items", len(result)).Msg("store: marking session completed")
	return s.UpdateSession(sess)
}

// MarkScanSessionCompleted sets the session status to "completed" with a scan result.
func (s *Store) MarkScanSessionCompleted(id string, scanResult *model.ScanResult) error {
	sess, err := s.GetSession(id)
	if err != nil {
		return err
	}
	if sess == nil {
		return fmt.Errorf("session %q not found", id)
	}

	now := time.Now()
	sess.Status = model.SessionStatusCompleted
	sess.CompletedAt = &now
	sess.ScanResult = scanResult

	log.Debug().Str("session_id", id).Int("documents", len(scanResult.Documents)).Msg("store: marking scan session completed")
	return s.UpdateSession(sess)
}

// StoredFile holds binary file data together with its MIME content type.
type StoredFile struct {
	Data        []byte
	ContentType string
}

// StoreFile stores binary file data and its content type under the given downloadID with a specific TTL.
func (s *Store) StoreFile(downloadID string, data []byte, contentType string, ttl time.Duration) error {
	log.Debug().Str("download_id", downloadID).Dur("ttl", ttl).Int("bytes", len(data)).Str("content_type", contentType).Msg("store: storing file")
	s.files.Set(fileKey(downloadID), &StoredFile{Data: data, ContentType: contentType}, ttl)
	return nil
}

// GetFile retrieves file data by downloadID. Returns nil if the file has
// expired or was never stored.
func (s *Store) GetFile(downloadID string) (*StoredFile, error) {
	v, found := s.files.Get(fileKey(downloadID))
	if !found {
		log.Debug().Str("download_id", downloadID).Msg("store: file not found or expired")
		return nil, nil
	}

	sf := v.(*StoredFile)
	log.Debug().Str("download_id", downloadID).Int("bytes", len(sf.Data)).Msg("store: file retrieved")
	return sf, nil
}

// AddScanPage appends a page to the accumulated scan pages for a session.
// The TTL is set to match the session's remaining TTL.
func (s *Store) AddScanPage(sessionID string, page ScanPageData, ttl time.Duration) error {
	key := scanPagesKey(sessionID)
	var pages []ScanPageData
	if v, found := s.scanPages.Get(key); found {
		pages = v.([]ScanPageData)
	}
	pages = append(pages, page)
	s.scanPages.Set(key, pages, ttl)
	log.Debug().Str("session_id", sessionID).Int("document_index", page.DocumentIndex).Int("page_index", page.PageIndex).Int("total_pages", len(pages)).Msg("store: scan page added")
	return nil
}

// GetScanPages returns all accumulated scan pages for a session.
func (s *Store) GetScanPages(sessionID string) ([]ScanPageData, error) {
	v, found := s.scanPages.Get(scanPagesKey(sessionID))
	if !found {
		return nil, nil
	}
	return v.([]ScanPageData), nil
}

// GetScanPageCount returns the number of accumulated scan pages for a session.
func (s *Store) GetScanPageCount(sessionID string) int {
	v, found := s.scanPages.Get(scanPagesKey(sessionID))
	if !found {
		return 0
	}
	return len(v.([]ScanPageData))
}

// ClearScanPages removes all accumulated scan pages for a session.
func (s *Store) ClearScanPages(sessionID string) {
	s.scanPages.Delete(scanPagesKey(sessionID))
	log.Debug().Str("session_id", sessionID).Msg("store: scan pages cleared")
}
