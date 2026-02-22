package client

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/cloudquery/plugin-sdk/v4/message"
	"github.com/cloudquery/plugin-sdk/v4/plugin"
	"github.com/cloudquery/plugin-sdk/v4/schema"
	"github.com/rs/zerolog"
)

// Client implements the CloudQuery SourceClient interface for S3.
type Client struct {
	plugin.UnimplementedDestination

	logger   zerolog.Logger
	spec     Spec
	s3Client *s3.Client
}

// Configure is the NewClientFunc that the plugin SDK calls to create a Client.
func Configure(ctx context.Context, logger zerolog.Logger, specBytes []byte, opts plugin.NewClientOptions) (plugin.Client, error) {
	var spec Spec
	if err := json.Unmarshal(specBytes, &spec); err != nil {
		return nil, fmt.Errorf("failed to unmarshal spec: %w", err)
	}
	spec.SetDefaults()
	if err := spec.Validate(); err != nil {
		return nil, fmt.Errorf("invalid spec: %w", err)
	}

	cfgOpts := []func(*config.LoadOptions) error{
		config.WithRegion(spec.Region),
	}
	if spec.LocalProfile != "" {
		cfgOpts = append(cfgOpts, config.WithSharedConfigProfile(spec.LocalProfile))
	}
	cfg, err := config.LoadDefaultConfig(ctx, cfgOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}
	var s3Opts []func(*s3.Options)
	if spec.Endpoint != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.BaseEndpoint = &spec.Endpoint
		})
	}
	if spec.PathStyle {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.UsePathStyle = true
		})
	}
	s3Client := s3.NewFromConfig(cfg, s3Opts...)

	return &Client{
		logger:   logger,
		spec:     spec,
		s3Client: s3Client,
	}, nil
}

// ID returns a unique identifier for this client instance.
func (c *Client) ID() string {
	return "cq-source-s3:" + c.spec.Bucket
}

// Tables returns the list of tables discovered from S3 key prefixes.
func (c *Client) Tables(ctx context.Context, options plugin.TableOptions) (schema.Tables, error) {
	tables, err := c.discover(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to discover tables: %w", err)
	}

	allTables := make(schema.Tables, 0, len(tables))
	for _, dt := range tables {
		allTables = append(allTables, dt.Table)
	}

	filtered, err := allTables.FilterDfs(options.Tables, options.SkipTables, options.SkipDependentTables)
	if err != nil {
		return nil, fmt.Errorf("failed to filter tables: %w", err)
	}

	return filtered, nil
}

// Sync streams data from S3 to the destination.
func (c *Client) Sync(ctx context.Context, options plugin.SyncOptions, res chan<- message.SyncMessage) error {
	return c.syncTables(ctx, options, res)
}

// Close releases resources held by the client.
func (c *Client) Close(ctx context.Context) error {
	return nil
}
