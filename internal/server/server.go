// Package server assembles the MCP server and its transports. Both stdio and
// streamable HTTP serve the same *mcp.Server; only the wiring differs.
package server

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/armenbarseghyan/moodle-mcp/internal/tools"
)

// MCPPath is where the streamable HTTP endpoint is mounted.
const MCPPath = "/mcp"

// New returns an MCP server with all tools registered.
func New(deps tools.Deps) *mcp.Server {
	s := mcp.NewServer(
		&mcp.Implementation{Name: "moodle-mcp", Title: "Moodle (read-only)", Version: deps.Version},
		&mcp.ServerOptions{Instructions: tools.Instructions},
	)
	tools.Register(s, deps)
	return s
}

// HTTPOptions configures the streamable HTTP transport.
type HTTPOptions struct {
	// Token, when set, is required as "Authorization: Bearer <token>".
	Token  string
	Logger *slog.Logger
}

// HTTPHandler serves s over streamable HTTP at MCPPath, plus GET /healthz.
//
// Protection layers: the SDK rejects DNS-rebinding requests (localhost
// address with a foreign Host header); browser cross-origin requests are
// rejected; and, with a token, every MCP request needs the bearer token.
func HTTPHandler(s *mcp.Server, opts HTTPOptions) http.Handler {
	var h http.Handler = mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return s },
		&mcp.StreamableHTTPOptions{SessionTimeout: 30 * time.Minute, Logger: opts.Logger},
	)
	h = http.NewCrossOriginProtection().Handler(h)
	if opts.Token != "" {
		h = auth.RequireBearerToken(staticToken(opts.Token), nil)(h)
	}
	mux := http.NewServeMux()
	mux.Handle(MCPPath, h)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok\n"))
	})
	return mux
}

// staticToken verifies a single shared secret in constant time.
func staticToken(want string) auth.TokenVerifier {
	return func(_ context.Context, got string, _ *http.Request) (*auth.TokenInfo, error) {
		if subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
			return nil, auth.ErrInvalidToken
		}
		// The secret does not expire; a far-future expiry satisfies the middleware.
		return &auth.TokenInfo{Expiration: time.Now().Add(24 * time.Hour), UserID: "owner"}, nil
	}
}

// CheckListenAddr refuses to expose the server beyond this machine without
// a bearer token: the server holds a personal Moodle token.
func CheckListenAddr(addr, token string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid address %q: %w", addr, err)
	}
	if isLoopback(host) || token != "" {
		return nil
	}
	return errors.New("address " + addr + " is reachable from other machines: set MCP_HTTP_TOKEN or listen on 127.0.0.1")
}

func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
