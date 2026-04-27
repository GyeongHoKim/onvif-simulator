// Command onvif-simulator is the CLI entry point for the simulator.
//
// Subcommands:
//
//	serve                    start simulator + loopback control server
//	tui                      start the TUI (delegates to internal/tui.Run)
//	config show              print the loaded config as JSON
//	config validate          validate the config file and exit
//	event <topic> <args...>  trigger an event via the loopback control port
//
// Running without arguments is equivalent to `serve`.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/GyeongHoKim/onvif-simulator/internal/config"
	"github.com/GyeongHoKim/onvif-simulator/internal/simulator"
	"github.com/GyeongHoKim/onvif-simulator/internal/tui"
)

const (
	shutdownTimeout      = 5 * time.Second
	controlServerTimeout = 5 * time.Second
	httpFaultThreshold   = 400
	dirMode              = 0o700
	fileMode             = 0o600
)

var (
	errUnknownSubcommand       = errors.New("unknown subcommand")
	errConfigSubcommandReq     = errors.New("config: subcommand required (show|validate)")
	errConfigUnknownSubcommand = errors.New("config: unknown subcommand")
	errEventSubcommandReq      = errors.New("event: subcommand required")
	errEventUnknownSubcommand  = errors.New("event: unknown subcommand")
	errEventUsageSimple        = errors.New("event: usage <token> on|off")
	errEventUsageSync          = errors.New("event sync: usage <topic> <source-item-name> " +
		"<source-token> <data-item-name> <state>")
	errUnrecognisedOnOff = errors.New("unrecognized on/off value")
	errControlServer     = errors.New("control server returned error")
	errControlNotRunning = errors.New("control: simulator does not appear to be running")
	errListenerNotTCP    = errors.New("control: listener returned non-TCP address")
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return runServe(nil)
	}
	switch args[0] {
	case "serve":
		return runServe(args[1:])
	case "tui":
		return runTUI(args[1:])
	case "config":
		return runConfig(args[1:])
	case "event":
		return runEvent(args[1:])
	case "-h", "--help", "help":
		printUsage(os.Stdout)
		return nil
	default:
		printUsage(os.Stderr)
		return fmt.Errorf("%w: %q", errUnknownSubcommand, args[0])
	}
}

func printUsage(w io.Writer) {
	lines := []string{
		"usage: onvif-simulator <command> [args]",
		"commands:",
		"  serve                          run simulator + loopback control server (default)",
		"  tui                            run the terminal UI",
		"  config show                    print the loaded config JSON",
		"  config validate                validate the config file and exit",
		"  event motion <token> on|off",
		"  event digital-input <token> on|off",
		"  event image-too-blurry <token> on|off",
		"  event image-too-dark <token> on|off",
		"  event image-too-bright <token> on|off",
		"  event sync <topic> <source-item-name> <source-token> <data-item-name> <state>",
	}
	_, _ = io.WriteString(w, strings.Join(lines, "\n")+"\n") //nolint:errcheck // usage output is best-effort.
}

// ---------- serve ---------------------------------------------------------------

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	cfgPath := fs.String("config", "", "path to onvif-simulator.json (overrides working directory default)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	opts := simulator.Options{
		ConfigPath: *cfgPath,
		OnEvent: func(e simulator.EventRecord) {
			fmt.Printf("[event] %s topic=%s source=%s payload=%s\n",
				e.Time.Format(time.RFC3339), e.Topic, e.Source, e.Payload)
		},
		OnMutation: func(m simulator.MutationRecord) {
			fmt.Printf("[mutation] %s kind=%s target=%s detail=%s\n",
				m.Time.Format(time.RFC3339), m.Kind, m.Target, m.Detail)
		},
	}
	sim, err := simulator.New(opts)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if startErr := sim.Start(ctx); startErr != nil {
		return startErr
	}

	status := sim.Status()
	fmt.Printf("listening on %s\n", status.ListenAddr)

	ctrl, ctrlErr := startControlServer(sim)
	if ctrlErr != nil {
		_ = sim.Stop(context.Background()) //nolint:errcheck // cleanup after failed start.
		return ctrlErr
	}
	fmt.Printf("control port %d\n", ctrl.port)

	portFilePath, writeErr := writeControlPortFile(ctrl.port)
	if writeErr != nil {
		fmt.Fprintf(os.Stderr, "warning: could not write control port file: %v\n", writeErr)
	}
	defer func() {
		if portFilePath != "" {
			_ = os.Remove(portFilePath) //nolint:errcheck // best-effort cleanup.
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	fmt.Println("\nshutting down")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()

	_ = ctrl.shutdown(shutdownCtx) //nolint:errcheck // best-effort loopback shutdown.
	return sim.Stop(shutdownCtx)
}

// ---------- tui -----------------------------------------------------------------

func runTUI(args []string) error {
	fs := flag.NewFlagSet("tui", flag.ContinueOnError)
	cfgPath := fs.String("config", "", "path to onvif-simulator.json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	sim, err := simulator.New(simulator.Options{ConfigPath: *cfgPath})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := sim.Start(ctx); err != nil {
		return err
	}
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer shutdownCancel()
		_ = sim.Stop(shutdownCtx) //nolint:errcheck // best-effort shutdown after TUI exits.
	}()
	return tui.Run(sim)
}

// ---------- config --------------------------------------------------------------

func runConfig(args []string) error {
	if len(args) == 0 {
		return errConfigSubcommandReq
	}
	sub := args[0]
	// Reject unknown subcommands BEFORE config.EnsureExists, otherwise a
	// typo'd subcommand would still create the user config directory and
	// write a default file.
	if sub != "show" && sub != "validate" {
		return fmt.Errorf("%w: %q", errConfigUnknownSubcommand, sub)
	}
	fs := flag.NewFlagSet("config "+sub, flag.ContinueOnError)
	cfgPath := fs.String("config", "", "path to onvif-simulator.json (defaults to user config dir)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	resolved, err := config.Resolve(*cfgPath)
	if err != nil {
		return err
	}
	// Same ordering rationale as simulator.New: EnsureExists takes the
	// path explicitly, so run it first and only mutate the package-level
	// active path on success.
	if _, err := config.EnsureExists(resolved); err != nil {
		return err
	}
	config.SetPath(resolved)

	switch sub {
	case "show":
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(cfg)
	case "validate":
		if _, err := config.Load(); err != nil {
			return err
		}
		fmt.Println("ok")
		return nil
	}
	// Unreachable: sub is guaranteed to be show or validate by the guard above.
	return nil
}

// ---------- event ---------------------------------------------------------------

type eventBody struct {
	Token          string `json:"token,omitempty"`
	State          bool   `json:"state"`
	Topic          string `json:"topic,omitempty"`
	SourceItemName string `json:"source_item_name,omitempty"`
	SourceToken    string `json:"source_token,omitempty"`
	DataItemName   string `json:"data_item_name,omitempty"`
}

func runEvent(args []string) error {
	if len(args) == 0 {
		return errEventSubcommandReq
	}
	port, err := readControlPortFile()
	if err != nil {
		return fmt.Errorf("%w: %w", errControlNotRunning, err)
	}
	sub := args[0]
	rest := args[1:]
	switch sub {
	case "motion", "digital-input", "image-too-blurry", "image-too-dark", "image-too-bright":
		return postSimpleEvent(port, sub, rest)
	case "sync":
		return postSyncEvent(port, rest)
	default:
		return fmt.Errorf("%w: %q", errEventUnknownSubcommand, sub)
	}
}

func postSimpleEvent(port int, path string, args []string) error {
	const wantArgs = 2
	if len(args) != wantArgs {
		return fmt.Errorf("%w: %s", errEventUsageSimple, path)
	}
	state, err := parseOnOff(args[1])
	if err != nil {
		return err
	}
	body, err := json.Marshal(eventBody{Token: args[0], State: state})
	if err != nil {
		return err
	}
	return postControl(port, "/events/"+path, body)
}

func postSyncEvent(port int, args []string) error {
	const wantArgs = 5
	if len(args) != wantArgs {
		return errEventUsageSync
	}
	state, err := parseOnOff(args[4])
	if err != nil {
		return err
	}
	body, err := json.Marshal(eventBody{
		Topic:          args[0],
		SourceItemName: args[1],
		SourceToken:    args[2],
		DataItemName:   args[3],
		State:          state,
	})
	if err != nil {
		return err
	}
	return postControl(port, "/events/sync", body)
}

func parseOnOff(s string) (bool, error) {
	switch strings.ToLower(s) {
	case "on", "true", "1":
		return true, nil
	case "off", "false", "0":
		return false, nil
	default:
		return false, fmt.Errorf("%w: %q", errUnrecognisedOnOff, s)
	}
}

func postControl(port int, path string, body []byte) error {
	reqURL := "http://127.0.0.1:" + strconv.Itoa(port) + path
	ctx, cancel := context.WithTimeout(context.Background(), controlServerTimeout)
	defer cancel()
	//nolint:gosec // reqURL targets the loopback control port written by this same process.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	//nolint:gosec // HTTP destination is the loopback control port.
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }() //nolint:errcheck // body close error is not actionable.
	if resp.StatusCode >= httpFaultThreshold {
		msg, _ := io.ReadAll(resp.Body) //nolint:errcheck // body read error is subordinate to the HTTP status.
		return fmt.Errorf("%w: %d: %s", errControlServer, resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	return nil
}

// ---------- control port file --------------------------------------------------

func controlPortFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".onvif-simulator", "control.port"), nil
}

func writeControlPortFile(port int) (string, error) {
	path, err := controlPortFilePath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		return "", err
	}
	return path, os.WriteFile(path, []byte(strconv.Itoa(port)+"\n"), fileMode)
}

func readControlPortFile() (int, error) {
	path, err := controlPortFilePath()
	if err != nil {
		return 0, err
	}
	data, err := os.ReadFile(path) //nolint:gosec // trusted path under user home.
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}
