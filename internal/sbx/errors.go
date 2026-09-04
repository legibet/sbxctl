package sbx

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Kind int

const (
	KindRemote Kind = iota
	KindConnect
	KindAuth
	KindTimeout
	KindUnsupported
	KindNotFound
	KindInvalid
	KindIncompatible
)

type Error struct {
	Kind Kind
	Op   string
	Err  error
}

func (e *Error) Error() string {
	detail := status.Convert(e.Err).Message()
	var message string
	switch e.Kind {
	case KindConnect:
		// grpc wraps dial failures as: connection error: desc = "transport: Error while dialing: <cause>"
		if _, cause, found := strings.Cut(detail, "Error while dialing: "); found {
			detail = strings.TrimSuffix(cause, "\"")
		}
		return "cannot connect to server: " + detail
	case KindAuth:
		message = "authentication failed (check secret)"
	case KindTimeout:
		message = "request timed out"
	case KindUnsupported:
		message = "not available on this server: " + detail
	case KindIncompatible:
		message = e.Err.Error()
	default:
		message = detail
	}
	return fmt.Sprintf("%s: %s", e.Op, message)
}

func (e *Error) Unwrap() error {
	return e.Err
}

func KindOf(err error) Kind {
	if rpcErr, ok := errors.AsType[*Error](err); ok {
		return rpcErr.Kind
	}
	return KindRemote
}

func rpcError(ctx context.Context, op string, err error, notFoundUnsupported bool) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return &Error{Kind: KindTimeout, Op: op, Err: err}
	}

	kind := KindRemote
	switch status.Code(err) {
	case codes.Unavailable:
		kind = KindConnect
	case codes.Unauthenticated:
		kind = KindAuth
	case codes.DeadlineExceeded:
		kind = KindTimeout
	case codes.Unimplemented:
		kind = KindUnsupported
	case codes.NotFound:
		if notFoundUnsupported {
			kind = KindUnsupported
		} else {
			kind = KindNotFound
		}
	case codes.InvalidArgument:
		kind = KindInvalid
	}
	return &Error{Kind: kind, Op: op, Err: err}
}
