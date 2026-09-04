package sbx

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
)

type Version struct {
	Version    string `json:"version"`
	APIVersion int    `json:"api_version"`
}

type Status struct {
	Memory           uint64 `json:"memory"`
	Goroutines       int    `json:"goroutines"`
	ConnectionsIn    int    `json:"connections_in"`
	ConnectionsOut   int    `json:"connections_out"`
	TrafficAvailable bool   `json:"traffic_available"`
	Uplink           int64  `json:"uplink"`
	Downlink         int64  `json:"downlink"`
	UplinkTotal      int64  `json:"uplink_total"`
	DownlinkTotal    int64  `json:"downlink_total"`
}

type DeprecatedWarning struct {
	Message           string `json:"message"`
	Impending         bool   `json:"impending"`
	MigrationLink     string `json:"migration_link,omitempty"`
	Description       string `json:"description,omitempty"`
	DeprecatedVersion string `json:"deprecated_version,omitempty"`
	ScheduledVersion  string `json:"scheduled_version,omitempty"`
}

type ClashMode struct {
	Current string   `json:"current"`
	Modes   []string `json:"modes"`
}

type Outbound struct {
	Tag      string    `json:"tag"`
	Type     string    `json:"type"`
	Delay    int       `json:"delay"`
	TestedAt time.Time `json:"tested_at,omitzero"`
}

type Group struct {
	Tag        string     `json:"tag"`
	Type       string     `json:"type"`
	Selectable bool       `json:"selectable"`
	Selected   string     `json:"selected"`
	Items      []Outbound `json:"items"`
}

type LogLevel int

const (
	LevelPanic LogLevel = iota
	LevelFatal
	LevelError
	LevelWarn
	LevelInfo
	LevelDebug
	LevelTrace
)

func (l LogLevel) String() string {
	switch l {
	case LevelPanic:
		return "panic"
	case LevelFatal:
		return "fatal"
	case LevelError:
		return "error"
	case LevelWarn:
		return "warn"
	case LevelInfo:
		return "info"
	case LevelDebug:
		return "debug"
	case LevelTrace:
		return "trace"
	default:
		return fmt.Sprintf("LogLevel(%d)", l)
	}
}

func ParseLogLevel(s string) (LogLevel, error) {
	switch strings.ToLower(s) {
	case "panic":
		return LevelPanic, nil
	case "fatal":
		return LevelFatal, nil
	case "error":
		return LevelError, nil
	case "warn", "warning":
		return LevelWarn, nil
	case "info":
		return LevelInfo, nil
	case "debug":
		return LevelDebug, nil
	case "trace":
		return LevelTrace, nil
	default:
		return 0, fmt.Errorf("invalid log level %q", s)
	}
}

func (l LogLevel) MarshalText() ([]byte, error) {
	if l < LevelPanic || l > LevelTrace {
		return nil, fmt.Errorf("invalid log level %d", l)
	}
	return []byte(l.String()), nil
}

type LogEntry struct {
	Level   LogLevel `json:"level"`
	Message string   `json:"message"`
}

type LogBatch struct {
	Reset   bool
	Entries []LogEntry
}

func StripANSI(s string) string {
	return ansi.Strip(s)
}

type Process struct {
	PID          uint32   `json:"pid"`
	UserID       int      `json:"user_id"`
	UserName     string   `json:"user_name,omitempty"`
	Path         string   `json:"path,omitempty"`
	PackageNames []string `json:"package_names,omitempty"`
}

type Connection struct {
	ID            string    `json:"id"`
	Inbound       string    `json:"inbound"`
	InboundType   string    `json:"inbound_type"`
	IPVersion     int       `json:"ip_version"`
	Network       string    `json:"network"`
	Source        string    `json:"source"`
	Destination   string    `json:"destination"`
	Domain        string    `json:"domain,omitempty"`
	Protocol      string    `json:"protocol,omitempty"`
	User          string    `json:"user,omitempty"`
	FromOutbound  string    `json:"from_outbound,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	ClosedAt      time.Time `json:"closed_at,omitzero"`
	Uplink        int64     `json:"uplink"`
	Downlink      int64     `json:"downlink"`
	UplinkTotal   int64     `json:"uplink_total"`
	DownlinkTotal int64     `json:"downlink_total"`
	Rule          string    `json:"rule,omitempty"`
	Outbound      string    `json:"outbound"`
	OutboundType  string    `json:"outbound_type"`
	Chain         []string  `json:"chain"`
	Process       *Process  `json:"process,omitempty"`
}

type ConnectionEventType int

const (
	ConnectionNew ConnectionEventType = iota
	ConnectionUpdate
	ConnectionClosed
)

type ConnectionEvent struct {
	Type          ConnectionEventType
	ID            string
	Connection    *Connection
	UplinkDelta   int64
	DownlinkDelta int64
	ClosedAt      time.Time
}

type ConnectionBatch struct {
	Reset  bool
	Events []ConnectionEvent
}

type ServiceStatus struct {
	State string
	Error string
}
