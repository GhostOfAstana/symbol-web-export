package recommender

import (
	"encoding/json"
	"strings"
	"testing"

	assets "github.com/GhostOfAstana/symbol-web-export"
	"github.com/GhostOfAstana/symbol-web-export/internal/ascii"
)

func TestWorkedExample(t *testing.T) {
	g, err := ascii.New(assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	art, err := g.Generate("hello", "standard")
	if err != nil {
		t.Fatal(err)
	}
	got := Analyze(art)
	if got.Content != (Content{Characters: 256, Lines: 8, Width: 31}) || got.SizeEstimates["txt"] != 256 || got.SizeEstimates["html"] != 1440 || got.SizeEstimates["png"] != 14880 || got.Recommended != "txt" {
		t.Fatalf("пример ТЗ: %+v", got)
	}
}

func TestBoundariesAndFormulas(t *testing.T) {
	for _, tc := range []struct {
		name, content       string
		chars, lines, width int
		color               bool
		format              string
	}{
		{"пустой", "", 0, 0, 0, false, "txt"},
		{"нечётное число байтов", "abc", 3, 1, 3, false, "txt"},
		{"последний LF", "abc\n", 4, 1, 3, false, "txt"},
		{"пустые строки", "\n\n", 2, 2, 0, false, "txt"},
		{"Windows", "ab\r\nc\r\n", 7, 2, 2, false, "txt"},
		{"ширина120", strings.Repeat("x", 120), 120, 1, 120, false, "txt"},
		{"ширина121", strings.Repeat("x", 121), 121, 1, 121, false, "html"},
		{"ANSI", "\x1b[31mred\x1b[0m\n", 13, 1, 3, true, "html"},
		{"Unicode", "Я!", 3, 1, 2, false, "txt"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Analyze(tc.content)
			want := Content{Characters: tc.chars, Lines: tc.lines, Width: tc.width, UsesColor: tc.color}
			if got.Content != want || got.Recommended != tc.format || got.Reason == "" {
				t.Fatalf("получено=%+v ожидалось=%+v", got, want)
			}
			if got.SizeEstimates["txt"] != float64(tc.chars) || got.SizeEstimates["html"] != float64(tc.chars)*2.5+800 || got.SizeEstimates["png"] != float64(tc.width*tc.lines*60) {
				t.Fatal("нарушены формулы", got.SizeEstimates)
			}
			if _, err := json.Marshal(got); err != nil {
				t.Fatal(err)
			}
		})
	}
	if got := Analyze(strings.Repeat("x", 121) + "\x1b[31m"); !strings.Contains(got.Reason, "цвет") {
		t.Fatal("правило цвета должно предшествовать ширине", got)
	}
}

func FuzzAnalyze(f *testing.F) {
	for _, text := range []string{"", "hello\n", "\x1b[31mred\x1b[0m", "Я", "\n\n"} {
		f.Add(text)
	}
	f.Fuzz(func(t *testing.T, text string) {
		got := Analyze(text)
		if got.Content.Characters != len(text) || got.Content.Lines < 0 || got.Content.Width < 0 {
			t.Fatal(got)
		}
		if got.SizeEstimates["txt"] != float64(len(text)) {
			t.Fatal("число байтов изменилось")
		}
	})
}
