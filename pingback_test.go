package pingback

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func signedRequest(t *testing.T, body string, secret string) *http.Request {
	t.Helper()
	ts := fmt.Sprintf("%d", time.Now().Unix())
	sig := computeHMAC(ts, body, secret)
	req := httptest.NewRequest("POST", "/api/pingback", bytes.NewBufferString(body))
	req.Header.Set("X-Pingback-Signature", sig)
	req.Header.Set("X-Pingback-Timestamp", ts)
	req.Header.Set("Content-Type", "application/json")
	return req
}

func TestHandler_Success(t *testing.T) {
	pb := New("key", "secret")
	pb.Cron("cleanup", "0 3 * * *", func(ctx *Context) (any, error) {
		ctx.Log("cleaned up", "count", 42)
		return map[string]int{"removed": 42}, nil
	})

	body := `{"function":"cleanup","executionId":"exec-1","attempt":1,"scheduledAt":"2026-04-22T03:00:00Z"}`
	req := signedRequest(t, body, "secret")
	rec := httptest.NewRecorder()
	pb.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp executionResult
	json.NewDecoder(rec.Body).Decode(&resp)

	if resp.Status != "success" {
		t.Fatalf("expected success, got %s", resp.Status)
	}
	if len(resp.Logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(resp.Logs))
	}
	if resp.Logs[0].Level != "info" {
		t.Fatalf("expected info level, got %s", resp.Logs[0].Level)
	}
}

func TestHandler_UnknownFunction(t *testing.T) {
	pb := New("key", "secret")

	body := `{"function":"nonexistent","executionId":"exec-1","attempt":1,"scheduledAt":"2026-04-22T03:00:00Z"}`
	req := signedRequest(t, body, "secret")
	rec := httptest.NewRecorder()
	pb.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestHandler_InvalidSignature(t *testing.T) {
	pb := New("key", "secret")
	pb.Task("job", func(ctx *Context) (any, error) { return nil, nil })

	body := `{"function":"job","executionId":"exec-1","attempt":1,"scheduledAt":"2026-04-22T03:00:00Z"}`
	req := signedRequest(t, body, "wrong-secret")
	rec := httptest.NewRecorder()
	pb.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestHandler_Error(t *testing.T) {
	pb := New("key", "secret")
	pb.Task("fail", func(ctx *Context) (any, error) {
		return nil, fmt.Errorf("something broke")
	})

	body := `{"function":"fail","executionId":"exec-1","attempt":1,"scheduledAt":"2026-04-22T03:00:00Z"}`
	req := signedRequest(t, body, "secret")
	rec := httptest.NewRecorder()
	pb.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}

	var resp executionResult
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Status != "error" {
		t.Fatalf("expected error status, got %s", resp.Status)
	}
	if resp.Error != "something broke" {
		t.Fatalf("expected 'something broke', got %s", resp.Error)
	}
}

func TestHandler_FanOut(t *testing.T) {
	pb := New("key", "secret")
	pb.Cron("parent", "* * * * *", func(ctx *Context) (any, error) {
		ctx.Task("child-a", map[string]string{"id": "1"})
		ctx.Task("child-b", map[string]string{"id": "2"})
		return nil, nil
	})

	body := `{"function":"parent","executionId":"exec-1","attempt":1,"scheduledAt":"2026-04-22T03:00:00Z"}`
	req := signedRequest(t, body, "secret")
	rec := httptest.NewRecorder()
	pb.Handler().ServeHTTP(rec, req)

	var resp executionResult
	json.NewDecoder(rec.Body).Decode(&resp)
	if len(resp.Tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(resp.Tasks))
	}
	if resp.Tasks[0].Name != "child-a" {
		t.Fatalf("expected child-a, got %s", resp.Tasks[0].Name)
	}
}

func TestHandler_Payload(t *testing.T) {
	pb := New("key", "secret")
	pb.Task("echo", func(ctx *Context) (any, error) {
		var p map[string]string
		json.Unmarshal(ctx.Payload, &p)
		return p, nil
	})

	body := `{"function":"echo","executionId":"exec-1","attempt":1,"scheduledAt":"2026-04-22T03:00:00Z","payload":{"msg":"hello"}}`
	req := signedRequest(t, body, "secret")
	rec := httptest.NewRecorder()
	pb.Handler().ServeHTTP(rec, req)

	var resp executionResult
	json.NewDecoder(rec.Body).Decode(&resp)
	result, _ := json.Marshal(resp.Result)
	if string(result) != `{"msg":"hello"}` {
		t.Fatalf("expected payload echo, got %s", string(result))
	}
}

func TestTrigger_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatal("missing auth header")
		}
		var tp triggerPayload
		json.NewDecoder(r.Body).Decode(&tp)
		if tp.Task != "send-email" {
			t.Fatalf("expected send-email, got %s", tp.Task)
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(triggerResponse{ExecutionID: "exec-123", Task: "send-email"})
	}))
	defer server.Close()

	pb := New("test-key", "secret", WithPlatformURL(server.URL))
	execID, err := pb.Trigger(context.Background(), "send-email", map[string]string{"to": "a@b.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if execID != "exec-123" {
		t.Fatalf("expected exec-123, got %s", execID)
	}
}

func TestTrigger_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `Task "nope" not found`)
	}))
	defer server.Close()

	pb := New("test-key", "secret", WithPlatformURL(server.URL))
	_, err := pb.Trigger(context.Background(), "nope", nil)
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

func TestRegister(t *testing.T) {
	var received registerPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatal("missing auth header")
		}
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"jobs": []map[string]string{{"name": "cleanup", "status": "active"}}})
	}))
	defer server.Close()

	pb := New("test-key", "secret", WithPlatformURL(server.URL))
	pb.Cron("cleanup", "0 3 * * *", func(ctx *Context) (any, error) { return nil, nil }, WithRetries(2))
	pb.Task("send-email", func(ctx *Context) (any, error) { return nil, nil }, WithTimeout("15s"))

	// Call register directly for testing (Handler() runs it async)
	pb.register()

	if len(received.Functions) != 2 {
		t.Fatalf("expected 2 functions, got %d", len(received.Functions))
	}
}

func TestTaskWith_TypedPayload(t *testing.T) {
	type EmailPayload struct {
		To      string `json:"to"`
		Subject string `json:"subject"`
	}

	pb := New("key", "secret")
	TaskWith(pb, "send-email", func(ctx *Context, payload EmailPayload) (any, error) {
		ctx.Log("Sending", "to", payload.To)
		return map[string]string{"to": payload.To, "subject": payload.Subject}, nil
	}, WithRetries(2))

	body := `{"function":"send-email","executionId":"exec-1","attempt":1,"scheduledAt":"2026-04-22T03:00:00Z","payload":{"to":"alice@example.com","subject":"Hello"}}`
	req := signedRequest(t, body, "secret")
	rec := httptest.NewRecorder()
	pb.Handler().ServeHTTP(rec, req)

	var resp executionResult
	json.NewDecoder(rec.Body).Decode(&resp)

	if resp.Status != "success" {
		t.Fatalf("expected success, got %s: %s", resp.Status, resp.Error)
	}

	result, _ := json.Marshal(resp.Result)
	if string(result) != `{"subject":"Hello","to":"alice@example.com"}` {
		t.Fatalf("unexpected result: %s", string(result))
	}
}

func TestTaskWith_InvalidPayload(t *testing.T) {
	type Payload struct {
		Count int `json:"count"`
	}

	pb := New("key", "secret")
	TaskWith(pb, "job", func(ctx *Context, payload Payload) (any, error) {
		return payload.Count, nil
	})

	body := `{"function":"job","executionId":"exec-1","attempt":1,"scheduledAt":"2026-04-22T03:00:00Z","payload":"not-json-object"}`
	req := signedRequest(t, body, "secret")
	rec := httptest.NewRecorder()
	pb.Handler().ServeHTTP(rec, req)

	var resp executionResult
	json.NewDecoder(rec.Body).Decode(&resp)

	if resp.Status != "error" {
		t.Fatalf("expected error for invalid payload, got %s", resp.Status)
	}
}

func TestTrigger_WithDelay(t *testing.T) {
	var received triggerPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(triggerResponse{
			ExecutionID: "exec-456",
			Task:        "send-email",
			ScheduledAt: "2026-04-27T15:15:00.000Z",
		})
	}))
	defer server.Close()

	pb := New("test-key", "secret", WithPlatformURL(server.URL))
	execID, err := pb.Trigger(context.Background(), "send-email", map[string]string{"to": "a@b.com"}, WithDelay("15m"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if execID != "exec-456" {
		t.Fatalf("expected exec-456, got %s", execID)
	}
	if received.Delay == nil {
		t.Fatal("expected delay to be set")
	}
	if received.Delay != "15m" {
		t.Fatalf("expected delay '15m', got '%v'", received.Delay)
	}
}

func TestTrigger_WithIntDelay(t *testing.T) {
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(triggerResponse{
			ExecutionID: "exec-789",
			Task:        "process",
			ScheduledAt: "2026-04-27T15:15:00.000Z",
		})
	}))
	defer server.Close()

	pb := New("test-key", "secret", WithPlatformURL(server.URL))
	_, err := pb.Trigger(context.Background(), "process", nil, WithDelay(900))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	delay, ok := received["delay"]
	if !ok {
		t.Fatal("expected delay in payload")
	}
	// JSON numbers decode as float64
	if delay != float64(900) {
		t.Fatalf("expected delay 900, got %v", delay)
	}
}

func TestTrigger_WithoutDelay_OmitsField(t *testing.T) {
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(triggerResponse{ExecutionID: "exec-000", Task: "quick"})
	}))
	defer server.Close()

	pb := New("test-key", "secret", WithPlatformURL(server.URL))
	pb.Trigger(context.Background(), "quick", nil)
	if _, ok := received["delay"]; ok {
		t.Fatal("expected no delay field when not specified")
	}
}
