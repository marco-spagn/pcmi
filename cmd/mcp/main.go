// pcmi-mcp exposes PCMI memory operations over Model Context Protocol (stdio JSON-RPC 2.0).
//
// Environment: PCMI_BASE_URL, PCMI_API_KEY (see internal/config and .env.example)
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/marco-spagn/pcmi/internal/config"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg := config.Load()
	if err := cfg.Validate(config.MCPRequiredFields...); err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	client := newHTTPPCMIClient(cfg.PCMIBaseURL, cfg.PCMIAPIKey)
	srv := NewServer(client, log)

	log.Info("pcmi-mcp starting", "base_url", cfg.PCMIBaseURL)
	if err := srv.Run(ctx, os.Stdin, os.Stdout); err != nil {
		log.Error("server", "err", err)
		os.Exit(1)
	}
}
