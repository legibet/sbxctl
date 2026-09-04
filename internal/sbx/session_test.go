package sbx

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/legibet/sbxctl/internal/daemon"
)

type sessionTestService struct {
	daemon.UnimplementedStartedServiceServer
	heartbeat func(grpc.ServerStreamingServer[daemon.ServiceStatus]) error
	outbounds func(grpc.ServerStreamingServer[daemon.OutboundList]) error
}

func (*sessionTestService) GetVersion(context.Context, *emptypb.Empty) (*daemon.Version, error) {
	return &daemon.Version{Version: "1.14.0", ApiVersion: 4}, nil
}

func (*sessionTestService) GetStartedAt(context.Context, *emptypb.Empty) (*daemon.StartedAt, error) {
	return &daemon.StartedAt{StartedAt: 1_700_000_000}, nil
}

func (*sessionTestService) GetDefaultLogLevel(context.Context, *emptypb.Empty) (*daemon.DefaultLogLevel, error) {
	return &daemon.DefaultLogLevel{Level: daemon.LogLevel_INFO}, nil
}

func (s *sessionTestService) SubscribeServiceStatus(_ *emptypb.Empty, stream grpc.ServerStreamingServer[daemon.ServiceStatus]) error {
	if s.heartbeat != nil {
		return s.heartbeat(stream)
	}
	<-stream.Context().Done()
	return stream.Context().Err()
}

func (*sessionTestService) SubscribeStatus(_ *daemon.SubscribeStatusRequest, stream grpc.ServerStreamingServer[daemon.Status]) error {
	<-stream.Context().Done()
	return stream.Context().Err()
}

func (*sessionTestService) SubscribeGroups(_ *emptypb.Empty, stream grpc.ServerStreamingServer[daemon.Groups]) error {
	<-stream.Context().Done()
	return stream.Context().Err()
}

func (*sessionTestService) GetClashModeStatus(context.Context, *emptypb.Empty) (*daemon.ClashModeStatus, error) {
	return &daemon.ClashModeStatus{CurrentMode: "rule", ModeList: []string{"rule", "global"}}, nil
}

func (*sessionTestService) SubscribeClashMode(_ *emptypb.Empty, stream grpc.ServerStreamingServer[daemon.ClashMode]) error {
	<-stream.Context().Done()
	return stream.Context().Err()
}

func (s *sessionTestService) SubscribeOutbounds(_ *emptypb.Empty, stream grpc.ServerStreamingServer[daemon.OutboundList]) error {
	if s.outbounds != nil {
		return s.outbounds(stream)
	}
	<-stream.Context().Done()
	return stream.Context().Err()
}

func TestSessionConnectedEventCarriesVersion(t *testing.T) {
	session := newSession(newTestClient(t, &sessionTestService{}, ""))
	defer session.Close()
	session.Start()
	event := nextConnEvent(t, session.Events())
	if event.State != StateConnected {
		t.Fatalf("state = %v, want connected", event.State)
	}
	if event.Info.Version != (Version{Version: "1.14.0", APIVersion: 4}) {
		t.Fatalf("version = %#v", event.Info.Version)
	}
}

func TestSessionUnsupportedStreamStopsAfterOneEvent(t *testing.T) {
	var calls atomic.Int32
	service := &sessionTestService{
		outbounds: func(grpc.ServerStreamingServer[daemon.OutboundList]) error {
			calls.Add(1)
			return status.Error(codes.Unimplemented, "outbounds unavailable")
		},
	}
	session := newSession(newTestClient(t, service, ""))
	defer session.Close()
	session.SetStream(StreamOutbounds, true)
	session.Start()

	deadline := time.After(2 * time.Second)
	for {
		select {
		case raw := <-session.Events():
			if event, ok := raw.(*UnavailableEvent); ok && event.Stream == StreamOutbounds {
				time.Sleep(20 * time.Millisecond)
				if got := calls.Load(); got != 1 {
					t.Fatalf("subscriptions = %d, want 1", got)
				}
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for unavailable event")
		}
	}
}

func TestSessionHeartbeatReconnectOrder(t *testing.T) {
	var calls atomic.Int32
	service := &sessionTestService{
		heartbeat: func(stream grpc.ServerStreamingServer[daemon.ServiceStatus]) error {
			if calls.Add(1) == 1 {
				return status.Error(codes.Unavailable, "lost connection")
			}
			if err := stream.Send(&daemon.ServiceStatus{Status: daemon.ServiceStatus_STARTED}); err != nil {
				return err
			}
			<-stream.Context().Done()
			return stream.Context().Err()
		},
	}
	session := newSession(newTestClient(t, service, ""))
	session.backoffStart = time.Millisecond
	session.backoffMax = time.Millisecond
	defer session.Close()
	session.Start()

	want := []ConnState{StateConnected, StateReconnecting, StateConnected}
	for i, state := range want {
		event := nextConnEvent(t, session.Events())
		if event.State != state {
			t.Fatalf("event %d state = %v, want %v", i, event.State, state)
		}
	}
}

func TestSessionCloseClosesEvents(t *testing.T) {
	session := newSession(newTestClient(t, &sessionTestService{}, ""))
	session.Start()
	_ = nextConnEvent(t, session.Events())
	done := make(chan struct{})
	go func() {
		session.Close()
		for range session.Events() {
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not return and close Events")
	}
}

func nextConnEvent(t *testing.T, events <-chan Event) *ConnEvent {
	t.Helper()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		select {
		case event, ok := <-events:
			if !ok {
				t.Fatal("events closed")
			}
			if connected, ok := event.(*ConnEvent); ok {
				return connected
			}
		case <-timer.C:
			t.Fatal("timed out waiting for connection event")
		}
	}
}
