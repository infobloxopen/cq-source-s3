package client

import (
	"context"
	"testing"
	"time"

	"github.com/cloudquery/plugin-sdk/v4/state"
)

func TestCursorKey(t *testing.T) {
	key := CursorKey("my-bucket", "data_2024")
	want := "s3/my-bucket/data_2024/last_modified_cursor"
	if key != want {
		t.Errorf("CursorKey = %q, want %q", key, want)
	}
}

func TestGetCursor_Empty(t *testing.T) {
	ctx := context.Background()
	sc := &state.NoOpClient{}

	cursor, err := GetCursor(ctx, sc, "bucket", "table")
	if err != nil {
		t.Fatalf("GetCursor: %v", err)
	}
	if !cursor.IsZero() {
		t.Errorf("expected zero time, got %v", cursor)
	}
}

func TestCursor_Roundtrip(t *testing.T) {
	// NoOpClient returns empty string for GetKey, so we can only test the format
	now := time.Now().UTC().Truncate(time.Nanosecond)
	formatted := now.Format(time.RFC3339Nano)
	parsed, err := time.Parse(time.RFC3339Nano, formatted)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !now.Equal(parsed) {
		t.Errorf("roundtrip failed: %v != %v", now, parsed)
	}
}
