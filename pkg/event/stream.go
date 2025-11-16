package event

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const defaultHeartbeat = 25 * time.Second

// Stream 封装 SSE 写入逻辑，将事件安全地推送给 HTTP 客户端。
type Stream struct {
	w         io.Writer
	flush     func()
	heartbeat time.Duration
	mu        sync.Mutex
}

// NewStream 构造 HTTP SSE 流。
func NewStream(w http.ResponseWriter) *Stream {
	headers := w.Header()
	headers.Set("Content-Type", "text/event-stream")
	headers.Set("Cache-Control", "no-cache")
	headers.Set("Connection", "keep-alive")

	var flushFn func()
	if f, ok := w.(http.Flusher); ok {
		flushFn = f.Flush
	}

	return &Stream{
		w:         w,
		flush:     flushFn,
		heartbeat: defaultHeartbeat,
	}
}

// NewStreamWriter 允许使用自定义 writer（例如测试）。
func NewStreamWriter(w io.Writer) *Stream {
	return &Stream{w: w}
}

// SetHeartbeat 调整心跳间隔，<=0 将关闭心跳。
func (s *Stream) SetHeartbeat(d time.Duration) {
	if d <= 0 {
		s.heartbeat = 0
		return
	}
	s.heartbeat = d
}

// StreamEvents 将事件通道转成 SSE 响应。
func (s *Stream) StreamEvents(ctx context.Context, events <-chan Event) error {
	if s == nil {
		return errors.New("event: stream is nil")
	}
	var ticker *time.Ticker
	if s.heartbeat > 0 {
		ticker = time.NewTicker(s.heartbeat)
		defer ticker.Stop()
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case evt, ok := <-events:
			if !ok {
				return s.write([]byte("event: complete\ndata: {}\n\n"))
			}
			if err := s.Send(evt); err != nil {
				return err
			}
		case <-heartbeatChan(ticker):
			if err := s.sendHeartbeat(); err != nil {
				return err
			}
		}
	}
}

// Send 将单个事件写入 SSE 流。
func (s *Stream) Send(evt Event) error {
	if s == nil {
		return errors.New("event: stream is nil")
	}
	normalized := normalizeEvent(evt)
	payload := struct {
		ID        string    `json:"id"`
		Type      EventType `json:"type"`
		Timestamp time.Time `json:"timestamp"`
		SessionID string    `json:"session_id,omitempty"`
		Data      any       `json:"data,omitempty"`
		Bookmark  *Bookmark `json:"bookmark,omitempty"`
	}{
		ID:        normalized.ID,
		Type:      normalized.Type,
		Timestamp: normalized.Timestamp,
		SessionID: normalized.SessionID,
		Data:      normalized.Data,
		Bookmark:  normalized.Bookmark,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("event: marshal SSE payload: %w", err)
	}

	frame := fmt.Sprintf("id: %s\nevent: %s\ndata: %s\n\n", payload.ID, payload.Type, body)
	return s.write([]byte(frame))
}

func (s *Stream) sendHeartbeat() error {
	if s == nil || s.w == nil || s.heartbeat <= 0 {
		return nil
	}
	payload := fmt.Sprintf(": ping %d\n\n", time.Now().Unix())
	return s.write([]byte(payload))
}

func (s *Stream) write(data []byte) error {
	if s == nil || s.w == nil {
		return errors.New("event: stream writer not configured")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := s.w.Write(data); err != nil {
		return err
	}
	if s.flush != nil {
		s.flush()
	}
	return nil
}

func heartbeatChan(t *time.Ticker) <-chan time.Time {
	if t == nil {
		return nil
	}
	return t.C
}
