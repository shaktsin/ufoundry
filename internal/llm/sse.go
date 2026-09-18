package llm

import (
	"bufio"
	"io"
	"strings"
)

// sseEvent is one server-sent event.
type sseEvent struct {
	Event string
	Data  string
}

// readSSE calls fn for each event in r until EOF, an error, or fn returns false.
func readSSE(r io.Reader, fn func(sseEvent) bool) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 16<<20)
	var ev sseEvent
	var data []string
	flush := func() bool {
		if len(data) == 0 && ev.Event == "" {
			return true
		}
		ev.Data = strings.Join(data, "\n")
		ok := fn(ev)
		ev, data = sseEvent{}, nil
		return ok
	}
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			if !flush() {
				return nil
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "event":
			ev.Event = value
		case "data":
			data = append(data, value)
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	flush()
	return nil
}
