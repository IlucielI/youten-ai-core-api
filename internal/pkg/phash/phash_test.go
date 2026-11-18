package phash

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func createSolidImage(w, h int, c color.Color) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

func createGradientImage(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			val := uint8((x * 255) / w)
			img.Set(x, y, color.Gray{Y: val})
		}
	}
	return img
}

func TestDHash_NilAndEmpty(t *testing.T) {
	if hash := DHash(nil); hash != 0 {
		t.Errorf("expected 0 for nil image, got %d", hash)
	}

	emptyImg := image.NewRGBA(image.Rect(0, 0, 0, 0))
	if hash := DHash(emptyImg); hash != 0 {
		t.Errorf("expected 0 for empty image, got %d", hash)
	}
}

func TestDHash_IdenticalAndDuplicate(t *testing.T) {
	img1 := createGradientImage(100, 100)
	img2 := createGradientImage(100, 100)

	h1 := DHash(img1)
	h2 := DHash(img2)

	if h1 != h2 {
		t.Errorf("expected identical hashes, got %d and %d", h1, h2)
	}

	if dist := HammingDistance(h1, h2); dist != 0 {
		t.Errorf("expected Hamming distance 0, got %d", dist)
	}

	if !IsDuplicate(h1, h2) {
		t.Errorf("expected images to be marked as duplicate")
	}
}

func TestDHash_Distinct(t *testing.T) {
	// Gradient from dark to bright
	img1 := createGradientImage(100, 100)

	// Inverted gradient from bright to dark
	img2 := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			val := uint8(255 - ((x * 255) / 100))
			img2.Set(x, y, color.Gray{Y: val})
		}
	}

	h1 := DHash(img1)
	h2 := DHash(img2)

	dist := HammingDistance(h1, h2)
	if dist <= DefaultHammingThreshold {
		t.Errorf("expected large distance between opposite gradients, got %d", dist)
	}

	if IsDuplicate(h1, h2) {
		t.Errorf("expected opposite gradients not to be duplicate")
	}
}

func TestDHashFromBytes(t *testing.T) {
	img := createGradientImage(50, 50)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("failed to encode png: %v", err)
	}

	h, err := DHashFromBytes(buf.Bytes())
	if err != nil {
		t.Fatalf("unexpected error decoding bytes: %v", err)
	}

	directHash := DHash(img)
	if h != directHash {
		t.Errorf("expected DHashFromBytes %d to match direct DHash %d", h, directHash)
	}

	// Corrupted bytes
	_, err = DHashFromBytes([]byte("corrupt-not-an-image"))
	if err == nil {
		t.Errorf("expected error for corrupted bytes, got nil")
	}
}

