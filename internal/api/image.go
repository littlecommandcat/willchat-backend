package api

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"

	_ "image/gif"
	_ "image/png"

	xdraw "golang.org/x/image/draw"
)

const (
	maxAttachmentBytes       = 5 << 20
	maxMessageFormBytes      = 25 << 20
	maxAttachmentsPerMessage = 4
	maxImageDimension        = 1080
	jpegQuality              = 90
	minJPEGQuality           = 60
	minShortEdge             = 360
	minTargetBytes           = 100 * 1024
	maxTargetBytes           = 512 * 1024
	maxMultipartMemory       = 32 << 20
)

var (
	errUnsupportedImage   = errors.New("attachment must be a JPEG, PNG or GIF image")
	errAttachmentTooLarge = errors.New("attachment is too large")
	errTooManyAttachments = errors.New("too many attachments")
)

// targetKBForRaw returns the per-image output budget for the
// quality-first 1/5 plan: raw/5 clamped to [100KB, 512KB].
func targetKBForRaw(rawLen int) int {
	target := rawLen / 5
	if target < minTargetBytes {
		target = minTargetBytes
	}
	if target > maxTargetBytes {
		target = maxTargetBytes
	}
	kb := target / 1024
	if kb <= 0 {
		kb = 1
	}
	return kb
}

// compressImage
func compressImage(data []byte, maxKB int) ([]byte, string, int, int, error) {
	if len(data) == 0 {
		return nil, "", 0, 0, errUnsupportedImage
	}

	if maxKB <= 0 {
		return nil, "", 0, 0, errors.New("invalid max image size")
	}

	source, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", 0, 0, errUnsupportedImage
	}

	if format != "jpeg" && format != "png" && format != "gif" {
		return nil, "", 0, 0, errUnsupportedImage
	}

	normalized := image.NewNRGBA(source.Bounds())
	draw.Draw(
		normalized,
		normalized.Bounds(),
		source,
		source.Bounds().Min,
		draw.Src,
	)

	scaled := downscale(normalized, maxImageDimension)
	flattened := image.NewRGBA(scaled.Bounds())

	draw.Draw(
		flattened,
		flattened.Bounds(),
		image.NewUniform(color.White),
		image.Point{},
		draw.Src,
	)

	draw.Draw(
		flattened,
		flattened.Bounds(),
		scaled,
		scaled.Bounds().Min,
		draw.Over,
	)

	maxBytes := maxKB * 1024

	tryEncode := func(img image.Image) ([]byte, bool) {
		for quality := jpegQuality; quality >= minJPEGQuality; quality -= 5 {
			result, err := encodeJPEG(img, quality)
			if err != nil {
				return nil, false
			}
			if len(result) <= maxBytes {
				return result, true
			}
		}
		return nil, false
	}

	if result, ok := tryEncode(flattened); ok {
		return result, "image/jpeg",
			flattened.Bounds().Dx(),
			flattened.Bounds().Dy(),
			nil
	}

	current := flattened

	for current.Bounds().Dx() > 1 || current.Bounds().Dy() > 1 {
		short := current.Bounds().Dx()
		if current.Bounds().Dy() < short {
			short = current.Bounds().Dy()
		}
		if short < minShortEdge {
			break
		}

		width := current.Bounds().Dx() * 80 / 100
		height := current.Bounds().Dy() * 80 / 100

		if width < 1 {
			width = 1
		}

		if height < 1 {
			height = 1
		}

		current = scaleRGBA(current, width, height)

		if result, ok := tryEncode(current); ok {
			return result, "image/jpeg",
				current.Bounds().Dx(),
				current.Bounds().Dy(),
				nil
		}
	}

	result, err := encodeJPEG(current, minJPEGQuality)
	if err != nil {
		return nil, "", 0, 0, err
	}

	return result, "image/jpeg",
		current.Bounds().Dx(),
		current.Bounds().Dy(),
		nil
}

// encodeJPEG
func encodeJPEG(img image.Image, quality int) ([]byte, error) {
	var buffer bytes.Buffer

	if err := jpeg.Encode(
		&buffer,
		img,
		&jpeg.Options{
			Quality: quality,
		},
	); err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

// scaleRGBA high-quality resample via CatmullRom.
func scaleRGBA(src *image.RGBA, width, height int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
	return dst
}

func scaleNRGBA(src *image.NRGBA, width, height int) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, width, height))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
	return dst
}

func downscale(src *image.NRGBA, maxDimension int) *image.NRGBA {
	bounds := src.Bounds()

	width := bounds.Dx()
	height := bounds.Dy()

	if width <= maxDimension && height <= maxDimension {
		return src
	}

	if width >= height {
		height = height * maxDimension / width
		width = maxDimension
	} else {
		width = width * maxDimension / height
		height = maxDimension
	}

	if width < 1 {
		width = 1
	}

	if height < 1 {
		height = 1
	}

	return scaleNRGBA(src, width, height)
}
