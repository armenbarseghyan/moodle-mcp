// Command mcpcall calls one moodle-mcp tool from the shell, through the real
// server binary over stdio. Development only (skill evals, manual checks).
//
//	mcpcall -list
//	mcpcall moodle_deadlines '{"days": 7}'
//
// The server binary is $MOODLE_MCP_BIN (default bin/moodle-mcp) and gets the
// current environment (MOODLE_URL, MOODLE_TOKEN, ...).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	list := flag.Bool("list", false, "list tools with their descriptions and input schemas")
	flag.Parse()
	if err := run(*list, flag.Args()); err != nil {
		fmt.Fprintln(os.Stderr, "mcpcall:", err)
		os.Exit(1)
	}
}

func run(list bool, args []string) error {
	bin := os.Getenv("MOODLE_MCP_BIN")
	if bin == "" {
		bin = "bin/moodle-mcp"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin) //nolint:gosec // dev tool: runs the server binary the developer points it at
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "mcpcall"}, nil).Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		return err
	}
	defer cs.Close()

	if list {
		res, err := cs.ListTools(ctx, nil)
		if err != nil {
			return err
		}
		for _, t := range res.Tools {
			schema, _ := json.Marshal(t.InputSchema)
			_, _ = fmt.Fprintf(os.Stdout, "## %s\n%s\ninput: %s\n\n", t.Name, t.Description, schema)
		}
		return nil
	}
	if len(args) == 0 {
		return fmt.Errorf("usage: mcpcall -list | mcpcall <tool> ['{json args}']")
	}
	params := map[string]any{}
	if len(args) > 1 && strings.TrimSpace(args[1]) != "" {
		if err := json.Unmarshal([]byte(args[1]), &params); err != nil {
			return fmt.Errorf("arguments must be a JSON object: %w", err)
		}
	}
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: args[0], Arguments: params})
	if err != nil {
		return err
	}
	for _, c := range res.Content {
		if t, ok := c.(*mcp.TextContent); ok {
			_, _ = fmt.Fprint(os.Stdout, t.Text)
		}
	}
	if res.IsError {
		_, _ = fmt.Fprintln(os.Stdout, "\n(tool returned an error)")
	}
	return nil
}
