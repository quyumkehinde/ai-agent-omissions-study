package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"
)

var errBadSignature = errors.New("invalid signature")

// sign returns the v1 signature for a body signed at t.
func sign(secret string, t int64, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(t, 10)))
	mac.Write([]byte("."))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// verifySignature checks a `t=<unix>,v1=<hex>` header against the raw body.
// Timestamps further than tolerance from now are rejected to limit replays.
func verifySignature(secret, header string, body []byte, now time.Time, tolerance time.Duration) error {
	var (
		t    int64
		hasT bool
		v1s  []string
	)
	for _, part := range strings.Split(header, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch k {
		case "t":
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return errBadSignature
			}
			t, hasT = n, true
		case "v1":
			v1s = append(v1s, v)
		}
	}
	if !hasT || len(v1s) == 0 {
		return errBadSignature
	}

	expected := []byte(sign(secret, t, body))
	match := false
	for _, v := range v1s {
		if hmac.Equal([]byte(v), expected) {
			match = true
		}
	}
	if !match {
		return errBadSignature
	}

	diff := now.Sub(time.Unix(t, 0))
	if diff < 0 {
		diff = -diff
	}
	if diff > tolerance {
		return errors.New("timestamp outside tolerance")
	}
	return nil
}
