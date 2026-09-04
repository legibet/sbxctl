package sbx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/legibet/sbxctl/internal/daemon"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

const MinAPIVersion = 4

// ErrStop ends a Watch call from inside its callback without reporting an error.
var ErrStop = errors.New("stop")

func (c *Client) Version(ctx context.Context) (Version, error) {
	response, err := c.svc.GetVersion(ctx, &emptypb.Empty{})
	if err != nil {
		return Version{}, rpcError(ctx, "get version", err, false)
	}
	return Version{Version: response.Version, APIVersion: int(response.ApiVersion)}, nil
}

func (c *Client) CheckVersion(ctx context.Context) (Version, error) {
	version, err := c.Version(ctx)
	if err != nil {
		return Version{}, err
	}
	if version.APIVersion < MinAPIVersion {
		return version, &Error{
			Kind: KindIncompatible,
			Op:   "check version",
			Err:  fmt.Errorf("server API version %d is older than supported %d", version.APIVersion, MinAPIVersion),
		}
	}
	return version, nil
}

func (c *Client) StartedAt(ctx context.Context) (time.Time, error) {
	response, err := c.svc.GetStartedAt(ctx, &emptypb.Empty{})
	if err != nil {
		return time.Time{}, rpcError(ctx, "get start time", err, false)
	}
	return unixMilliseconds(response.StartedAt), nil
}

func (c *Client) DeprecatedWarnings(ctx context.Context) ([]DeprecatedWarning, error) {
	response, err := c.svc.GetDeprecatedWarnings(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, rpcError(ctx, "get deprecated warnings", err, false)
	}
	warnings := make([]DeprecatedWarning, 0, len(response.Warnings))
	for _, warning := range response.Warnings {
		warnings = append(warnings, DeprecatedWarning{
			Message:           warning.Message,
			Impending:         warning.Impending,
			MigrationLink:     warning.MigrationLink,
			Description:       warning.Description,
			DeprecatedVersion: warning.DeprecatedVersion,
			ScheduledVersion:  warning.ScheduledVersion,
		})
	}
	return warnings, nil
}

func (c *Client) ClashMode(ctx context.Context) (ClashMode, error) {
	response, err := c.svc.GetClashModeStatus(ctx, &emptypb.Empty{})
	if err != nil {
		return ClashMode{}, rpcError(ctx, "get clash mode", err, true)
	}
	modes := make([]string, 0, len(response.ModeList))
	modes = append(modes, response.ModeList...)
	return ClashMode{Current: response.CurrentMode, Modes: modes}, nil
}

func (c *Client) SetClashMode(ctx context.Context, mode string) error {
	_, err := c.svc.SetClashMode(ctx, &daemon.ClashMode{Mode: mode})
	return rpcError(ctx, "set clash mode", err, true)
}

func (c *Client) DefaultLogLevel(ctx context.Context) (LogLevel, error) {
	response, err := c.svc.GetDefaultLogLevel(ctx, &emptypb.Empty{})
	if err != nil {
		return 0, rpcError(ctx, "get default log level", err, false)
	}
	return LogLevel(response.Level), nil
}

func (c *Client) ClearLogs(ctx context.Context) error {
	_, err := c.svc.ClearLogs(ctx, &emptypb.Empty{})
	return rpcError(ctx, "clear logs", err, false)
}

func (c *Client) SelectOutbound(ctx context.Context, group, outbound string) error {
	_, err := c.svc.SelectOutbound(ctx, &daemon.SelectOutboundRequest{GroupTag: group, OutboundTag: outbound})
	return rpcError(ctx, "select outbound", err, false)
}

func (c *Client) URLTest(ctx context.Context, tag string) error {
	_, err := c.svc.URLTest(ctx, &daemon.URLTestRequest{OutboundTag: tag})
	return rpcError(ctx, "test outbound", err, false)
}

func (c *Client) CloseConnection(ctx context.Context, id string) error {
	_, err := c.svc.CloseConnection(ctx, &daemon.CloseConnectionRequest{Id: id})
	return rpcError(ctx, "close connection", err, false)
}

func (c *Client) CloseAllConnections(ctx context.Context) error {
	_, err := c.svc.CloseAllConnections(ctx, &emptypb.Empty{})
	return rpcError(ctx, "close all connections", err, false)
}

// recvLoop drives a server stream: every message is converted and handed to fn
// until the stream ends, ctx is done, or fn returns an error. ErrStop from fn
// ends the loop with a nil error.
func recvLoop[P, T any](ctx context.Context, op string, stream grpc.ServerStreamingClient[P], convert func(*P) T, fn func(T) error, notFoundUnsupported bool) error {
	for {
		message, err := stream.Recv()
		if errors.Is(err, io.EOF) && ctx.Err() == nil {
			return nil
		}
		if err != nil {
			return rpcError(ctx, op, err, notFoundUnsupported)
		}
		if err := fn(convert(message)); err != nil {
			if errors.Is(err, ErrStop) {
				return nil
			}
			return err
		}
	}
}

func (c *Client) WatchServiceStatus(ctx context.Context, fn func(ServiceStatus) error) error {
	const op = "watch service status"
	stream, err := c.svc.SubscribeServiceStatus(ctx, &emptypb.Empty{})
	if err != nil {
		return rpcError(ctx, op, err, false)
	}
	return recvLoop(ctx, op, stream, convertServiceStatus, fn, false)
}

// WatchStatus streams runtime status every interval; 0 lets the server use its 1s default.
func (c *Client) WatchStatus(ctx context.Context, interval time.Duration, fn func(Status) error) error {
	const op = "watch status"
	stream, err := c.svc.SubscribeStatus(ctx, &daemon.SubscribeStatusRequest{Interval: int64(interval)})
	if err != nil {
		return rpcError(ctx, op, err, false)
	}
	return recvLoop(ctx, op, stream, convertStatus, fn, false)
}

func (c *Client) WatchGroups(ctx context.Context, fn func([]Group) error) error {
	const op = "watch groups"
	stream, err := c.svc.SubscribeGroups(ctx, &emptypb.Empty{})
	if err != nil {
		return rpcError(ctx, op, err, false)
	}
	return recvLoop(ctx, op, stream, func(groups *daemon.Groups) []Group { return convertGroups(groups.Group) }, fn, false)
}

func (c *Client) WatchOutbounds(ctx context.Context, fn func([]Outbound) error) error {
	const op = "watch outbounds"
	stream, err := c.svc.SubscribeOutbounds(ctx, &emptypb.Empty{})
	if err != nil {
		return rpcError(ctx, op, err, false)
	}
	return recvLoop(ctx, op, stream, func(list *daemon.OutboundList) []Outbound { return convertOutbounds(list.Outbounds) }, fn, false)
}

func (c *Client) WatchClashMode(ctx context.Context, fn func(string) error) error {
	const op = "watch clash mode"
	stream, err := c.svc.SubscribeClashMode(ctx, &emptypb.Empty{})
	if err != nil {
		return rpcError(ctx, op, err, true)
	}
	return recvLoop(ctx, op, stream, func(mode *daemon.ClashMode) string { return mode.Mode }, fn, true)
}

// WatchConnections streams connection events; traffic updates arrive every interval (0 = server default 1s).
func (c *Client) WatchConnections(ctx context.Context, interval time.Duration, fn func(ConnectionBatch) error) error {
	const op = "watch connections"
	stream, err := c.svc.SubscribeConnections(ctx, &daemon.SubscribeConnectionsRequest{Interval: int64(interval)})
	if err != nil {
		return rpcError(ctx, op, err, false)
	}
	return recvLoop(ctx, op, stream, convertConnectionBatch, fn, false)
}

func (c *Client) WatchLogs(ctx context.Context, fn func(LogBatch) error) error {
	const op = "watch logs"
	stream, err := c.svc.SubscribeLog(ctx, &emptypb.Empty{})
	if err != nil {
		return rpcError(ctx, op, err, false)
	}
	return recvLoop(ctx, op, stream, convertLogBatch, fn, false)
}
