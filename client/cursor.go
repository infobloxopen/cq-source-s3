package client

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudquery/plugin-sdk/v4/state"
)

// CursorKey returns the state backend key for a table's incremental cursor.
func CursorKey(bucket, tableName string) string {
	return fmt.Sprintf("s3/%s/%s/last_modified_cursor", bucket, tableName)
}

// GetCursor retrieves the stored cursor timestamp for a table.
// Returns zero-time if no cursor exists or the value cannot be parsed.
func GetCursor(ctx context.Context, sc state.Client, bucket, tableName string) (time.Time, error) {
	key := CursorKey(bucket, tableName)
	val, err := sc.GetKey(ctx, key)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to get cursor for %s: %w", tableName, err)
	}
	if val == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339Nano, val)
	if err != nil {
		// Log warning and treat as zero-time
		return time.Time{}, nil
	}
	return t, nil
}

// SetCursor stores the cursor timestamp for a table.
func SetCursor(ctx context.Context, sc state.Client, bucket, tableName string, cursor time.Time) error {
	key := CursorKey(bucket, tableName)
	return sc.SetKey(ctx, key, cursor.Format(time.RFC3339Nano))
}
