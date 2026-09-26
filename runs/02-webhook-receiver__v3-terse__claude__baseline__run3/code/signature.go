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

// timestampTolerance bounds how old (or how far in the future) a signed
// timestamp may be. Signatures cover the timestamp, so without this a
// captured request could be replayed forever.
const timestampTolerance = 5 * time.Minute

var errBadSignature = errors.New("invalid signature")

func sign(secret string, t int64, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(t, 10)))
	mac.Write([]byte("."))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// verifySignature checks a `t=<unix>,v1=<hex>` header against the raw body.
func verifySignature(secret, header string, body []byte, now time.Time) error {
	var (
		ts    int64
		haveT bool
		sigs  []string
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
			ts, haveT = n, true
		case "v1":
			sigs = append(sigs, v)
		}
	}
	if !haveT || len(sigs) == 0 {
		return errBadSignature
	}

	expected := []byte(sign(secret, ts, body))
	matched := false
	for _, s := range sigs {
		if hmac.Equal(expected, []byte(s)) {
			matched = true
		}
	}
	if !matched {
		return errBadSignature
	}

	skew := now.Sub(time.Unix(ts, 0))
	if skew < 0 {
		skew = -skew
	}
	if skew > timestampTolerance {
		return errors.New("timestamp outside tolerance")
	}
	return nil
}
