package client

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/cloudquery/plugin-sdk/v4/message"
	"github.com/cloudquery/plugin-sdk/v4/plugin"
	"github.com/cloudquery/plugin-sdk/v4/schema"
	"github.com/cloudquery/plugin-sdk/v4/state"
)

// syncTables performs the full sync pipeline: discover tables, emit migrations,
// then stream records for each object with optional incremental filtering.
func (c *Client) syncTables(ctx context.Context, options plugin.SyncOptions, res chan<- message.SyncMessage) error {
	// Initialize state client for incremental sync
	stateClient, err := state.NewConnectedClient(ctx, options.BackendOptions)
	if err != nil {
		return fmt.Errorf("failed to initialize state backend: %w", err)
	}
	defer func() {
		if err := stateClient.Close(); err != nil {
			c.logger.Warn().Err(err).Msg("failed to close state client")
		}
	}()

	tables, err := c.discover(ctx)
	if err != nil {
		return fmt.Errorf("discovery failed: %w", err)
	}

	c.logger.Info().
		Int("discovered_tables", len(tables)).
		Msg("discovery complete")

	allTables := make(schema.Tables, 0, len(tables))
	tableMap := make(map[string]*DiscoveredTable, len(tables))
	for i := range tables {
		allTables = append(allTables, tables[i].Table)
		tableMap[tables[i].Name] = &tables[i]
	}

	filtered, err := allTables.FilterDfs(options.Tables, options.SkipTables, options.SkipDependentTables)
	if err != nil {
		return fmt.Errorf("failed to filter tables: %w", err)
	}

	c.logger.Info().Int("tables", len(filtered)).Msg("starting sync")

	for _, table := range filtered {
		if err := ctx.Err(); err != nil {
			return err
		}

		dt, ok := tableMap[table.Name]
		if !ok {
			continue
		}

		cursor, err := GetCursor(ctx, stateClient, c.spec.Bucket, table.Name)
		if err != nil {
			c.logger.Warn().Err(err).Str("table", table.Name).Msg("failed to read cursor, performing full sync for table")
			cursor = time.Time{}
		}

		objects := filterObjectsByCursor(dt.Objects, cursor)

		c.logger.Info().
			Str("table", table.Name).
			Int("total_objects", len(dt.Objects)).
			Int("new_objects", len(objects)).
			Bool("incremental", !cursor.IsZero()).
			Msg("syncing table")

		if len(objects) == 0 {
			c.logger.Debug().Str("table", table.Name).Msg("no new objects, skipping table")
			res <- &message.SyncMigrateTable{Table: table}
			continue
		}

		res <- &message.SyncMigrateTable{Table: table}

		if err := c.syncTableObjects(ctx, table, objects, res); err != nil {
			return fmt.Errorf("failed to sync table %s: %w", table.Name, err)
		}

		maxMod := maxLastModified(objects)
		if !maxMod.IsZero() {
			if err := SetCursor(ctx, stateClient, c.spec.Bucket, table.Name, maxMod); err != nil {
				c.logger.Warn().Err(err).Str("table", table.Name).Msg("failed to set cursor")
			}
		}
	}

	if err := stateClient.Flush(ctx); err != nil {
		c.logger.Warn().Err(err).Msg("failed to flush state backend")
	}

	c.logger.Info().Msg("sync complete")
	return nil
}

// syncTableObjects processes all objects for a single table with concurrency control.
func (c *Client) syncTableObjects(ctx context.Context, table *schema.Table, objects []S3Object, res chan<- message.SyncMessage) error {
	concurrency := c.spec.Concurrency

	if concurrency == 1 {
		for _, obj := range objects {
			if err := c.syncObject(ctx, table, obj, res); err != nil {
				return err
			}
		}
		return nil
	}

	var sem chan struct{}
	if concurrency > 0 {
		sem = make(chan struct{}, concurrency)
	}

	var (
		mu       sync.Mutex
		firstErr error
		wg       sync.WaitGroup
	)

	for _, obj := range objects {
		if err := ctx.Err(); err != nil {
			return err
		}

		mu.Lock()
		hasErr := firstErr != nil
		mu.Unlock()
		if hasErr {
			break
		}

		if sem != nil {
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		wg.Add(1)
		go func(o S3Object) {
			defer wg.Done()
			if sem != nil {
				defer func() { <-sem }()
			}
			if err := c.syncObject(ctx, table, o, res); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
			}
		}(obj)
	}

	wg.Wait()
	return firstErr
}

// syncObject streams records from a single S3 object and emits SyncInsert messages.
func (c *Client) syncObject(ctx context.Context, table *schema.Table, obj S3Object, res chan<- message.SyncMessage) error {
	records := make(chan arrow.RecordBatch, 1)
	errCh := make(chan error, 1)

	go func() {
		defer close(records)
		errCh <- c.streamRecords(ctx, obj.Key, c.spec.RowsPerRecord, records)
	}()

	var totalRows int64
	for rec := range records {
		totalRows += rec.NumRows()
		res <- &message.SyncInsert{Record: rec}
	}

	if err := <-errCh; err != nil {
		var noSuchKey *types.NoSuchKey
		if errors.As(err, &noSuchKey) {
			c.logger.Warn().
				Str("key", obj.Key).
				Str("table", table.Name).
				Msg("object deleted between list and read, skipping")
			return nil
		}

		if isMalformedParquetError(err) {
			c.logger.Warn().
				Err(err).
				Str("key", obj.Key).
				Str("table", table.Name).
				Msg("malformed parquet file, skipping")
			return nil
		}

		return fmt.Errorf("failed to sync object %s: %w", obj.Key, err)
	}

	c.logger.Debug().
		Str("key", obj.Key).
		Str("table", table.Name).
		Int64("rows", totalRows).
		Int64("size_bytes", obj.Size).
		Msg("object synced")

	return nil
}

// isMalformedParquetError checks if the error is from a malformed Parquet file.
func isMalformedParquetError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	malformedPatterns := []string{
		"not a parquet file",
		"invalid parquet",
		"parquet: invalid",
		"magic number",
		"failed to open parquet",
	}
	for _, pattern := range malformedPatterns {
		if strings.Contains(msg, pattern) {
			return true
		}
	}
	return false
}
