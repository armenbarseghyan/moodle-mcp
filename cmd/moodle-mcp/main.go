// Command moodle-mcp is a read-only MCP server for Moodle.
//
// Transports:
//
//	moodle-mcp                          stdio (default; for Claude Code and other local clients)
//	moodle-mcp -http 127.0.0.1:8765     streamable HTTP at /mcp
//
// Configuration comes from the environment only:
//
//	MOODLE_URL           site URL, e.g. https://moodle.fh-joanneum.at
//	MOODLE_TOKEN         personal web service token (moodle_mobile_app)
//	MOODLE_DOWNLOAD_DIR  default download directory (default ~/Downloads/moodle)
//	MOODLE_LOG_LEVEL     debug | info | warn | error (default info)
//	MCP_HTTP_ADDR        same as -http
//	MCP_HTTP_TOKEN       bearer token required by the HTTP transport
//	                     (mandatory when listening on a non-loopback address)
//
// Logs go to stderr; with stdio, stdout belongs to the MCP protocol.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"moodle-mcp/internal/cache"
	"moodle-mcp/internal/moodle"
	"moodle-mcp/internal/server"
	"moodle-mcp/internal/study"
	"moodle-mcp/internal/tools"
)

var version = "dev"

func main() {
	if err := run(); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, "moodle-mcp:", err)
		os.Exit(1)
	}
}

func run() error {
	httpAddr := flag.String("http", os.Getenv("MCP_HTTP_ADDR"), "serve streamable HTTP on this address instead of stdio, e.g. 127.0.0.1:8765")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Fprintln(os.Stderr, "moodle-mcp", version)
		return nil
	}

	token := os.Getenv("MOODLE_TOKEN")
	httpToken := os.Getenv("MCP_HTTP_TOKEN")
	logger := slog.New(moodle.NewRedactingHandler(
		slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel(os.Getenv("MOODLE_LOG_LEVEL"))}),
		token,
	))

	deps := tools.Deps{Version: version, Logger: logger}
	client, err := newClient(token, logger)
	if err != nil {
		// Start anyway: every tool reports the problem, which is far more
		// helpful than the client's "Connection closed".
		logger.Error("configuration", "err", err)
		deps.InitErr = err
	} else {
		deps.Service = study.New(cache.NewSource(client, nil), study.Options{
			DownloadDir: os.Getenv("MOODLE_DOWNLOAD_DIR"),
			Logger:      logger,
		})
	}
	srv := server.New(deps)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if *httpAddr == "" {
		logger.Info("moodle-mcp started", "version", version, "transport", "stdio")
		return srv.Run(ctx, &mcp.StdioTransport{})
	}
	return serveHTTP(ctx, srv, *httpAddr, httpToken, logger)
}

func serveHTTP(ctx context.Context, srv *mcp.Server, addr, token string, logger *slog.Logger) error {
	if err := server.CheckListenAddr(addr, token); err != nil {
		return err
	}
	hs := &http.Server{
		Addr:              addr,
		Handler:           server.HTTPHandler(srv, server.HTTPOptions{Token: token, Logger: logger}),
		ReadHeaderTimeout: 10 * time.Second,
	}
	errc := make(chan error, 1)
	go func() { errc <- hs.ListenAndServe() }()
	logger.Info("moodle-mcp started", "version", version, "transport", "http",
		"url", "http://"+addr+server.MCPPath, "auth", token != "")

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return hs.Shutdown(shutdown)
	}
}

func newClient(token string, logger *slog.Logger) (*moodle.Client, error) {
	base := os.Getenv("MOODLE_URL")
	switch {
	case base == "" && token == "":
		return nil, errors.New("не заданы MOODLE_URL и MOODLE_TOKEN")
	case base == "":
		return nil, errors.New("не задан MOODLE_URL")
	case token == "":
		return nil, errors.New("не задан MOODLE_TOKEN")
	}
	return moodle.New(moodle.Config{BaseURL: base, Token: token, Logger: logger})
}

func logLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
