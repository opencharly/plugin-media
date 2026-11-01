---
name: transcode
description: 'The `transcode:` capture-evidence pipeline verb — transcode a captured

  MJPEG stream into an H.264 MP4 with the host''s `ffmpeg`, served

  out-of-process by candy/plugin-media. Use before authoring a

  `transcode:` step in an instrument''s `pipeline:`, or when a capture

  artifact needs converting to a playable video.'
---

# transcode — capture-evidence MJPEG to H.264 MP4

The `transcode:` pipeline verb converts a captured MJPEG stream into an
H.264/yuv420p MP4 with the **host's** `ffmpeg`. It is **NOT a host
`charly check` subcommand** — it is a declarative pipeline word served
out-of-process by its plugin (`candy/plugin-media`), parallel to the
`record:`/`spice:` capture verbs. The MJPEG artifact a capture session
flushes (e.g. `spice: record`) is already host-side, so the provider
reaches only the local filesystem and the host `ffmpeg` — no
reverse-channel venue machinery.

**Served out-of-process — no host CLI subcommand.** The host dispatches
the `transcode:` word through the provider registry exactly like a
built-in (`ResolveVerb("transcode")` → the out-of-process gRPC provider
→ `Provider.Invoke` with the full `Op`), executing in the **evidence
phase** BEFORE venue teardown. Authoring is unchanged from a built-in
verb: you write `transcode: mp4`, never `plugin: transcode`.

## Authoring a `transcode:` step

The word is authored in an instrument's `pipeline:` list. The scalar form
is the `to` primary-field shorthand (`transcode: mp4` ==
`transcode: {to: mp4}`); the exclusive fields live INSIDE the `transcode:`
map.

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
|-------|---------|
| `to` | container format; only `mp4` is supported (the default; empty/omitted means mp4) |
| `artifact` | host path the MP4 is written to; empty → derived from the source (`<source>.mp4`) |
| `artifact_min_bytes` | post-transcode artifact-size assertion (the output MP4 must be at least N bytes) |
| `artifact_not_uniform` | SOURCE-frame motion assertion, evaluated on the SOURCE MJPEG frames (pre-encode) |

The shared matchers (`exit_status`/`stdout`/`stderr`) and `timeout` stay
on core `#Op` and are self-evaluated by the provider against the ffmpeg
run.

## Behavior

The provider runs the one canonical invocation (even-dimension scale +
timescale pin for robustness):

```
ffmpeg -y -loglevel error -i <src> -vf scale=trunc(iw/2)*2:trunc(ih/2)*2 \
  -c:v libx264 -pix_fmt yuv420p -video_track_timescale 1000 <out>
```

**`artifact_not_uniform` runs on the SOURCE MJPEG frames (pre-encode),
NEVER on the transcoded output** — exact hashes of x264-encoded frames
are vacuous (QP noise). The provider splits the source stream into its
constituent JPEGs, decodes a stream-wide sample, and requires at least
two distinct frame signatures: identical frames FAIL honestly (a real
capture shows motion), differing frames PASS.

**DEPENDENCY:** requires host `ffmpeg` with `libx264` + `yuv420p`
support.

## Verification

`plan:` `check:` runs the deterministic command form with the literal
`true` target (the plugin envelope is internal only; a live transcode
needs a session artifact). The live R10 witness is the cutover-A
nested-capture bed that produces a session MJPEG and transcodes it.

## Cross-References

- `/charly-check:check` — the check-step surface the `transcode:` word is
  authored through, and the capture-evidence pipeline it runs in.
- `/charly-check:record` — the `record:` verb that produces the source
  MJPEG artifact the transcode consumes.
- `/charly-check:spice` — `spice: {method: record}` flushes the source
  MJPEG the transcode consumes.
- `/charly-internals:plugin` — the plugin/provider model (out-of-process
  placement, the per-plugin CUE-schema contract).
