package media

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mjpeg_test.go — the SOURCE-frame artifact_not_uniform validator + the MJPEG
// splitter. The fixture JPEGs are encoded by the stdlib (image/jpeg) exactly as
// the provider decodes them, so the tests are hermetic (no ffmpeg needed).

func jpegFrame(t *testing.T, w, h int, fill func(x, y int) color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, fill(x, y))
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatalf("encode fixture JPEG: %v", err)
	}
	return buf.Bytes()
}

func solid(c color.Color) func(x, y int) color.Color {
	return func(x, y int) color.Color { return c }
}

func gradient() func(x, y int) color.Color {
	return func(x, y int) color.Color { return color.RGBA{uint8(x * 4), uint8(y * 4), 128, 255} }
}

func writeFixture(t *testing.T, dir, name string, frames ...[]byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, bytes.Join(frames, nil), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func TestSplitMJPEGFrames(t *testing.T) {
	f1 := jpegFrame(t, 16, 16, solid(color.RGBA{200, 30, 30, 255}))
	f2 := jpegFrame(t, 16, 16, solid(color.RGBA{30, 200, 30, 255}))
	f3 := jpegFrame(t, 16, 16, gradient())

	t.Run("three framed stream", func(t *testing.T) {
		frames := splitMJPEGFrames(bytes.Join([][]byte{f1, f2, f3}, nil))
		if len(frames) != 3 {
			t.Fatalf("got %d frames, want 3", len(frames))
		}
		if !bytes.Equal(frames[0], f1) || !bytes.Equal(frames[2], f3) {
			t.Error("frame split corrupted frame boundaries")
		}
	})
	t.Run("inter-frame mux padding", func(t *testing.T) {
		// garbage bytes between frames (mux padding) must be skipped, not parsed.
		pad := []byte{0x00, 0x01, 0xDE, 0xAD, 0xBE, 0xEF}
		frames := splitMJPEGFrames(bytes.Join([][]byte{f1, pad, f2, pad, f3}, nil))
		if len(frames) != 3 {
			t.Fatalf("got %d frames with padding, want 3", len(frames))
		}
	})
	t.Run("unterminated tail frame dropped", func(t *testing.T) {
		tail := f2[:len(f2)-4] // truncate the final EOI
		frames := splitMJPEGFrames(bytes.Join([][]byte{f1, tail}, nil))
		if len(frames) != 1 {
			t.Fatalf("got %d frames, want 1 (unterminated tail dropped)", len(frames))
		}
	})
	t.Run("empty and garbage-only streams", func(t *testing.T) {
		if frames := splitMJPEGFrames(nil); len(frames) != 0 {
			t.Fatalf("empty stream => %d frames, want 0", len(frames))
		}
		if frames := splitMJPEGFrames([]byte("not a jpeg stream at all")); len(frames) != 0 {
			t.Fatalf("garbage stream => %d frames, want 0", len(frames))
		}
	})
	t.Run("single frame", func(t *testing.T) {
		frames := splitMJPEGFrames(f1)
		if len(frames) != 1 {
			t.Fatalf("got %d frames, want 1", len(frames))
		}
	})
}

func TestSourceNotUniform(t *testing.T) {
	dir := t.TempDir()
	sameA := jpegFrame(t, 32, 32, solid(color.RGBA{90, 90, 90, 255}))
	sameB := jpegFrame(t, 32, 32, solid(color.RGBA{90, 90, 90, 255}))
	diffA := jpegFrame(t, 32, 32, solid(color.RGBA{10, 200, 10, 255}))
	diffB := jpegFrame(t, 32, 32, gradient())

	t.Run("identical frames fail honestly", func(t *testing.T) {
		path := writeFixture(t, dir, "identical.mjpeg", sameA, sameB, sameA, sameB)
		if err := checkSourceNotUniform(path); err == nil {
			t.Fatal("identical frames: expected UNIFORM failure, got nil")
		} else if !strings.Contains(err.Error(), "UNIFORM") {
			t.Fatalf("failure should name the uniform condition, got: %v", err)
		}
	})
	t.Run("differing frames pass", func(t *testing.T) {
		path := writeFixture(t, dir, "moving.mjpeg", sameA, diffA, sameA, diffA)
		if err := checkSourceNotUniform(path); err != nil {
			t.Fatalf("differing frames: expected pass, got %v", err)
		}
	})
	t.Run("gradient vs solid pass", func(t *testing.T) {
		path := writeFixture(t, dir, "moving2.mjpeg", diffA, diffB)
		if err := checkSourceNotUniform(path); err != nil {
			t.Fatalf("differing frames: expected pass, got %v", err)
		}
	})
	t.Run("single frame fails", func(t *testing.T) {
		path := writeFixture(t, dir, "one.mjpeg", sameA)
		if err := checkSourceNotUniform(path); err == nil {
			t.Fatal("single frame: expected failure (needs >= 2 frames), got nil")
		}
	})
	t.Run("not a jpeg stream fails", func(t *testing.T) {
		path := filepath.Join(dir, "garbage.mjpeg")
		if err := os.WriteFile(path, []byte("this is not an mjpeg stream"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := checkSourceNotUniform(path); err == nil {
			t.Fatal("garbage stream: expected failure, got nil")
		}
	})
	t.Run("missing file fails", func(t *testing.T) {
		if err := checkSourceNotUniform(filepath.Join(dir, "nope.mjpeg")); err == nil {
			t.Fatal("missing source: expected failure, got nil")
		}
	})
}

func TestFrameSignature(t *testing.T) {
	a := jpegFrame(t, 64, 48, solid(color.RGBA{5, 5, 5, 255}))
	b := jpegFrame(t, 64, 48, solid(color.RGBA{5, 5, 5, 255}))
	c := jpegFrame(t, 64, 48, solid(color.RGBA{5, 5, 255, 255}))

	sa, err := frameSignature(a)
	if err != nil {
		t.Fatalf("signature: %v", err)
	}
	sb, err := frameSignature(b)
	if err != nil {
		t.Fatalf("signature: %v", err)
	}
	sc, err := frameSignature(c)
	if err != nil {
		t.Fatalf("signature: %v", err)
	}
	if sa != sb {
		t.Error("visually identical frames must collide (JPEG-encode-invariant honesty)")
	}
	if sa == sc {
		t.Error("visually different frames must differ")
	}
}
