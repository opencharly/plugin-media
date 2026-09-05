package media

import (
	"context"
	"encoding/json"

	"github.com/opencharly/plugin-media/candy/plugin-media/params"
	"github.com/opencharly/sdk"
	"github.com/opencharly/sdk/kit"
	pb "github.com/opencharly/spec/proto"
	"github.com/opencharly/spec/spec"
)

// provider.go is the out-of-process `transcode` verb provider — charly's host
// dispatches a `transcode:` pipeline word to it through the registry
// (ResolveVerb("transcode") → this grpcProvider → invokeVerbProvider) with the
// FULL #Op marshaled as params_json and a CheckEnv snapshot as env. Because the
// out-of-process path does NOT run a host-side matcher pipeline, this Invoke
// OWNS the whole verdict: run the host ffmpeg transcode, then evaluate the
// exit_status/stdout/stderr matchers + the artifact validators itself (via the
// shared sdk implementation — R3) and return the wire {status,message} the host
// decodes.

// transcodeEnv is the plugin-side decode of the CheckEnv snapshot the host ships
// as Operation.Env for a `transcode:` pipeline word. The pipeline executes in the
// evidence phase (cutover A task 4, plugin-check) with the run-scoped context the
// bed-runner threads: the ONE field transcode needs is source_artifact — the HOST
// path of the source MJPEG the capture session flushed (B1: the MJPEG is already
// host-side, so no reverse-channel venue leg exists for this verb). The bed
// runner owns the threading; see the schema comment + README.
type transcodeEnv struct {
	SourceArtifact string `json:"source_artifact"`
}

type provider struct{ pb.UnimplementedProviderServer }

// Invoke runs one `transcode:` operation: it decodes the full #Op + the typed
// plugin input (params.TranscodeInput) + the env, resolves the output path
// (explicit artifact or the derived <source>.mp4), runs the host-side ffmpeg
// transcode (ffmpeg -y -loglevel error -i <mjpeg> -c:v libx264 -pix_fmt yuv420p
// <out>), and self-evaluates:
//
//   - the shared exit_status/stdout/stderr matchers against the ffmpeg run, and
//     artifact_min_bytes against the produced MP4 (sdk.VerbVerdict, the shared
//     pipeline — with the real output path injected into the plugin-input copy);
//   - artifact_not_uniform on the SOURCE MJPEG frames (pre-encode; the RDD-3
//     binding — never on the transcoded output, where exact frame hashes are
//     vacuous x264 QP noise). The shared validator would image-decode the MP4
//     (vacuous), so it is stripped from the copy passed to the shared pipeline
//     and this provider runs its own source-frame check afterwards.
func (p provider) Invoke(ctx context.Context, req *pb.InvokeRequest) (*pb.InvokeReply, error) {
	var op spec.Op
	if len(req.GetParamsJson()) > 0 {
		if err := json.Unmarshal(req.GetParamsJson(), &op); err != nil {
			return sdk.ResultJSON("fail", "transcode: decode op: "+err.Error())
		}
	}
	var in params.TranscodeInput
	kit.DecodeInput(op.PluginInput, &in)
	var env transcodeEnv
	if len(req.GetEnvJson()) > 0 {
		_ = json.Unmarshal(req.GetEnvJson(), &env)
	}
	// The source artifact rides the op input first (the bed-runner evidence phase
	// threads the entry's primary artifact as source_artifact — the env is fixed at
	// runner construction and cannot carry a per-entry path), with the check env as
	// the fallback for a direct/plan-step dispatch.
	source := env.SourceArtifact
	if s, ok := op.PluginInput["source_artifact"].(string); ok && s != "" {
		source = s
	}
	// The evidence phase threads the entry's primary artifact under BOTH artifact
	// (the shared validators' contract) and source_artifact (this verb's source).
	// When the injected artifact IS the source, it is not an authored output path
	// — clear it so the output derives from the source (the common authoring).
	if in.Artifact != "" && in.Artifact == source {
		in.Artifact = ""
	}

	outPath, _, runErr := runTranscode(ctx, &op, &in, source)
	out := outPath

	// Self-evaluation via the SHARED verdict pipeline (R3). Two transcode-specific
	// adjustments: the actual output path is injected into a plugin-input COPY so
	// the shared artifact_min_bytes validator stats the real file, and
	// artifact_not_uniform is stripped from that copy (the shared validator would
	// image-decode the MP4 — the vacuous behavior RDD-3 forbids).
	val := op
	pi := make(map[string]any, len(op.PluginInput)+1)
	for k, v := range op.PluginInput {
		pi[k] = v
	}
	pi["artifact"] = out
	if in.ArtifactNotUniform {
		delete(pi, "artifact_not_uniform")
	}
	val.PluginInput = pi
	reply, verr := sdk.VerbVerdict("transcode", "transcode", "", runErr, &val, true)
	if verr != nil {
		return nil, verr
	}
	if status, _ := replyStatus(reply); status != "pass" {
		return reply, nil
	}

	// artifact_not_uniform: the SOURCE-frame motion assertion (RDD-3). Runs on the
	// pre-encode MJPEG frames — after the shared verdict passed, so a bad transcode
	// fails on the shared pipeline first.
	if in.ArtifactNotUniform {
		if err := checkSourceNotUniform(env.SourceArtifact); err != nil {
			return sdk.ResultJSON("fail", "transcode: "+err.Error())
		}
	}
	return reply, nil
}

// replyStatus decodes the {status,message} wire every out-of-process check verb
// returns (ResultJSON → InvokeReply.ResultJson; the host's pluginCheckResult
// reads the same shape). The provider uses it to branch on the shared verdict
// pipeline's outcome without duplicating the wire contract.
func replyStatus(reply *pb.InvokeReply) (status, message string) {
	if reply == nil || len(reply.GetResultJson()) == 0 {
		return "", ""
	}
	var w struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(reply.GetResultJson(), &w); err != nil {
		return "", ""
	}
	return w.Status, w.Message
}
