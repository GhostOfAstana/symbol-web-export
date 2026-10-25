// Package handlers связывает формы, генератор, анализатор и загрузку файлов.
package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/GhostOfAstana/symbol-web-export/internal/ascii"
	exporter "github.com/GhostOfAstana/symbol-web-export/internal/export"
	"github.com/GhostOfAstana/symbol-web-export/internal/insights"
	"github.com/GhostOfAstana/symbol-web-export/internal/recommender"
	"github.com/GhostOfAstana/symbol-web-export/internal/textai"
)

const maxRequestBytes = 1 << 20

type Server struct {
	generator    *ascii.Generator
	templates    *template.Template
	sessions     *insights.Store
	heavyExports chan struct{}
}
type page struct {
	Text, Banner, Art, Error string
	HasArt                   bool
	Analysis                 recommender.Analysis
}

func New(fsys fs.FS) (http.Handler, error) {
	g, err := ascii.New(fsys)
	if err != nil {
		return nil, err
	}
	t, err := template.ParseFS(fsys, "templates/*.html")
	if err != nil {
		return nil, err
	}
	static, err := fs.Sub(fsys, "static")
	if err != nil {
		return nil, err
	}
	s := &Server{generator: g, templates: t, sessions: insights.New(), heavyExports: make(chan struct{}, 4)}
	mux := http.NewServeMux()
	assistant, err := textai.NewFromEnv()
	if err != nil {
		return nil, err
	}
	assistant.Register(mux)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			s.render(w, 404, page{Error: "Страница не найдена."})
			return
		}
		if !method(w, r, "GET") {
			return
		}
		s.render(w, 200, page{Banner: "standard"})
	})
	mux.HandleFunc("/symbol-art", s.generate)
	mux.HandleFunc("/export", s.download)
	mux.HandleFunc("/export-analyzer", s.analyze)
	mux.HandleFunc("/export-insights", func(w http.ResponseWriter, r *http.Request) {
		if !method(w, r, "GET") {
			return
		}
		writeJSON(w, 200, s.sessions.Read(w, r))
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if !method(w, r, "GET") {
			return
		}
		writeJSON(w, 200, map[string]string{"status": "ok"})
	})
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(static))))
	return secure(mux), nil
}

func method(w http.ResponseWriter, r *http.Request, allowed string) bool {
	if r.Method != allowed {
		w.Header().Set("Allow", allowed)
		http.Error(w, "Метод не поддерживается.", 405)
		return false
	}
	return true
}

func (s *Server) render(w http.ResponseWriter, status int, p page) {
	var body bytes.Buffer
	if err := s.templates.ExecuteTemplate(&body, "index.html", p); err != nil {
		log.Printf("ошибка шаблона: %v", err)
		http.Error(w, "Не удалось показать страницу.", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body.Bytes())
}

func (s *Server) generate(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, "POST") {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	if err := r.ParseForm(); err != nil {
		s.render(w, 400, page{Error: "Форма повреждена или слишком велика."})
		return
	}
	p := page{Text: r.PostForm.Get("text"), Banner: r.PostForm.Get("banner")}
	if p.Text == "" {
		p.Error = "Введите текст для рисунка."
		s.render(w, 400, p)
		return
	}
	art, err := s.generator.Generate(p.Text, p.Banner)
	if err != nil {
		p.Error = err.Error()
		s.render(w, 400, p)
		return
	}
	if len(art) > exporter.MaxContentBytes {
		p.Error = "Рисунок превышает 128 КиБ: сократите текст."
		s.render(w, 400, p)
		return
	}
	p.Art, p.HasArt, p.Analysis = art, true, recommender.Analyze(art)
	s.render(w, 200, p)
}

func (s *Server) analyze(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, "GET") {
		return
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "Некорректная строка запроса."})
		return
	}
	content := query.Get("content")
	if len(content) > exporter.MaxContentBytes || !utf8.ValidString(content) {
		writeJSON(w, 400, map[string]string{"error": "Рисунок должен быть корректным UTF-8 размером до 128 КиБ."})
		return
	}
	writeJSON(w, 200, recommender.Analyze(content))
}

type exportRequest struct {
	Content  string `json:"content"`
	Format   string `json:"format"`
	Filename string `json:"filename"`
}

func (s *Server) download(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" && r.URL.RawQuery == "" {
		s.render(w, 200, page{Banner: "standard"})
		return
	}
	if r.Method != "POST" && r.Method != "GET" {
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "Метод не поддерживается.", 405)
		return
	}
	var request exportRequest
	if r.Method == "GET" {
		query, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": "Некорректная строка запроса."})
			return
		}
		request = exportRequest{query.Get("content"), query.Get("format"), query.Get("filename")}
	} else {
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
		media, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if media == "application/json" {
			decoder := json.NewDecoder(r.Body)
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&request); err != nil {
				writeJSON(w, 400, map[string]string{"error": "Некорректный JSON."})
				return
			}
			if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
				writeJSON(w, 400, map[string]string{"error": "Ожидается один JSON-объект."})
				return
			}
		} else {
			if err := r.ParseForm(); err != nil {
				writeJSON(w, 400, map[string]string{"error": "Некорректная форма."})
				return
			}
			request = exportRequest{r.PostForm.Get("content"), r.PostForm.Get("format"), r.PostForm.Get("filename")}
		}
	}
	// PNG выделяет пиксельный буфер, PDF собирает страницы. Ограничиваем число
	// таких операций, чтобы всплеск запросов не исчерпал память процесса.
	if request.Format == "png" || request.Format == "pdf" {
		select {
		case s.heavyExports <- struct{}{}:
			defer func() { <-s.heavyExports }()
		default:
			w.Header().Set("Retry-After", "1")
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Сервер занят подготовкой файлов. Повторите загрузку через секунду."})
			return
		}
	}
	file, err := exporter.Build(request.Content, request.Format)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	name := safeFilename(request.Filename, file.Extension)
	s.sessions.Record(w, r, request.Content, file.Extension)
	w.Header().Set("Content-Type", file.MIME)
	w.Header().Set("Content-Length", strconv.Itoa(len(file.Data)))
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	_, _ = w.Write(file.Data)
}

func safeFilename(name, extension string) string {
	name = path.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.TrimSuffix(name, path.Ext(name))
	var clean strings.Builder
	for _, char := range name {
		if unicode.IsLetter(char) || unicode.IsDigit(char) || char == '-' || char == '_' {
			clean.WriteRune(char)
		}
		if clean.Len() >= 80 {
			break
		}
	}
	if clean.Len() == 0 {
		return "symbol-art." + extension
	}
	return clean.String() + "." + extension
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(data); err != nil {
		http.Error(w, "Ошибка формирования ответа.", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body.Bytes())
}

func secure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("неожиданная ошибка запроса: %v", recovered)
				http.Error(w, "Внутренняя ошибка сервера.", 500)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
