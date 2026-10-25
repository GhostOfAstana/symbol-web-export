package insights

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func sessionRequest(cookie *http.Cookie) *http.Request {
	r := httptest.NewRequest("GET", "/", nil)
	if cookie != nil {
		r.AddCookie(cookie)
	}
	return r
}

func createSession(t *testing.T, s *Store) *http.Cookie {
	t.Helper()
	w := httptest.NewRecorder()
	s.Read(w, sessionRequest(nil))
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal("нет cookie")
	}
	return cookies[0]
}

func TestCountsRepeatAndIsolation(t *testing.T) {
	s := New()
	a := createSession(t, s)
	b := createSession(t, s)
	for _, format := range []string{"txt", "html", "png", "pdf", "txt"} {
		s.Record(httptest.NewRecorder(), sessionRequest(a), "hello", format)
	}
	got := s.Read(httptest.NewRecorder(), sessionRequest(a))
	other := s.Read(httptest.NewRecorder(), sessionRequest(b))
	if got.Total != 5 || got.Counts["png"] != 1 || got.MostExported != "txt" || !got.RepeatContent || got.Optimization == "" || other.Total != 0 {
		t.Fatalf("%+v %+v", got, other)
	}
	// Клиент получает копию карты и не может изменить серверную статистику.
	got.Counts["txt"] = 10000
	if again := s.Read(httptest.NewRecorder(), sessionRequest(a)); again.Counts["txt"] != 2 {
		t.Fatal("карта статистики утекла наружу")
	}
}

func TestSessionExpiryAndBounds(t *testing.T) {
	s := New()
	cookie := createSession(t, s)
	s.Record(httptest.NewRecorder(), sessionRequest(cookie), "x", "txt")
	s.sessions[cookie.Value].updated = time.Now().Add(-2 * time.Hour)
	w := httptest.NewRecorder()
	got := s.Read(w, sessionRequest(cookie))
	if got.Total != 0 || len(w.Result().Cookies()) != 1 || w.Result().Cookies()[0].Value == cookie.Value {
		t.Fatal("истёкшая сессия восстановлена")
	}
	for range maxSessions + 20 {
		s.Read(httptest.NewRecorder(), sessionRequest(nil))
	}
	if len(s.sessions) > maxSessions {
		t.Fatal("лимит сессий", len(s.sessions))
	}
	fresh := createSession(t, s)
	for i := range maxHashes + 10 {
		s.Record(httptest.NewRecorder(), sessionRequest(fresh), fmt.Sprintf("content-%d", i), "txt")
	}
	current := s.sessions[fresh.Value]
	if len(current.hashes) != maxHashes || len(current.hashOrder) != maxHashes {
		t.Fatal("лимит хэшей", len(current.hashes), len(current.hashOrder))
	}
	s.Record(httptest.NewRecorder(), sessionRequest(fresh), fmt.Sprintf("content-%d", maxHashes+9), "html")
	if !s.Read(httptest.NewRecorder(), sessionRequest(fresh)).RepeatContent {
		t.Fatal("потерян недавний повтор после заполнения хэшей")
	}
}

func TestConcurrentSession(t *testing.T) {
	s := New()
	cookie := createSession(t, s)
	var group sync.WaitGroup
	for range 64 {
		group.Add(1)
		go func() {
			defer group.Done()
			for range 20 {
				s.Record(httptest.NewRecorder(), sessionRequest(cookie), "same", "txt")
				s.Read(httptest.NewRecorder(), sessionRequest(cookie))
			}
		}()
	}
	group.Wait()
	got := s.Read(httptest.NewRecorder(), sessionRequest(cookie))
	if got.Total != 1280 || !got.RepeatContent {
		t.Fatal(got)
	}
}
