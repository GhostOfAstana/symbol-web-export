// Package recommender применяет только формулы и правила из задания.
package recommender

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// ANSI распознаёт команды цвета SGR; остальные управляющие команды не исполняются.
var ANSI = regexp.MustCompile(`\x1b\[[0-9;]*m`)

type Content struct {
	Characters int  `json:"characters"`
	Lines      int  `json:"lines"`
	Width      int  `json:"width"`
	UsesColor  bool `json:"uses_color"`
}

type Analysis struct {
	Content       Content            `json:"content"`
	SizeEstimates map[string]float64 `json:"size_estimates"`
	Recommended   string             `json:"recommended"`
	Reason        string             `json:"reason"`
}

func Analyze(content string) Analysis {
	plain := ANSI.ReplaceAllString(content, "")
	info := Content{Characters: len(content), UsesColor: ANSI.MatchString(content)}
	if plain != "" {
		lines := strings.Split(strings.TrimSuffix(plain, "\n"), "\n")
		info.Lines = len(lines)
		for _, line := range lines {
			info.Width = max(info.Width, utf8.RuneCountInString(strings.TrimSuffix(line, "\r")))
		}
	}
	a := Analysis{Content: info, SizeEstimates: map[string]float64{
		"txt": float64(len(content)), "html": float64(len(content))*2.5 + 800,
		"png": float64(info.Width) * float64(info.Lines) * 60,
	}, Recommended: "txt", Reason: "Наименьший размер; читается практически везде."}
	if info.UsesColor {
		a.Recommended, a.Reason = "html", "Сохраняет цвета ANSI в HTML-документе."
	} else if info.Width > 120 {
		a.Recommended, a.Reason = "html", "Ширина больше 120: TXT может переноситься в узких просмотрщиках."
	}
	return a
}
