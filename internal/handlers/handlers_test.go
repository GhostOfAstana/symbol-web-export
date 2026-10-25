package handlers

import (
	"bytes"
	"encoding/json"
	"html"
	"io/fs"
	"mime"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	assets "github.com/GhostOfAstana/symbol-web-export"
	"github.com/GhostOfAstana/symbol-web-export/internal/ascii"
	exporter "github.com/GhostOfAstana/symbol-web-export/internal/export"
	"github.com/GhostOfAstana/symbol-web-export/internal/insights"
)

func newHandler(t *testing.T) http.Handler {
	t.Helper()
	t.Setenv("LLM_BASE_URL", "")
	h, err := New(assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func request(h http.Handler, method, target, contentType, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestGenerateAndDownloadExactContent(t *testing.T) {
	h := newHandler(t)
	g, err := ascii.New(assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	for _, banner := range []string{"standard", "shadow", "thinkertoy"} {
		form := url.Values{"text": {"hello"}, "banner": {banner}}
		page := request(h, "POST", "/symbol-art", "application/x-www-form-urlencoded", form.Encode(), nil)
		if page.Code != 200 {
			t.Fatal(page.Code, page.Body.String())
		}
		match := regexp.MustCompile(`(?s)<pre id="art"[^>]*>(.*?)</pre>`).FindStringSubmatch(page.Body.String())
		if len(match) != 2 {
			t.Fatal("нет рисунка на странице")
		}
		art, _ := g.Generate("hello", banner)
		if html.UnescapeString(match[1]) != art {
			t.Fatal("рисунок на странице отличается от генератора")
		}
		for _, format := range []string{"txt", "html", "png", "pdf"} {
			payload, _ := json.Marshal(map[string]string{"content": art, "format": format, "filename": "../../мой рисунок\r\nX-Injected: yes.exe"})
			got := request(h, "POST", "/export", "application/json", string(payload), nil)
			if got.Code != 200 {
				t.Fatal(format, got.Code, got.Body.String())
			}
			if got.Header().Get("Content-Length") != strconv.Itoa(got.Body.Len()) {
				t.Fatal("Content-Length не совпадает с байтами тела")
			}
			kind, params, err := mime.ParseMediaType(got.Header().Get("Content-Disposition"))
			if err != nil {
				t.Fatal(err)
			}
			if kind != "attachment" || !strings.HasSuffix(params["filename"], "."+format) || strings.ContainsAny(params["filename"], "/\\\r\n") || got.Header().Get("X-Injected") != "" {
				t.Fatal("небезопасное имя", params)
			}
			if format == "txt" && !bytes.Equal(got.Body.Bytes(), []byte(art)) {
				t.Fatal("скачанные байты изменились")
			}
			if got.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatal("нет nosniff")
			}
		}
	}
}

func TestRoutesMethodsAndInvalidBodies(t *testing.T) {
	h := newHandler(t)
	for _, tc := range []struct {
		method, path, kind, body string
		status                   int
	}{
		{"GET", "/", "", "", 200}, {"GET", "/missing", "", "", 404}, {"PUT", "/", "", "", 405},
		{"GET", "/symbol-art", "", "", 405}, {"POST", "/symbol-art", "application/x-www-form-urlencoded", "text=&banner=standard", 400},
		{"POST", "/symbol-art", "application/x-www-form-urlencoded", "text=hello&banner=../../etc/passwd", 400},
		{"POST", "/symbol-art", "application/x-www-form-urlencoded", "text=%D0%AF&banner=standard", 400},
		{"POST", "/symbol-art", "application/x-www-form-urlencoded", "text=%ZZ&banner=standard", 400},
		{"POST", "/symbol-art", "application/x-www-form-urlencoded", "text=" + strings.Repeat("x", 4097) + "&banner=standard", 400},
		{"POST", "/symbol-art", "application/x-www-form-urlencoded", strings.Repeat("x", maxRequestBytes+1), 400},
		{"GET", "/export", "", "", 200}, {"PUT", "/export", "", "", 405},
		{"POST", "/export", "application/json", "{", 400}, {"POST", "/export", "application/json", `null`, 400},
		{"POST", "/export", "application/json", `{"content":"x","format":"txt"} {}`, 400},
		{"POST", "/export", "application/json", `{"content":"x","format":"txt","unknown":true}`, 400},
		{"POST", "/export", "application/json", `{"content":"x","format":"exe"}`, 400},
		{"POST", "/export", "application/json", `{"content":"Я","format":"pdf"}`, 400},
		{"POST", "/export", "application/json", `{"content":"Я","format":"png"}`, 400},
		{"POST", "/export", "application/x-www-form-urlencoded", "content=%FF&format=txt", 400},
		{"POST", "/export", "application/x-www-form-urlencoded", "content=%ZZ&format=txt", 400},
		{"GET", "/export?content=%ZZ&format=txt", "", "", 400},
		{"POST", "/export", "application/json", `{"content":"` + strings.Repeat("x", exporter.MaxContentBytes+1) + `","format":"txt"}`, 400},
		{"POST", "/export", "application/json", strings.Repeat(" ", maxRequestBytes+1), 400},
		{"GET", "/export-analyzer?content=%ZZ", "", "", 400}, {"GET", "/export-analyzer?content=%FF", "", "", 400},
		{"POST", "/export-analyzer", "", "", 405}, {"POST", "/export-insights", "", "", 405}, {"POST", "/healthz", "", "", 405},
		{"GET", "/static/css/style.css", "", "", 200}, {"GET", "/static/js/script.js", "", "", 200}, {"GET", "/static/missing.js", "", "", 404},
		{"GET", "/healthz", "", "", 200},
	} {
		got := request(h, tc.method, tc.path, tc.kind, tc.body, nil)
		if got.Code != tc.status {
			t.Fatalf("%s %s тело=%q: получено=%d нужно=%d %s", tc.method, tc.path, tc.body[:min(len(tc.body), 80)], got.Code, tc.status, got.Body.String()[:min(got.Body.Len(), 200)])
		}
		if tc.status == 405 && got.Header().Get("Allow") == "" {
			t.Fatal("405 без Allow", tc.path)
		}
	}
}

func TestAnalyzerAndUnicodeDownloads(t *testing.T) {
	h := newHandler(t)
	got := request(h, "GET", "/export-analyzer?"+url.Values{"content": {"abc"}}.Encode(), "", "", nil)
	var result struct {
		Content       struct{ Characters, Lines, Width int }
		SizeEstimates map[string]float64 `json:"size_estimates"`
	}
	if err := json.Unmarshal(got.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Content.Characters != 3 || result.Content.Lines != 1 || result.Content.Width != 3 || result.SizeEstimates["html"] != 807.5 {
		t.Fatal(got.Body.String())
	}
	for _, format := range []string{"txt", "html"} {
		content := "Привет <script>alert(1)</script> & 世界\n"
		query := url.Values{"content": {content}, "format": {format}, "filename": {"рисунок"}}
		got := request(h, "GET", "/export?"+query.Encode(), "", "", nil)
		if got.Code != 200 || got.Header().Get("Content-Length") != strconv.Itoa(got.Body.Len()) {
			t.Fatal(got.Code)
		}
		if format == "txt" && got.Body.String() != content {
			t.Fatal("Unicode изменился")
		}
		if format == "html" && strings.Contains(got.Body.String(), "<script>") {
			t.Fatal("XSS в HTML-файле")
		}
	}
	long := request(h, "GET", "/export-analyzer?"+url.Values{"content": {strings.Repeat("x", exporter.MaxContentBytes+1)}}.Encode(), "", "", nil)
	if long.Code != 400 {
		t.Fatal(long.Code)
	}
}

func summary(t *testing.T, h http.Handler, cookie *http.Cookie) (insights.Summary, *http.Cookie) {
	t.Helper()
	got := request(h, "GET", "/export-insights", "", "", cookie)
	var result insights.Summary
	if err := json.Unmarshal(got.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if cookies := got.Result().Cookies(); len(cookies) > 0 {
		cookie = cookies[0]
	}
	return result, cookie
}

func TestCookiesSessionIsolationAndRepeatExports(t *testing.T) {
	h := newHandler(t)
	_, a := summary(t, h, nil)
	_, b := summary(t, h, nil)
	if a == nil || b == nil || a.Value == b.Value || !a.HttpOnly || a.SameSite != http.SameSiteLaxMode {
		t.Fatal("ошибка cookie")
	}
	for _, format := range []string{"txt", "txt", "html", "png", "pdf"} {
		body, _ := json.Marshal(map[string]string{"content": "hello", "format": format})
		got := request(h, "POST", "/export", "application/json", string(body), a)
		if got.Code != 200 {
			t.Fatal(got.Code, got.Body.String())
		}
	}
	first, _ := summary(t, h, a)
	second, _ := summary(t, h, b)
	if first.Total != 5 || first.Counts["txt"] != 2 || first.MostExported != "txt" || !first.RepeatContent || second.Total != 0 {
		t.Fatalf("сессии: %+v %+v", first, second)
	}
	// Неуспешный экспорт не увеличивает счётчик.
	request(h, "POST", "/export", "application/json", `{"content":"x","format":"invalid"}`, a)
	after, _ := summary(t, h, a)
	if after.Total != first.Total {
		t.Fatal("ошибка посчитана как экспорт")
	}
	var group sync.WaitGroup
	for range 32 {
		group.Add(1)
		go func() {
			defer group.Done()
			got := request(h, "POST", "/export", "application/json", `{"content":"hello","format":"txt"}`, a)
			if got.Code != 200 {
				t.Errorf("status=%d", got.Code)
			}
		}()
	}
	group.Wait()
	after, _ = summary(t, h, a)
	if after.Total != 37 {
		t.Fatal(after.Total)
	}
}

func TestStartupResourcesAndSafeNames(t *testing.T) {
	if _, err := New(fstest.MapFS{}); err == nil {
		t.Fatal("нет ошибки отсутствующих шрифтов")
	}
	files := fstest.MapFS{}
	for _, name := range []string{"standard.txt", "shadow.txt", "thinkertoy.txt"} {
		data, err := fs.ReadFile(assets.FS, name)
		if err != nil {
			t.Fatal(err)
		}
		files[name] = &fstest.MapFile{Data: data}
	}
	files["templates/index.html"] = &fstest.MapFile{Data: []byte("{{invalid")}
	if _, err := New(files); err == nil {
		t.Fatal("неверный шаблон принят")
	}
	for _, name := range []string{"", "../", "a\r\nInjected: yes.exe", `C:\folder\name.exe`, strings.Repeat("Я", 100)} {
		got := safeFilename(name, "txt")
		if !strings.HasSuffix(got, ".txt") || strings.ContainsAny(got, "/\\\r\n") || len(got) > 90 {
			t.Fatal("опасное имя", got)
		}
	}
}

func TestHeavyExportOverloadAndRelease(t *testing.T) {
	s := &Server{sessions: insights.New(), heavyExports: make(chan struct{}, 1)}
	s.heavyExports <- struct{}{}
	for _, format := range []string{"png", "pdf"} {
		r := httptest.NewRequest("POST", "/export", strings.NewReader(`{"content":"hello","format":"`+format+`"}`))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.download(w, r)
		if w.Code != 503 || w.Header().Get("Retry-After") != "1" {
			t.Fatal("перегрузка не обработана", w.Code)
		}
	}
	<-s.heavyExports
	// Ошибка формата содержимого тоже обязана освободить место в семафоре.
	for _, content := range []string{"Я", "hello"} {
		payload, _ := json.Marshal(map[string]string{"content": content, "format": "pdf"})
		r := httptest.NewRequest("POST", "/export", bytes.NewReader(payload))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.download(w, r)
		want := 200
		if content == "Я" {
			want = 400
		}
		if w.Code != want || len(s.heavyExports) != 0 {
			t.Fatal("место не освобождено", w.Code, len(s.heavyExports))
		}
	}
}
