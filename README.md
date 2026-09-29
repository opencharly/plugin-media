# plugin-media

Host-side video transcoding for OpenCharly — the `transcode:` pipeline check
verb, converting a captured MJPEG stream into an H.264 MP4 with the host's
`ffmpeg`.

The plugin is an out-of-tree Go module: charly fetches this repo at the pinned
tag, go-builds the provider on the host, and serves it **out-of-process** over
go-plugin gRPC via the plugin SDK. The verb dispatches through the provider
registry exactly like a built-in, executing in the evidence phase before venue
teardown.

## What it provides

| Capability | Surface |
|---|---|
| `verb:transcode` | the `transcode:` pipeline verb — source MJPEG → H.264 MP4 |

The verb is **host-side by construction**: the MJPEG artifact a capture session
flushes (e.g. `spice: record`) is already host-side, so the provider runs
`ffmpeg -y -loglevel error -i <mjpeg> -c:v libx264 -pix_fmt yuv420p <out>`
locally and writes the MP4 to the input's `artifact` (or the derived
`<source>.mp4` path). It self-evaluates the artifact validators and the shared
`exit_status`/`stdout`/`stderr` matchers.

**Requires host `ffmpeg`** (with libx264 + yuv420p support).

## How to use it

Compose the plugin candy in a box or check bed's `candy:` list, then use it in an
instrument's `pipeline:`:

```yaml
instrument:
    - id: screen
      phase: [live]
      spice: {method: session, fps: 5}
      pipeline:
          - transcode: {to: mp4}          # map form
          - transcode: mp4                # scalar-sugar equal (primary field: to)
```

| Field | Meaning |
|---|---|
| `to` | container format; only `mp4` is supported (default) |
| `artifact` | host path the MP4 is written to; empty → derived from the source (`<source>.mp4`) |
| `artifact_min_bytes` | post-transcode artifact-size assertion |
| `artifact_not_uniform` | SOURCE-frame motion assertion, evaluated on the SOURCE MJPEG frames (pre-encode) — exact hashes of x264-encoded frames are vacuous (QP noise) |

The shared matchers (`exit_status`/`stdout`/`stderr`) and `timeout` stay on core
`#Op` and are self-evaluated by the provider against the ffmpeg run.

## Layout

- `candy/plugin-media/` — the plugin module: `plugin.go` (provider + meta
  registration), `provider.go` (the `transcode` verb provider),
  `transcode.go`, `mjpeg.go`, `schema/transcode.cue` (the self-contained
  `#TranscodeInput`), `params/cue_types_gen.go`, and `cmd/serve/main.go`.
- `charly.yml` — the root project manifest (`discover: candy`).
- `.github/workflows/tag-on-merge.yml` — CalVer tag + `CHANGELOG/` on merge.

## Related

- Owning skill: `/charly-check:transcode` — the `transcode:` capture-evidence
  pipeline verb reference (the plugin's user-facing surface; the candy carries
  its own `transcode-skill:` entity).
- `/charly-check:check` — the check verb catalog and plan-step surface the
  `transcode:` verb is authored through.
- `/charly-internals:plugin` — the plugin/provider model.
- [`opencharly/charly`](https://github.com/opencharly/charly) — the charly CLI.
