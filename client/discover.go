package client

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"time"
	"strings"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/cloudquery/plugin-sdk/v4/schema"
	"github.com/infobloxopen/cq-source-s3/internal/naming"
)

// S3Object holds metadata about a single S3 object.
type S3Object struct {
	Key          string
	Size         int64
	LastModified string // RFC3339Nano
}

// DiscoveredTable represents a logical table derived from S3 key prefixes.
type DiscoveredTable struct {
	Name        string
	Prefix      string
	Objects     []S3Object
	ArrowSchema *arrow.Schema
	Table       *schema.Table
}

// discover lists S3 objects, groups them by prefix into tables, reads schemas,
// validates schema consistency, and builds CQ tables.
func (c *Client) discover(ctx context.Context) ([]DiscoveredTable, error) {
	objects, err := c.listObjects(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list objects: %w", err)
	}

	tables := groupByPrefix(objects)

	for i := range tables {
		if len(tables[i].Objects) == 0 {
			continue
		}

		// Read schema from first file
		sc, err := c.readParquetSchema(ctx, tables[i].Objects[0].Key)
		if err != nil {
			return nil, fmt.Errorf("failed to read schema from %s: %w", tables[i].Objects[0].Key, err)
		}
		tables[i].ArrowSchema = sc

		// Validate all files have the same schema
		for j := 1; j < len(tables[i].Objects); j++ {
			sc2, err := c.readParquetSchema(ctx, tables[i].Objects[j].Key)
			if err != nil {
				return nil, fmt.Errorf("failed to read schema from %s: %w", tables[i].Objects[j].Key, err)
			}
			if !sc.Equal(sc2) {
				return nil, fmt.Errorf(
					"schema mismatch in table %s: file %s has %v, file %s has %v",
					tables[i].Name,
					tables[i].Objects[0].Key, sc.Fields(),
					tables[i].Objects[j].Key, sc2.Fields(),
				)
			}
		}

		// Build CQ table from Arrow schema fields
		columns := make(schema.ColumnList, sc.NumFields())
		for fi := 0; fi < sc.NumFields(); fi++ {
			columns[fi] = schema.NewColumnFromArrowField(sc.Field(fi))
		}
		table := &schema.Table{
			Name:          tables[i].Name,
			Columns:       columns,
			IsIncremental: true,
		}
		schema.AddCqIDs(table)
		tables[i].Table = table
	}

	return tables, nil
}

// listObjects uses ListObjectsV2 pagination to list all .parquet objects in the bucket.
func (c *Client) listObjects(ctx context.Context) ([]S3Object, error) {
	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(c.spec.Bucket),
	}
	if c.spec.PathPrefix != "" {
		input.Prefix = aws.String(c.spec.PathPrefix)
	}

	var objects []S3Object
	paginator := s3.NewListObjectsV2Paginator(c.s3Client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list objects in bucket %s: %w", c.spec.Bucket, err)
		}
		for _, obj := range page.Contents {
			key := aws.ToString(obj.Key)
			if !strings.HasSuffix(strings.ToLower(key), "."+c.spec.FileType) {
				continue
			}
			objects = append(objects, S3Object{
				Key:          key,
				Size:         aws.ToInt64(obj.Size),
				LastModified: obj.LastModified.Format("2006-01-02T15:04:05.999999999Z07:00"),
			})
		}
	}

	return objects, nil
}

// groupByPrefix groups S3 objects by their normalized table name.
func groupByPrefix(objects []S3Object) []DiscoveredTable {
	byName := make(map[string]*DiscoveredTable)
	for _, obj := range objects {
		name := naming.Normalize(obj.Key)
		if name == "" {
			continue
		}
		dt, ok := byName[name]
		if !ok {
			prefix := ""
			dir := filepath.Dir(obj.Key)
			if dir != "." {
				prefix = dir + "/"
			}
			dt = &DiscoveredTable{
				Name:   name,
				Prefix: prefix,
			}
			byName[name] = dt
		}
		dt.Objects = append(dt.Objects, obj)
	}

	tables := make([]DiscoveredTable, 0, len(byName))
	for _, dt := range byName {
		tables = append(tables, *dt)
	}
	sort.Slice(tables, func(i, j int) bool {
		return tables[i].Name < tables[j].Name
	})

	return tables
}

// filterObjectsByCursor returns objects with LastModified strictly after the cursor.
// If cursor is zero-time, all objects are returned.
func filterObjectsByCursor(objects []S3Object, cursor time.Time) []S3Object {
	if cursor.IsZero() {
		return objects
	}
	var filtered []S3Object
	for _, obj := range objects {
		t, err := time.Parse(time.RFC3339Nano, obj.LastModified)
		if err != nil {
			filtered = append(filtered, obj)
			continue
		}
		if t.After(cursor) {
			filtered = append(filtered, obj)
		}
	}
	return filtered
}

// maxLastModified returns the maximum LastModified timestamp from a list of objects.
func maxLastModified(objects []S3Object) time.Time {
	var max time.Time
	for _, obj := range objects {
		t, err := time.Parse(time.RFC3339Nano, obj.LastModified)
		if err != nil {
			continue
		}
		if t.After(max) {
			max = t
		}
	}
	return max
}

// wrapS3Error wraps common S3 error messages with user-friendly context.
func wrapS3Error(errMsg string, bucket string) string {
	lower := strings.ToLower(errMsg)
	if strings.Contains(lower, "accessdenied") || strings.Contains(lower, "403") {
		return fmt.Sprintf("access denied for bucket %q: verify IAM permissions (s3:ListBucket, s3:GetObject) -- original error: %s", bucket, errMsg)
	}
	if strings.Contains(lower, "nosuchbucket") || strings.Contains(lower, "404") {
		return fmt.Sprintf("bucket not found: %q -- verify the bucket name and region -- original error: %s", bucket, errMsg)
	}
	return errMsg
}
