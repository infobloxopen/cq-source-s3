package client

import (
	"fmt"
)

// Spec is the user-facing configuration for the S3 source plugin.
type Spec struct {
	Bucket        string `json:"bucket"`
	Region        string `json:"region"`
	LocalProfile  string `json:"local_profile,omitempty"`
	PathPrefix    string `json:"path_prefix,omitempty"`
	FileType      string `json:"filetype,omitempty"`
	RowsPerRecord int    `json:"rows_per_record,omitempty"`
	Concurrency   int    `json:"concurrency,omitempty"`
	Endpoint      string `json:"endpoint,omitempty"`
	PathStyle     bool   `json:"path_style,omitempty"`
}

// SetDefaults applies default values for optional fields.
func (s *Spec) SetDefaults() {
	if s.FileType == "" {
		s.FileType = "parquet"
	}
	if s.RowsPerRecord == 0 {
		s.RowsPerRecord = 500
	}
	if s.Concurrency == 0 {
		s.Concurrency = 50
	}
}

// Validate checks that required fields are set and values are valid.
func (s *Spec) Validate() error {
	if s.Bucket == "" {
		return fmt.Errorf("bucket is required")
	}
	if s.Region == "" {
		return fmt.Errorf("region is required")
	}
	if s.FileType != "parquet" {
		return fmt.Errorf("unsupported filetype: %q; supported: parquet", s.FileType)
	}
	if s.RowsPerRecord < 1 {
		return fmt.Errorf("rows_per_record must be at least 1")
	}
	return nil
}
