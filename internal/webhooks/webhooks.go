// Package webhooks dispatches events to customer endpoints (T063):
// HMAC-SHA256 signature + timestamp + event id, retry with backoff+jitter,
// dead-letter queue after max retries. Consumers dedupe on event_id
// (Grilling §5.5 — documented in docs/).
package webhooks

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/tide-telematics/tide/schemas/events"
)

const (
	HeaderEventID   = "X-Tide-Event-Id"
	HeaderTimestamp = "X-Tide-Timestamp"
	HeaderSignature = "X-Tide-Signature"
)

// Sign builds the replay-resistant signature: HMAC(secret, timestamp + "." + eventID + "." + body).
func Sign(secret, eventID string, ts time.Time, body []byte) (sig string, timestamp string) {
	timestamp = strconv.FormatInt(ts.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "." + eventID + "." + string(body)))
	return hex.EncodeToString(mac.Sum(nil)), timestamp
}

// Verify checks a received webhook (consumer guidance, Grilling §5.5).
// Rejects timestamps older than 5 minutes (replay resistance).
func Verify(secret string, r *http.Request, body []byte) error {
	id := r.Header.Get(HeaderEventID)
	ts := r.Header.Get(HeaderTimestamp)
	got := r.Header.Get(HeaderSignature)
	if id == "" || ts == "" || got == "" {
		return fmt.Errorf("webhook: missing signature headers")
	}
	unix, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return fmt.Errorf("webhook: bad timestamp")
	}
	if time.Since(time.Unix(unix, 0)).Abs() > 5*time.Minute {
		return fmt.Errorf("webhook: stale timestamp (replay?)")
	}
	want, _ := Sign(secret, id, time.Unix(unix, 0), body)
	if !hmac.Equal([]byte(want), []byte(got)) {
		return fmt.Errorf("webhook: bad signature")
	}
	return nil
}

type Delivery struct {
	EventID string
	URL     string
	Status  int
	Err     string
	At      time.Time
}

// Dispatcher delivers with retry+backoff+jitter and a dead-letter queue.
type Dispatcher struct {
	Client     *http.Client
	MaxRetries int
	BaseDelay  time.Duration

	mu       sync.Mutex
	Delivered []Delivery
	DeadLetter []Delivery
}

func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		Client:     &http.Client{Timeout: 5 * time.Second},
		MaxRetries: 5,
		BaseDelay:  200 * time.Millisecond,
	}
}

// Dispatch delivers one event to url signed with secret. A failing endpoint
// dead-letters after MaxRetries without blocking the caller beyond the
// retry budget (T063).
func (d *Dispatcher) Dispatch(url, secret string, e events.Event) error {
	body, err := json.Marshal(e)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	sig, ts := Sign(secret, e.ID, now, body)
	delay := d.BaseDelay
	var lastErr error
	for attempt := 0; attempt <= d.MaxRetries; attempt++ {
		req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			lastErr = err
			break
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(HeaderEventID, e.ID)
		req.Header.Set(HeaderTimestamp, ts)
		req.Header.Set(HeaderSignature, sig)
		resp, err := d.Client.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode < 500 && resp.StatusCode != 429 {
				if resp.StatusCode >= 400 {
					d.record(false, url, e.ID, resp.StatusCode, fmt.Sprintf("client error %d", resp.StatusCode))
					return fmt.Errorf("webhook: endpoint returned %d (dead-lettered, no retry)", resp.StatusCode)
				}
				d.record(true, url, e.ID, resp.StatusCode, "")
				return nil
			}
			lastErr = fmt.Errorf("endpoint returned %d", resp.StatusCode)
		} else {
			lastErr = err
		}
		time.Sleep(delay + time.Duration(rand.Int63n(int64(delay)))) // backoff+jitter
		delay *= 2
		if delay > 10*time.Second {
			delay = 10 * time.Second
		}
	}
	d.record(false, url, e.ID, 0, lastErr.Error())
	return fmt.Errorf("webhook: dead-lettered after %d retries: %w", d.MaxRetries, lastErr)
}

func (d *Dispatcher) record(ok bool, url, id string, status int, errStr string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	del := Delivery{EventID: id, URL: url, Status: status, Err: errStr, At: time.Now()}
	if ok {
		d.Delivered = append(d.Delivered, del)
	} else {
		d.DeadLetter = append(d.DeadLetter, del)
	}
}
