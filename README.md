# sbxctl

Terminal client for the [sing-box API service](https://sing-box.sagernet.org/configuration/service/api/).

Requires sing-box 1.14.0 or later with the API service enabled.

## Install

```sh
go install ./cmd/sbxctl
```

## Quick start

Add a target:

```sh
sbxctl target add home http://127.0.0.1:9090 --secret <secret>
```

Open the TUI:

```sh
sbxctl
```

Press `?` for the keys of the focused workspace.

`--url` and `--secret` connect without saving a target. Saved targets go to `~/.config/sbxctl/config.toml`.

## Commands

```sh
sbxctl status                  # version, uptime, traffic
sbxctl select PROXY hk-01      # select an outbound
sbxctl test PROXY --wait 20s   # run a URL test
sbxctl connections --watch     # watch connections
```

Subcommands accept `--output json` or `jsonl`. See `sbxctl --help` for all commands.

## Development

```sh
just build  # build the binary
just check  # fmt, lint, and test
```

## License

GPL-3.0-or-later, see [LICENSE](LICENSE). A third-party client, not affiliated with the sing-box project. It embeds a verbatim copy of the upstream `started_service.proto`, recorded in [api/UPSTREAM](api/UPSTREAM).
