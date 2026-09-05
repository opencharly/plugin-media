# plugin-media

The `transcode` pipeline verb plugin candy of the [opencharly/charly](https://github.com/opencharly/charly)
candy library, as a standalone repo (the nested-capture cutover A, the only NEW org repo).
The Go module lives at `candy/plugin-media/` with module path
`github.com/opencharly/plugin-media/candy/plugin-media`; the charly resolver fetches this repo at the pinned tag and
the compiled-in wiring imports the module at that path.

## The verb

`transcode:` is a SINGLE-PURPOSE host-side video transcode verb: it converts a source MJPEG
video (the artifact a capture session flushed host-side, e.g. `spice: record`) into an
H.264 MP4 container via the host's `ffmpeg`:

```yaml
instrument:
    - id: screen
      phase: [live]
      spice: {method: session, fps: 5}
      pipeline:
          - transcode: {to: mp4}          # map form
          - transcode: mp4                # scalar-sugar equal (primary field: to)
```

- **`to`** — the container format; only `mp4` is supported (single-purpose verb, default).
- **`artifact`** — the host path the transcoded MP4 is written to; empty → derived from the
  source path (`<source>.mp4`).
- **`artifact_min_bytes`** — post-transcode artifact-size assertion.
- **`artifact_not_uniform`** — SOURCE-frame motion assertion: evaluated on the SOURCE MJPEG
  frames (pre-encode), never on the transcoded output — exact hashes of x264-encoded frames
  are vacuous (QP noise, the RDD-3 binding of the nested-capture plan).
- The shared assertion matchers (`exit_status`/`stdout`/`stderr`) and `timeout` stay on
  core #Op and are self-evaluated by the provider against the ffmpeg run.

**Source contract:** the provider resolves the source MJPEG from the check-env snapshot key
`source_artifact` (the host path of the session's flushed artifact), threaded by the
bed-runner evidence phase (Cutover A task 4, `plugin-check`). **Requires host `ffmpeg`**
(the dependency note in the candy description).

## Developer

```
cd candy/plugin-media
go build ./... && go vet ./... && go test ./...
```

First tag convention: `candy/plugin-media/v0.<CalVer>` (subdirectory module tag). The candy
`version:` stamp + `NewMeta` calver equal the tag's CalVer.
