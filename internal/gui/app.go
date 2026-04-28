package gui

import (
	"context"
	"errors"
	"io/fs"
	"log"
	"time"

	runtime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/GyeongHoKim/onvif-simulator/internal/config"
	"github.com/GyeongHoKim/onvif-simulator/internal/logger"
)

// EventRecord mirrors simulator.EventRecord. Kept local so the GUI compiles
// before internal/simulator exists.
type EventRecord struct {
	Time    time.Time `json:"time"`
	Topic   string    `json:"topic"`
	Source  string    `json:"source"`
	Payload string    `json:"payload"`
}

// MutationRecord mirrors simulator.MutationRecord.
type MutationRecord struct {
	Time   time.Time `json:"time"`
	Kind   string    `json:"kind"`
	Target string    `json:"target"`
	Detail string    `json:"detail"`
}

// Status mirrors simulator.Status.
type Status struct {
	Running        bool          `json:"running"`
	ListenAddr     string        `json:"listenAddr"`
	StartedAt      time.Time     `json:"startedAt"`
	Uptime         time.Duration `json:"uptime"`
	DiscoveryMode  string        `json:"discoveryMode"`
	ProfileCount   int           `json:"profileCount"`
	TopicCount     int           `json:"topicCount"`
	UserCount      int           `json:"userCount"`
	ActivePullSubs int           `json:"activePullSubs"`
	RecentEvents   []EventRecord `json:"recentEvents"`
}

// UserView mirrors simulator.UserView.
type UserView struct {
	Username string   `json:"username"`
	Roles    []string `json:"roles"`
}

// LogEntry is one row of the on-disk log surfaced to the GUI. Kind
// discriminates the populated fields ("event" → Topic/Source/Payload,
// "mutation" → Op/Target/Detail) and matches the TS discriminated union
// rendered by the Log screen.
type LogEntry struct {
	Time    time.Time `json:"time"`
	Kind    string    `json:"kind"`
	Topic   string    `json:"topic,omitempty"`
	Source  string    `json:"source,omitempty"`
	Payload string    `json:"payload,omitempty"`
	Op      string    `json:"op,omitempty"`
	Target  string    `json:"target,omitempty"`
	Detail  string    `json:"detail,omitempty"`
}

// LogPage is the GetLogs response — a slice of entries newest-first plus
// the total currently on disk so the frontend can show a counter.
type LogPage struct {
	Entries []LogEntry `json:"entries"`
	Total   int        `json:"total"`
}

// simulatorAPI is the subset of *simulator.Simulator the GUI consumes. Both
// the real *simulator.Simulator and the in-memory fake satisfy this interface
// — the swap is one line in NewApp. Pointer parameters keep the GUI lint
// clean (gocritic hugeParam) without changing the upstream backend signatures.
type simulatorAPI interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Running() bool
	Status() Status

	ConfigSnapshot() config.Config
	Users() []UserView

	Motion(sourceToken string, state bool)
	ImageTooBlurry(sourceToken string, state bool)
	ImageTooDark(sourceToken string, state bool)
	ImageTooBright(sourceToken string, state bool)
	DigitalInput(inputToken string, logicalState bool)
	SyncProperty(topic, sourceItemName, sourceToken, dataItemName string, state bool)
	PublishRaw(topic, message string)

	SetDiscoveryMode(mode string) error
	SetHostname(name string) error

	AddProfile(p *config.ProfileConfig) error
	RemoveProfile(token string) error
	SetProfileMediaFilePath(token, path string) error
	SetProfileSnapshotURI(token, uri string) error

	SetTopicEnabled(name string, enabled bool) error
	SetEventsTopics(topics []config.TopicConfig) error

	AddUser(u *config.UserConfig) error
	UpsertUser(u *config.UserConfig) error
	RemoveUser(username string) error
	SetAuthEnabled(enabled bool) error
}

// App is the Wails binding surface. Every exported method is callable from
// the frontend via window.go.main.App.<Name>. Methods MUST use Go types that
// Wails can marshal — primitives, []T, and plain structs from internal/config
// or this package.
type App struct {
	ctx    context.Context
	sim    simulatorAPI
	logger *logger.Logger
}

// NewApp constructs the Wails app with its simulator backend. When a config
// file is present in the working directory we wire the real
// *simulator.Simulator; otherwise we fall back to the in-memory stub so
// `wails dev` boots even on a fresh checkout without a configured device.
func NewApp() *App {
	app := &App{}

	lg, err := logger.New(logger.Options{})
	if err != nil {
		log.Printf("onvif-simulator: logger disabled: %v", err)
		lg = logger.NoOp()
	}
	app.logger = lg

	emitEvent := func(r EventRecord) {
		app.logger.Write(logger.Entry{
			Time:    r.Time,
			Kind:    "event",
			Topic:   r.Topic,
			Source:  r.Source,
			Payload: r.Payload,
		})
	}
	emitMutation := func(r MutationRecord) {
		app.logger.Write(logger.Entry{
			Time:   r.Time,
			Kind:   "mutation",
			Op:     r.Kind,
			Target: r.Target,
			Detail: r.Detail,
		})
	}

	adapter, adapterErr := newSimulatorAdapter("", emitEvent, emitMutation)
	if adapterErr == nil {
		app.sim = adapter
		return app
	}
	if !errors.Is(adapterErr, fs.ErrNotExist) {
		log.Printf("onvif-simulator: config error: %v", adapterErr)
	}

	stub := newSimulatorStub(emitEvent, emitMutation)
	app.sim = stub
	return app
}

// OnStartup captures the Wails runtime context. Currently unused — kept so
// future Wails-runtime calls (dialogs, clipboard) have a context handle.
func (a *App) OnStartup(ctx context.Context) {
	a.ctx = ctx
}

// OnShutdown drains the on-disk logger so no entries are lost on app exit.
func (a *App) OnShutdown(_ context.Context) {
	if a.logger == nil {
		return
	}
	if err := a.logger.Close(); err != nil {
		log.Printf("onvif-simulator: logger close: %v", err)
	}
}

// Lifecycle ---------------------------------------------------------------

// Start boots the simulator.
func (a *App) Start() error { return a.sim.Start(a.ctx) }

// Stop shuts the simulator down.
func (a *App) Stop() error { return a.sim.Stop(a.ctx) }

// Running reports whether the simulator is live.
func (a *App) Running() bool { return a.sim.Running() }

// Status returns a dashboard snapshot.
func (a *App) Status() Status { return a.sim.Status() }

// Reads -------------------------------------------------------------------

// ConfigSnapshot returns a deep copy of the on-disk configuration.
func (a *App) ConfigSnapshot() config.Config { return a.sim.ConfigSnapshot() }

// Users returns the live auth user store projection.
func (a *App) Users() []UserView { return a.sim.Users() }

// Event triggers ----------------------------------------------------------

// Motion publishes tns1:VideoSource/MotionAlarm.
func (a *App) Motion(sourceToken string, state bool) { a.sim.Motion(sourceToken, state) }

// ImageTooBlurry publishes tns1:VideoSource/ImageTooBlurry.
func (a *App) ImageTooBlurry(sourceToken string, state bool) {
	a.sim.ImageTooBlurry(sourceToken, state)
}

// ImageTooDark publishes tns1:VideoSource/ImageTooDark.
func (a *App) ImageTooDark(sourceToken string, state bool) { a.sim.ImageTooDark(sourceToken, state) }

// ImageTooBright publishes tns1:VideoSource/ImageTooBright.
func (a *App) ImageTooBright(sourceToken string, state bool) {
	a.sim.ImageTooBright(sourceToken, state)
}

// DigitalInput publishes tns1:Device/Trigger/DigitalInput.
func (a *App) DigitalInput(inputToken string, logicalState bool) {
	a.sim.DigitalInput(inputToken, logicalState)
}

// SyncProperty re-emits the "Initialized" notification for a topic.
func (a *App) SyncProperty(topic, sourceItemName, sourceToken, dataItemName string, state bool) {
	a.sim.SyncProperty(topic, sourceItemName, sourceToken, dataItemName, state)
}

// PublishRaw is the raw-XML escape hatch for topics without a typed helper.
func (a *App) PublishRaw(topic, message string) { a.sim.PublishRaw(topic, message) }

// Config mutators ---------------------------------------------------------

// SetDiscoveryMode persists the WS-Discovery mode.
func (a *App) SetDiscoveryMode(mode string) error { return a.sim.SetDiscoveryMode(mode) }

// SetHostname persists the device hostname.
func (a *App) SetHostname(name string) error { return a.sim.SetHostname(name) }

// AddProfile appends a new media profile. The frontend sends a value object;
// we forward by pointer so the simulator avoids copying the heavy struct.
//
//nolint:gocritic // value param required for Wails JSON unmarshalling
func (a *App) AddProfile(p config.ProfileConfig) error { return a.sim.AddProfile(&p) }

// RemoveProfile removes a media profile by token.
func (a *App) RemoveProfile(token string) error { return a.sim.RemoveProfile(token) }

// SetProfileMediaFile points the named profile at a local mp4 file. The
// embedded RTSP server reads and loops the file so GetStreamUri returns a
// URI pointing at the simulator itself.
func (a *App) SetProfileMediaFile(token, path string) error {
	return a.sim.SetProfileMediaFilePath(token, path)
}

// PickMediaFile opens the OS-native open-file dialog, filters for mp4/mov
// containers, and returns the absolute path the user selected. Returns an
// empty string (and nil error) when the user cancels — same convention as
// runtime.OpenFileDialog.
func (a *App) PickMediaFile() (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select an MP4 file",
		Filters: []runtime.FileFilter{
			{DisplayName: "Video files (*.mp4, *.mov)", Pattern: "*.mp4;*.mov"},
		},
	})
}

// SetProfileSnapshotURI updates the pass-through snapshot URI for a profile.
func (a *App) SetProfileSnapshotURI(token, uri string) error {
	return a.sim.SetProfileSnapshotURI(token, uri)
}

// SetTopicEnabled toggles advertisement + publish-routing for a topic.
func (a *App) SetTopicEnabled(name string, enabled bool) error {
	return a.sim.SetTopicEnabled(name, enabled)
}

// SetEventsTopics replaces the full topic list.
func (a *App) SetEventsTopics(topics []config.TopicConfig) error {
	return a.sim.SetEventsTopics(topics)
}

// AddUser creates a new auth user.
func (a *App) AddUser(u config.UserConfig) error { return a.sim.AddUser(&u) }

// UpsertUser creates or updates an auth user.
func (a *App) UpsertUser(u config.UserConfig) error { return a.sim.UpsertUser(&u) }

// RemoveUser deletes an auth user.
func (a *App) RemoveUser(username string) error { return a.sim.RemoveUser(username) }

// SetAuthEnabled toggles the authentication scheme chain.
func (a *App) SetAuthEnabled(enabled bool) error { return a.sim.SetAuthEnabled(enabled) }

// Log surface ------------------------------------------------------------

// GetLogs returns up to limit entries (newest-first) starting at offset,
// plus the total number of entries currently on disk.
func (a *App) GetLogs(offset, limit int) (LogPage, error) {
	entries, total, err := a.logger.Read(offset, limit)
	if err != nil {
		return LogPage{Entries: []LogEntry{}}, err
	}
	out := make([]LogEntry, len(entries))
	for i := range entries {
		e := &entries[i]
		out[i] = LogEntry{
			Time:    e.Time,
			Kind:    e.Kind,
			Topic:   e.Topic,
			Source:  e.Source,
			Payload: e.Payload,
			Op:      e.Op,
			Target:  e.Target,
			Detail:  e.Detail,
		}
	}
	return LogPage{Entries: out, Total: total}, nil
}

// ClearLogs rotates the active log file. The previous content is preserved
// as the next backup so the user retains an audit trail.
func (a *App) ClearLogs() error { return a.logger.Clear() }
