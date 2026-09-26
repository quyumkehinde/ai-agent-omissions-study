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

// signatureTolerance bounds how far the signed timestamp may be from now.
// The signature covers the timestamp, so without this a captured request
// could be replayed indefinitely.
const signatureTolerance = 5 * time.Minute

var errInvalidSignature = errors.New("invalid signature")

func computeSignature(secret string, t int64, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(t, 10)))
	mac.Write([]byte("."))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// verifySignature checks a `t=<unix>,v1=<hex>` header against the raw body.
func verifySignature(secret, header string, body []byte, now time.Time) error {
	var ts int64
	var haveT bool
	var sigs []string
	for _, part := range strings.Split(header, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch k {
		case "t":
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return errInvalidSignature
			}
			ts, haveT = n, true
		case "v1":
			sigs = append(sigs, v)
		}
	}
	if !haveT || len(sigs) == 0 {
		return errInvalidSignature
	}

	expected := []byte(computeSignature(secret, ts, body))
	matched := false
	for _, s := range sigs {
		if hmac.Equal([]byte(s), expected) {
			matched = true
		}
	}
	if !matched {
		return errInvalidSignature
	}

	age := now.Sub(time.Unix(ts, 0))
	if age > signatureTolerance || age < -signatureTolerance {
		return errors.New("timestamp outside tolerance")
	}
	return nil
}
