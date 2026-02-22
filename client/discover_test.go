package client

import (
	"testing"
	"time"
)

func TestGroupByPrefix(t *testing.T) {
	objects := []S3Object{
		{Key: "data/2024/file_a.parquet", Size: 100},
		{Key: "data/2024/file_b.parquet", Size: 200},
		{Key: "logs/access.parquet", Size: 300},
		{Key: "root_file.parquet", Size: 400},
	}

	tables := groupByPrefix(objects)

	if len(tables) != 3 {
		t.Fatalf("expected 3 tables, got %d", len(tables))
	}

	// Tables are sorted by name
	expected := []struct {
		name       string
		objectCnt  int
	}{
		{"data_2024", 2},
		{"logs", 1},
		{"root_file", 1},
	}

	for i, exp := range expected {
		if tables[i].Name != exp.name {
			t.Errorf("table[%d].Name = %q, want %q", i, tables[i].Name, exp.name)
		}
		if len(tables[i].Objects) != exp.objectCnt {
			t.Errorf("table[%d] has %d objects, want %d", i, len(tables[i].Objects), exp.objectCnt)
		}
	}
}

func TestGroupByPrefix_ParquetFilter(t *testing.T) {
	// groupByPrefix doesn't filter by extension (that happens in listObjects),
	// but it should skip objects whose normalized name is empty.
	objects := []S3Object{
		{Key: "data/2024/file.parquet", Size: 100},
	}

	tables := groupByPrefix(objects)
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	if tables[0].Name != "data_2024" {
		t.Errorf("Name = %q, want %q", tables[0].Name, "data_2024")
	}
}

func TestGroupByPrefix_PathPrefix(t *testing.T) {
	// Objects from different nested paths
	objects := []S3Object{
		{Key: "data/region/us-east/file1.parquet", Size: 100},
		{Key: "data/region/us-east/file2.parquet", Size: 200},
		{Key: "data/region/eu-west/file1.parquet", Size: 300},
	}

	tables := groupByPrefix(objects)

	if len(tables) != 2 {
		t.Fatalf("expected 2 tables, got %d", len(tables))
	}

	// Sorted: data_region_eu_west, data_region_us_east
	if tables[0].Name != "data_region_eu_west" {
		t.Errorf("tables[0].Name = %q, want %q", tables[0].Name, "data_region_eu_west")
	}
	if tables[1].Name != "data_region_us_east" {
		t.Errorf("tables[1].Name = %q, want %q", tables[1].Name, "data_region_us_east")
	}
}

func TestFilterObjectsByCursor(t *testing.T) {
	objects := []S3Object{
		{Key: "data/old.parquet", LastModified: "2024-01-01T00:00:00Z"},
		{Key: "data/new.parquet", LastModified: "2024-06-15T12:00:00Z"},
		{Key: "data/newest.parquet", LastModified: "2024-12-01T00:00:00Z"},
	}

	cursor, _ := time.Parse(time.RFC3339Nano, "2024-06-01T00:00:00Z")
	filtered := filterObjectsByCursor(objects, cursor)
	if len(filtered) != 2 {
		t.Fatalf("expected 2 objects after cursor, got %d", len(filtered))
	}
	if filtered[0].Key != "data/new.parquet" {
		t.Errorf("filtered[0].Key = %q, want %q", filtered[0].Key, "data/new.parquet")
	}

	filtered = filterObjectsByCursor(objects, time.Time{})
	if len(filtered) != 3 {
		t.Fatalf("expected 3 objects with zero cursor, got %d", len(filtered))
	}

	cursor, _ = time.Parse(time.RFC3339Nano, "2025-01-01T00:00:00Z")
	filtered = filterObjectsByCursor(objects, cursor)
	if len(filtered) != 0 {
		t.Fatalf("expected 0 objects after future cursor, got %d", len(filtered))
	}
}

func TestMaxLastModified(t *testing.T) {
	objects := []S3Object{
		{Key: "a.parquet", LastModified: "2024-01-01T00:00:00Z"},
		{Key: "b.parquet", LastModified: "2024-12-01T00:00:00Z"},
		{Key: "c.parquet", LastModified: "2024-06-15T00:00:00Z"},
	}
	max := maxLastModified(objects)
	want, _ := time.Parse(time.RFC3339Nano, "2024-12-01T00:00:00Z")
	if !max.Equal(want) {
		t.Errorf("maxLastModified = %v, want %v", max, want)
	}
}

func TestMaxLastModified_Empty(t *testing.T) {
	max := maxLastModified(nil)
	if !max.IsZero() {
		t.Errorf("maxLastModified(nil) = %v, want zero", max)
	}
}
