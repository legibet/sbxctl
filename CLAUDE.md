# sbxctl

Terminal client for the sing-box API service. [SPEC.md](SPEC.md) records what the server actually does and how the packages fit together; read it before changing the gRPC layer, the session and its streams, or the TUI's frame and colors.

- Bubble Tea, Lip Gloss and Bubbles v2 live under `charm.land/`. The `github.com/charmbracelet/` paths still serve v1 and will not resolve for these three.
- `--url` and `--target` may point at the user's live proxy. Selecting an outbound, closing connections or clearing logs there changes what their traffic does, so ask first or run against a throwaway instance.
