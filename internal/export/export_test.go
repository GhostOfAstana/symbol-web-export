package export

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"html"
	"image/png"
	"io"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestTXTExactBytes(t *testing.T) {
	for _, content := range []string{"", "a\n  b  \n", "Я < & >", "\x1b[31mred\x1b[0m", strings.Repeat("x", MaxContentBytes)} {
		got, err := Build(content, "txt")
		if err != nil || !bytes.Equal(got.Data, []byte(content)) || got.MIME != "text/plain; charset=utf-8" || got.Extension != "txt" {
			t.Fatalf("TXT: %v %+v", err, got)
		}
	}
}

func preText(t *testing.T, document string) string {
	t.Helper()
	match := regexp.MustCompile(`(?s)<pre>(.*?)</pre>`).FindStringSubmatch(document)
	if len(match) != 2 {
		t.Fatal("нет pre")
	}
	return html.UnescapeString(regexp.MustCompile(`<span(?: style="[^"]*")?>|</span>`).ReplaceAllString(match[1], ""))
}

func TestHTMLEscapingAndColor(t *testing.T) {
	for _, content := range []string{"", "\n leading  \n\n", `<script>alert("x")</script><img src=x onerror=alert(1)> & '`, "Unicode: Я и <>&"} {
		file, err := Build(content, "html")
		if err != nil {
			t.Fatal(err)
		}
		doc := string(file.Data)
		if preText(t, doc) != content {
			t.Fatalf("HTML изменил рисунок %q", content)
		}
		if strings.Contains(doc, "<script>") || strings.Contains(doc, "<img") {
			t.Fatal("пользовательские теги не экранированы")
		}
		if strings.Contains(doc, "<pre>\n") {
			t.Fatal("браузер удалит начальный LF")
		}
	}
	for _, tc := range []struct{ ansi, color string }{
		{"\x1b[31m", "color:#aa0000;"}, {"\x1b[1;94m", "color:#5555ff;font-weight:bold;"},
		{"\x1b[48;2;1;2;3m", "background-color:#010203;"}, {"\x1b[38;5;196m", "color:#ff0000;"},
		{"\x1b[38;5;244m", "color:#808080;"},
	} {
		file, err := Build(tc.ansi+"<&>\x1b[0m!", "html")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(file.Data), tc.color) || preText(t, string(file.Data)) != "<&>!" {
			t.Fatal("ANSI не преобразован", string(file.Data))
		}
		if strings.Count(string(file.Data), "<span") != strings.Count(string(file.Data), "</span>") {
			t.Fatal("непарный span")
		}
	}
}

func pdfStreams(t *testing.T, data []byte) [][]byte {
	t.Helper()
	var streams [][]byte
	for remaining := data; ; {
		start := bytes.Index(remaining, []byte("stream\n"))
		if start < 0 {
			break
		}
		remaining = remaining[start+7:]
		end := bytes.Index(remaining, []byte("\nendstream"))
		if end < 0 {
			t.Fatal("незавершённый PDF stream")
		}
		compressed := remaining[:end]
		reader, err := zlib.NewReader(bytes.NewReader(compressed))
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(reader)
		_ = reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		streams = append(streams, body)
		remaining = remaining[end+10:]
	}
	return streams
}

func TestPDFStructureAndContent(t *testing.T) {
	file, err := Build("Hello (PDF) \\ test\n<&>", "pdf")
	if err != nil {
		t.Fatal(err)
	}
	if file.MIME != "application/pdf" || !bytes.HasPrefix(file.Data, []byte("%PDF-")) || !bytes.HasSuffix(file.Data, []byte("%%EOF\n")) {
		t.Fatal("некорректная сигнатура PDF")
	}
	streams := pdfStreams(t, file.Data)
	if len(streams) != 1 || !bytes.Contains(streams[0], []byte(`(Hello \(PDF\) \\ test) Tj`)) || !bytes.Contains(streams[0], []byte(`(<&>) Tj`)) {
		t.Fatalf("текст потерян: %q", streams)
	}
	// Проверяем реальные смещения объектов в таблице xref, а не только сигнатуру.
	xref := bytes.LastIndex(file.Data, []byte("\nxref\n"))
	if xref < 0 {
		t.Fatal("нет xref")
	}
	lines := strings.Split(string(file.Data[xref+6:]), "\n")
	var first, count int
	if _, err := fmt.Sscanf(lines[0], "%d %d", &first, &count); err != nil {
		t.Fatal(err)
	}
	for i := 1; i < count; i++ {
		offset, err := strconv.Atoi(strings.Fields(lines[i+1])[0])
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.HasPrefix(file.Data[offset:], []byte(fmt.Sprintf("%d 0 obj", i))) {
			t.Fatalf("xref[%d]=%d неверен", i, offset)
		}
	}
	wide, err := Build(strings.Repeat("W", 161)+"\n"+strings.Repeat("x\n", 52), "pdf")
	if err != nil {
		t.Fatal(err)
	}
	if len(pdfStreams(t, wide.Data)) != 4 {
		t.Fatal("широкий и длинный рисунок не разбит на четыре панели")
	}
	colored, err := Build("\x1b[31mHello\x1b[0m", "pdf")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(pdfStreams(t, colored.Data)[0], []byte("(Hello) Tj")) {
		t.Fatal("PDF не удалил ANSI")
	}
}

func TestFormatAndResourceLimits(t *testing.T) {
	for _, tc := range []struct {
		content, format string
		want            error
	}{
		{"x", "exe", ErrFormat}, {strings.Repeat("x", MaxContentBytes+1), "txt", ErrContent},
		{string([]byte{0xff}), "html", ErrContent}, {"Я", "pdf", ErrPDF},
		{strings.Repeat("x", 64001), "pdf", ErrPDF}, {"tab\t", "pdf", ErrPDF},
		{strings.Repeat("\n", 20000), "pdf", ErrPDF},
		{"Я", "png", ErrPNG}, {strings.Repeat("x", 509), "png", ErrPNG}, {strings.Repeat("\n", 407), "png", ErrPNG},
		{strings.Repeat(strings.Repeat("x", 500)+"\n", 400), "png", ErrContent},
	} {
		if _, err := Build(tc.content, tc.format); !errors.Is(err, tc.want) {
			t.Fatalf("%s: получено=%v нужно=%v", tc.format, err, tc.want)
		}
	}
	if _, err := Build("line\r\nnext", "pdf"); err != nil {
		t.Fatal(err)
	}
	if _, err := Build("", "pdf"); err != nil {
		t.Fatal(err)
	}
}

func TestPNGRoundTripAndLimits(t *testing.T) {
	content := "A B\nC"
	file, err := Build(content, "png")
	if err != nil {
		t.Fatal(err)
	}
	if file.MIME != "image/png" || file.Extension != "png" {
		t.Fatal(file.MIME, file.Extension)
	}
	picture, err := png.Decode(bytes.NewReader(file.Data))
	if err != nil {
		t.Fatal(err)
	}
	if picture.Bounds().Dx() != 56 || picture.Bounds().Dy() != 52 {
		t.Fatal(picture.Bounds())
	}
	// Пиксели каждой буквы сравниваем с исходным bitmap, включая пустой пробел.
	for lineIndex, line := range strings.Split(content, "\n") {
		for column := range line {
			for row, bits := range bitmapFont[line[column]] {
				for bit := range 8 {
					r, g, b, _ := picture.At(16+column*8+bit, 16+lineIndex*10+row).RGBA()
					dark := r < 32768 && g < 32768 && b < 32768
					if dark != (bits&(1<<bit) != 0) {
						t.Fatal("пиксель глифа изменился")
					}
				}
			}
		}
	}
	colored, err := Build("\x1b[31m"+content+"\x1b[0m", "png")
	if err != nil || !bytes.Equal(colored.Data, file.Data) {
		t.Fatal("PNG не нормализовал ANSI", err)
	}
	// Обе стороны допустимы отдельно, но суммарное число пикселей слишком велико.
	if _, err := Build(strings.Repeat("x", 500)+strings.Repeat("\n", 399), "png"); !errors.Is(err, ErrPNG) {
		t.Fatal("не применён лимит площади", err)
	}
	if _, err := Build("", "png"); err != nil {
		t.Fatal(err)
	}
}

func FuzzBuildTextFormats(f *testing.F) {
	for _, text := range []string{"", "<>&", "\x1b[38;2;255;0;4mcolor\x1b[0m", "Я", "\n"} {
		f.Add(text)
	}
	f.Fuzz(func(t *testing.T, text string) {
		for _, format := range []string{"txt", "html"} {
			file, err := Build(text, format)
			if err == nil && format == "txt" && !bytes.Equal(file.Data, []byte(text)) {
				t.Fatal("TXT изменил байты")
			}
			if err == nil && format == "html" && strings.Count(string(file.Data), "<span") != strings.Count(string(file.Data), "</span>") {
				t.Fatal("непарный span")
			}
		}
	})
}
