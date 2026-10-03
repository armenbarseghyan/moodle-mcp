// Command fakemoodle serves the test fixture world (moodletest.World) over
// HTTP, for trying moodle-mcp and its skills without touching the real site.
// Development only; it is not part of the server.
//
//	go run ./dev/fakemoodle -url-file /tmp/fake.url
//
// The URL is printed on stdout (and written to -url-file). Use it as
// MOODLE_URL with MOODLE_TOKEN set to moodletest.Token; any other token gets
// the real "invalidtoken" answer.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/armenbarseghyan/moodle-mcp/internal/moodletest"
)

type logReporter struct{}

func (logReporter) Helper()                           {}
func (logReporter) Errorf(format string, args ...any) { log.Printf("fakemoodle: "+format, args...) }
func (logReporter) Fatalf(format string, args ...any) { log.Fatalf("fakemoodle: "+format, args...) }
func (logReporter) Cleanup(func())                    {}

func main() {
	urlFile := flag.String("url-file", "", "also write the server URL to this file")
	printToken := flag.Bool("token", false, "print the accepted token and exit")
	printCreds := flag.Bool("creds", false, "print the accepted username and password (two lines) and exit")
	flag.Parse()
	if *printCreds {
		_, _ = fmt.Fprintln(os.Stdout, moodletest.Username)
		_, _ = fmt.Fprintln(os.Stdout, moodletest.Password)
		return
	}
	if *printToken {
		_, _ = fmt.Fprintln(os.Stdout, moodletest.Token)
		return
	}
	r := logReporter{}
	srv := moodletest.New(r, moodletest.World(r)...)
	defer srv.Close()
	_, _ = fmt.Fprintln(os.Stdout, srv.URL)
	if *urlFile != "" {
		if err := os.WriteFile(*urlFile, []byte(srv.URL), 0o600); err != nil {
			srv.Close()
			log.Fatal(err) //nolint:gocritic // server closed explicitly above
		}
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
}
