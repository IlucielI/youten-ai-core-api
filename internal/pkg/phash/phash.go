package phash

import (
	"bytes"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"math/bits"
)

// DefaultHammingThreshold is the maximum bit difference for two frames to be considered identical/duplicate.
const DefaultHammingThreshold = 5

// DHash computes a 64-bit difference hash (dHash) for an image.Image.
// It resizes the image to 9x8 grayscale and sets each bit based on whether
// the left pixel is brighter than the right pixel.
func DHash(img image.Image) uint64 {
	if img == nil {
		return 0
	}

	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return 0
	}

	// Downsample to 9x8 grayscale matrix using nearest-neighbor sampling
	const targetW = 9
	const targetH = 8

	var matrix [targetH][targetW]uint8
	for y := 0; y < targetH; y++ {
		srcY := bounds.Min.Y + (y*height)/targetH
		for x := 0; x < targetW; x++ {
			srcX := bounds.Min.X + (x*width)/targetW
			c := img.At(srcX, srcY)
			matrix[y][x] = toGrayscale(c)
		}
	}

	// Compute 64-bit gradient hash: 8 rows * 8 comparisons
	var hash uint64
	var bitPos uint
	for y := 0; y < targetH; y++ {
		for x := 0; x < targetW-1; x++ {
			if matrix[y][x] > matrix[y][x+1] {
				hash |= (1 << bitPos)
			}
			bitPos++
		}
	}

	return hash
}

// DHashFromBytes decodes an image from raw bytes and calculates its 64-bit dHash.
func DHashFromBytes(data []byte) (uint64, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return 0, err
	}
	return DHash(img), nil
}

// HammingDistance calculates the number of differing bits between two 64-bit perceptual hashes.
func HammingDistance(h1, h2 uint64) int {
	return bits.OnesCount64(h1 ^ h2)
}

// IsDuplicate checks whether two hashes are within the specified Hamming distance threshold.
func IsDuplicate(h1, h2 uint64, maxDiff ...int) bool {
	threshold := DefaultHammingThreshold
	if len(maxDiff) > 0 && maxDiff[0] >= 0 {
		threshold = maxDiff[0]
	}
	return HammingDistance(h1, h2) <= threshold
}

// toGrayscale converts any color.Color into 8-bit luma (rec601).
func toGrayscale(c color.Color) uint8 {
	r, g, b, _ := c.RGBA()
	// Standard Rec. 601 luma coefficients: 0.299 R + 0.587 G + 0.114 B
	// Since RGBA values are 0..65535, we scale down to 0..255
	luma := (19595*r + 38470*g + 7471*b) >> 24
	return uint8(luma)
}

