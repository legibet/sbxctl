# sbxctl

Terminal client for the [sing-box API service](https://sing-box.sagernet.org/configuration/service/api/). It works on a running instance, over the API service's gRPC port: outbound groups, URL tests, Clash mode, connections and logs. Configuration and the sing-box process itself are out of reach by design.

Requires sing-box 1.14.0 or later with the API service enabled.

## Build

```sh
go build -o sbxctl ./cmd/sbxctl
```

## Use

```sh
sbxctl target add home http://127.0.0.1:9090 --secret <secret>
sbxctl
```

`sbxctl` with no subcommand opens the TUI, where `?` lists the keys of the focused workspace. Subcommands are non-interactive and take `--output json` or `jsonl`:

```sh
sbxctl status
sbxctl select PROXY hk-01
sbxctl test PROXY --wait 20s
sbxctl connections --watch
```

`sbxctl --help` covers the rest. Targets are stored under your user config directory with mode 0600; `--url` and `--secret` connect without saving anything.

## License

GPL-3.0-or-later, see [LICENSE](LICENSE). A third-party client, not affiliated with the sing-box project. It embeds a verbatim copy of the upstream `started_service.proto`, recorded in [api/UPSTREAM](api/UPSTREAM).
