package client

import (
	"testing"
)

func TestSpec_SetDefaults(t *testing.T) {
	t.Run("applies all defaults to zero-value spec", func(t *testing.T) {
		s := Spec{Bucket: "b", Region: "us-east-1"}
		s.SetDefaults()

		if s.FileType != "parquet" {
			t.Errorf("FileType = %q, want %q", s.FileType, "parquet")
		}
		if s.RowsPerRecord != 500 {
			t.Errorf("RowsPerRecord = %d, want %d", s.RowsPerRecord, 500)
		}
		if s.Concurrency != 50 {
			t.Errorf("Concurrency = %d, want %d", s.Concurrency, 50)
		}
	})

	t.Run("does not override explicit values", func(t *testing.T) {
		s := Spec{
			Bucket:        "b",
			Region:        "us-east-1",
			FileType:      "parquet",
			RowsPerRecord: 100,
			Concurrency:   10,
		}
		s.SetDefaults()

		if s.RowsPerRecord != 100 {
			t.Errorf("RowsPerRecord = %d, want %d", s.RowsPerRecord, 100)
		}
		if s.Concurrency != 10 {
			t.Errorf("Concurrency = %d, want %d", s.Concurrency, 10)
		}
	})
}

func TestSpec_Validate(t *testing.T) {
	validSpec := func() Spec {
		return Spec{
			Bucket:        "my-bucket",
			Region:        "us-east-1",
			FileType:      "parquet",
			RowsPerRecord: 500,
			Concurrency:   50,
		}
	}

	t.Run("valid config passes", func(t *testing.T) {
		s := validSpec()
		if err := s.Validate(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("missing bucket", func(t *testing.T) {
		s := validSpec()
		s.Bucket = ""
		err := s.Validate()
		if err == nil {
			t.Fatal("expected error for missing bucket")
		}
		if got := err.Error(); got != "bucket is required" {
			t.Errorf("error = %q, want %q", got, "bucket is required")
		}
	})

	t.Run("missing region", func(t *testing.T) {
		s := validSpec()
		s.Region = ""
		err := s.Validate()
		if err == nil {
			t.Fatal("expected error for missing region")
		}
		if got := err.Error(); got != "region is required" {
			t.Errorf("error = %q, want %q", got, "region is required")
		}
	})

	t.Run("invalid filetype", func(t *testing.T) {
		s := validSpec()
		s.FileType = "csv"
		err := s.Validate()
		if err == nil {
			t.Fatal("expected error for invalid filetype")
		}
	})

	t.Run("rows_per_record less than 1", func(t *testing.T) {
		s := validSpec()
		s.RowsPerRecord = 0
		err := s.Validate()
		if err == nil {
			t.Fatal("expected error for rows_per_record < 1")
		}
	})

	t.Run("concurrency -1 is allowed (unlimited)", func(t *testing.T) {
		s := validSpec()
		s.Concurrency = -1
		if err := s.Validate(); err != nil {
			t.Errorf("unexpected error for concurrency -1: %v", err)
		}
	})

	t.Run("concurrency 1 is allowed", func(t *testing.T) {
		s := validSpec()
		s.Concurrency = 1
		if err := s.Validate(); err != nil {
			t.Errorf("unexpected error for concurrency 1: %v", err)
		}
	})
}
