// Command manual starts a pkgsite server for the current module and opens
// the package index directly in the default browser.
//
// Usage (via make):
//
//	make manual
//	make manual DOCS_PORT=3000
//
// Run directly:
//
//	go run ./cmd/manual -port 3000
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const (
	defaultPort  = 8080
	waitDeadline = 30 * time.Second
	pollInterval = 300 * time.Millisecond
	dialTimeout  = time.Second
)

func main() {
	port := flag.Int("port", defaultPort, "port to serve docs on")
	flag.Parse()

	ctx := context.Background()
	module := moduleFromGoMod()
	addr := fmt.Sprintf(":%d", *port)
	url := fmt.Sprintf("http://localhost:%d/%s", *port, module)

	// #nosec G204 -- pkgsite address is constructed from a validated port flag, not user input
	srv := exec.CommandContext(ctx, "go", "run", "golang.org/x/pkgsite/cmd/pkgsite@latest", "-http="+addr, ".")
	srv.Stdout = os.Stdout
	srv.Stderr = os.Stderr
	if err := srv.Start(); err != nil {
		log.Fatalf("failed to start pkgsite: %v", err)
	}

	fmt.Printf("waiting for pkgsite on %s ...\n", addr)
	waitReady(ctx, addr)
	fmt.Printf("opening %s\n", url)
	openBrowser(ctx, url)

	if err := srv.Wait(); err != nil {
		log.Printf("pkgsite exited: %v", err)
	}
}

func waitReady(ctx context.Context, addr string) {
	deadline := time.Now().Add(waitDeadline)
	dialer := &net.Dialer{Timeout: dialTimeout}
	for time.Now().Before(deadline) {
		conn, err := dialer.DialContext(ctx, "tcp", addr)
		if err == nil {
			if cerr := conn.Close(); cerr != nil {
				log.Printf("close probe conn: %v", cerr)
			}
			return
		}
		time.Sleep(pollInterval)
	}
}

func openBrowser(ctx context.Context, url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// #nosec G204 -- fixed executable, only the URL argument varies
		cmd = exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		// #nosec G204 -- fixed executable, only the URL argument varies
		cmd = exec.CommandContext(ctx, "open", url)
	default:
		// #nosec G204 -- fixed executable, only the URL argument varies
		cmd = exec.CommandContext(ctx, "xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		fmt.Printf("could not open browser: open %s manually\n", url)
	}
}

func moduleFromGoMod() string {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		log.Fatalf("read go.mod: %v", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	log.Fatal("module directive not found in go.mod")
	return ""
}
