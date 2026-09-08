// Package webhooks dispatches events to customer endpoints (T063):
// HMAC-SHA256 signature + timestamp + event id, retry with backoff+jitter,
// dead-letter queue after max retries. Consumers dedupe on event_id
// (Grilling §5.5 — documented in docs/).
package webhooks

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
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

// Dispatch delivers with retry+backoff+jitter and a dead-letter queue.
// AllowPrivate lifts the SSRF private-net block for tests/dev only —
// production must leave it false (default).
//
// Delivery itself is ASYNC via DispatchAsync + Run: rule evaluation (and
// therefore ingestion) must never wait on a downstream endpoint (ADV-0006).
// The queue is bounded; overflow dead-letters immediately instead of
// blocking the caller.
type Dispatcher struct {
	Client       *http.Client
	MaxRetries   int
	BaseDelay    time.Duration
	AllowPrivate bool
	QueueSize    int

	mu         sync.Mutex
	Delivered  []Delivery
	DeadLetter []Delivery
	queue      chan DispatchJob
	running    bool
}

// DispatchJob is one queued delivery.
type DispatchJob struct {
	URL    string
	Secret string
	Event  events.Event
}

func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		Client:     &http.Client{Timeout: 5 * time.Second},
		MaxRetries: 5,
		BaseDelay:  200 * time.Millisecond,
		QueueSize:  1000,
		running:    true,
	}
}

func (d *Dispatcher) ensureQueue() chan DispatchJob {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.queue == nil {
		d.queue = make(chan DispatchJob, d.queueSize())
	}
	return d.queue
}

// DispatchAsync enqueues one delivery without blocking. Full queue or
// stopped workers dead-letter immediately — the caller never waits.
func (d *Dispatcher) DispatchAsync(url, secret string, e events.Event) error {
	d.mu.Lock()
	running := d.running
	d.mu.Unlock()
	if !running {
		d.record(false, url, e.ID, 0, "webhook: workers not running")
		return fmt.Errorf("webhook: workers not running")
	}
	select {
	case d.ensureQueue() <- DispatchJob{URL: url, Secret: secret, Event: e}:
		return nil
	default:
		d.record(false, url, e.ID, 0, "webhook: queue full")
		return fmt.Errorf("webhook: queue full")
	}
}

func (d *Dispatcher) queueSize() int {
	if d.QueueSize > 0 {
		return d.QueueSize
	}
	return 1000
}

// Run starts workers draining the queue until ctx ends. Remaining queued
// jobs are dead-lettered on shutdown — visible, never silently dropped.
func (d *Dispatcher) Run(ctx context.Context, workers int) {
	d.mu.Lock()
	if d.queue == nil {
		d.queue = make(chan DispatchJob, d.queueSize())
	}
	d.running = true
	d.mu.Unlock()
	if workers <= 0 {
		workers = 2
	}
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case job, ok := <-d.queue:
					if !ok {
						return
					}
					_ = d.Dispatch(job.URL, job.Secret, job.Event)
				}
			}
		}()
	}
	<-ctx.Done()
	wg.Wait()
	d.mu.Lock()
	var leftovers []DispatchJob
	for {
		select {
		case job := <-d.queue:
			leftovers = append(leftovers, job)
		default:
			d.running = false
			d.mu.Unlock()
			for _, job := range leftovers {
				d.record(false, job.URL, job.Event.ID, 0, "webhook: shutdown with job queued")
			}
			return
		}
	}
}
// dead-letters after MaxRetries without blocking the caller beyond the
// retry budget (T063).
func (d *Dispatcher) Dispatch(url, secret string, e events.Event) error {
	body, err := json.Marshal(e)
	if err != nil {
		return err
	}
	// SSRF gate BEFORE any byte is sent: blocked targets dead-letter
	// immediately, with zero requests emitted (A01).
	if err := ValidateURL(url, d.AllowPrivate); err != nil {
		d.record(false, url, e.ID, 0, err.Error())
		return err
	}
	client := d.guardedClient(d.AllowPrivate)
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
		resp, err := client.Do(req)
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
		time.Sleep(delay + jitter(delay)) // crypto-random backoff+jitter
		delay *= 2
		if delay > 10*time.Second {
			delay = 10 * time.Second
		}
	}
	d.record(false, url, e.ID, 0, lastErr.Error())
	return fmt.Errorf("webhook: dead-lettered after %d retries: %w", d.MaxRetries, lastErr)
}

// jitter returns [0,n) from crypto randomness — backoff jitter only.
func jitter(n time.Duration) time.Duration {
	if n <= 0 {
		return 0
	}
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0
	}
	return time.Duration(v.Int64())
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
