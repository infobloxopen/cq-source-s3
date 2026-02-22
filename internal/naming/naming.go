// Package naming provides table name normalization from S3 object keys.
package naming

import (
	"path"
	"regexp"
	"strings"
)

var invalidChars = regexp.MustCompile(`[^a-zA-Z0-9_]`)
var multiUnderscores = regexp.MustCompile(`_+`)

// Normalize converts an S3 object key into a table name.
//
// For keys with directory prefixes: the filename is stripped, and the remaining
// path segments are joined with "_". For root-level keys (no directory): the
// filename without its extension is used. Invalid characters (hyphens, dots,
// spaces, etc.) are replaced with "_", and consecutive underscores are
// collapsed to a single "_". Leading and trailing underscores are trimmed.
func Normalize(key string) string {
	// Check if original key was a directory (trailing slash) before trimming
	isDir := strings.HasSuffix(key, "/")

	// Remove trailing slashes
	key = strings.TrimRight(key, "/")

	dir := path.Dir(key)
	base := path.Base(key)

	var raw string
	if isDir {
		// Directory path: use the full cleaned key as the prefix
		raw = key
	} else if dir == "." || dir == "" {
		// Root-level file: use filename without extension
		ext := path.Ext(base)
		raw = strings.TrimSuffix(base, ext)
	} else {
		// Nested file: use the directory prefix
		raw = dir
	}

	// Replace path separators with underscores
	raw = strings.ReplaceAll(raw, "/", "_")

	// Replace invalid characters with underscores
	raw = invalidChars.ReplaceAllString(raw, "_")

	// Collapse consecutive underscores
	raw = multiUnderscores.ReplaceAllString(raw, "_")

	// Trim leading and trailing underscores
	raw = strings.Trim(raw, "_")

	return raw
}
