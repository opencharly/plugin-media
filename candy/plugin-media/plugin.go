// Package media is the charly plugin serving the `transcode` pipeline check verb
// (an importable root package + its own go.mod) — the ONLY new org repo of the
// nested-capture cutover A. It runs a HOST-side ffmpeg transcode of a source MJPEG
// video (the artifact a capture session flushed — e.g. `spice: record`) into an
// H.264/yuv420p MP4 container and SELF-EVALUATES the artifact validators + the
// shared exit/stdout/stderr matchers. The host go-builds this binary and serves it
// OUT-OF-PROCESS over go-plugin gRPC via the charly plugin SDK, so the
// `transcode:` pipeline word dispatches through the provider registry exactly like
// a built-in (ResolveVerb → grpcProvider → invokeVerbProvider hands it the full
// #Op). Host-side by construction: the source MJPEG is already a host file, so the
// provider owns NO reverse-channel venue machinery — it reaches only the local
// filesystem and the host's ffmpeg binary.
//
// RDD-3 binding (the plan's validator semantics): `artifact_not_uniform` runs on
// the SOURCE MJPEG frames (pre-encode), never on the transcoded output — exact
// hashes of transcoded frames are vacuous (x264 QP noise). The provider splits the
// source stream into constituent JPEGs, decodes a stream-wide sample, and requires
// at least two distinct frame signatures; identical frames FAIL the assertion
// honestly, differing frames PASS.
//
// Dual-placement by construction: the SAME NewProvider()/NewMeta() compile INTO
// charly in-process when listed in compiled_plugins, or cmd/serve serves them
// OUT-OF-PROCESS over go-plugin gRPC when they are not — placement is invisible
// above the registry.
package media

import (
	"embed"

	"github.com/opencharly/sdk"
	pb "github.com/opencharly/spec/proto"
)

//go:embed schema/*.cue
var schemaFS embed.FS

// NewProvider returns the transcode provider.
func NewProvider() pb.ProviderServer { return &provider{} }

// NewMeta advertises verb:transcode + the plugin's self-contained CUE schema (via
// sdk.NewMeta → BuildCapabilities). The verb's entire authoring contract — the
// single-purpose `to` format + the artifact validators — lives in the served
// #TranscodeInput (schema/transcode.cue), which the host splices onto the base and
// validates every authored `transcode:` pipeline word's plugin_input against.
// Primary: "to" declares the scalar-sugar field (`transcode: mp4` ≡
// `transcode: {to: mp4}`), mirrored in the candy manifest's plugin.primary map so
// the byte-gated prescan knows it BEFORE the provider connects.
func NewMeta() pb.PluginMetaServer {
	return sdk.NewMeta("2026.248.1230",
		[]sdk.ProvidedCapability{{Class: "verb", Word: "transcode", InputDef: "#TranscodeInput", Primary: "to"}},
		schemaFS)
}
