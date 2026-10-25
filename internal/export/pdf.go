package export

import (
	"bytes"
	"strings"
	"time"

	"github.com/GhostOfAstana/symbol-web-export/internal/recommender"
	"github.com/go-pdf/fpdf"
)

func pdfDocument(content string) ([]byte, error) {
	plain := recommender.ANSI.ReplaceAllString(content, "")
	plain = strings.ReplaceAll(plain, "\r\n", "\n")
	if len(plain) > 64000 {
		return nil, ErrPDF
	}
	width := 0
	lines := strings.Split(strings.TrimSuffix(plain, "\n"), "\n")
	for _, line := range lines {
		width = max(width, len(line))
		for i := range line {
			if line[i] < 32 || line[i] > 126 {
				return nil, ErrPDF
			}
		}
	}
	// Широкий рисунок делится на панели, длинный — на страницы: ни одна строка
	// не переносится внутри панели и ни один символ не обрезается.
	const columns, rows = 160, 52
	if max(1, (width+columns-1)/columns)*max(1, (len(lines)+rows-1)/rows) > 256 {
		return nil, ErrPDF
	}
	pdf := fpdf.New("L", "pt", "A4", "")
	pdf.SetCreationDate(time.Unix(0, 0).UTC())
	pdf.SetModificationDate(time.Unix(0, 0).UTC())
	pdf.SetMargins(20, 20, 20)
	pdf.SetAutoPageBreak(false, 20)
	pdf.SetFont("Courier", "", 8)
	for start := 0; start < len(lines); start += rows {
		for left := 0; left < max(1, width); left += columns {
			pdf.AddPage()
			for row := start; row < min(len(lines), start+rows); row++ {
				line := lines[row]
				part := ""
				if left < len(line) {
					part = line[left:min(left+columns, len(line))]
				}
				pdf.Text(20, 30+float64(row-start)*10, part)
			}
		}
	}
	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}
