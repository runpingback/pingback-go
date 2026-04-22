package pingback

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"time"
)

const maxClockSkew = 5 * time.Minute

func computeHMAC(timestamp, body, secret string) string {
	message := fmt.Sprintf("%s.%s", timestamp, body)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

func verifySignature(signature, timestamp, body, secret string) error {
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid timestamp: %w", err)
	}

	age := time.Duration(math.Abs(float64(time.Now().Unix()-ts))) * time.Second
	if age > maxClockSkew {
		return fmt.Errorf("timestamp expired: %s old", age)
	}

	expected := computeHMAC(timestamp, body, secret)
	expectedBytes, _ := hex.DecodeString(expected)
	signatureBytes, err := hex.DecodeString(signature)
	if err != nil {
		return fmt.Errorf("invalid signature encoding: %w", err)
	}

	if !hmac.Equal(expectedBytes, signatureBytes) {
		return fmt.Errorf("signature mismatch")
	}

	return nil
}
