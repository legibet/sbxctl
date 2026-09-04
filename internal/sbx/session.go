package sbx

import (
	"context"
	"errors"
	"sync"
	"time"
)

type Stream int

const (
	StreamServiceStatus Stream = iota
	StreamStatus
	StreamGroups
	StreamClashMode
	StreamOutbounds
	StreamConnections
	StreamLogs
)

type ConnState int

const (
	StateConnecting ConnState = iota
	StateConnected
	StateReconnecting
	StateFailed
)

type ServerInfo struct {
	Version         Version
	StartedAt       time.Time
	DefaultLogLevel LogLevel
}

type Event interface{ sessionEvent() }

type ConnEvent struct {
	State   ConnState
	Attempt int
	Info    ServerInfo
	Err     error
}

type ServiceEvent struct{ Status ServiceStatus }
type StatusEvent struct{ Status Status }
type GroupsEvent struct{ Groups []Group }
type OutboundsEvent struct{ Outbounds []Outbound }
type ClashModeEvent struct {
	Mode  string
	Modes []string
}
type ConnectionsEvent struct{ Batch ConnectionBatch }
type LogsEvent struct{ Batch LogBatch }
type UnavailableEvent struct {
	Stream Stream
	Err    error
}

func (*ConnEvent) sessionEvent()        {}
func (*ServiceEvent) sessionEvent()     {}
func (*StatusEvent) sessionEvent()      {}
func (*GroupsEvent) sessionEvent()      {}
func (*OutboundsEvent) sessionEvent()   {}
func (*ClashModeEvent) sessionEvent()   {}
func (*ConnectionsEvent) sessionEvent() {}
func (*LogsEvent) sessionEvent()        {}
func (*UnavailableEvent) sessionEvent() {}

type streamRun struct {
	id     uint64
	cancel context.CancelFunc
}

type Session struct {
	client *Client
	ctx    context.Context
	cancel context.CancelFunc
	events chan Event

	startOnce sync.Once
	closeOnce sync.Once
	wg        sync.WaitGroup

	mu           sync.Mutex
	closed       bool
	connected    bool
	optional     map[Stream]bool
	streams      map[Stream]streamRun
	nextStreamID uint64
	wake         chan struct{}
	backoffStart time.Duration
	backoffMax   time.Duration
}

func NewSession(ep Endpoint) (*Session, error) {
	client, err := Dial(ep)
	if err != nil {
		return nil, err
	}
	return newSession(client), nil
}

func newSession(client *Client) *Session {
	ctx, cancel := context.WithCancel(context.Background())
	return &Session{
		client:       client,
		ctx:          ctx,
		cancel:       cancel,
		events:       make(chan Event, 64),
		optional:     make(map[Stream]bool),
		streams:      make(map[Stream]streamRun),
		wake:         make(chan struct{}),
		backoffStart: time.Second,
		backoffMax:   10 * time.Second,
	}
}

func (s *Session) Client() *Client {
	return s.client
}

func (s *Session) Events() <-chan Event {
	return s.events
}

func (s *Session) Start() {
	s.startOnce.Do(func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.closed {
			return
		}
		s.wg.Add(1)
		go s.connectLoop()
	})
}

func (s *Session) SetStream(stream Stream, enabled bool) {
	if stream != StreamOutbounds && stream != StreamConnections && stream != StreamLogs {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.optional[stream] == enabled {
		return
	}
	s.optional[stream] = enabled
	if !enabled {
		if running, ok := s.streams[stream]; ok {
			running.cancel()
			delete(s.streams, stream)
		}
		return
	}
	if s.connected {
		s.startStreamLocked(stream)
	}
}

func (s *Session) Reconnect() {
	s.mu.Lock()
	close(s.wake)
	s.wake = make(chan struct{})
	s.mu.Unlock()
}

func (s *Session) Close() {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()
		s.cancel()
		s.wg.Wait()
		close(s.events)
		_ = s.client.Close()
	})
}

func (s *Session) connectLoop() {
	defer s.wg.Done()
	attempt := 0
	delay := s.backoffStart
	for {
		info, err := s.serverInfo(s.ctx)
		if err == nil {
			s.emit(&ConnEvent{State: StateConnected, Info: info})
			s.mu.Lock()
			s.connected = true
			for _, stream := range []Stream{StreamServiceStatus, StreamStatus, StreamGroups, StreamClashMode} {
				s.startStreamLocked(stream)
			}
			for _, stream := range []Stream{StreamOutbounds, StreamConnections, StreamLogs} {
				if s.optional[stream] {
					s.startStreamLocked(stream)
				}
			}
			s.mu.Unlock()
			return
		}
		if s.ctx.Err() != nil {
			return
		}
		if kind := KindOf(err); kind == KindAuth || kind == KindIncompatible {
			s.emit(&ConnEvent{State: StateFailed, Err: err})
			return
		}
		attempt++
		s.emit(&ConnEvent{State: StateReconnecting, Attempt: attempt, Err: err})
		if !s.wait(s.ctx, delay) {
			return
		}
		delay = min(delay*2, s.backoffMax)
	}
}

func (s *Session) serverInfo(ctx context.Context) (ServerInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	version, err := s.client.CheckVersion(ctx)
	if err != nil {
		return ServerInfo{}, err
	}
	startedAt, err := s.client.StartedAt(ctx)
	if err != nil {
		return ServerInfo{}, err
	}
	level, err := s.client.DefaultLogLevel(ctx)
	if err != nil {
		return ServerInfo{}, err
	}
	return ServerInfo{Version: version, StartedAt: startedAt, DefaultLogLevel: level}, nil
}

func (s *Session) startStreamLocked(stream Stream) {
	if _, ok := s.streams[stream]; ok || s.ctx.Err() != nil {
		return
	}
	ctx, cancel := context.WithCancel(s.ctx)
	s.nextStreamID++
	id := s.nextStreamID
	s.streams[stream] = streamRun{id: id, cancel: cancel}
	s.wg.Add(1)
	go s.runStream(ctx, stream, id)
}

func (s *Session) runStream(ctx context.Context, stream Stream, id uint64) {
	defer s.wg.Done()
	defer func() {
		s.mu.Lock()
		if running, ok := s.streams[stream]; ok && running.id == id {
			delete(s.streams, stream)
		}
		s.mu.Unlock()
	}()

	attempt := 0
	delay := s.backoffStart
	modesLoaded := false
	for {
		if stream == StreamClashMode && !modesLoaded {
			mode, err := s.clashMode(ctx)
			if err != nil {
				if !s.retryStream(ctx, stream, &attempt, &delay, err) {
					return
				}
				continue
			}
			modesLoaded = true
			delay = s.backoffStart
			s.emit(&ClashModeEvent{Mode: mode.Current, Modes: mode.Modes})
		}

		delivered := false
		err := s.watch(ctx, stream, func(event Event) error {
			if stream == StreamServiceStatus && attempt > 0 && !delivered {
				info, err := s.serverInfo(ctx)
				if err != nil {
					return err
				}
				s.emit(&ConnEvent{State: StateConnected, Info: info})
			}
			delivered = true
			attempt = 0
			delay = s.backoffStart
			s.emit(event)
			return nil
		})
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			err = errors.New("stream ended")
		}
		if !s.retryStream(ctx, stream, &attempt, &delay, err) {
			return
		}
	}
}

func (s *Session) clashMode(ctx context.Context) (ClashMode, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return s.client.ClashMode(ctx)
}

func (s *Session) watch(ctx context.Context, stream Stream, fn func(Event) error) error {
	switch stream {
	case StreamServiceStatus:
		return s.client.WatchServiceStatus(ctx, func(status ServiceStatus) error {
			return fn(&ServiceEvent{Status: status})
		})
	case StreamStatus:
		return s.client.WatchStatus(ctx, time.Second, func(status Status) error {
			return fn(&StatusEvent{Status: status})
		})
	case StreamGroups:
		return s.client.WatchGroups(ctx, func(groups []Group) error {
			return fn(&GroupsEvent{Groups: groups})
		})
	case StreamClashMode:
		return s.client.WatchClashMode(ctx, func(mode string) error {
			return fn(&ClashModeEvent{Mode: mode})
		})
	case StreamOutbounds:
		return s.client.WatchOutbounds(ctx, func(outbounds []Outbound) error {
			return fn(&OutboundsEvent{Outbounds: outbounds})
		})
	case StreamConnections:
		return s.client.WatchConnections(ctx, time.Second, func(batch ConnectionBatch) error {
			return fn(&ConnectionsEvent{Batch: batch})
		})
	case StreamLogs:
		return s.client.WatchLogs(ctx, func(batch LogBatch) error {
			return fn(&LogsEvent{Batch: batch})
		})
	default:
		return errors.New("unknown stream")
	}
}

func (s *Session) retryStream(ctx context.Context, stream Stream, attempt *int, delay *time.Duration, err error) bool {
	if ctx.Err() != nil {
		return false
	}
	if KindOf(err) == KindUnsupported {
		s.emit(&UnavailableEvent{Stream: stream, Err: err})
		return false
	}
	(*attempt)++
	if stream == StreamServiceStatus {
		s.emit(&ConnEvent{State: StateReconnecting, Attempt: *attempt, Err: err})
	}
	if !s.wait(ctx, *delay) {
		return false
	}
	*delay = min(*delay*2, s.backoffMax)
	return true
}

func (s *Session) wait(ctx context.Context, delay time.Duration) bool {
	s.mu.Lock()
	wake := s.wake
	s.mu.Unlock()
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	case <-wake:
		return true
	}
}

func (s *Session) emit(event Event) {
	select {
	case s.events <- event:
	case <-s.ctx.Done():
	}
}
