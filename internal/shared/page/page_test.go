package page

import (
	"errors"
	"testing"
	"time"
)

func TestCursorRoundTrip(t *testing.T) {
	k := Key{Time: time.Date(2026, 11, 16, 9, 0, 0, 123456789, time.UTC), ID: "01jd7x2k9m3n4p5q6r7s8t9v0w"}
	got, ok, err := Decode(Encode(k))
	if err != nil || !ok || !got.Time.Equal(k.Time) || got.ID != k.ID {
		t.Fatalf("Decode(Encode(%v)) = %v, %v, %v", k, got, ok, err)
	}
}

func TestDecode(t *testing.T) {
	if _, ok, err := Decode(""); ok || err != nil {
		t.Fatalf("empty cursor: ok=%v err=%v", ok, err)
	}
	for _, bad := range []string{"!!", "bm9waXBl", "MTIz"} {
		if _, _, err := Decode(bad); !errors.Is(err, ErrInvalidCursor) {
			t.Errorf("Decode(%q) = %v, want ErrInvalidCursor", bad, err)
		}
	}
}

func TestLimit(t *testing.T) {
	for in, want := range map[int32]int{0: Default, -1: Default, 10: 10, 500: Max} {
		if got := Limit(in); got != want {
			t.Errorf("Limit(%d) = %d, want %d", in, got, want)
		}
	}
}
