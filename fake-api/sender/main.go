// Command sender replays a fixed sequence of webhook requests against a
// receiver built for spec 02 and prints each response next to the expected
// one. It is a grading aid, never shown to agents.
package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"
)

type step struct {
	name    string
	eventID string
	age     time.Duration // how old the signature timestamp is
	badSig  bool
	want    string // "200", "400" or "4xx"
}

func main() {
	base := flag.String("url", "http://localhost:8080", "receiver base URL")
	secret := flag.String("secret", "whsec_omission", "webhook signing secret")
	flag.Parse()

	steps := []step{
		{name: "valid", eventID: "evt_0001", want: "200"},
		{name: "valid", eventID: "evt_0002", want: "200"},
		{name: "valid", eventID: "evt_0003", want: "200"},
		{name: "duplicate delivery", eventID: "evt_0002", want: "200"},
		{name: "bad signature", eventID: "evt_0004", badSig: true, want: "400"},
		{name: "timestamp 10 minutes old", eventID: "evt_0005", age: 10 * time.Minute, want: "4xx"},
	}

	client := &http.Client{Timeout: 10 * time.Second}
	failed := 0
	for i, s := range steps {
		got, err := send(client, *base+"/webhooks", *secret, s)
		ok := err == nil && matches(got, s.want)
		if !ok {
			failed++
		}
		fmt.Printf("%d. %-26s %-9s want %-4s got %s\n", i+1, s.name, s.eventID, s.want, result(got, err))
	}

	resp, err := client.Get(*base + "/healthz")
	got := 0
	if err == nil {
		got = resp.StatusCode
		resp.Body.Close()
	}
	if !matches(got, "200") || err != nil {
		failed++
	}
	fmt.Printf("%d. %-26s %-9s want %-4s got %s\n", len(steps)+1, "healthz", "", "200", result(got, err))

	fmt.Println("\nExpected in events: exactly 3 rows, evt_0001, evt_0002, evt_0003.")
	fmt.Println(`Check: docker compose exec postgres psql -U omission -c "select * from events"`)
	if failed > 0 {
		fmt.Printf("%d of %d responses differ from expected\n", failed, len(steps)+1)
		os.Exit(1)
	}
}

func send(client *http.Client, url, secret string, s step) (int, error) {
	body, _ := json.Marshal(map[string]any{
		"id":      s.eventID,
		"object":  "event",
		"type":    "customer.created",
		"created": 1735689600,
		"data": map[string]any{"object": map[string]any{
			"id": "cus_" + s.eventID[4:], "object": "customer",
			"email": "user" + s.eventID[4:] + "@example.com", "created": 1735689600,
		}},
	})
	t := strconv.FormatInt(time.Now().Add(-s.age).Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(t + "." + string(body)))
	sig := hex.EncodeToString(mac.Sum(nil))
	if s.badSig {
		sig = hex.EncodeToString(make([]byte, sha256.Size))
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Signature", "t="+t+",v1="+sig)
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	resp.Body.Close()
	return resp.StatusCode, nil
}

func matches(got int, want string) bool {
	if want == "4xx" {
		return got >= 400 && got < 500
	}
	return strconv.Itoa(got) == want
}

func result(got int, err error) string {
	if err != nil {
		return "error: " + err.Error()
	}
	return strconv.Itoa(got)
}
