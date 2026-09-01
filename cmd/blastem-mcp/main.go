package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/akiyan/BlastEM-MCP/internal/mcpserver"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	app := mcpserver.New(version)
	defer app.Close()
	if err := app.Server().Run(ctx, &mcp.StdioTransport{}); err != nil && ctx.Err() == nil {
		log.Fatal(err)
	}
}
