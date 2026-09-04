// Package daemon contains the gRPC client generated from the sing-box
// StartedService protocol definition vendored in api/daemon.
//
// The protocol definition is copyright nekohasekai and licensed under
// GPL-3.0-or-later; see api/LICENSE.sing-box and api/UPSTREAM.
package daemon

//go:generate protoc -I ../../api/daemon --go_out=. --go_opt=paths=source_relative --go_opt=Mstarted_service.proto=github.com/legibet/sbxctl/internal/daemon --go-grpc_out=. --go-grpc_opt=paths=source_relative --go-grpc_opt=Mstarted_service.proto=github.com/legibet/sbxctl/internal/daemon started_service.proto
