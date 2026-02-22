// Package plugin provides the CloudQuery plugin wiring for cq-source-s3.
package plugin

import (
	"github.com/infobloxopen/cq-source-s3/client"

	"github.com/cloudquery/plugin-sdk/v4/plugin"
)

// Version is set at build time via ldflags.
var (
	Version = "development"
)

// Plugin returns a new CloudQuery source plugin configured for S3.

func Plugin() *plugin.Plugin {
	return plugin.NewPlugin(
		"cq-source-s3",
		Version,
		client.Configure,
	)
}
