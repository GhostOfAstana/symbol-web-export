package export

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"

	"github.com/GhostOfAstana/symbol-web-export/internal/recommender"
)

const maxPNGSide = 4096
const maxPNGPixels = 4 << 20

func pngDocument(content string) ([]byte, error) {
	plain := strings.ReplaceAll(recommender.ANSI.ReplaceAllString(content, ""), "\r\n", "\n")
	lines := strings.Split(strings.TrimSuffix(plain, "\n"), "\n")
	width := 0
	for _, line := range lines {
		width = max(width, len(line))
		for i := range line {
			if line[i] < 32 || line[i] > 126 {
				return nil, ErrPNG
			}
		}
	}
	// Проверяем размеры до выделения пиксельного буфера. Палитра из двух цветов
	// занимает один байт на пиксель и сохраняет достаточный контраст текста.
	imageWidth, imageHeight := width*8+32, len(lines)*10+32
	if imageWidth > maxPNGSide || imageHeight > maxPNGSide || imageWidth*imageHeight > maxPNGPixels {
		return nil, ErrPNG
	}
	palette := color.Palette{color.RGBA{248, 250, 252, 255}, color.RGBA{16, 24, 39, 255}}
	picture := image.NewPaletted(image.Rect(0, 0, imageWidth, imageHeight), palette)
	for lineIndex, line := range lines {
		for column := range line {
			character := bitmapFont[line[column]]
			for row, bits := range character {
				for bit := range 8 {
					if bits&(1<<bit) != 0 {
						x, y := 16+column*8+bit, 16+lineIndex*10+row
						picture.Pix[y*picture.Stride+x] = 1
					}
				}
			}
		}
	}
	var output bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := encoder.Encode(&output, picture); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}
