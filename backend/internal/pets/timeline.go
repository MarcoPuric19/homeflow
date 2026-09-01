package pets

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"homeflow/backend/internal/platform/id"
)

const TimelineDefaultLimit = 30
const TimelineMaxLimit = 100

var ErrInvalidTimeline = errors.New("invalid pet timeline query")

type TimelineItem struct {
	Type     string `json:"type"`
	Date     string `json:"date"`
	Title    string `json:"title"`
	SourceID string `json:"sourceId"`
}

type TimelinePage struct {
	Items      []TimelineItem `json:"items"`
	NextCursor string         `json:"nextCursor,omitempty"`
}

type timelineCursor struct {
	Date, Type, SourceID string
}

type TimelineRepository interface {
	ListTimeline(context.Context, string, string, int, *timelineCursor) (TimelinePage, error)
}

func (s *Service) ListTimeline(ctx context.Context, userID, petID string, limit int, cursor string) (TimelinePage, error) {
	if !id.IsUUID(petID) {
		return TimelinePage{}, ErrInvalidID
	}
	if limit == 0 {
		limit = TimelineDefaultLimit
	}
	if limit < 1 || limit > TimelineMaxLimit {
		return TimelinePage{}, ErrInvalidTimeline
	}
	parsed, err := decodeTimelineCursor(cursor)
	if err != nil {
		return TimelinePage{}, ErrInvalidTimeline
	}
	repository, ok := s.repository.(TimelineRepository)
	if !ok {
		return TimelinePage{}, errors.New("timeline repository unavailable")
	}
	return repository.ListTimeline(ctx, userID, petID, limit, parsed)
}

func encodeTimelineCursor(cursor timelineCursor) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strings.Join([]string{cursor.Date, cursor.Type, cursor.SourceID}, "|")))
}

func decodeTimelineCursor(value string) (*timelineCursor, error) {
	if value == "" {
		return nil, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(string(decoded), "|")
	if len(parts) != 3 || parts[1] == "" || !id.IsUUID(parts[2]) {
		return nil, ErrInvalidTimeline
	}
	if _, err := time.Parse(time.DateOnly, parts[0]); err != nil {
		return nil, ErrInvalidTimeline
	}
	return &timelineCursor{Date: parts[0], Type: parts[1], SourceID: parts[2]}, nil
}
