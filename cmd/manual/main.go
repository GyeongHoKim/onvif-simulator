// Command manual starts a pkgsite server for the current module and opens
// the package index directly in the default browser.
//
// Usage:
//
//	make manual
//	make manual DOCS_PORT=3000
package main

import (
	"context"
	"errors"
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

	// pkgsiteVersion pins the pkgsite tool version so `make manual` is reproducible.
	pkgsiteVersion = "v0.0.0-20260421174859-26eab2f0c5ff"
)

var errPkgsiteNotReady = errors.New("pkgsite server did not become ready")

func main() {
	port := flag.Int("port", defaultPort, "port to serve docs on")
	flag.Parse()

	ctx := context.Background()
	module := moduleFromGoMod()
	addr := fmt.Sprintf(":%d", *port)
	url := fmt.Sprintf("http://localhost:%d/%s", *port, module)

	// #nosec G204 -- pkgsite address is constructed from a validated port flag, not user input
	srv := exec.CommandContext(ctx, "go", "run", "golang.org/x/pkgsite/cmd/pkgsite@"+pkgsiteVersion, "-http="+addr, ".")
	srv.Stdout = os.Stdout
	srv.Stderr = os.Stderr
	if err := srv.Start(); err != nil {
		log.Fatalf("failed to start pkgsite: %v", err)
	}

	fmt.Printf("waiting for pkgsite on %s ...\n", addr)

	readyCh := make(chan error, 1)
	exitCh := make(chan error, 1)
	go func() { readyCh <- waitReady(ctx, addr) }()
	go func() { exitCh <- srv.Wait() }()

	select {
	case readyErr := <-readyCh:
		if readyErr != nil {
			log.Printf("aborting: %v", readyErr)
			if killErr := srv.Process.Kill(); killErr != nil {
				log.Printf("kill pkgsite: %v", killErr)
			}
			<-exitCh
			return
		}
		fmt.Printf("opening %s\n", url)
		openBrowser(ctx, url)
		if exitErr := <-exitCh; exitErr != nil {
			log.Printf("pkgsite exited: %v", exitErr)
		}
	case exitErr := <-exitCh:
		log.Printf("pkgsite exited unexpectedly: %v", exitErr)
	}
}

func waitReady(ctx context.Context, addr string) error {
	deadline := time.Now().Add(waitDeadline)
	dialer := &net.Dialer{Timeout: dialTimeout}
	for time.Now().Before(deadline) {
		conn, err := dialer.DialContext(ctx, "tcp", addr)
		if err == nil {
			if cerr := conn.Close(); cerr != nil {
				log.Printf("close probe conn: %v", cerr)
			}
			return nil
		}
		time.Sleep(pollInterval)
	}
	return fmt.Errorf("pkgsite on %s not ready after %s: %w", addr, waitDeadline, errPkgsiteNotReady)
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
