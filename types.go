package pingback

import (
	"encoding/json"
	"time"
)

// HandlerFunc is the signature for cron and task handlers.
type HandlerFunc func(ctx *Context) (any, error)

// Option configures the Pingback client.
type Option func(*options)

type options struct {
	platformURL string
	baseURL     string
}

// WithPlatformURL sets the Pingback platform URL (default: https://api.pingback.lol).
func WithPlatformURL(url string) Option {
	return func(o *options) { o.platformURL = url }
}

// WithBaseURL sets this app's public URL for registration.
func WithBaseURL(url string) Option {
	return func(o *options) { o.baseURL = url }
}

// FuncOption configures a cron or task function.
type FuncOption func(*funcOptions)

type funcOptions struct {
	retries     int
	timeout     string
	concurrency int
}

// WithRetries sets the retry count for a function (default: 0).
func WithRetries(n int) FuncOption {
	return func(o *funcOptions) { o.retries = n }
}

// WithTimeout sets the timeout for a function (e.g. "30s", "5m").
func WithTimeout(d string) FuncOption {
	return func(o *funcOptions) { o.timeout = d }
}

// WithConcurrency sets the concurrency limit for a function (default: 1).
func WithConcurrency(n int) FuncOption {
	return func(o *funcOptions) { o.concurrency = n }
}

type functionDef struct {
	name     string
	funcType string // "cron" or "task"
	schedule string // cron expression, empty for tasks
	handler  HandlerFunc
	options  funcOptions
}

// executionPayload is the JSON body the platform sends to execute a function.
type executionPayload struct {
	Function    string          `json:"function"`
	ExecutionID string          `json:"executionId"`
	Attempt     int             `json:"attempt"`
	ScheduledAt string          `json:"scheduledAt"`
	Payload     json.RawMessage `json:"payload,omitempty"`
}

// logEntry is a structured log emitted during execution.
type logEntry struct {
	Timestamp int64  `json:"timestamp"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	Meta      any    `json:"meta,omitempty"`
}

// taskRequest is a fan-out task collected during execution.
type taskRequest struct {
	Name    string `json:"name"`
	Payload any    `json:"payload"`
}

// executionResult is the JSON response returned to the platform.
type executionResult struct {
	Status     string        `json:"status"`
	Result     any           `json:"result,omitempty"`
	Error      string        `json:"error,omitempty"`
	Logs       []logEntry    `json:"logs"`
	Tasks      []taskRequest `json:"tasks"`
	DurationMs int64         `json:"durationMs"`
}

// registerPayload is the JSON body sent to register functions.
type registerPayload struct {
	Functions   []registerFunc `json:"functions"`
	EndpointURL string         `json:"endpoint_url,omitempty"`
}

type registerFunc struct {
	Name     string          `json:"name"`
	Type     string          `json:"type"`
	Schedule string          `json:"schedule,omitempty"`
	Options  registerOptions `json:"options"`
}

type registerOptions struct {
	Retries     int    `json:"retries,omitempty"`
	Timeout     string `json:"timeout,omitempty"`
	Concurrency int    `json:"concurrency,omitempty"`
}

// triggerPayload is the JSON body sent to trigger a task.
type triggerPayload struct {
	Task    string `json:"task"`
	Payload any    `json:"payload,omitempty"`
}

// triggerResponse is the JSON response from the trigger endpoint.
type triggerResponse struct {
	ExecutionID string `json:"executionId"`
	Task        string `json:"task"`
}

// Context holds per-execution state passed to handlers.
type Context struct {
	ExecutionID string
	Attempt     int
	ScheduledAt time.Time
	Payload     json.RawMessage

	logs  []logEntry
	tasks []taskRequest
}
