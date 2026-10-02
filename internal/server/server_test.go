package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"moodle-mcp/internal/cache"
	"moodle-mcp/internal/moodle"
	"moodle-mcp/internal/moodletest"
	"moodle-mcp/internal/server"
	"moodle-mcp/internal/study"
	"moodle-mcp/internal/tools"
)

const httpToken = "local-secret"

func newHTTP(t *testing.T, token string) *httptest.Server {
	t.Helper()
	fake := moodletest.New(t, moodletest.Fixture("core_webservice_get_site_info", "real"))
	client, err := moodle.New(moodle.Config{BaseURL: fake.URL, Token: moodletest.Token})
	if err != nil {
		t.Fatal(err)
	}
	svc := study.New(cache.NewSource(client, nil), study.Options{})
	srv := server.New(tools.Deps{Service: svc, Version: "test"})
	hs := httptest.NewServer(server.HTTPHandler(srv, server.HTTPOptions{Token: token}))
	t.Cleanup(hs.Close)
	return hs
}

type bearer struct {
	token string
	host  string
}

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	if b.token != "" {
		r.Header.Set("Authorization", "Bearer "+b.token)
	}
	if b.host != "" {
		r.Host = b.host
	}
	return http.DefaultTransport.RoundTrip(r)
}

func connect(t *testing.T, url string, rt http.RoundTripper) (*mcp.ClientSession, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tr := &mcp.StreamableClientTransport{Endpoint: url + server.MCPPath, HTTPClient: &http.Client{Transport: rt}, MaxRetries: -1}
	return mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, tr, nil)
}

func TestHTTP_ToolCall(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		server string
		client string
	}{
		{"with bearer token", httpToken, httpToken},
		{"without auth on loopback", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			hs := newHTTP(t, tc.server)
			cs, err := connect(t, hs.URL, bearer{token: tc.client})
			if err != nil {
				t.Fatal(err)
			}
			defer cs.Close()
			res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "moodle_whoami", Arguments: map[string]any{}})
			if err != nil || res.IsError {
				t.Fatalf("call: %v %+v", err, res)
			}
			if txt := res.Content[0].(*mcp.TextContent).Text; !strings.Contains(txt, "FH JOANNEUM Moodle") {
				t.Errorf("unexpected answer: %s", txt)
			}
		})
	}
}

func TestHTTP_Rejections(t *testing.T) {
	t.Parallel()
	hs := newHTTP(t, httpToken)
	for _, tc := range []struct {
		name string
		rt   bearer
	}{
		{"missing token", bearer{}},
		{"wrong token", bearer{token: "guess"}},
		{"right token, DNS rebinding Host header", bearer{token: httpToken, host: "evil.example:80"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if cs, err := connect(t, hs.URL, tc.rt); err == nil {
				cs.Close()
				t.Fatal("connection must be refused")
			}
		})
	}

	t.Run("status codes", func(t *testing.T) {
		t.Parallel()
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, hs.URL+server.MCPPath, strings.NewReader("{}"))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", resp.StatusCode)
		}
	})
}

func TestHTTP_Healthz(t *testing.T) {
	t.Parallel()
	hs := newHTTP(t, httpToken)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, hs.URL+"/healthz", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("healthz = %d (must not require the token)", resp.StatusCode)
	}
}

func TestCheckListenAddr(t *testing.T) {
	t.Parallel()
	tests := []struct {
		addr, token string
		ok          bool
	}{
		{"127.0.0.1:8765", "", true},
		{"localhost:8765", "", true},
		{"[::1]:8765", "", true},
		{"0.0.0.0:8765", "", false},
		{":8765", "", false},
		{"192.168.1.10:8765", "", false},
		{"0.0.0.0:8765", "secret", true},
		{"no-port", "", false},
	}
	for _, tt := range tests {
		err := server.CheckListenAddr(tt.addr, tt.token)
		if (err == nil) != tt.ok {
			t.Errorf("CheckListenAddr(%q, %q) = %v, want ok=%v", tt.addr, tt.token, err, tt.ok)
		}
	}
}
