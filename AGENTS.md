# AGENTS.md — plugin-media

Standalone out-of-tree plugin repo serving the host-side `transcode` pipeline
verb (`verb:transcode`). The plugin is a Go module at `candy/plugin-media/`
(module path `github.com/opencharly/plugin-media/candy/plugin-media`); the root
`charly.yml` only declares `discover: candy` so the repo is a project and its
candy is scanned.

Canonical files:

- `candy/plugin-media/charly.yml` — the `plugin-media:` candy entity (`plugin:`
  block, `plan:` check).
- `candy/plugin-media/` — the Go source: `plugin.go`, `provider.go`,
  `transcode.go`, `mjpeg.go`, `schema/transcode.cue`,
  `params/cue_types_gen.go`, `cmd/serve/main.go`.
- `.github/workflows/tag-on-merge.yml` — CalVer tag + `CHANGELOG/` on merge.
- `README.md` — user overview only; never agent guidance.

## Load these skills first (R0)

- `/charly-internals:plugin` — the plugin authoring reference: the `plugin:`
  block, the unified Provider model, the per-plugin CUE-schema contract,
  placement. Load before touching the provider or schema.
- `/charly-check:check` — the declarative check-step surface the `transcode:`
  verb is authored through, and the capture-evidence pipeline it runs in.
- `/charly-check:record` — the `record:` verb that produces the source MJPEG
  artifact the transcode consumes.
- `/charly-internals:git-workflow` — before any git/PR action.

## Build / validate / test

- `go build ./...` in `candy/plugin-media/` — compile the plugin module.
- `go test ./...` in `candy/plugin-media/` — the plugin's Go tests (the
  transcode + MJPEG seams).
- `charly box validate` at the repo root — the structural check (the candy +
  `plugin:` block, CUE schema).
- The merge gate is the **org-wide** `charly/pr-validator` (required check
  `validate / validate`, defined in `opencharly/.github`); this repo has **no**
  per-repo candy gate.
- The live R10 witness is the cutover-A nested-capture bed that produces a
  session MJPEG and transcodes it.

## Modify this repo

- Edit the `plugin-media:` candy entity, the Go source, and
  `schema/transcode.cue` **together** — the schema is the single source for the
  verb's `params/` struct, so a field change not mirrored in the schema desyncs
  the generated types.
- `artifact_not_uniform` runs on the SOURCE MJPEG frames (pre-encode); exact
  hashes of x264-encoded frames are vacuous (QP noise). Do not move the
  assertion onto the transcoded output.

## Landing

Load `/charly-internals:git-workflow` before any git/PR action; it owns the
landing mechanics. The authoritative rulebook is the umbrella `AGENTS.md` in
`opencharly/opencharly` and `charly/AGENTS.md` in the charly repo.
