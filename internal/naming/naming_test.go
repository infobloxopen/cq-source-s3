package naming

import (
	"testing"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want string
	}{
		{
			name: "root file uses filename sans extension",
			key:  "datafile_0.parquet",
			want: "datafile_0",
		},
		{
			name: "nested prefix joined with underscores",
			key:  "data/2024/file.parquet",
			want: "data_2024",
		},
		{
			name: "deeply nested prefix",
			key:  "data/region/us-east/2024/file.parquet",
			want: "data_region_us_east_2024",
		},
		{
			name: "hyphens replaced with underscores",
			key:  "my-data/some-file.parquet",
			want: "my_data",
		},
		{
			name: "dots replaced with underscores in prefix",
			key:  "data.v2/file.parquet",
			want: "data_v2",
		},
		{
			name: "consecutive separators collapsed",
			key:  "data//2024///file.parquet",
			want: "data_2024",
		},
		{
			name: "multiple files same prefix yield same name",
			key:  "data/2024/file_a.parquet",
			want: "data_2024",
		},
		{
			name: "multiple files same prefix yield same name (second file)",
			key:  "data/2024/file_b.parquet",
			want: "data_2024",
		},
		{
			name: "trailing slash in prefix",
			key:  "data/2024/",
			want: "data_2024",
		},
		{
			name: "single directory prefix",
			key:  "logs/access.parquet",
			want: "logs",
		},
		{
			name: "spaces replaced with underscores",
			key:  "my data/file name.parquet",
			want: "my_data",
		},
		{
			name: "root file with hyphens",
			key:  "my-data-file.parquet",
			want: "my_data_file",
		},
		{
			name: "root file with dots in name",
			key:  "data.v2.parquet",
			want: "data_v2",
		},
		{
			name: "leading underscores trimmed",
			key:  "_private/data.parquet",
			want: "private",
		},
		{
			name: "trailing underscores trimmed",
			key:  "data_/file.parquet",
			want: "data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Normalize(tt.key)
			if got != tt.want {
				t.Errorf("Normalize(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}
