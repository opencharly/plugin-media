package media

import (
	"bytes"
	"encoding/json"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opencharly/plugin-media/candy/plugin-media/params"
	pb "github.com/opencharly/spec/proto"
	"github.com/opencharly/spec/spec"
)

// transcode_test.go — the provider Invoke end to end: fixture MJPEG (concatenated
// JPEGs) → host ffmpeg transcode → valid MP4 (verified with ffprobe), plus the
// self-evaluated verdict paths: the shared exit_status/stdout/stderr matchers
// against the ffmpeg run, artifact_min_bytes against the produced file, and the
// SOURCE-frame artifact_not_uniform check (RDD-3). The ffmpeg-dependent tests
// skip honestly when the host lacks ffmpeg (the verb's documented dependency).

// --- helpers ---

func transcodeRequest(t *testing.T, op spec.Op, env transcodeEnv) *pb.InvokeRequest {
	t.Helper()
	pj, err := json.Marshal(op)
	if err != nil {
		t.Fatalf("marshal op: %v", err)
	}
	ej, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal env: %v", err)
	}
	return &pb.InvokeRequest{ParamsJson: pj, EnvJson: ej}
}

func invokeTranscode(t *testing.T, req *pb.InvokeRequest) (status, msg string) {
	t.Helper()
	reply, err := (provider{}).Invoke(t.Context(), req)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	return replyStatus(reply)
}

func hasFFmpeg(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("host ffmpeg not installed — skipping the transcode artifact test (verb dependency)")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("host ffprobe not installed — skipping the mp4 verification")
	}
}

// ffprobeKeys returns the ffprobe -show_entries key=value lines for the produced
// artifact (format_name + stream codec_name — the proof the output is a real MP4).
func ffprobeKeys(t *testing.T, path string) map[string]string {
	t.Helper()
	out, err := exec.Command("ffprobe", "-v", "error",
		"-show_entries", "format=format_name:stream=codec_name",
		"-of", "default=nw=1", path).CombinedOutput()
	if err != nil {
		t.Fatalf("ffprobe %q: %v (%s)", path, err, strings.TrimSpace(string(out)))
	}
	kv := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if i := strings.Index(line, "="); i > 0 {
			kv[line[:i]] = line[i+1:]
		}
	}
	return kv
}

func twoFrameMJPEG(t *testing.T, dir string) string {
	t.Helper()
	f1 := jpegFrame(t, 64, 48, solid(color.RGBA{120, 40, 40, 255}))
	f2 := jpegFrame(t, 64, 48, gradient())
	path := filepath.Join(dir, "source.mjpeg")
	if err := os.WriteFile(path, bytes.Join([][]byte{f1, f2}, nil), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

// --- end to end ---

func TestTranscodeToMp4(t *testing.T) {
	hasFFmpeg(t)
	dir := t.TempDir()
	source := twoFrameMJPEG(t, dir)
	out := filepath.Join(dir, "out.mp4")

	status, msg := invokeTranscode(t, transcodeRequest(t, spec.Op{
		PluginInput: map[string]any{
			"artifact":             out,
			"artifact_min_bytes":   100,  // a valid MP4 header + frames exceeds this
			"artifact_not_uniform": true, // differing source frames must pass
		},
	}, transcodeEnv{SourceArtifact: source}))
	if status != "pass" {
		t.Fatalf("transcode verdict = %s: %s", status, msg)
	}
	info, err := os.Stat(out)
	if err != nil {
		t.Fatalf("transcoded artifact missing: %v", err)
	}
	if info.Size() < 100 {
		t.Fatalf("artifact size %d < 100", info.Size())
	}
	kv := ffprobeKeys(t, out)
	// ffprobe reports mp4 under its mov-family alias
	// ("mov,mp4,m4a,3gp,3g2,mj2") — the mp4 container IS a member of that family.
	if !strings.Contains(kv["format_name"], "mp4") {
		t.Errorf("format = %q, want an mp4-family container", kv["format_name"])
	}
	if kv["codec_name"] != "h264" {
		t.Errorf("codec = %q, want h264", kv["codec_name"])
	}
}

// TestTranscodeArgvPinsSaneTrackTimescale is the screenrecord-duration robustness
// contract (E-5 run 2026.251.1258 evidence phase): an Appium stopRecordingScreen
// MP4 declares a 90000-Hz video track (r_frame_rate=90000/1, time_base 1/90000) with
// non-monotonic dts; un-pinned, the mp4 muxer derives a garbage output duration and
// dies with "Application provided duration: 3469067760 in stream 0 is invalid"
// (mux error -22 — reproduced live against the 1258 pulled mp4: old argv exit 234,
// the pinned argv exit 0). The transcoder must ALWAYS pin a sane output track
// timescale so the duration computation stays in range for every source. The real
// vector is proven by the E-5 bed's evidence phase; this test pins the argv contract.
func TestTranscodeArgvPinsSaneTrackTimescale(t *testing.T) {
	argv := transcodeArgv("/src/appium-1.mp4", "/out/appium-1.mp4")
	pinned := false
	for i, a := range argv {
		if a == "-video_track_timescale" && i+1 < len(argv) && argv[i+1] == "1000" {
			pinned = true
		}
	}
	if !pinned {
		t.Fatalf("transcode argv %v must pin -video_track_timescale 1000 (the screenrecord-duration overflow guard)", argv)
	}
	if argv[len(argv)-1] != "/out/appium-1.mp4" {
		t.Fatalf("transcode argv %v must end with the output path", argv)
	}
}

func TestTranscodeDefaultOutputPath(t *testing.T) {
	hasFFmpeg(t)
	dir := t.TempDir()
	source := filepath.Join(dir, "session-42.mjpeg")
	same := jpegFrame(t, 64, 48, solid(color.RGBA{90, 90, 90, 255}))
	if err := os.WriteFile(source, bytes.Join([][]byte{same, same}, nil), 0o644); err != nil {
		t.Fatal(err)
	}
	// No artifact in the input: the provider derives <source>.mp4. No not-uniform
	// assertion here — identical frames would fail it honestly (the next test).
	status, msg := invokeTranscode(t, transcodeRequest(t, spec.Op{}, transcodeEnv{SourceArtifact: source}))
	if status != "pass" {
		t.Fatalf("verdict = %s: %s", status, msg)
	}
	derived := filepath.Join(dir, "session-42.mp4")
	if _, err := os.Stat(derived); err != nil {
		t.Fatalf("derived output %q missing: %v", derived, err)
	}
}

// --- verdict honesty ---

func TestTranscodeUniformSourceFailsHonestly(t *testing.T) {
	hasFFmpeg(t)
	dir := t.TempDir()
	same := jpegFrame(t, 64, 48, solid(color.RGBA{90, 90, 90, 255}))
	source := filepath.Join(dir, "uniform.mjpeg")
	if err := os.WriteFile(source, bytes.Join([][]byte{same, same, same}, nil), 0o644); err != nil {
		t.Fatal(err)
	}
	status, msg := invokeTranscode(t, transcodeRequest(t, spec.Op{
		PluginInput: map[string]any{
			"artifact":             filepath.Join(dir, "u.mp4"),
			"artifact_not_uniform": true,
		},
	}, transcodeEnv{SourceArtifact: source}))
	if status != "fail" {
		t.Fatalf("identical source frames must FAIL artifact_not_uniform, verdict = %s: %s", status, msg)
	}
	if !strings.Contains(msg, "UNIFORM") {
		t.Errorf("failure must name the uniform condition, got: %s", msg)
	}
}

func TestTranscodeSharedMatcherSelfEvaluation(t *testing.T) {
	hasFFmpeg(t)
	dir := t.TempDir()
	source := twoFrameMJPEG(t, dir)

	t.Run("exit_status asserted equal passes", func(t *testing.T) {
		zero := 0
		status, msg := invokeTranscode(t, transcodeRequest(t, spec.Op{
			ExitStatus:  &zero,
			PluginInput: map[string]any{"artifact": filepath.Join(dir, "es0.mp4")},
		}, transcodeEnv{SourceArtifact: source}))
		if status != "pass" {
			t.Fatalf("verdict = %s: %s", status, msg)
		}
	})

	t.Run("exit_status mismatch fails", func(t *testing.T) {
		want := 7
		status, msg := invokeTranscode(t, transcodeRequest(t, spec.Op{
			ExitStatus:  &want,
			PluginInput: map[string]any{"artifact": filepath.Join(dir, "es7.mp4")},
		}, transcodeEnv{SourceArtifact: source}))
		if status != "fail" || !strings.Contains(msg, "exit=0, want 7") {
			t.Errorf("mismatched exit_status must name exit=0 want 7, got %s: %s", status, msg)
		}
	})

	t.Run("stderr matcher asserted on a failing run passes", func(t *testing.T) {
		// A corrupt source makes ffmpeg fail (runErr → exit=1 + stderr). Asserting
		// that failure via the SHARED matchers (exit_status: 1 + stderr contains the
		// ffmpeg diagnostic) must PASS the step — the shared self-evaluation loop.
		garbage := filepath.Join(dir, "corrupt.mjpeg")
		if err := os.WriteFile(garbage, []byte("definitely not a video"), 0o644); err != nil {
			t.Fatal(err)
		}
		want := 1
		status, msg := invokeTranscode(t, transcodeRequest(t, spec.Op{
			ExitStatus:  &want,
			Stderr:      spec.MatcherList{spec.Matcher{Op: "contains", Value: "ffmpeg transcode failed"}},
			PluginInput: map[string]any{"artifact": filepath.Join(dir, "stderr.mp4")},
		}, transcodeEnv{SourceArtifact: garbage}))
		if status != "pass" {
			t.Fatalf("asserted ffmpeg failure must pass, verdict = %s: %s", status, msg)
		}
	})

	t.Run("stderr matcher not matched fails", func(t *testing.T) {
		garbage := filepath.Join(dir, "corrupt2.mjpeg")
		if err := os.WriteFile(garbage, []byte("still not a video"), 0o644); err != nil {
			t.Fatal(err)
		}
		want := 1
		status, msg := invokeTranscode(t, transcodeRequest(t, spec.Op{
			ExitStatus:  &want,
			Stderr:      spec.MatcherList{spec.Matcher{Op: "contains", Value: "this exact text never appears"}},
			PluginInput: map[string]any{"artifact": filepath.Join(dir, "stderr2.mp4")},
		}, transcodeEnv{SourceArtifact: garbage}))
		if status != "fail" {
			t.Fatalf("unmatched stderr matcher must fail, verdict = %s: %s", status, msg)
		}
	})

	t.Run("artifact_min_bytes failure", func(t *testing.T) {
		status, msg := invokeTranscode(t, transcodeRequest(t, spec.Op{
			PluginInput: map[string]any{
				"artifact":           filepath.Join(dir, "big.mp4"),
				"artifact_min_bytes": 1 << 40, // absurd — the real file can never reach it
			},
		}, transcodeEnv{SourceArtifact: source}))
		if status != "fail" || !strings.Contains(msg, "min_bytes") {
			t.Errorf("min_bytes failure must name min_bytes, got %s: %s", status, msg)
		}
	})
}

func TestTranscodePreflightFailures(t *testing.T) {
	// A real source file (the pre-flight gates run BEFORE ffmpeg: the format gate
	// and the extension gate must trip without ever reaching the transcode).
	dir := t.TempDir()
	srcReal := filepath.Join(dir, "existing.mjpeg")
	if err := os.WriteFile(srcReal, []byte("a file that exists — pre-flight never reaches it"), 0o644); err != nil {
		t.Fatal(err)
	}
	srcMissing := filepath.Join(dir, "missing.mjpeg")
	cases := []struct {
		name string
		in   map[string]any
		env  transcodeEnv
		want string
	}{
		{"missing source env", map[string]any{}, transcodeEnv{}, "source_artifact"},
		{"missing source file", map[string]any{"artifact": filepath.Join(dir, "a.mp4")}, transcodeEnv{SourceArtifact: srcMissing}, "source artifact"},
		{"unsupported format", map[string]any{"to": "webm"}, transcodeEnv{SourceArtifact: srcReal}, "single-purpose"},
		{"non-mp4 artifact extension", map[string]any{"artifact": filepath.Join(dir, "x.mkv")}, transcodeEnv{SourceArtifact: srcReal}, ".mp4"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, msg := invokeTranscode(t, transcodeRequest(t, spec.Op{PluginInput: tc.in}, tc.env))
			if status != "fail" || !strings.Contains(msg, tc.want) {
				t.Errorf("want fail mentioning %q, got %s: %s", tc.want, status, msg)
			}
		})
	}
}

// --- pure helpers ---

func TestValidateFormat(t *testing.T) {
	if err := validateFormat(""); err != nil {
		t.Errorf("empty to: %v", err)
	}
	if err := validateFormat("mp4"); err != nil {
		t.Errorf("mp4 to: %v", err)
	}
	if err := validateFormat("mpegts"); err == nil {
		t.Error("mpegts must be rejected (single-purpose verb)")
	}
}

func TestOutputPath(t *testing.T) {
	if got := outputPathOrFail(t, params.TranscodeInput{}, "/sessions/screen.mjpeg"); got != "/sessions/screen.mp4" {
		t.Errorf("derived path = %q", got)
	}
	if got := outputPathOrFail(t, params.TranscodeInput{Artifact: "/out/cap.mp4"}, "/sessions/screen.mjpeg"); got != "/out/cap.mp4" {
		t.Errorf("explicit path = %q", got)
	}
	// Case-insensitive container check: .MP4 / .Mp4 are the SAME mp4 container and
	// must be accepted (only a DIFFERENT extension is rejected — the preflight test).
	if got := outputPathOrFail(t, params.TranscodeInput{Artifact: "/out/cap.MP4"}, "/sessions/screen.mjpeg"); got != "/out/cap.MP4" {
		t.Errorf("case-variant mp4 artifact = %q", got)
	}
	if _, err := outputPath(&params.TranscodeInput{Artifact: "/out/cap.avi"}, "/sessions/screen.mjpeg"); err == nil {
		t.Error("non-mp4 artifact extension must be rejected")
	}
}

func outputPathOrFail(t *testing.T, in params.TranscodeInput, source string) string {
	t.Helper()
	out, err := outputPath(&in, source)
	if err != nil {
		t.Fatalf("outputPath: %v", err)
	}
	return out
}

// TestTranscodeSourceResolution covers the evidence-phase threading contract
// (B12): the source artifact rides the op input first (source_artifact — the
// bed-runner evidence phase threads the entry's primary artifact there), with
// the check env as the fallback for a direct/plan-step dispatch. And when the
// evidence phase's injected artifact (the shared validators' contract) IS the
// source, it is not an authored output path — the output must derive from the
// source instead of being rejected as a non-mp4 artifact.
func TestTranscodeSourceResolution(t *testing.T) {
	hasFFmpeg(t)
	dir := t.TempDir()
	src := twoFrameMJPEG(t, dir)
	envSrc := filepath.Join(dir, "env.mjpeg")
	if err := os.WriteFile(envSrc, []byte("env source — must NOT be used when the input threads one"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Input-first: source_artifact in the op input wins over the env (the
	// evidence phase's threading). The output derives from the INPUT source.
	status, msg := invokeTranscode(t, transcodeRequest(t, spec.Op{PluginInput: map[string]any{
		"source_artifact": src,
	}}, transcodeEnv{SourceArtifact: envSrc}))
	if status != "pass" {
		t.Fatalf("input-first: want pass (input source transcodes), got %s: %s", status, msg)
	}
	if _, err := os.Stat(filepath.Join(dir, "source.mp4")); err != nil {
		t.Errorf("input-first: output derived from the input source (source.mp4): %v", err)
	}
	// Env fallback: no input source_artifact → the env's source is used.
	status, msg = invokeTranscode(t, transcodeRequest(t, spec.Op{PluginInput: map[string]any{}}, transcodeEnv{SourceArtifact: src}))
	if status != "pass" {
		t.Fatalf("env fallback: want pass (env source transcodes), got %s: %s", status, msg)
	}
	// Artifact==source clearing: the evidence phase injects artifact = the source
	// (the shared validators' contract). It must NOT be treated as an authored
	// output path — the output derives from the source (source.mp4), so the
	// transcode passes instead of tripping the non-mp4 artifact gate.
	status, msg = invokeTranscode(t, transcodeRequest(t, spec.Op{PluginInput: map[string]any{
		"source_artifact": src,
		"artifact":        src,
	}}, transcodeEnv{}))
	if status != "pass" {
		t.Fatalf("artifact==source: want pass (artifact gate skipped, output derives from source), got %s: %s", status, msg)
	}
}
