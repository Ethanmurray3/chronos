// Command chronos-mcp is a Model Context Protocol server (stdio transport)
// exposing a running Chronos instance to AI agents. It is a separate,
// optional binary: point it at your Chronos server and register it with your
// agent of choice, e.g.:
//
//	claude mcp add chronos -- /path/to/chronos-mcp --url http://localhost:8787
//
// Configuration: --url flag or CHRONOS_URL env (default http://localhost:8787);
// CHRONOS_TOKEN env is sent as a Bearer token when set (for when the API
// grows real authentication).
package main

import (
	"flag"
	"log"
	"os"

	"chronos/internal/mcpserver"
)

func main() {
	log.SetOutput(os.Stderr) // stdout belongs to the protocol
	log.SetPrefix("chronos-mcp: ")

	def := os.Getenv("CHRONOS_URL")
	if def == "" {
		def = "http://localhost:8787"
	}
	url := flag.String("url", def, "base URL of the Chronos server")
	flag.Parse()

	api := mcpserver.NewAPIClient(*url, os.Getenv("CHRONOS_TOKEN"))
	if err := mcpserver.New(api).Run(os.Stdin, os.Stdout); err != nil {
		log.Fatalf("stdio loop: %v", err)
	}
}
