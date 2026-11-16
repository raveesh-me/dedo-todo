// Package ids makes the IDs the service gives resources: lowercase ULIDs,
// 26 characters that sort by creation time, such as
// 01jd7x2k9m3n4p5q6r7s8t9v0w.
package ids

import (
	"crypto/rand"
	"encoding/binary"
	"time"
)

// alphabet is Crockford's base32, lowercase.
const alphabet = "0123456789abcdefghjkmnpqrstvwxyz"

// New returns a new ID for a resource created at now.
func New(now time.Time) string {
	var b [16]byte
	binary.BigEndian.PutUint64(b[:8], uint64(now.UnixMilli())<<16)
	_, _ = rand.Read(b[6:])
	// 128 bits as 26 base32 characters, most significant first.
	hi := binary.BigEndian.Uint64(b[:8])
	lo := binary.BigEndian.Uint64(b[8:])
	var out [26]byte
	for i := 25; i >= 0; i-- {
		out[i] = alphabet[lo&31]
		lo = lo>>5 | hi<<59
		hi >>= 5
	}
	return string(out[:])
}
