// Package export собирает файлы в памяти, не создавая общих временных файлов.
package export

import (
	"fmt"
	"html"
	"strconv"
	"strings"

	"github.com/GhostOfAstana/symbol-web-export/internal/recommender"
)

var basicColors = []string{"#000000", "#aa0000", "#008000", "#aa5500", "#0000aa", "#aa00aa", "#008888", "#aaaaaa", "#555555", "#ff5555", "#55ff55", "#ffff55", "#5555ff", "#ff55ff", "#55ffff", "#ffffff"}

func color256(n int) string {
	if n < 16 {
		return basicColors[n]
	}
	if n >= 232 {
		v := 8 + (n-232)*10
		return fmt.Sprintf("#%02x%02x%02x", v, v, v)
	}
	n -= 16
	level := func(v int) int {
		if v == 0 {
			return 0
		}
		return 55 + 40*v
	}
	return fmt.Sprintf("#%02x%02x%02x", level(n/36), level(n/6%6), level(n%6))
}

func htmlDocument(content string) string {
	var output strings.Builder
	// Начальный span сохраняет первый LF: HTML-парсер обычно удаляет перевод
	// строки, который следует непосредственно за открывающим тегом pre.
	output.WriteString("<!doctype html><html lang=\"ru\"><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width\"><title>Символьный рисунок</title><style>body{background:#101827;color:#f8fafc;margin:2rem}pre{font:14px/1.2 monospace;white-space:pre;overflow:auto}h1{font:24px sans-serif}</style><h1>Символьный рисунок</h1><pre><span>")
	foreground, background, bold := "", "", false
	position := 0
	for _, match := range recommender.ANSI.FindAllStringIndex(content, -1) {
		output.WriteString(html.EscapeString(content[position:match[0]]))
		codes := strings.Split(content[match[0]+2:match[1]-1], ";")
		values := make([]int, len(codes))
		for i, s := range codes {
			values[i], _ = strconv.Atoi(s)
		}
		for i := 0; i < len(values); i++ {
			n := values[i]
			switch {
			case n == 0:
				foreground, background, bold = "", "", false
			case n == 1:
				bold = true
			case n == 22:
				bold = false
			case n == 39:
				foreground = ""
			case n == 49:
				background = ""
			case n >= 30 && n <= 37:
				foreground = basicColors[n-30]
			case n >= 90 && n <= 97:
				foreground = basicColors[n-90+8]
			case n >= 40 && n <= 47:
				background = basicColors[n-40]
			case n >= 100 && n <= 107:
				background = basicColors[n-100+8]
			case n == 38 || n == 48:
				color := ""
				if i+2 < len(values) && values[i+1] == 5 && values[i+2] >= 0 && values[i+2] <= 255 {
					color = color256(values[i+2])
					i += 2
				} else if i+4 < len(values) && values[i+1] == 2 {
					r, g, b := values[i+2], values[i+3], values[i+4]
					if r >= 0 && r <= 255 && g >= 0 && g <= 255 && b >= 0 && b <= 255 {
						color = fmt.Sprintf("#%02x%02x%02x", r, g, b)
					}
					i += 4
				}
				if n == 38 {
					foreground = color
				} else {
					background = color
				}
			}
		}
		// Все значения style получены из чисел, а пользовательский текст экранирован.
		if position > 0 {
			output.WriteString("</span>")
		}
		output.WriteString("<span style=\"")
		if foreground != "" {
			output.WriteString("color:" + foreground + ";")
		}
		if background != "" {
			output.WriteString("background-color:" + background + ";")
		}
		if bold {
			output.WriteString("font-weight:bold;")
		}
		output.WriteString("\">")
		position = match[1]
	}
	output.WriteString(html.EscapeString(content[position:]))
	if position > 0 {
		output.WriteString("</span>")
	}
	output.WriteString("</span></pre></html>\n")
	return output.String()
}
