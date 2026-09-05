package media

import (
	"bytes"
	"errors"
	"fmt"
	"hash/fnv"
	"image"
	_ "image/jpeg" // register the JPEG decoder for image.Decode on the source MJPEG frames
	"os"
)

// mjpeg.go — the SOURCE-frame artifact_not_uniform validator (the RDD-3 binding
// of the nested-capture plan). A video's motion is judged on the SOURCE MJPEG
// frames (pre-encode), NEVER on the transcoded output: exact hashes of
// transcoded frames are vacuous (x264 QP noise), and the stdlib image decoder
// cannot decode an MP4 at all. This file splits an MJPEG stream (a concatenation
// of full JPEG frames) into its constituent JPEGs, decodes a stream-wide sample,
// and requires at least TWO distinct frame signatures — identical frames FAIL
// honestly (a real capture shows motion), differing frames PASS.
//
// R3 note: the shared sdk.artifact validator implements the single-image
// not-uniform check (sdk.RunArtifactValidators → assertArtifactNotUniform); it
// cannot reach a VIDEO stream, so the frame-level check lives here (a different
// artifact type — JPEG-stream video — not a duplicate of the single-image
// checker). The sampling granularity (100 pixel points per frame) mirrors the
// shared validator's.

// maxUniformSampleFrames caps the frames sampled from a stream
// (evenly across the whole stream, so late motion is still seen).
const maxUniformSampleFrames = 32

// notUniformSampleRaster is the per-frame sample raster (rows x cols = 100
// points — the same count the shared single-image validator samples).
const notUniformSampleRaster = 10

// checkSourceNotUniform fails unless the MJPEG at path shows motion: the stream
// is split into its JPEG frames and a stream-wide sample must yield at least two
// distinct frame signatures. A stream with fewer than two frames is uniform by
// definition; one whose sampled frames all fail to decode is not the artifact
// this verb exists for — both fail honestly.
func checkSourceNotUniform(path string) error {
	if path == "" {
		return errors.New("no source artifact to evaluate (source_artifact empty)")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("source artifact %q: %w", path, err)
	}
	frames := splitMJPEGFrames(data)
	if len(frames) < 2 {
		return fmt.Errorf("source %q has %d MJPEG frame(s); artifact_not_uniform (motion) requires at least 2 frames", path, len(frames))
	}

	// Stream-wide even sampling: step covers the whole file, so motion anywhere in
	// the recording is seen, not just at the start.
	step := (len(frames) + maxUniformSampleFrames - 1) / maxUniformSampleFrames
	if step < 1 {
		step = 1
	}
	distinct := map[uint32]struct{}{}
	decoded, corrupt := 0, 0
	for i := 0; i < len(frames); i += step {
		sig, ferr := frameSignature(frames[i])
		if ferr != nil {
			corrupt++
			continue
		}
		decoded++
		distinct[sig] = struct{}{}
		if len(distinct) >= 2 {
			return nil
		}
	}
	if decoded == 0 {
		return fmt.Errorf("source %q: %d sampled frame(s) failed to decode — not a decodable JPEG/MJPEG stream", path, corrupt)
	}
	return fmt.Errorf("source %q frames are UNIFORM: %d sampled frame(s), 1 distinct signature — the recording shows no motion (identical frames; artifact_not_uniform fails honestly)", path, decoded)
}

// frameSignature decodes ONE JPEG frame and hashes a fixed raster of pixel
// samples — a deterministic, content-sensitive per-frame fingerprint. It
// intentionally hashes SAMPLED PIXELS (not the encoded bytes): two visually
// identical frames with different JPEG encodings still collide (the honest
// uniformity question), while any visible variation changes the fingerprint.
func frameSignature(frame []byte) (uint32, error) {
	img, _, err := image.Decode(bytes.NewReader(frame))
	if err != nil {
		return 0, fmt.Errorf("frame decode: %w", err)
	}
	b := img.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 {
		return 0, errors.New("frame decode: empty image bounds")
	}
	var buf bytes.Buffer
	for r := 0; r < notUniformSampleRaster; r++ {
		y := b.Min.Y + pixelIndex(b.Dy(), r)
		for c := 0; c < notUniformSampleRaster; c++ {
			x := b.Min.X + pixelIndex(b.Dx(), c)
			rr, gg, bb, _ := img.At(x, y).RGBA()
			buf.WriteByte(byte(rr >> 8))
			buf.WriteByte(byte(gg >> 8))
			buf.WriteByte(byte(bb >> 8))
		}
	}
	h := fnv.New32a()
	_, _ = h.Write(buf.Bytes())
	return h.Sum32(), nil
}

// pixelIndex maps raster step r (0..N-1) to a pixel offset inside a dimension:
// even integer sampling with the final step pinned to the last pixel, so a
// 1-pixel image still samples that pixel and a large image is covered edge to
// edge. All samples stay in bounds by construction.
func pixelIndex(size, r int) int {
	if size <= 1 {
		return 0
	}
	p := int(int64(size) * int64(r) / int64(notUniformSampleRaster))
	if p >= size {
		p = size - 1
	}
	return p
}

// splitMJPEGFrames splits a stream of concatenated JPEGs into its frames via the
// JPEG SOI (0xFFD8) / EOI (0xFFD9) markers. Marker-safe by construction: a valid
// JPEG stream escapes every 0xFF inside entropy-coded data (0xFF 0x00) and every
// marker frame is terminated by its EOI, so scanning for the raw marker bytes is
// unambiguous. An unterminated trailing frame (truncated capture tail) is
// dropped, and any non-JPEG bytes between frames are skipped — the stream may
// carry mux padding, and honesty favors the decodable frames.
func splitMJPEGFrames(data []byte) [][]byte {
	const (
		soi = uint16(0xFFD8)
		eoi = uint16(0xFFD9)
	)
	var frames [][]byte
	for i := 0; i+1 < len(data); {
		start := findMarker(data, i, soi)
		if start < 0 {
			break
		}
		end := findMarker(data, start+2, eoi)
		if end < 0 {
			break // unterminated tail frame — not a complete frame, drop it
		}
		frames = append(frames, data[start:end+2])
		i = end + 2
	}
	return frames
}

// findMarker scans for the two-byte JPEG marker m starting at from.
func findMarker(data []byte, from int, m uint16) int {
	hi, lo := byte(m>>8), byte(m&0xFF)
	for i := from; i+1 < len(data); i++ {
		if data[i] == hi && data[i+1] == lo {
			return i
		}
	}
	return -1
}
