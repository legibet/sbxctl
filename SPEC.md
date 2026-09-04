# sbxctl design notes

How sbxctl is put together, and what the sing-box API service actually does. Usage is in the [README](README.md).

sbxctl manages a running instance. The remote API registers an attached `StartedService`, which exposes no configuration or process lifecycle RPCs, so runtime state is all there is to manage.

## Server behavior

Read from sing-box v1.14.0 source and confirmed against a running instance. Little of this is visible in the proto file.

### Transport and auth

- HTTP uses h2c. HTTPS uses TLS with `h2` forced into the ALPN list.
- The secret travels as `authorization: Bearer <secret>` metadata, checked by both the unary and the stream interceptor. An empty secret disables the check.
- A service that has not finished starting fails unary calls with `os.ErrInvalid`, which reaches the client as an Unknown code carrying the text "invalid argument". An attached remote instance never gets there, but the error mapping covers it.

### Outbounds and URL tests

- `SubscribeGroups` and `SubscribeOutbounds` push full snapshots on every change, at most one per 250ms, so the client replaces its state instead of merging. `readGroups` skips groups with no members.
- `Group.selectable` is true only for selectors. `Group.isExpand` is a fold state the official mobile app keeps in its cache file; sbxctl ignores it.
- `URLTest` returns as soon as it triggers. Results arrive on the Groups and Outbounds streams. For a urltest group it calls `CheckOutbounds`, for any other group it tests every member, for a single outbound it tests once.
- A failed test deletes that outbound's history entry rather than recording an error. A newer `urlTestTime` means success, a vanished entry means failure. An outbound with no entry to begin with produces no signal at all when it fails, which is why `test --wait` can only report `timeout` for it.
- `SelectOutbound` returns NotFound for an unknown group, InvalidArgument for a group that is not a selector, and NotFound for an outbound outside the group.

### Status and connections

- The `interval` field of `SubscribeStatus` and `SubscribeConnections` is nanoseconds. Anything not positive gets the server default of one second.
- `GetStartedAt` returns milliseconds, and 0 before the service starts. For an attached instance it is the moment the API service itself started.
- With `Status.trafficAvailable` false there is no traffic or connection count. An instance without connection tracking fails `SubscribeConnections` with Unimplemented.
- The first `SubscribeConnections` frame has `reset = true` and carries live connections along with the closed ones the server still holds, the latter with `closedAt`. After that come NEW, UPDATE and CLOSED events. UPDATE carries only the id and the new cumulative totals; CLOSED may carry a full connection.
- The server never sets `Connection.uplink` and `Connection.downlink`, the per-second rate fields, so they arrive as 0. `ConnectionTable` derives rates from the difference between consecutive totals and the stream interval.

### Logs and Clash mode

- The first `SubscribeLog` frame has `reset = true` and holds up to 3000 buffered lines. `ClearLogs` pushes another `reset` frame. Messages come from the platform formatter with ANSI colors and a `LEVEL[seconds]` prefix already in them.
- The server does not filter by level. `GetDefaultLogLevel` returns the level the instance was configured with, which the client uses as its default filter.
- Clash mode RPCs return NotFound when the instance has no mode manager. In v1.14.0 the Clash API server provides it, so an instance without `clash_api` has none; after commit a556d49 it lives in a standalone `clashmode.Manager` that exists whenever the API service does. The RPCs and the proto are identical either way, so handling NotFound covers both.
- The server reads `accept-language` metadata for deprecation warnings and Tailscale text. sbxctl sends none.

## Layout

```
internal/cli  CLI          internal/ui  TUI
                 \        /
             internal/sbx  session, domain types, conversion, operations, errors
                      |
             internal/daemon  generated gRPC client
```

`internal/sbx` is the only package that touches generated types. Its domain types are plain structs with JSON tags: the CLI serializes them, the TUI renders them, and both frontends work from one model and one set of operations. The connection table, log buffer and URL test tracker sit there for the same reason.

Nothing imports `github.com/sagernet/sing-box`; its `daemon` package pulls the whole core into the build.

Errors become `sbx.Error` carrying a `Kind`: Remote, Connect, Auth, Timeout, Unsupported, NotFound, Invalid or Incompatible. `internal/sbx/errors.go` maps gRPC codes onto it, and `cli.Main` picks the exit code from it.

### Session

One `sbx.Session` per target owns the `grpc.ClientConn`, a root context and every stream goroutine, and reports to the TUI through a single event channel. The CLI skips it and calls `sbx.Client` directly for a snapshot or a stream.

ServiceStatus, Status, Groups and ClashMode run for the life of the session, ServiceStatus doubling as the heartbeat. Outbounds, Connections and Logs start and stop with the workspace that needs them. ClashMode stops for good on NotFound.

Each stream reconnects on its own, backing off from 1s to a 10s ceiling. An auth failure or a version mismatch moves the session to Failed without retrying. Switching targets cancels the old context, waits for its goroutines, then builds a new session and clears every workspace.

### Generated code

`api/daemon/started_service.proto` is a byte-for-byte copy of the upstream file, with its tag, commit and sha256 in `api/UPSTREAM`. `go generate ./internal/daemon` regenerates the client and overrides the upstream `go_package` through `--go_opt=M`, so the proto itself stays untouched. The generated files are committed, including messages for Tailscale, USB and other services sbxctl never calls.

That proto is GPL-3.0-or-later with an additional name clause (`api/LICENSE.sing-box`), which is why the project is GPL. A newer proto means re-recording tag, commit and sha256.

Protobuf keeps working as upstream adds fields. New RPCs get synced when sbxctl has a use for them.

## TUI

Keys follow nvim and yazi: single keystrokes for common actions, and no modes beyond filter input and confirmation. `internal/ui/keys.go` holds the bindings, and the `?` overlay is built from the bindings of the focused workspace, so the two cannot drift apart.

Proxies, Connections and Logs each implement `workspace` (`setSize`, `handleKey`, `setFilter`, `tick`, `view`, `bindings`). Keys dispatch in a fixed order: confirmation, overlay, filter, global, workspace.

### Frame

The top bar carries target name, connection dot, version, uptime, Clash mode, rates and live connection count, dropping the traffic fields when the server has none. A tab row and a footer take one line each. Filtering, confirmation and messages borrow the footer rather than opening a window. Help, connection details and the target picker are centered overlays, the only places that draw a border.

Each workspace emits exactly `width` by `height` cells through `exactLines` and renders only the rows inside the viewport. 80x24 is the floor, below which the app shows a message instead. Proxies splits into two panes at 100 columns and above, the left one taking a third.

### Color

Roles map onto the terminal's 16 ANSI colors in `internal/ui/theme.go`: blue for accent, green for success, yellow for warning, red for danger, bright black for secondary text, and the terminal's own foreground for body text, so the UI follows whatever scheme the user runs. Depth comes from brightness, weight and spacing. The focused row inverts and the current selection is accent and bold. Under `--no-color` only bold and reverse survive, which keeps every state readable.

### Data

Proxies marks an outbound `testing` after triggering a test, until the stream updates it or 10 seconds pass, and shows a selection once the stream reports it rather than when the call returns. Connections keeps 1000 closed entries, Logs 5000 lines against the server's 3000 of history. While reconnecting, the top bar shows a yellow dot and the attempt count; after an auth failure it stops retrying and asks for a different target.

### Width

Bubble Tea measures text with wcwidth until the terminal answers its mode 2027 query, then switches to grapheme widths. The two can disagree about a cluster such as a flag emoji, so a frame drawn before the answer may wrap on the terminal while Bubble Tea's screen buffer believes it fit, leaving rows below that nothing will erase. `app.Update` answers the `ModeReportMsg` for `ansi.ModeUnicodeCore` with `tea.ClearScreen`.

## Upstream sources

- [`daemon/started_service.proto`](https://github.com/SagerNet/sing-box/blob/v1.14.0/daemon/started_service.proto) and [`daemon/started_service.go`](https://github.com/SagerNet/sing-box/blob/v1.14.0/daemon/started_service.go): the RPCs and their implementations
- [`daemon/server.go`](https://github.com/SagerNet/sing-box/blob/v1.14.0/daemon/server.go): gRPC server and auth interceptors
- [`daemon/attached_service.go`](https://github.com/SagerNet/sing-box/blob/v1.14.0/daemon/attached_service.go): what an attached instance exposes
- [`service/api/server.go`](https://github.com/SagerNet/sing-box/blob/v1.14.0/service/api/server.go): TCP, TLS, h2c and gRPC routing
- [`log/observable.go`](https://github.com/SagerNet/sing-box/blob/v1.14.0/log/observable.go): log buffer and formatter
- [commit a556d49](https://github.com/SagerNet/sing-box/commit/a556d49491c23db6117aea241a778e2c3a9e498f): Clash mode moved out of the Clash API server
