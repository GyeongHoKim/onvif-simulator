package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"

	"github.com/GyeongHoKim/onvif-simulator/internal/simulator"
)

// controlServer hosts a tiny loopback HTTP endpoint that the `event`
// subcommand POSTs to. It is unauthenticated and bound to 127.0.0.1 only.
type controlServer struct {
	sim    *simulator.Simulator
	port   int
	server *http.Server
}

func startControlServer(sim *simulator.Simulator) (*controlServer, error) {
	lc := &net.ListenConfig{}
	listener, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("control: listen: %w", err)
	}
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		_ = listener.Close() //nolint:errcheck // surfacing the addr error takes priority.
		return nil, errListenerNotTCP
	}

	c := &controlServer{sim: sim, port: addr.Port}

	mux := http.NewServeMux()
	mux.HandleFunc("/events/motion", c.handleMotion)
	mux.HandleFunc("/events/digital-input", c.handleDigitalInput)
	mux.HandleFunc("/events/image-too-blurry", c.handleImageTooBlurry)
	mux.HandleFunc("/events/image-too-dark", c.handleImageTooDark)
	mux.HandleFunc("/events/image-too-bright", c.handleImageTooBright)
	mux.HandleFunc("/events/sync", c.handleSync)

	c.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: controlServerTimeout,
	}
	go func() {
		_ = c.server.Serve(listener) //nolint:errcheck // exit is expected on Shutdown.
	}()
	return c, nil
}

func (c *controlServer) shutdown(ctx context.Context) error {
	return c.server.Shutdown(ctx)
}

type tokenState struct {
	Token string `json:"token"`
	State bool   `json:"state"`
}

type syncRequest struct {
	Topic          string `json:"topic"`
	SourceItemName string `json:"source_item_name"`
	SourceToken    string `json:"source_token"`
	DataItemName   string `json:"data_item_name"`
	State          bool   `json:"state"`
}

func (c *controlServer) handleMotion(w http.ResponseWriter, r *http.Request) {
	var req tokenState
	if !decodeOrFail(w, r, &req) {
		return
	}
	c.sim.Motion(req.Token, req.State)
	w.WriteHeader(http.StatusNoContent)
}

func (c *controlServer) handleDigitalInput(w http.ResponseWriter, r *http.Request) {
	var req tokenState
	if !decodeOrFail(w, r, &req) {
		return
	}
	c.sim.DigitalInput(req.Token, req.State)
	w.WriteHeader(http.StatusNoContent)
}

func (c *controlServer) handleImageTooBlurry(w http.ResponseWriter, r *http.Request) {
	var req tokenState
	if !decodeOrFail(w, r, &req) {
		return
	}
	c.sim.ImageTooBlurry(req.Token, req.State)
	w.WriteHeader(http.StatusNoContent)
}

func (c *controlServer) handleImageTooDark(w http.ResponseWriter, r *http.Request) {
	var req tokenState
	if !decodeOrFail(w, r, &req) {
		return
	}
	c.sim.ImageTooDark(req.Token, req.State)
	w.WriteHeader(http.StatusNoContent)
}

func (c *controlServer) handleImageTooBright(w http.ResponseWriter, r *http.Request) {
	var req tokenState
	if !decodeOrFail(w, r, &req) {
		return
	}
	c.sim.ImageTooBright(req.Token, req.State)
	w.WriteHeader(http.StatusNoContent)
}

func (c *controlServer) handleSync(w http.ResponseWriter, r *http.Request) {
	var req syncRequest
	if !decodeOrFail(w, r, &req) {
		return
	}
	c.sim.SyncProperty(req.Topic, req.SourceItemName, req.SourceToken, req.DataItemName, req.State)
	w.WriteHeader(http.StatusNoContent)
}

func decodeOrFail(w http.ResponseWriter, r *http.Request, dst any) bool {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return false
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}
