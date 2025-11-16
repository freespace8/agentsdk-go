package event

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Bookmark 记录断点续播所需的 WAL 位点和状态快照。
type Bookmark struct {
	ID       string `json:"id"`
	Position int64  `json:"position"`
	State    []byte `json:"state,omitempty"`
}

var (
	errNilBookmark = errors.New("bookmark: nil reference")
)

// NewBookmark 创建新的断点标记，state 将以 JSON 形式持久化。
func NewBookmark(id string, position int64, state any) (*Bookmark, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("bookmark: id is empty")
	}
	if position < 0 {
		return nil, fmt.Errorf("bookmark: invalid position %d", position)
	}
	snapshot, err := encodeState(state)
	if err != nil {
		return nil, err
	}
	return &Bookmark{ID: id, Position: position, State: snapshot}, nil
}

// Clone 深拷贝 Bookmark，避免共享底层 slice。
func (b *Bookmark) Clone() *Bookmark {
	if b == nil {
		return nil
	}
	clone := *b
	if len(b.State) > 0 {
		clone.State = append([]byte(nil), b.State...)
	}
	return &clone
}

// Snapshot 使用新的状态快照更新 Bookmark。
func (b *Bookmark) Snapshot(state any) error {
	if b == nil {
		return errNilBookmark
	}
	snapshot, err := encodeState(state)
	if err != nil {
		return err
	}
	b.State = snapshot
	return nil
}

// Restore 将状态快照解码到目标对象。
func (b *Bookmark) Restore(target any) error {
	if b == nil || len(b.State) == 0 || target == nil {
		return nil
	}
	return json.Unmarshal(b.State, target)
}

// Advance 更新 WAL 位置，防止回退。
func (b *Bookmark) Advance(position int64) error {
	if b == nil {
		return errNilBookmark
	}
	if position < b.Position {
		return fmt.Errorf("bookmark: position rollback %d -> %d", b.Position, position)
	}
	b.Position = position
	return nil
}

func encodeState(state any) ([]byte, error) {
	switch v := state.(type) {
	case nil:
		return nil, nil
	case []byte:
		return append([]byte(nil), v...), nil
	case json.RawMessage:
		return append([]byte(nil), v...), nil
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("bookmark: marshal state: %w", err)
		}
		return data, nil
	}
}
