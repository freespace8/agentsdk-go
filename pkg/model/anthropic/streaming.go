package anthropic

import (
	"bufio"
	"context"
	"io"
	"strings"
)

// consumeSSE parses a Server-Sent Events stream, invoking fn for each event.
func consumeSSE(ctx context.Context, r io.Reader, fn func(event, data string) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var eventName string
	var dataBuf strings.Builder
	flush := func() error {
		if dataBuf.Len() == 0 {
			eventName = ""
			return nil
		}
		payload := dataBuf.String()
		dataBuf.Reset()
		return fn(eventName, payload)
	}

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		line := scanner.Text()
		if line == "" {
			if err := flush(); err != nil {
				return err
			}
			eventName = ""
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "event:") {
			eventName = strings.TrimSpace(line[6:])
			continue
		}
		if strings.HasPrefix(line, "data:") {
			if dataBuf.Len() > 0 {
				dataBuf.WriteByte('\n')
			}
			dataBuf.WriteString(strings.TrimSpace(line[5:]))
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return flush()
}
