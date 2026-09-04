package sbx

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/legibet/sbxctl/internal/daemon"
)

func TestCheckVersion(t *testing.T) {
	for _, apiVersion := range []int32{3, 4, 5} {
		t.Run(strconv.Itoa(int(apiVersion)), func(t *testing.T) {
			service := &testService{}
			service.getVersion = func(context.Context) (*daemon.Version, error) {
				return &daemon.Version{Version: "test", ApiVersion: apiVersion}, nil
			}
			client := newTestClient(t, service, "")
			_, err := client.CheckVersion(context.Background())
			if apiVersion == 3 {
				if KindOf(err) != KindIncompatible {
					t.Fatalf("KindOf(error) = %v, want %v; error: %v", KindOf(err), KindIncompatible, err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRPCErrorKinds(t *testing.T) {
	tests := []struct {
		name string
		call func(*Client) error
		want Kind
	}{
		{
			name: "unauthenticated",
			call: func(client *Client) error {
				_, err := client.Version(context.Background())
				return err
			},
			want: KindAuth,
		},
		{
			name: "unimplemented",
			call: func(client *Client) error {
				return client.WatchStatus(context.Background(), 0, func(Status) error { return nil })
			},
			want: KindUnsupported,
		},
		{
			name: "clash mode not found",
			call: func(client *Client) error {
				_, err := client.ClashMode(context.Background())
				return err
			},
			want: KindUnsupported,
		},
		{
			name: "outbound not found",
			call: func(client *Client) error {
				return client.SelectOutbound(context.Background(), "group", "missing")
			},
			want: KindNotFound,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &testService{}
			service.getVersion = func(context.Context) (*daemon.Version, error) {
				return nil, status.Error(codes.Unauthenticated, "bad secret")
			}
			service.subscribeStatus = func(context.Context, *daemon.SubscribeStatusRequest, grpc.ServerStreamingServer[daemon.Status]) error {
				return status.Error(codes.Unimplemented, "status unavailable")
			}
			service.getClashMode = func(context.Context) (*daemon.ClashModeStatus, error) {
				return nil, status.Error(codes.NotFound, "mode unavailable")
			}
			service.selectOutbound = func(context.Context, *daemon.SelectOutboundRequest) error {
				return status.Error(codes.NotFound, "outbound missing")
			}
			client := newTestClient(t, service, "")
			err := test.call(client)
			if KindOf(err) != test.want {
				t.Fatalf("KindOf(error) = %v, want %v; error: %v", KindOf(err), test.want, err)
			}
		})
	}
}

func TestWatchGroupsStopsOnErrStop(t *testing.T) {
	service := &testService{}
	service.subscribeGroups = func(_ context.Context, stream grpc.ServerStreamingServer[daemon.Groups]) error {
		return stream.Send(&daemon.Groups{Group: []*daemon.Group{{
			Tag:   "proxy",
			Items: []*daemon.GroupItem{{Tag: "fast"}, {Tag: "direct"}},
		}}})
	}

	client := newTestClient(t, service, "")
	called := false
	err := client.WatchGroups(context.Background(), func(groups []Group) error {
		called = true
		if len(groups) != 1 || len(groups[0].Items) != 2 {
			t.Fatalf("groups = %#v", groups)
		}
		return ErrStop
	})
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("callback was not called")
	}
}

func TestWatchCallbackErrorReturnedAsIs(t *testing.T) {
	want := errors.New("callback failed")
	service := &testService{}
	service.subscribeGroups = func(_ context.Context, stream grpc.ServerStreamingServer[daemon.Groups]) error {
		return stream.Send(&daemon.Groups{})
	}
	client := newTestClient(t, service, "")
	err := client.WatchGroups(context.Background(), func([]Group) error { return want })
	if err != want {
		t.Fatalf("error = %v, want exact callback error", err)
	}
}

func TestWatchContextCancellation(t *testing.T) {
	service := &testService{}
	service.subscribeGroups = func(ctx context.Context, _ grpc.ServerStreamingServer[daemon.Groups]) error {
		<-ctx.Done()
		return ctx.Err()
	}
	client := newTestClient(t, service, "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := client.WatchGroups(ctx, func([]Group) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}
