package main

import (
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

	// Task: send a single email — using typed payload
	type EmailPayload struct {
		To string `json:"to"`
	}
	pingback.TaskWith(pb, "send-email", func(ctx *pingback.Context, payload EmailPayload) (any, error) {
		ctx.Log("Sending email", "to", payload.To)
		time.Sleep(100 * time.Millisecond) // simulate
		ctx.Log("Email sent", "to", payload.To)
		return map[string]string{"sent": payload.To}, nil
	}, pingback.WithRetries(3), pingback.WithTimeout("15s"))

	// Task: process a webhook (untyped — uses ctx.Payload directly)
	pb.Task("process-webhook", func(ctx *pingback.Context) (any, error) {
		ctx.Log("Processing webhook", "executionId", ctx.ExecutionID)
		ctx.Debug("Raw payload", "bytes", len(ctx.Payload))
		return map[string]bool{"processed": true}, nil
	}, pingback.WithTimeout("30s"))

	// --- Workflow chain: process-orders → validate-order → charge-payment → send-confirmation ---

	type Order struct {
		OrderID string  `json:"orderId"`
		Amount  float64 `json:"amount"`
		Email   string  `json:"email"`
	}

	type Confirmation struct {
		OrderID string  `json:"orderId"`
		Email   string  `json:"email"`
		Amount  float64 `json:"amount"`
	}

	type Failure struct {
		OrderID string `json:"orderId"`
		Email   string `json:"email"`
		Reason  string `json:"reason"`
	}

	// Cron: find pending orders and kick off validation
	pb.Cron("process-orders", "*/10 * * * *", func(ctx *pingback.Context) (any, error) {
		ctx.Log("Checking for pending orders")
		orders := []Order{
			{OrderID: "ord-001", Amount: 49.99, Email: "alice@example.com"},
			{OrderID: "ord-002", Amount: 149.00, Email: "bob@example.com"},
			{OrderID: "ord-003", Amount: 0, Email: "eve@example.com"},
		}
		for _, order := range orders {
			ctx.Task("validate-order", order)
		}
		ctx.Log("Dispatched order validations", "count", len(orders))
		return map[string]int{"dispatched": len(orders)}, nil
	}, pingback.WithRetries(1))

	// Step 1: validate → charge-payment or notify-failure
	pingback.TaskWith(pb, "validate-order", func(ctx *pingback.Context, order Order) (any, error) {
		ctx.Log("Validating order", "orderId", order.OrderID)

		if order.Amount <= 0 {
			ctx.Warn("Invalid order amount", "orderId", order.OrderID, "amount", order.Amount)
			ctx.Task("notify-failure", Failure{OrderID: order.OrderID, Email: order.Email, Reason: "Invalid amount"})
			return map[string]any{"valid": false, "orderId": order.OrderID}, nil
		}

		ctx.Log("Order valid, proceeding to payment", "orderId", order.OrderID)
		ctx.Task("charge-payment", order)
		return map[string]any{"valid": true, "orderId": order.OrderID}, nil
	}, pingback.WithRetries(2), pingback.WithTimeout("15s"))

	// Step 2: charge → send-confirmation
	pingback.TaskWith(pb, "charge-payment", func(ctx *pingback.Context, p Order) (any, error) {
		ctx.Log("Charging payment", "orderId", p.OrderID, "amount", p.Amount)
		time.Sleep(50 * time.Millisecond) // simulate
		ctx.Log("Payment charged", "orderId", p.OrderID)
		ctx.Task("send-confirmation", Confirmation{OrderID: p.OrderID, Email: p.Email, Amount: p.Amount})
		return map[string]any{"charged": true, "orderId": p.OrderID}, nil
	}, pingback.WithRetries(3), pingback.WithTimeout("30s"))

	// Step 3: send confirmation (end of chain)
	pingback.TaskWith(pb, "send-confirmation", func(ctx *pingback.Context, p Confirmation) (any, error) {
		ctx.Log("Sending confirmation", "orderId", p.OrderID, "email", p.Email)
		time.Sleep(20 * time.Millisecond) // simulate
		ctx.Log("Confirmation sent", "orderId", p.OrderID)
		return map[string]any{"sent": true, "orderId": p.OrderID}, nil
	}, pingback.WithRetries(2), pingback.WithTimeout("15s"))

	// Failure branch
	pingback.TaskWith(pb, "notify-failure", func(ctx *pingback.Context, p Failure) (any, error) {
		ctx.Error("Order failed, notifying customer", "orderId", p.OrderID, "reason", p.Reason)
		time.Sleep(20 * time.Millisecond) // simulate
		ctx.Log("Failure notification sent", "orderId", p.OrderID)
		return map[string]any{"notified": true, "orderId": p.OrderID}, nil
	}, pingback.WithRetries(2), pingback.WithTimeout("15s"))

	// Register all functions with the platform
	pb.Register()

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
