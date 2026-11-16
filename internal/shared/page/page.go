// Package page reads the limit and cursor of a List request and writes the
// cursor of the next page. A cursor is opaque to callers: the sort key of
// the last item of a page, a time and an ID.
package page

import (
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"
)

// Default and Max bound how many items one page holds.
const (
	Default = 50
	Max     = 200
)

// ErrInvalidCursor is a cursor this service did not write.
var ErrInvalidCursor = errors.New("page: invalid cursor")

// Limit returns how many items to return for a request's limit.
func Limit(limit int32) int {
	switch {
	case limit <= 0:
		return Default
	case limit > Max:
		return Max
	}
	return int(limit)
}

// Key is the position of an item in a list sorted by time, then ID, both
// descending.
type Key struct {
	Time time.Time
	ID   string
}

// Encode returns the cursor that continues after k.
func Encode(k Key) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(k.Time.UnixNano(), 10) + "|" + k.ID))
}

// Decode reads a cursor; ok is false for an empty one, the first page.
func Decode(cursor string) (k Key, ok bool, err error) {
	if cursor == "" {
		return Key{}, false, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return Key{}, false, ErrInvalidCursor
	}
	nanos, id, found := strings.Cut(string(raw), "|")
	n, err := strconv.ParseInt(nanos, 10, 64)
	if !found || err != nil || id == "" {
		return Key{}, false, ErrInvalidCursor
	}
	return Key{Time: time.Unix(0, n).UTC(), ID: id}, true, nil
}
