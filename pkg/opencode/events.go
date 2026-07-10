package opencode

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Event is a single frame from the opencode /event SSE stream. Properties
// stays raw until the consumer switches on Type and decodes the payload it
// cares about (e.g. PermissionAsked for EventPermissionAsked).
type Event struct {
	ID         string
	Type       string
	Properties json.RawMessage
}

const (
	sseInitialBackoff = 500 * time.Millisecond
	sseMaxBackoff     = 30 * time.Second
	// sseMaxFrameSize bounds a single SSE line; opencode can emit very large
	// frames (whole file contents inside message parts), well past
	// bufio.Scanner's 64KB default.
	sseMaxFrameSize   = 16 << 20 // 16MB
	sseBufferedEvents = 32
)

// Events subscribes to GET /event and returns a channel of parsed events.
// The subscription survives connection loss: it reconnects with exponential
// backoff (capped at 30s) until ctx is cancelled, at which point the channel
// is closed. The server emits server.connected as the first event after
// every (re)connect, which consumers can use as a resync signal.
//
// The stream request never times out (the injected client is cloned with
// Timeout 0); lifetime is controlled solely by ctx.
func (c *Client) Events(ctx context.Context) (<-chan Event, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/event", nil)
	if err != nil {
		return nil, fmt.Errorf("opencode: build /event request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	req.SetBasicAuth(basicAuthUser, c.password)

	// Clone the injected client with the overall timeout disabled: an SSE
	// stream is expected to stay open indefinitely.
	hc := *c.hc
	hc.Timeout = 0

	ch := make(chan Event, sseBufferedEvents)
	go c.streamEvents(ctx, &hc, req, ch)
	return ch, nil
}

// streamEvents owns the connect/read/reconnect loop and closes ch on exit.
func (c *Client) streamEvents(ctx context.Context, hc *http.Client, req *http.Request, ch chan<- Event) {
	defer close(ch)

	backoff := sseInitialBackoff
	for attempt := 0; ; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > sseMaxBackoff {
				backoff = sseMaxBackoff
			}
		}
		if ctx.Err() != nil {
			return
		}

		resp, err := hc.Do(req.Clone(ctx))
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			c.log.Debug("sse connect failed", "error", err)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			c.log.Debug("sse connect rejected", "status", resp.StatusCode)
			continue
		}

		// Connected: reset backoff so the next drop reconnects quickly.
		backoff = sseInitialBackoff
		err = c.readEvents(ctx, resp.Body, ch)
		resp.Body.Close()
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			c.log.Debug("sse stream ended", "error", err)
		}
	}
}

// readEvents scans one SSE stream, dispatching complete frames onto ch until
// the stream ends or ctx is cancelled.
func (c *Client) readEvents(ctx context.Context, body io.Reader, ch chan<- Event) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), sseMaxFrameSize)

	var dataLines []string
	var lastID string
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")

		// A blank line terminates the frame; dispatch accumulated data.
		if line == "" {
			if len(dataLines) > 0 {
				if ev, ok := c.parseEvent(lastID, strings.Join(dataLines, "\n")); ok {
					select {
					case ch <- ev:
					case <-ctx.Done():
						return ctx.Err()
					}
				}
				dataLines = dataLines[:0]
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue // SSE comment / keep-alive
		}

		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "data":
			dataLines = append(dataLines, value)
		case "id":
			lastID = value
		}
		// "event" and "retry" fields are ignored: opencode carries the event
		// type inside the JSON payload.
	}
	return scanner.Err()
}

// parseEvent decodes one joined data payload; malformed frames are skipped
// rather than killing the stream.
func (c *Client) parseEvent(id, data string) (Event, bool) {
	var frame struct {
		Type       string          `json:"type"`
		Properties json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal([]byte(data), &frame); err != nil {
		c.log.Debug("sse frame is not valid event JSON, skipping", "error", err)
		return Event{}, false
	}
	return Event{ID: id, Type: frame.Type, Properties: frame.Properties}, true
}
