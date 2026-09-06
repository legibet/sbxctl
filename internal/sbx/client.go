package sbx

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/url"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"github.com/legibet/sbxctl/internal/daemon"
)

type Endpoint struct {
	URL        string
	Secret     string
	CAFile     string
	ServerName string
	Insecure   bool
}

type Client struct {
	conn *grpc.ClientConn
	svc  daemon.StartedServiceClient
}

func ParseURL(rawURL string) error {
	_, _, _, err := parseURL(rawURL)
	return err
}

func parseURL(rawURL string) (scheme, host, target string, err error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", "", "", err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", "", "", fmt.Errorf("invalid URL scheme %q: expected http or https", parsed.Scheme)
	}
	host = parsed.Hostname()
	if host == "" {
		return "", "", "", fmt.Errorf("missing URL host")
	}
	port := parsed.Port()
	if port == "" {
		if parsed.Scheme == "http" {
			port = "80"
		} else {
			port = "443"
		}
	}
	return parsed.Scheme, host, net.JoinHostPort(host, port), nil
}

func Dial(ep Endpoint) (*Client, error) {
	scheme, host, target, err := parseURL(ep.URL)
	if err != nil {
		return nil, err
	}

	var transport credentials.TransportCredentials
	if scheme == "http" {
		transport = insecure.NewCredentials()
	} else {
		serverName := ep.ServerName
		if serverName == "" {
			serverName = host
		}
		tlsConfig := &tls.Config{
			ServerName:         serverName,
			InsecureSkipVerify: ep.Insecure,
		}
		if ep.CAFile != "" {
			pem, err := os.ReadFile(ep.CAFile)
			if err != nil {
				return nil, fmt.Errorf("read CA file %q: %w", ep.CAFile, err)
			}
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(pem) {
				return nil, fmt.Errorf("CA file %q contains no certificates", ep.CAFile)
			}
			tlsConfig.RootCAs = pool
		}
		transport = credentials.NewTLS(tlsConfig)
	}

	auth := authInterceptor{secret: ep.Secret}
	conn, err := grpc.NewClient(
		target,
		// API endpoints do not use DNS service configs; skip the extra TXT lookup.
		grpc.WithDisableServiceConfig(),
		grpc.WithTransportCredentials(transport),
		grpc.WithChainUnaryInterceptor(auth.unary),
		grpc.WithChainStreamInterceptor(auth.stream),
	)
	if err != nil {
		return nil, &Error{Kind: KindConnect, Op: "connect", Err: err}
	}
	return newClient(conn), nil
}

func newClient(conn *grpc.ClientConn) *Client {
	return &Client{conn: conn, svc: daemon.NewStartedServiceClient(conn)}
}

func (c *Client) Close() {
	_ = c.conn.Close()
}

type authInterceptor struct {
	secret string
}

func (a authInterceptor) context(ctx context.Context) context.Context {
	if a.secret == "" {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+a.secret)
}

func (a authInterceptor) unary(
	ctx context.Context,
	method string,
	req, reply any,
	cc *grpc.ClientConn,
	invoker grpc.UnaryInvoker,
	opts ...grpc.CallOption,
) error {
	return invoker(a.context(ctx), method, req, reply, cc, opts...)
}

func (a authInterceptor) stream(
	ctx context.Context,
	desc *grpc.StreamDesc,
	cc *grpc.ClientConn,
	method string,
	streamer grpc.Streamer,
	opts ...grpc.CallOption,
) (grpc.ClientStream, error) {
	return streamer(a.context(ctx), desc, cc, method, opts...)
}
