package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	pingback "github.com/runpingback/pingback-go"
)

func main() {
	apiKey := os.Getenv("PINGBACK_API_KEY")
	cronSecret := os.Getenv("PINGBACK_CRON_SECRET")
	baseURL := os.Getenv("BASE_URL") // e.g. https://your-app.com

	if apiKey == "" || cronSecret == "" {
		log.Fatal("PINGBACK_API_KEY and PINGBACK_CRON_SECRET are required")
	}

	opts := []pingback.Option{}
	if baseURL != "" {
		opts = append(opts, pingback.WithBaseURL(baseURL))
	}

	pb := pingback.New(apiKey, cronSecret, opts...)

	// Cron: runs every minute
	pb.Cron("health-check", "* * * * *", func(ctx *pingback.Context) (any, error) {
		ctx.Log("Health check started")
		ctx.Log("All systems operational", "timestamp", time.Now().Unix())
		return map[string]string{"status": "healthy"}, nil
	})

	// Cron: daily cleanup at 3 AM
	pb.Cron("daily-cleanup", "0 3 * * *", func(ctx *pingback.Context) (any, error) {
		ctx.Log("Starting cleanup")
		// Simulate cleanup work
		removed := 42
		ctx.Log("Cleanup complete", "removed", removed)
		return map[string]int{"removed": removed}, nil
	}, pingback.WithRetries(2), pingback.WithTimeout("60s"))

	// Cron: fan-out email sending
	pb.Cron("send-emails", "*/15 * * * *", func(ctx *pingback.Context) (any, error) {
		// Simulate finding pending emails
		emails := []string{"user1@example.com", "user2@example.com", "user3@example.com"}
		for _, email := range emails {
			ctx.Task("send-email", map[string]string{"to": email})
		}
		ctx.Log("Dispatched emails", "count", len(emails))
		return map[string]int{"dispatched": len(emails)}, nil
	})

	// Task: send a single email (triggered via fan-out or programmatically)
	pb.Task("send-email", func(ctx *pingback.Context) (any, error) {
		var payload struct {
			To string `json:"to"`
		}
		if err := json.Unmarshal(ctx.Payload, &payload); err != nil {
			ctx.Error("Failed to parse payload", "error", err.Error())
			return nil, err
		}
		ctx.Log("Sending email", "to", payload.To)
		// Simulate sending
		time.Sleep(100 * time.Millisecond)
		ctx.Log("Email sent", "to", payload.To)
		return map[string]string{"sent": payload.To}, nil
	}, pingback.WithRetries(3), pingback.WithTimeout("15s"))

	// Task: process a webhook
	pb.Task("process-webhook", func(ctx *pingback.Context) (any, error) {
		ctx.Log("Processing webhook", "executionId", ctx.ExecutionID)
		ctx.Debug("Raw payload", "bytes", len(ctx.Payload))
		return map[string]bool{"processed": true}, nil
	}, pingback.WithTimeout("30s"))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.Handle("/api/pingback", pb.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Pingback Go Example - running on port %s", port)
	})

	log.Printf("Starting server on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
