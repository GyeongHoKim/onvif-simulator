package tui

import (
	"context"
	"time"

	"github.com/GyeongHoKim/onvif-simulator/internal/config"
)

// SimulatorAPI is the subset of *simulator.Simulator that the TUI consumes.
// It mirrors the contract in doc/design/simulator-api.md so the TUI compiles
// independently of backend-dev's in-progress implementation and substitutes a
// zero-cost adapter once the real simulator package is wired in via Run.
type SimulatorAPI interface {
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

	AddProfile(p config.ProfileConfig) error
	RemoveProfile(token string) error
	SetProfileRTSP(token, rtsp string) error
	SetProfileMediaFilePath(token, path string) error
	SetProfileSnapshotURI(token, uri string) error
	SetProfileEncoder(token, encoding string, width, height, fps, bitrate, gop int) error

	SetTopicEnabled(name string, enabled bool) error

	AddUser(u config.UserConfig) error
	UpsertUser(u config.UserConfig) error
	RemoveUser(username string) error
	SetAuthEnabled(enabled bool) error
}

// EventRecord is one line of the recent-events ring buffer surfaced by Status.
type EventRecord struct {
	Time    time.Time
	Topic   string
	Source  string
	Payload string
}

// MutationRecord summarizes a persisted config mutation for the Log screen.
type MutationRecord struct {
	Time   time.Time
	Kind   string
	Target string
	Detail string
}

// Status is the low-cost snapshot the Dashboard polls each tick.
type Status struct {
	Running        bool
	ListenAddr     string
	StartedAt      time.Time
	Uptime         time.Duration
	DiscoveryMode  string
	ProfileCount   int
	TopicCount     int
	UserCount      int
	ActivePullSubs int
	RecentEvents   []EventRecord
}

// UserView is the safe-to-render projection of an auth user record.
type UserView struct {
	Username string
	Roles    []string
}
