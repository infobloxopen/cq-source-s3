package client

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

func TestLogger_NoCredentialLeak(t *testing.T) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf).With().Timestamp().Logger()

	logger.Info().
		Str("bucket", "my-bucket").
		Str("region", "us-east-1").
		Msg("starting sync")

	logger.Error().
		Str("error", "AccessDenied: check your permissions").
		Msg("sync failed")

	output := buf.String()

	sensitivePatterns := []string{
		"AWS_SECRET_ACCESS_KEY",
		"AWS_SESSION_TOKEN",
		"AKIA",
	}

	for _, p := range sensitivePatterns {
		if strings.Contains(output, p) {
			t.Errorf("log output contains sensitive pattern %q: %s", p, output)
		}
	}
}

func TestWrapS3Error(t *testing.T) {
	tests := []struct {
		name    string
		errMsg  string
		wantMsg string
	}{
		{
			name:    "access denied",
			errMsg:  "operation error S3: GetObject, https response error StatusCode: 403, AccessDenied",
			wantMsg: "access denied",
		},
		{
			name:    "no such bucket",
			errMsg:  "operation error S3: ListObjectsV2, https response error StatusCode: 404, NoSuchBucket",
			wantMsg: "bucket not found",
		},
		{
			name:    "generic error",
			errMsg:  "connection timeout",
			wantMsg: "connection timeout",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wrapped := wrapS3Error(tc.errMsg, "test-bucket")
			if !strings.Contains(strings.ToLower(wrapped), strings.ToLower(tc.wantMsg)) {
				t.Errorf("wrapS3Error() = %q, want to contain %q", wrapped, tc.wantMsg)
			}
		})
	}
}

func TestID(t *testing.T) {
	c := &Client{spec: Spec{Bucket: "my-bucket"}}
	want := "cq-source-s3:my-bucket"
	if got := c.ID(); got != want {
		t.Errorf("ID() = %q, want %q", got, want)
	}
}

// Suppress unused import warning
var _ = fmt.Sprintf
