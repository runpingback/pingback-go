package pingback

import "time"

func newContext(ep executionPayload) *Context {
	t, _ := time.Parse(time.RFC3339, ep.ScheduledAt)
	return &Context{
		ExecutionID: ep.ExecutionID,
		Attempt:     ep.Attempt,
		ScheduledAt: t,
		Payload:     ep.Payload,
		logs:        make([]logEntry, 0),
		tasks:       make([]taskRequest, 0),
	}
}

func (c *Context) addLog(level, msg string, meta ...any) {
	entry := logEntry{
		Timestamp: time.Now().UnixMilli(),
		Level:     level,
		Message:   msg,
	}
	if len(meta) == 2 {
		entry.Meta = map[string]any{meta[0].(string): meta[1]}
	} else if len(meta) > 2 && len(meta)%2 == 0 {
		m := make(map[string]any, len(meta)/2)
		for i := 0; i < len(meta); i += 2 {
			m[meta[i].(string)] = meta[i+1]
		}
		entry.Meta = m
	}
	c.logs = append(c.logs, entry)
}

// Log adds an info-level log entry.
func (c *Context) Log(msg string, meta ...any) {
	c.addLog("info", msg, meta...)
}

// Warn adds a warn-level log entry.
func (c *Context) Warn(msg string, meta ...any) {
	c.addLog("warn", msg, meta...)
}

// Error adds an error-level log entry.
func (c *Context) Error(msg string, meta ...any) {
	c.addLog("error", msg, meta...)
}

// Debug adds a debug-level log entry.
func (c *Context) Debug(msg string, meta ...any) {
	c.addLog("debug", msg, meta...)
}

// Task queues a fan-out task to be dispatched after this handler completes.
func (c *Context) Task(name string, payload any) {
	c.tasks = append(c.tasks, taskRequest{Name: name, Payload: payload})
}
