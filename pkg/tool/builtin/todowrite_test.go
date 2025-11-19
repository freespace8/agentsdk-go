package toolbuiltin

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
)

func TestTodoWriteUpdatesState(t *testing.T) {
	tool := NewTodoWriteTool()
	params := map[string]interface{}{
		"todos": []interface{}{
			map[string]interface{}{"content": "Plan tests", "status": "pending", "activeForm": "default"},
			map[string]interface{}{"content": "Implement feature", "status": "in_progress", "activeForm": "default"},
			map[string]interface{}{"content": "Ship", "status": "completed", "activeForm": "default"},
		},
	}
	res, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if res == nil || res.Data == nil {
		t.Fatalf("expected result data")
	}
	data, ok := res.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected data type %T", res.Data)
	}
	if total, ok := data["total"].(int); ok && total != 3 {
		t.Fatalf("expected total=3 got %v", total)
	}
	snapshot := tool.Snapshot()
	if len(snapshot) != 3 {
		t.Fatalf("expected snapshot len 3, got %d", len(snapshot))
	}
	want := []string{"Implement feature", "Plan tests", "Ship"}
	if got := OrderedTodoContent(snapshot); !slices.Equal(got, want) {
		t.Fatalf("snapshot mismatch: want %v got %v", want, got)
	}
	if counts, ok := data["counts"].(map[string]int); ok {
		if counts["pending"] != 1 || counts["in_progress"] != 1 || counts["completed"] != 1 {
			t.Fatalf("unexpected counts: %+v", counts)
		}
	}
}

func TestTodoWriteAllowsClearing(t *testing.T) {
	tool := NewTodoWriteTool()
	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"todos": []interface{}{
			map[string]interface{}{"content": "x", "status": "pending", "activeForm": "default"},
		},
	})
	if err != nil {
		t.Fatalf("seed execute failed: %v", err)
	}
	_, err = tool.Execute(context.Background(), map[string]interface{}{"todos": []interface{}{}})
	if err != nil {
		t.Fatalf("clearing todos returned error: %v", err)
	}
	if got := tool.Snapshot(); len(got) != 0 {
		t.Fatalf("expected empty snapshot, got %d entries", len(got))
	}
}

func TestTodoWriteRejectsInvalidStatus(t *testing.T) {
	tool := NewTodoWriteTool()
	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"todos": []interface{}{
			map[string]interface{}{"content": "x", "status": "blocked", "activeForm": "f"},
		},
	})
	if err == nil {
		t.Fatalf("expected error for invalid status")
	}
}

func TestTodoWriteAcceptsTypedArray(t *testing.T) {
	tool := NewTodoWriteTool()
	params := map[string]interface{}{
		"todos": []map[string]interface{}{
			{"content": "typed", "status": "completed", "activeForm": "default"},
		},
	}
	if _, err := tool.Execute(context.Background(), params); err != nil {
		t.Fatalf("expected typed array to be accepted, got %v", err)
	}
}

func TestTodoWriteConcurrentExecutions(t *testing.T) {
	tool := NewTodoWriteTool()
	const workers = 8
	const perWorker = 3
	states := make([][]string, workers)

	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		i := i
		states[i] = make([]string, perWorker)
		go func() {
			defer wg.Done()
			todos := make([]interface{}, perWorker)
			for j := 0; j < perWorker; j++ {
				name := fmt.Sprintf("worker-%d-task-%d", i, j)
				states[i][j] = name
				todos[j] = map[string]interface{}{
					"content":    name,
					"status":     "pending",
					"activeForm": fmt.Sprintf("form-%d", i),
				}
			}
			if _, err := tool.Execute(context.Background(), map[string]interface{}{"todos": todos}); err != nil {
				t.Errorf("worker %d execute error: %v", i, err)
			}
		}()
	}
	wg.Wait()

	snapshot := tool.Snapshot()
	if len(snapshot) != perWorker {
		t.Fatalf("expected snapshot len %d, got %d", perWorker, len(snapshot))
	}
	got := OrderedTodoContent(snapshot)
	found := false
	for _, state := range states {
		target := append([]string(nil), state...)
		slices.Sort(target)
		if slices.Equal(got, target) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("final snapshot %v not in %v", got, states)
	}
}

func TestTodoWriteMetadata(t *testing.T) {
	tool := NewTodoWriteTool()
	if tool.Name() != "TodoWrite" {
		t.Fatalf("unexpected name %q", tool.Name())
	}
	if tool.Schema() == nil {
		t.Fatalf("schema is nil")
	}
	if desc := tool.Description(); !strings.Contains(desc, "todo list") {
		t.Fatalf("unexpected description %q", desc)
	}
}

func TestTodoWriteMissingTodos(t *testing.T) {
	tool := NewTodoWriteTool()
	if _, err := tool.Execute(context.Background(), map[string]interface{}{}); err == nil {
		t.Fatalf("expected error when todos missing")
	}
	if _, err := tool.Execute(context.Background(), nil); err == nil {
		t.Fatalf("expected error for nil params")
	}
}

func TestTodoWriteRejectsEmptyContent(t *testing.T) {
	tool := NewTodoWriteTool()
	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"todos": []interface{}{
			map[string]interface{}{"content": "   ", "status": "pending", "activeForm": "f"},
		},
	})
	if err == nil {
		t.Fatalf("expected error for empty content")
	}
}

func TestTodoWriteRejectsNonArray(t *testing.T) {
	tool := NewTodoWriteTool()
	if _, err := tool.Execute(context.Background(), map[string]interface{}{"todos": "oops"}); err == nil {
		t.Fatalf("expected error for non-array todos")
	}
	if _, err := tool.Execute(context.Background(), map[string]interface{}{"todos": []interface{}{"bad"}}); err == nil {
		t.Fatalf("expected error for non-object entry")
	}
	if _, err := tool.Execute(context.Background(), map[string]interface{}{
		"todos": []interface{}{
			map[string]interface{}{"content": "ok", "status": "pending"},
		},
	}); err == nil {
		t.Fatalf("expected error for missing activeForm")
	}
}
