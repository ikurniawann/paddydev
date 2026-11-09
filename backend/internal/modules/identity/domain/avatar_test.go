package domain

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

// EPIC-087 R3: foto profil dipotong persegi di tengah, maks 512, JPEG.

func bands(w, h int, cols ...color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, cols[x*len(cols)/w])
		}
	}
	return img
}

func encPNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func decodeOut(t *testing.T, data []byte) image.Image {
	t.Helper()
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil || format != "jpeg" {
		t.Fatalf("hasil bukan JPEG: %v %s", err, format)
	}
	return img
}

func near(c color.Color, r, g, b uint8) bool {
	cr, cg, cb, _ := c.RGBA()
	d := func(a uint32, x uint8) bool { v := int(a>>8) - int(x); return v < 40 && v > -40 }
	return d(cr, r) && d(cg, g) && d(cb, b)
}

var (
	red   = color.NRGBA{255, 0, 0, 255}
	green = color.NRGBA{0, 255, 0, 255}
	blue  = color.NRGBA{0, 0, 255, 255}
)

func TestSquareAvatarCropsCenterAndShrinks(t *testing.T) {
	out, err := SquareAvatar(encPNG(t, bands(1200, 800, red, green, blue)))
	if err != nil {
		t.Fatal(err)
	}
	img := decodeOut(t, out)
	if b := img.Bounds(); b.Dx() != AvatarSize || b.Dy() != AvatarSize {
		t.Fatalf("ukuran %v", b)
	}
	if c := img.At(256, 256); !near(c, 0, 255, 0) {
		t.Fatalf("tengah bukan hijau: %v", c)
	}
}

func TestSquareAvatarDoesNotUpscale(t *testing.T) {
	out, err := SquareAvatar(encPNG(t, bands(300, 200, red)))
	if err != nil {
		t.Fatal(err)
	}
	if b := decodeOut(t, out).Bounds(); b.Dx() != 200 || b.Dy() != 200 {
		t.Fatalf("ukuran %v, want 200x200", b)
	}
}

func TestSquareAvatarTransparentIsWhite(t *testing.T) {
	out, err := SquareAvatar(encPNG(t, image.NewNRGBA(image.Rect(0, 0, 100, 100))))
	if err != nil {
		t.Fatal(err)
	}
	if c := decodeOut(t, out).At(0, 0); !near(c, 255, 255, 255) {
		t.Fatalf("latar bukan putih: %v", c)
	}
}

// withOrientation sisipkan segmen EXIF APP1 berorientasi o setelah SOI JPEG.
func withOrientation(jpg []byte, o uint16) []byte {
	tiff := []byte{'M', 'M', 0, 42, 0, 0, 0, 8, 0, 1, 0x01, 0x12, 0, 3, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0}
	binary.BigEndian.PutUint16(tiff[18:], o)
	payload := append([]byte("Exif\x00\x00"), tiff...)
	seg := []byte{0xff, 0xe1, 0, 0}
	binary.BigEndian.PutUint16(seg[2:], uint16(len(payload)+2))
	out := append([]byte{}, jpg[:2]...)
	out = append(out, seg...)
	out = append(out, payload...)
	return append(out, jpg[2:]...)
}

// Review Focus 1: foto HP berorientasi EXIF 6 (diputar 90° searah jarum jam).
func TestSquareAvatarAppliesExifOrientation(t *testing.T) {
	var src bytes.Buffer
	if err := jpeg.Encode(&src, bands(400, 200, red, blue), &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	out, err := SquareAvatar(withOrientation(src.Bytes(), 6))
	if err != nil {
		t.Fatal(err)
	}
	img := decodeOut(t, out)
	if b := img.Bounds(); b.Dx() != 200 || b.Dy() != 200 {
		t.Fatalf("ukuran %v", b)
	}
	// Tegak: kiri-merah/kanan-biru menjadi atas-merah/bawah-biru.
	if !near(img.At(100, 10), 255, 0, 0) || !near(img.At(100, 190), 0, 0, 255) {
		t.Fatalf("orientasi salah: atas %v bawah %v", img.At(100, 10), img.At(100, 190))
	}
}

func TestSquareAvatarRejectsOtherFormats(t *testing.T) {
	var g bytes.Buffer
	pal := image.NewPaletted(image.Rect(0, 0, 10, 10), color.Palette{color.Black, color.White})
	if err := gif.Encode(&g, pal, nil); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"gif": g.Bytes(), "teks": []byte("bukan gambar")} {
		if _, err := SquareAvatar(data); !errors.Is(err, ErrAvatarFormat) {
			t.Errorf("%s: err = %v, want ErrAvatarFormat", name, err)
		}
	}
	if ErrAvatarFormat.Error() != "Foto harus berupa gambar JPG, PNG, atau WebP" {
		t.Errorf("pesan format = %q", ErrAvatarFormat.Error())
	}
}

func TestDecodeDataURL(t *testing.T) {
	data, err := DecodeDataURL("data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("abc")))
	if err != nil || string(data) != "abc" {
		t.Fatalf("data = %q, %v", data, err)
	}
	if _, err := DecodeDataURL("abc"); !errors.Is(err, ErrAvatarFormat) {
		t.Errorf("bukan data URL: %v", err)
	}
	big := "data:image/png;base64," + base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, AvatarMaxBytes+1))
	if _, err := DecodeDataURL(big); !errors.Is(err, ErrAvatarTooLarge) {
		t.Errorf("> 5 MB: %v", err)
	}
	if ErrAvatarTooLarge.Error() != "Ukuran foto maksimal 5 MB" || !strings.Contains(ErrAvatarFormat.Error(), "WebP") {
		t.Errorf("pesan ukuran = %q", ErrAvatarTooLarge.Error())
	}
}

// Review akhir (Important 1): gambar kecil berdimensi raksasa ditolak sebelum
// di-decode penuh (header PNG 16000×16000 tanpa data piksel).
func TestSquareAvatarRejectsHugeDimensions(t *testing.T) {
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], 16000)
	binary.BigEndian.PutUint32(ihdr[4:], 16000)
	ihdr[8], ihdr[9] = 8, 6 // 8-bit RGBA
	chunk := func(kind string, data []byte) []byte {
		out := make([]byte, 4)
		binary.BigEndian.PutUint32(out, uint32(len(data)))
		out = append(out, kind...)
		out = append(out, data...)
		sum := make([]byte, 4)
		binary.BigEndian.PutUint32(sum, crc32.ChecksumIEEE(append([]byte(kind), data...)))
		return append(out, sum...)
	}
	data := append([]byte("\x89PNG\r\n\x1a\n"), chunk("IHDR", ihdr)...)
	data = append(data, chunk("IEND", nil)...)
	if _, err := SquareAvatar(data); !errors.Is(err, ErrAvatarTooLarge) {
		t.Fatalf("err = %v, want ErrAvatarTooLarge", err)
	}
}
