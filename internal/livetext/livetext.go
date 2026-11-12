package livetext

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/events"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const (
	Channel  = "slashobserve_run_text"
	MaxRunes = 2000

	maxChunkBytes    = 3000
	maxEntries       = 1024
	entryTTL         = 10 * time.Minute
	listenBackoff    = time.Second
	maxListenBackoff = 30 * time.Second
)

type entry struct {
	attempt int
	text    string
	touched time.Time
}

type message struct {
	Run     string `json:"run"`
	Attempt int    `json:"attempt"`
	Text    string `json:"text"`
}

type Store struct {
	db     *db.DB
	listen events.Listener
	logger *slog.Logger
	now    func() time.Time

	mu   sync.Mutex
	runs map[string]*entry
}

func New(database *db.DB, listen events.Listener, logger *slog.Logger) *Store {
	return &Store{db: database, listen: listen, logger: logger, now: time.Now, runs: map[string]*entry{}}
}

func (s *Store) Append(ctx context.Context, runID string, attempt int, text string) error {
	for _, chunk := range chunks(clean(text)) {
		payload, err := encode(message{Run: runID, Attempt: attempt, Text: chunk})
		if err != nil {
			return err
		}
		if err := s.db.Queries().NotifyEvent(ctx, dbsqlc.NotifyEventParams{Channel: Channel, Payload: payload}); err != nil {
			return fmt.Errorf("notify live text: %w", err)
		}
	}
	return nil
}

func (s *Store) Run(ctx context.Context) {
	backoff := listenBackoff
	for ctx.Err() == nil {
		established := false
		err := s.listen(ctx, Channel, func() { established = true }, s.apply)
		if ctx.Err() != nil {
			return
		}
		s.logger.Warn("live text listener failed", logattrs.Failure("live_text_listener_failed", err, slog.Duration("retry_in", backoff))...)
		if established {
			backoff = listenBackoff
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, maxListenBackoff)
	}
}

func (s *Store) Text(runID string, attempt int) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept, ok := s.runs[runID]
	if !ok || kept.attempt != attempt {
		return "", false
	}
	return kept.text, true
}

func (s *Store) apply(payload string) {
	var msg message
	if err := json.Unmarshal([]byte(payload), &msg); err != nil || msg.Run == "" {
		s.logger.Debug("live text payload dropped")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	s.dropExpired(now)
	kept, ok := s.runs[msg.Run]
	switch {
	case !ok:
		s.makeRoom()
		s.runs[msg.Run] = &entry{attempt: msg.Attempt, text: tail(msg.Text), touched: now}
	case msg.Attempt > kept.attempt:
		*kept = entry{attempt: msg.Attempt, text: tail(msg.Text), touched: now}
	case msg.Attempt == kept.attempt:
		kept.text = tail(kept.text + msg.Text)
		kept.touched = now
	}
}

func (s *Store) dropExpired(now time.Time) {
	for id, kept := range s.runs {
		if now.Sub(kept.touched) >= entryTTL {
			delete(s.runs, id)
		}
	}
}

func (s *Store) makeRoom() {
	for len(s.runs) >= maxEntries {
		oldestID := ""
		var oldest time.Time
		for id, kept := range s.runs {
			if oldestID == "" || kept.touched.Before(oldest) {
				oldestID, oldest = id, kept.touched
			}
		}
		delete(s.runs, oldestID)
	}
}

func tail(text string) string {
	excess := utf8.RuneCountInString(text) - MaxRunes
	if excess <= 0 {
		return text
	}
	for i := range text {
		if excess == 0 {
			return text[i:]
		}
		excess--
	}
	return ""
}

func clean(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, strings.ToValidUTF8(text, ""))
}

func chunks(text string) []string {
	var out []string
	for len(text) > maxChunkBytes {
		cut := maxChunkBytes
		for !utf8.RuneStart(text[cut]) {
			cut--
		}
		out = append(out, text[:cut])
		text = text[cut:]
	}
	if text != "" {
		out = append(out, text)
	}
	return out
}

func encode(msg message) (string, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(msg); err != nil {
		return "", fmt.Errorf("encode live text: %w", err)
	}
	return strings.TrimSuffix(buf.String(), "\n"), nil
}
