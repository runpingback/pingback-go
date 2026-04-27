package pingback

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

// Pingback is the SDK client.
type Pingback struct {
	apiKey     string
	cronSecret string
	opts       options
	functions  map[string]functionDef
	once       sync.Once
}

// New creates a new Pingback client.
func New(apiKey string, cronSecret string, opts ...Option) *Pingback {
	o := options{
		platformURL: "https://api.pingback.lol",
	}
	for _, opt := range opts {
		opt(&o)
	}
	return &Pingback{
		apiKey:     apiKey,
		cronSecret: cronSecret,
		opts:       o,
		functions:  make(map[string]functionDef),
	}
}

// Cron registers a cron job.
func (p *Pingback) Cron(name, schedule string, handler HandlerFunc, opts ...FuncOption) {
	fo := funcOptions{concurrency: 1}
	for _, opt := range opts {
		opt(&fo)
	}
	p.functions[name] = functionDef{
		name:     name,
		funcType: "cron",
		schedule: schedule,
		handler:  handler,
		options:  fo,
	}
}

// Task registers a background task.
func (p *Pingback) Task(name string, handler HandlerFunc, opts ...FuncOption) {
	fo := funcOptions{concurrency: 1}
	for _, opt := range opts {
		opt(&fo)
	}
	p.functions[name] = functionDef{
		name:     name,
		funcType: "task",
		handler:  handler,
		options:  fo,
	}
}

// TaskWith registers a background task with a typed payload.
// The payload is automatically unmarshalled from JSON into the specified type.
func TaskWith[T any](p *Pingback, name string, handler TypedHandlerFunc[T], opts ...FuncOption) {
	p.Task(name, func(ctx *Context) (any, error) {
		var payload T
		if ctx.Payload != nil {
			if err := json.Unmarshal(ctx.Payload, &payload); err != nil {
				return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
			}
		}
		return handler(ctx, payload)
	}, opts...)
}

// CronWith registers a cron job with a typed payload (useful for manually triggered crons with payloads).
func CronWith[T any](p *Pingback, name, schedule string, handler TypedHandlerFunc[T], opts ...FuncOption) {
	p.Cron(name, schedule, func(ctx *Context) (any, error) {
		var payload T
		if ctx.Payload != nil {
			if err := json.Unmarshal(ctx.Payload, &payload); err != nil {
				return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
			}
		}
		return handler(ctx, payload)
	}, opts...)
}

// Register registers all functions with the Pingback platform.
// Call this after defining all cron jobs and tasks, before starting the server.
func (p *Pingback) Register() {
	p.once.Do(func() {
		if p.apiKey != "" {
			p.register()
		}
	})
}

// Handler returns an http.Handler that processes execution requests.
func (p *Pingback) Handler() http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}

		sig := r.Header.Get("X-Pingback-Signature")
		ts := r.Header.Get("X-Pingback-Timestamp")
		if err := verifySignature(sig, ts, string(body), p.cronSecret); err != nil {
			http.Error(w, fmt.Sprintf("unauthorized: %v", err), http.StatusUnauthorized)
			return
		}

		var ep executionPayload
		if err := json.Unmarshal(body, &ep); err != nil {
			http.Error(w, "invalid payload", http.StatusBadRequest)
			return
		}

		fn, ok := p.functions[ep.Function]
		if !ok {
			http.Error(w, fmt.Sprintf("function %q not found", ep.Function), http.StatusNotFound)
			return
		}

		ctx := newContext(ep)
		start := time.Now()
		result, handlerErr := fn.handler(ctx)
		durationMs := time.Since(start).Milliseconds()

		resp := executionResult{
			Logs:       ctx.logs,
			Tasks:      ctx.tasks,
			DurationMs: durationMs,
		}

		w.Header().Set("Content-Type", "application/json")

		if handlerErr != nil {
			resp.Status = "error"
			resp.Error = handlerErr.Error()
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			resp.Status = "success"
			resp.Result = result
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Printf("[pingback] failed to encode response: %v", err)
		}
	})
}

// Trigger programmatically triggers a registered task by name.
func (p *Pingback) Trigger(ctx context.Context, taskName string, payload any, opts ...TriggerOption) (string, error) {
	tp := triggerPayload{Task: taskName, Payload: payload}
	for _, opt := range opts {
		opt(&tp)
	}
	body, err := json.Marshal(tp)
	if err != nil {
		return "", fmt.Errorf("failed to marshal trigger payload: %w", err)
	}

	url := p.opts.platformURL + "/api/v1/trigger"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create trigger request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("trigger request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("trigger failed (%d): %s", resp.StatusCode, string(respBody))
	}

	var tr triggerResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", fmt.Errorf("failed to decode trigger response: %w", err)
	}
	return tr.ExecutionID, nil
}
