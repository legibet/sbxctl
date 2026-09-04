package sbx

import (
	"context"
	"net"
	"reflect"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/legibet/sbxctl/internal/daemon"
)

type testService struct {
	daemon.UnimplementedStartedServiceServer
	getVersion      func(context.Context) (*daemon.Version, error)
	subscribeStatus func(context.Context, *daemon.SubscribeStatusRequest, grpc.ServerStreamingServer[daemon.Status]) error
	subscribeGroups func(context.Context, grpc.ServerStreamingServer[daemon.Groups]) error
	getClashMode    func(context.Context) (*daemon.ClashModeStatus, error)
	selectOutbound  func(context.Context, *daemon.SelectOutboundRequest) error
}

func (s *testService) GetVersion(ctx context.Context, _ *emptypb.Empty) (*daemon.Version, error) {
	return s.getVersion(ctx)
}

func (s *testService) SubscribeStatus(request *daemon.SubscribeStatusRequest, stream grpc.ServerStreamingServer[daemon.Status]) error {
	return s.subscribeStatus(stream.Context(), request, stream)
}

func (s *testService) SubscribeGroups(_ *emptypb.Empty, stream grpc.ServerStreamingServer[daemon.Groups]) error {
	return s.subscribeGroups(stream.Context(), stream)
}

func (s *testService) GetClashModeStatus(ctx context.Context, _ *emptypb.Empty) (*daemon.ClashModeStatus, error) {
	return s.getClashMode(ctx)
}

func (s *testService) SelectOutbound(ctx context.Context, request *daemon.SelectOutboundRequest) (*emptypb.Empty, error) {
	if err := s.selectOutbound(ctx, request); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func newTestClient(t *testing.T, service daemon.StartedServiceServer, secret string) *Client {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	daemon.RegisterStartedServiceServer(server, service)
	go func() {
		_ = server.Serve(listener)
	}()

	auth := authInterceptor{secret: secret}
	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(auth.unary),
		grpc.WithChainStreamInterceptor(auth.stream),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
		server.Stop()
		_ = listener.Close()
	})
	return newClient(conn)
}

func TestAuthorizationMetadata(t *testing.T) {
	for _, test := range []struct {
		name   string
		secret string
		want   []string
	}{
		{name: "present", secret: "top-secret", want: []string{"Bearer top-secret"}},
		{name: "absent", want: nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := &testService{}
			service.getVersion = func(ctx context.Context) (*daemon.Version, error) {
				md, _ := metadata.FromIncomingContext(ctx)
				if got := md.Get("authorization"); !reflect.DeepEqual(got, test.want) {
					t.Errorf("unary authorization = %v, want %v", got, test.want)
				}
				return &daemon.Version{Version: "1.14.0", ApiVersion: 4}, nil
			}
			service.subscribeStatus = func(ctx context.Context, _ *daemon.SubscribeStatusRequest, stream grpc.ServerStreamingServer[daemon.Status]) error {
				md, _ := metadata.FromIncomingContext(ctx)
				if got := md.Get("authorization"); !reflect.DeepEqual(got, test.want) {
					t.Errorf("stream authorization = %v, want %v", got, test.want)
				}
				return stream.Send(&daemon.Status{})
			}

			client := newTestClient(t, service, test.secret)
			if _, err := client.Version(context.Background()); err != nil {
				t.Fatal(err)
			}
			if err := client.WatchStatus(context.Background(), 0, func(Status) error { return ErrStop }); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestParseURL(t *testing.T) {
	tests := []struct {
		url        string
		wantScheme string
		wantHost   string
		wantTarget string
		wantError  bool
	}{
		{url: "http://example.com/path?q=1#fragment", wantScheme: "http", wantHost: "example.com", wantTarget: "example.com:80"},
		{url: "https://[2001:db8::1]", wantScheme: "https", wantHost: "2001:db8::1", wantTarget: "[2001:db8::1]:443"},
		{url: "https://example.com:8443", wantScheme: "https", wantHost: "example.com", wantTarget: "example.com:8443"},
		{url: "grpc://example.com", wantError: true},
		{url: "http:///missing", wantError: true},
	}
	for _, test := range tests {
		t.Run(test.url, func(t *testing.T) {
			scheme, host, target, err := parseURL(test.url)
			if (err != nil) != test.wantError {
				t.Fatalf("parseURL() error = %v, wantError %v", err, test.wantError)
			}
			if scheme != test.wantScheme || host != test.wantHost || target != test.wantTarget {
				t.Fatalf("parseURL() = %q, %q, %q; want %q, %q, %q", scheme, host, target, test.wantScheme, test.wantHost, test.wantTarget)
			}
		})
	}
}
