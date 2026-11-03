package domain

import (
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"strings"

	"nuhabit/backend/internal/platform/imageproc"
)

// Foto profil akun (EPIC-087 R3): JPG/PNG/WebP maks 5 MB, dipotong persegi
// di tengah, diperkecil ke maks 512×512, latar transparan putih, JPEG 85.

const (
	AvatarMaxBytes = 5 << 20
	AvatarSize     = 512
	avatarQuality  = 85
)

var (
	ErrAvatarFormat   = errors.New("Foto harus berupa gambar JPG, PNG, atau WebP")
	ErrAvatarTooLarge = errors.New("Ukuran foto maksimal 5 MB")
)

// DecodeDataURL reads the bytes of a "data:image/...;base64,..." URL.
func DecodeDataURL(s string) ([]byte, error) {
	head, body, ok := strings.Cut(s, ",")
	if !ok || !strings.HasPrefix(head, "data:image/") || !strings.HasSuffix(head, ";base64") {
		return nil, ErrAvatarFormat
	}
	if base64.StdEncoding.DecodedLen(len(body)) > AvatarMaxBytes+3 {
		return nil, ErrAvatarTooLarge
	}
	data, err := base64.StdEncoding.DecodeString(body)
	if err != nil {
		return nil, ErrAvatarFormat
	}
	if len(data) > AvatarMaxBytes {
		return nil, ErrAvatarTooLarge
	}
	return data, nil
}

// SquareAvatar turns an uploaded picture into the stored profile photo.
// imageproc.Decode already applies the EXIF orientation.
func SquareAvatar(data []byte) ([]byte, error) {
	img, format, err := imageproc.Decode(data)
	if err != nil {
		return nil, ErrAvatarFormat
	}
	switch format {
	case "jpeg", "png", "webp":
	default:
		return nil, ErrAvatarFormat
	}
	b := img.Bounds()
	side := min(b.Dx(), b.Dy())
	x0, y0 := b.Min.X+(b.Dx()-side)/2, b.Min.Y+(b.Dy()-side)/2
	square := image.NewNRGBA(image.Rect(0, 0, side, side))
	draw.Draw(square, square.Bounds(), img, image.Pt(x0, y0), draw.Src)
	small := imageproc.FitInside(square, AvatarSize, AvatarSize)
	return imageproc.EncodeJPEG(imageproc.Flatten(small, color.White), avatarQuality)
}
