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
)

const (
	maxAttachmentBytes       = 5 << 20
	maxMessageFormBytes      = 25 << 20
	maxAttachmentsPerMessage = 4
	maxImageDimension        = 1080
	jpegQuality              = 80
	minJPEGQuality           = 30
	maxMultipartMemory       = 32 << 20
)

var (
	errUnsupportedImage   = errors.New("attachment must be a JPEG, PNG or GIF image")
	errAttachmentTooLarge = errors.New("attachment is too large")
	errTooManyAttachments = errors.New("too many attachments")
)

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

	for quality := jpegQuality; quality >= minJPEGQuality; quality -= 5 {
		result, err := encodeJPEG(flattened, quality)
		if err != nil {
			return nil, "", 0, 0, err
		}

		if len(result) <= maxBytes {
			return result, "image/jpeg",
				flattened.Bounds().Dx(),
				flattened.Bounds().Dy(),
				nil
		}
	}

	// Quality
	current := flattened

	for current.Bounds().Dx() > 1 || current.Bounds().Dy() > 1 {
		width := current.Bounds().Dx() * 80 / 100
		height := current.Bounds().Dy() * 80 / 100

		if width < 1 {
			width = 1
		}

		if height < 1 {
			height = 1
		}

		current = resizeRGBA(current, width, height)

		for quality := jpegQuality; quality >= minJPEGQuality; quality -= 5 {
			result, err := encodeJPEG(current, quality)
			if err != nil {
				return nil, "", 0, 0, err
			}

			if len(result) <= maxBytes {
				return result, "image/jpeg",
					current.Bounds().Dx(),
					current.Bounds().Dy(),
					nil
			}
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

// resizeRGBA
func resizeRGBA(src *image.RGBA, width, height int) *image.RGBA {
	srcBounds := src.Bounds()

	dst := image.NewRGBA(image.Rect(0, 0, width, height))

	for dy := 0; dy < height; dy++ {
		yStart := srcBounds.Min.Y + dy*srcBounds.Dy()/height
		yEnd := srcBounds.Min.Y + (dy+1)*srcBounds.Dy()/height

		if yEnd <= yStart {
			yEnd = yStart + 1
		}

		for dx := 0; dx < width; dx++ {
			xStart := srcBounds.Min.X + dx*srcBounds.Dx()/width
			xEnd := srcBounds.Min.X + (dx+1)*srcBounds.Dx()/width

			if xEnd <= xStart {
				xEnd = xStart + 1
			}

			var red, green, blue, alpha, count uint64

			for y := yStart; y < yEnd; y++ {
				for x := xStart; x < xEnd; x++ {
					pixel := src.RGBAAt(x, y)

					red += uint64(pixel.R)
					green += uint64(pixel.G)
					blue += uint64(pixel.B)
					alpha += uint64(pixel.A)
					count++
				}
			}

			dst.SetRGBA(
				dx,
				dy,
				color.RGBA{
					R: uint8(red / count),
					G: uint8(green / count),
					B: uint8(blue / count),
					A: uint8(alpha / count),
				},
			)
		}
	}

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

	dst := image.NewNRGBA(image.Rect(0, 0, width, height))

	for dy := 0; dy < height; dy++ {
		yStart := bounds.Min.Y + dy*bounds.Dy()/height
		yEnd := bounds.Min.Y + (dy+1)*bounds.Dy()/height

		if yEnd <= yStart {
			yEnd = yStart + 1
		}

		for dx := 0; dx < width; dx++ {
			xStart := bounds.Min.X + dx*bounds.Dx()/width
			xEnd := bounds.Min.X + (dx+1)*bounds.Dx()/width

			if xEnd <= xStart {
				xEnd = xStart + 1
			}

			var red, green, blue, alpha, count uint64

			for y := yStart; y < yEnd; y++ {
				for x := xStart; x < xEnd; x++ {
					pixel := src.NRGBAAt(x, y)

					red += uint64(pixel.R)
					green += uint64(pixel.G)
					blue += uint64(pixel.B)
					alpha += uint64(pixel.A)
					count++
				}
			}

			dst.SetNRGBA(
				dx,
				dy,
				color.NRGBA{
					R: uint8(red / count),
					G: uint8(green / count),
					B: uint8(blue / count),
					A: uint8(alpha / count),
				},
			)
		}
	}

	return dst
}
