// Created by the SKGO MCP scaffold. This file belongs to the application.
package main

import (
	_ "embed"
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/mark3labs/mcp-go/server"
	client "github.com/tylergannon/skgo/plugins/cli/example/internal/cliclient"
	"github.com/tylergannon/skgo/plugins/cli/remoteclient"
)

//go:embed instructions.md
var instructions string

func main() {
	baseURL := flag.String("base-url", os.Getenv("SKGO_BASE_URL"), "application base URL (or SKGO_BASE_URL)")
	flag.Parse()
	if *baseURL == "" {
		log.Fatal("-base-url or SKGO_BASE_URL is required")
	}

	// Configure the application's authentication and HTTP connection here.
	// These credentials go to the application, never into tool documentation.
	headers := http.Header{}
	if token := os.Getenv("SKGO_TOKEN"); token != "" {
		headers.Set("Authorization", "Bearer "+token)
	}
	if key := os.Getenv("SKGO_API_KEY"); key != "" {
		headers.Set("X-API-Key", key)
	}
	c := &client.Client{Remote: remoteclient.Client{
		BaseURL: *baseURL, Headers: headers,
		// Set HTTP to your own *http.Client for application-specific transport.
	}}
	s := server.NewMCPServer("TaskList", "dev",
		server.WithInstructions(instructions),
		server.WithToolCapabilities(false),
	)
	s.AddTools(c.Tools()...)
	// Register additional application tools, resources and prompts here.
	if err := server.ServeStdio(s); err != nil {
		log.Fatal(err)
	}
}
