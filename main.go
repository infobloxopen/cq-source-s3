package main

import (
	"context"
	"log"

	"github.com/cloudquery/plugin-sdk/v4/serve"
	"github.com/infobloxopen/cq-source-s3/plugin"
)

func main() {
	ctx := context.Background()
	p := serve.Plugin(plugin.Plugin())
	if err := p.Serve(ctx); err != nil {
		log.Fatalf("failed to serve plugin: %v", err)
	}
}
