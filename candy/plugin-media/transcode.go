package media

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/opencharly/plugin-media/candy/plugin-media/params"
	"github.com/opencharly/spec/spec"
)

// transcode.go — the host-side ffmpeg transcode + output path resolution +
// format gate. The MJPEG artifact is already host-side (B1), so this verb needs
// NO reverse-channel venue leg: it reaches only the local filesystem and the
// host's ffmpeg binary (the dependency the candy description notes).

// transcodeFormat is the closed set of supported container formats. SINGLE-PURPOSE
// verb (the plan's binding): only "mp4", and an empty/omitted `to` means mp4.
const transcodeFormat = "mp4"

// maxTranscodeStderr caps the ffmpeg output folded into the run error, so a
// verbose ffmpeg complaint never floods a verdict message.
const maxTranscodeStderr = 2048

// runTranscode resolves the source + output paths, validates the single-purpose
// `to` format, and runs the host ffmpeg transcode:
//
//	ffmpeg -y -loglevel error -i <mjpeg> -vf scale=trunc(iw/2)*2:trunc(ih/2)*2 -c:v libx264 -pix_fmt yuv420p <out>
//
// It returns the resolved output path (the artifact the shared validators + the
// caller stat), the ffmpeg run output (for the shared stdout/stderr/exit
// matchers), and a non-nil error for any pre-flight or run failure. A non-zero
// ffmpeg exit becomes a runErr carrying the (capped) ffmpeg stderr, which the
// shared verdict pipeline maps to exit=1 + stderr and matches against
// op.ExitStatus/op.Stderr.
func runTranscode(ctx context.Context, op *spec.Op, in *params.TranscodeInput, source string) (outPath, ffmpegOutput string, runErr error) {
	if source == "" {
		return "", "", errors.New("no source artifact in the check env (source_artifact empty — the bed-runner evidence phase must thread the session's flushed MJPEG path)")
	}
	if _, err := os.Stat(source); err != nil {
		return "", "", fmt.Errorf("source artifact %q: %w", source, err)
	}
	if err := validateFormat(in.To); err != nil {
		return "", "", err
	}
	out, err := outputPath(in, source)
	if err != nil {
		return "", "", err
	}

	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return "", "", errors.New("host ffmpeg not found — the transcode verb requires host ffmpeg with libx264 + yuv420p support (dependency noted in the candy description)")
	}
	// Even-dimension scale (RCA 2026-09-07, Cutover E-3 R10 bed): the capture verbs
	// are free to hand the recorder whatever the browser/viewer produces — the E-3
	// chrome-headless screencast measured 780x437 (odd height) — and libx264
	// rejects non-even frame dimensions. Scale to the nearest even size (trunc,
	// never ceil, so a 1px dimension stays >= 0; aspect ratio preserved via the
	// same factor on both axes).
	argv := []string{"-y", "-loglevel", "error", "-i", source, "-vf", "scale=trunc(iw/2)*2:trunc(ih/2)*2", "-c:v", "libx264", "-pix_fmt", "yuv420p", out}
	cmd := exec.CommandContext(ctx, ffmpeg, argv...)
	bout, rerr := cmd.CombinedOutput()
	if rerr != nil {
		msg := strings.TrimSpace(string(bout))
		if len(msg) > maxTranscodeStderr {
			msg = msg[:maxTranscodeStderr] + "…"
		}
		return out, "", fmt.Errorf("ffmpeg transcode failed: %s", msg)
	}
	if _, statErr := os.Stat(out); statErr != nil {
		return out, "", fmt.Errorf("transcode produced no output at %q: %v", out, statErr)
	}
	return out, string(bout), nil
}

// validateFormat enforces the single-purpose `to` field: "" or "mp4" only. An
// empty value means the default (mp4). Any other container is a HARD fail — the
// verb is mp4-only by design, so a typo never silently produces a surprise.
func validateFormat(to string) error {
	if to != "" && to != transcodeFormat {
		return fmt.Errorf("single-purpose verb: to=%q unsupported (only %q)", to, transcodeFormat)
	}
	return nil
}

// outputPath resolves where the transcoded file lands: the explicit input
// `artifact` when set, otherwise the source path with its extension swapped to
// ".mp4" (the common `transcode: {to: mp4}` authoring needs no path at all).
// An explicit artifact that is not an .mp4 is rejected (container/extension
// honesty — the output is ALWAYS the mp4 container).
func outputPath(in *params.TranscodeInput, source string) (string, error) {
	if in.Artifact != "" {
		if ext := strings.ToLower(filepath.Ext(in.Artifact)); ext != "."+transcodeFormat {
			return "", fmt.Errorf("artifact %q: single-purpose verb writes only the .mp4 container", in.Artifact)
		}
		return in.Artifact, nil
	}
	base := strings.TrimSuffix(filepath.Base(source), filepath.Ext(source))
	return filepath.Join(filepath.Dir(source), base+"."+transcodeFormat), nil
}
