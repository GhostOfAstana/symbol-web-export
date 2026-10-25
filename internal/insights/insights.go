// Package insights хранит ограниченную статистику отдельно для каждого браузера.
package insights

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sync"
	"time"
)

const maxSessions = 1024
const maxHashes = 128

type Summary struct {
	Total         int            `json:"total"`
	Counts        map[string]int `json:"counts"`
	MostExported  string         `json:"most_exported"`
	RepeatContent bool           `json:"repeat_content"`
	Optimization  string         `json:"optimization"`
}

type session struct {
	updated   time.Time
	counts    map[string]int
	hashes    map[[32]byte]bool
	hashOrder [][32]byte
	repeated  bool
}
type Store struct {
	mu       sync.Mutex
	sessions map[string]*session
}

func New() *Store { return &Store{sessions: make(map[string]*session)} }

func (s *Store) current(w http.ResponseWriter, r *http.Request) *session {
	now := time.Now()
	id := ""
	if cookie, err := r.Cookie("symbol_session"); err == nil {
		id = cookie.Value
	}
	if existing := s.sessions[id]; existing != nil && now.Sub(existing.updated) < time.Hour {
		existing.updated = now
		return existing
	}
	for key, value := range s.sessions {
		if now.Sub(value.updated) >= time.Hour {
			delete(s.sessions, key)
		}
	}
	if len(s.sessions) >= maxSessions {
		oldestID := ""
		oldest := now
		for key, value := range s.sessions {
			if value.updated.Before(oldest) {
				oldest, oldestID = value.updated, key
			}
		}
		delete(s.sessions, oldestID)
	}
	var random [32]byte
	if _, err := rand.Read(random[:]); err != nil {
		// При отказе источника случайности запрос работает без постоянной сессии.
		return &session{updated: now, counts: map[string]int{}, hashes: map[[32]byte]bool{}}
	}
	id = hex.EncodeToString(random[:])
	http.SetCookie(w, &http.Cookie{Name: "symbol_session", Value: id, Path: "/", HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteLaxMode, MaxAge: 3600})
	created := &session{updated: now, counts: map[string]int{}, hashes: map[[32]byte]bool{}}
	s.sessions[id] = created
	return created
}

func (s *Store) Record(w http.ResponseWriter, r *http.Request, content, format string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current := s.current(w, r)
	current.counts[format]++
	hash := sha256.Sum256([]byte(content))
	if current.hashes[hash] {
		current.repeated = true
	}
	if !current.hashes[hash] {
		if len(current.hashes) >= maxHashes {
			delete(current.hashes, current.hashOrder[0])
			current.hashOrder = current.hashOrder[1:]
		}
		current.hashOrder = append(current.hashOrder, hash)
	}
	current.hashes[hash] = true
}

func (s *Store) Read(w http.ResponseWriter, r *http.Request) Summary {
	s.mu.Lock()
	defer s.mu.Unlock()
	current := s.current(w, r)
	result := Summary{Counts: map[string]int{"txt": 0, "html": 0, "pdf": 0, "png": 0}, RepeatContent: current.repeated, Optimization: "Скачайте рисунок, чтобы получить совет по вашим форматам."}
	best := 0
	// Фиксированный порядок разрешает равенство счётчиков воспроизводимо.
	for _, format := range []string{"txt", "html", "pdf", "png"} {
		count := current.counts[format]
		result.Counts[format] = count
		result.Total += count
		if count > best {
			best, result.MostExported = count, format
		}
	}
	if result.Total > 0 {
		result.Optimization = "Чаще всего вы выбираете " + result.MostExported + "; используйте его как формат по умолчанию."
	}
	if result.RepeatContent {
		result.Optimization += " Повторный рисунок можно сохранить и использовать снова без генерации."
	}
	return result
}
