package export

import (
	"errors"
	"unicode/utf8"
)

const MaxContentBytes = 128 << 10

var ErrFormat = errors.New("выберите формат txt, html, png или pdf")
var ErrContent = errors.New("содержимое должно быть корректным UTF-8 размером до 128 КиБ")
var ErrPDF = errors.New("PDF принимает печатный ASCII и не более 64000 ячеек; для большего содержимого выберите TXT или HTML")
var ErrPNG = errors.New("PNG принимает печатный ASCII; изображение должно укладываться в 4096×4096 и 4 мегапикселя: сократите рисунок или выберите TXT/HTML")

type File struct {
	Data      []byte
	MIME      string
	Extension string
}

func Build(content, format string) (File, error) {
	if len(content) > MaxContentBytes || !utf8.ValidString(content) {
		return File{}, ErrContent
	}
	switch format {
	case "txt":
		return File{[]byte(content), "text/plain; charset=utf-8", "txt"}, nil
	case "html":
		return File{[]byte(htmlDocument(content)), "text/html; charset=utf-8", "html"}, nil
	case "pdf":
		data, err := pdfDocument(content)
		return File{data, "application/pdf", "pdf"}, err
	case "png":
		data, err := pngDocument(content)
		return File{data, "image/png", "png"}, err
	default:
		return File{}, ErrFormat
	}
}
