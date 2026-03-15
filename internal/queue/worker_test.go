package queue

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"c0de-webhook/internal/mail"
	"c0de-webhook/internal/store"
)

// --- mock senders ---

type sentMessage struct {
	to, subject, text, html string
}

type mockSender struct {
	mu         sync.Mutex
	sent       []sentMessage
	failNext   bool
	failErr    error
	alwaysFail bool
}

var _ mail.Sender = (*mockSender)(nil)

func (m *mockSender) Send(to, subject, text, html string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.alwaysFail {
		return m.failErr
	}
	if m.failNext {
		m.failNext = false
		return m.failErr
	}
	m.sent = append(m.sent, sentMessage{to, subject, text, html})
	return nil
}

func (m *mockSender) getSent() []sentMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]sentMessage, len(m.sent))
	copy(cp, m.sent)
	return cp
}

// blockingMockSender calls onSend during Send, allowing tests to cancel
// the context while a message is in flight.
type blockingMockSender struct {
	mu     sync.Mutex
	called bool
	onSend func()
}

var _ mail.Sender = (*blockingMockSender)(nil)

func (b *blockingMockSender) Send(_, _, _, _ string) error {
	b.mu.Lock()
	b.called = true
	b.mu.Unlock()
	if b.onSend != nil {
		b.onSend()
	}
	return nil
}

// closingMockSender closes the store inside Send so that the subsequent
// MarkSent call in the worker will fail, covering that error branch.
type closingMockSender struct {
	mu     sync.Mutex
	called bool
	store  *store.Store
}

var _ mail.Sender = (*closingMockSender)(nil)

func (c *closingMockSender) Send(_, _, _, _ string) error {
	c.mu.Lock()
	c.called = true
	c.mu.Unlock()
	c.store.Close()
	return nil
}

// --- helpers ---

// newTestStore creates an in-memory SQLite store with shared cache so that
// all connections in the sql.DB pool share the same database tables.
// Plain ":memory:" gives each pooled connection its own empty database.
func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	st, err := store.New(dsn)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// pollUntil repeatedly calls check every interval until it returns true or
// the deadline is reached.
func pollUntil(deadline, interval time.Duration, check func() bool) bool {
	timer := time.NewTimer(deadline)
	defer timer.Stop()
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		if check() {
			return true
		}
		select {
		case <-timer.C:
			return check()
		case <-tick.C:
		}
	}
}

// --- tests ---

func TestNewWorker(t *testing.T) {
	st := newTestStore(t)
	defer st.Close()
	ms := &mockSender{}
	w := NewWorker(st, ms, 4, 500*time.Millisecond)

	if w.store != st {
		t.Error("store not set correctly")
	}
	if w.sender != ms {
		t.Error("sender not set correctly")
	}
	if w.workers != 4 {
		t.Errorf("workers: got %d, want 4", w.workers)
	}
	if w.retryDelay != 500*time.Millisecond {
		t.Errorf("retryDelay: got %v, want 500ms", w.retryDelay)
	}
	if w.ch == nil {
		t.Error("channel should be initialised")
	}
	if cap(w.ch) != 8 {
		t.Errorf("channel capacity: got %d, want 8 (workers*2)", cap(w.ch))
	}
}

func TestWorkerProcessesMessages(t *testing.T) {
	st := newTestStore(t)
	ms := &mockSender{}
	w := NewWorker(st, ms, 2, 10*time.Millisecond)

	for i := 0; i < 3; i++ {
		_, err := st.EnqueueMessage(nil,
			"user@example.com",
			fmt.Sprintf("Subject %c", 'A'+i),
			"text body", "html body", 3)
		if err != nil {
			t.Fatalf("enqueue %d: %v", i, err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)

	ok := pollUntil(15*time.Second, 100*time.Millisecond, func() bool {
		return len(ms.getSent()) >= 3
	})

	cancel()
	w.Stop()

	if !ok {
		t.Fatalf("expected 3 sent messages, got %d", len(ms.getSent()))
	}

	// Verify store status
	msgs, _, err := st.ListMessages("sent", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 3 {
		t.Errorf("expected 3 sent in store, got %d", len(msgs))
	}

	// Verify content
	for _, s := range ms.getSent() {
		if s.to != "user@example.com" {
			t.Errorf("unexpected to: %s", s.to)
		}
		if s.text != "text body" {
			t.Errorf("unexpected text: %s", s.text)
		}
		if s.html != "html body" {
			t.Errorf("unexpected html: %s", s.html)
		}
	}

	st.Close()
}

func TestWorkerRetryOnFailure(t *testing.T) {
	st := newTestStore(t)
	ms := &mockSender{
		failNext: true,
		failErr:  errors.New("temporary error"),
	}
	w := NewWorker(st, ms, 1, 1*time.Millisecond)

	_, err := st.EnqueueMessage(nil,
		"retry@example.com", "Retry Subject",
		"retry text", "retry html", 3)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)

	ok := pollUntil(15*time.Second, 100*time.Millisecond, func() bool {
		return len(ms.getSent()) >= 1
	})

	cancel()
	w.Stop()

	if !ok {
		t.Fatal("expected message to be sent after retry")
	}

	msgs, _, err := st.ListMessages("sent", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Errorf("expected 1 sent in store, got %d", len(msgs))
	}

	st.Close()
}

func TestWorkerMaxRetries(t *testing.T) {
	st := newTestStore(t)
	ms := &mockSender{
		alwaysFail: true,
		failErr:    errors.New("permanent error"),
	}
	w := NewWorker(st, ms, 1, 1*time.Millisecond)

	_, err := st.EnqueueMessage(nil,
		"fail@example.com", "Fail Subject",
		"fail text", "fail html", 2)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)

	ok := pollUntil(30*time.Second, 200*time.Millisecond, func() bool {
		msgs, _, err := st.ListMessages("failed", 10, 0)
		return err == nil && len(msgs) == 1
	})

	cancel()
	w.Stop()

	if !ok {
		t.Fatal("expected message to be marked failed after max retries")
	}

	msgs, _, err := st.ListMessages("failed", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if msgs[0].LastError != "permanent error" {
		t.Errorf("unexpected last error: %q", msgs[0].LastError)
	}
	if msgs[0].Attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", msgs[0].Attempts)
	}
	if len(ms.getSent()) != 0 {
		t.Errorf("expected 0 sent messages, got %d", len(ms.getSent()))
	}

	st.Close()
}

func TestWorkerGracefulShutdown(t *testing.T) {
	st := newTestStore(t)
	ms := &mockSender{}
	w := NewWorker(st, ms, 2, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)

	cancel()

	done := make(chan struct{})
	go func() {
		w.Stop()
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(5 * time.Second):
		t.Fatal("Stop() did not return within 5 seconds")
	}

	st.Close()
}

func TestWorkerResetStuckMessages(t *testing.T) {
	st := newTestStore(t)
	ms := &mockSender{}

	msg, err := st.EnqueueMessage(nil,
		"stuck@example.com", "Stuck Subject",
		"stuck text", "stuck html", 3)
	if err != nil {
		t.Fatal(err)
	}

	// Move message to 'sending' to simulate a crash-interrupted state.
	claimed, err := st.ClaimPendingMessages(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 1 || claimed[0].ID != msg.ID {
		t.Fatalf("expected to claim message %d, got %v", msg.ID, claimed)
	}

	// Verify it is now in 'sending' state.
	sendingMsgs, _, err := st.ListMessages("sending", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(sendingMsgs) != 1 {
		t.Fatalf("expected 1 sending message, got %d", len(sendingMsgs))
	}

	// Start() calls ResetStuckMessages, moving it back to 'queued'.
	w := NewWorker(st, ms, 1, 10*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)

	ok := pollUntil(10*time.Second, 100*time.Millisecond, func() bool {
		return len(ms.getSent()) >= 1
	})

	cancel()
	w.Stop()

	if !ok {
		t.Fatal("expected stuck message to be reset and then sent")
	}

	sentMsgs, _, err := st.ListMessages("sent", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(sentMsgs) != 1 {
		t.Errorf("expected 1 sent message, got %d", len(sentMsgs))
	}

	sent := ms.getSent()
	if sent[0].to != "stuck@example.com" {
		t.Errorf("unexpected to: %s", sent[0].to)
	}

	st.Close()
}

func TestWorkerNoMessagesNoError(t *testing.T) {
	st := newTestStore(t)
	ms := &mockSender{}
	w := NewWorker(st, ms, 1, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)

	// Let a couple of dispatch cycles run (ticker is 2s).
	time.Sleep(3 * time.Second)

	cancel()
	w.Stop()
	st.Close()

	if len(ms.getSent()) != 0 {
		t.Errorf("expected 0 sent messages, got %d", len(ms.getSent()))
	}
}

func TestWorkerMultipleWorkers(t *testing.T) {
	st := newTestStore(t)
	ms := &mockSender{}
	w := NewWorker(st, ms, 4, 10*time.Millisecond)

	for i := 0; i < 10; i++ {
		_, err := st.EnqueueMessage(nil,
			"multi@example.com", "Multi Subject",
			"text", "html", 3)
		if err != nil {
			t.Fatalf("enqueue %d: %v", i, err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)

	// ClaimPendingMessages uses limit=workers (4), so multiple dispatch
	// cycles are needed to process all 10 messages.
	ok := pollUntil(20*time.Second, 200*time.Millisecond, func() bool {
		return len(ms.getSent()) >= 10
	})

	cancel()
	w.Stop()

	if !ok {
		t.Fatalf("expected 10 sent messages, got %d", len(ms.getSent()))
	}

	st.Close()
}

func TestWorkerCancelDuringProcessing(t *testing.T) {
	// Covers the ctx.Done() select case inside process() by cancelling
	// the context while the sender is executing.
	st := newTestStore(t)
	var cancelFn context.CancelFunc

	sender := &blockingMockSender{
		onSend: func() {
			// Cancel the context while Send is in progress
			cancelFn()
			// Let goroutines react
			time.Sleep(50 * time.Millisecond)
		},
	}

	w := NewWorker(st, sender, 1, 10*time.Millisecond)

	_, err := st.EnqueueMessage(nil,
		"cancel@example.com", "Cancel Subject",
		"text", "html", 3)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancelFn = cancel
	w.Start(ctx)

	done := make(chan struct{})
	go func() {
		w.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Worker stopped cleanly after context cancellation during processing.
	case <-time.After(10 * time.Second):
		t.Fatal("worker did not stop within 10 seconds")
	}

	st.Close()
}

func TestWorkerClaimErrorContinues(t *testing.T) {
	// Covers the dispatch error logging branch by closing the store after
	// the worker starts, forcing ClaimPendingMessages to fail.
	st := newTestStore(t)
	ms := &mockSender{}
	w := NewWorker(st, ms, 1, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)

	// Close the store to force ClaimPendingMessages to fail.
	st.Close()

	// Let a dispatch cycle fail.
	time.Sleep(3 * time.Second)

	cancel()

	done := make(chan struct{})
	go func() {
		w.Stop()
		close(done)
	}()

	select {
	case <-done:
		// success - worker handled claim errors gracefully
	case <-time.After(5 * time.Second):
		t.Fatal("Stop() did not return within 5 seconds")
	}
}

func TestWorkerMarkSentError(t *testing.T) {
	// Covers the MarkSent error branch by closing the store inside Send
	// so that the subsequent MarkSent call will fail.
	st := newTestStore(t)

	sender := &closingMockSender{store: st}
	w := NewWorker(st, sender, 1, 10*time.Millisecond)

	_, err := st.EnqueueMessage(nil,
		"markerr@example.com", "Mark Error",
		"text", "html", 3)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)

	// Wait for the sender to be called (it closes the store internally).
	ok := pollUntil(10*time.Second, 100*time.Millisecond, func() bool {
		sender.mu.Lock()
		defer sender.mu.Unlock()
		return sender.called
	})
	if !ok {
		t.Fatal("sender was never called")
	}

	cancel()

	done := make(chan struct{})
	go func() {
		w.Stop()
		close(done)
	}()

	select {
	case <-done:
		// success - worker handled MarkSent error gracefully
	case <-time.After(10 * time.Second):
		t.Fatal("Stop() did not return within 10 seconds")
	}
}

func TestWorkerMarkFailedError(t *testing.T) {
	// Covers the MarkFailed error branch by closing the store after
	// the sender returns an error, causing MarkFailed to fail.
	st := newTestStore(t)

	sender := &closingMockSender{store: st}
	// Override Send to fail (so MarkFailed is called) then close the store.
	failCloseSender := &failThenCloseSender{
		store:   st,
		sendErr: errors.New("send failed"),
	}

	w := NewWorker(st, failCloseSender, 1, 10*time.Millisecond)

	_, err := st.EnqueueMessage(nil,
		"markfailerr@example.com", "MarkFailed Error",
		"text", "html", 3)
	if err != nil {
		t.Fatal(err)
	}
	_ = sender // not used directly

	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)

	// Wait for the sender to be called (it closes the store internally).
	ok := pollUntil(10*time.Second, 100*time.Millisecond, func() bool {
		failCloseSender.mu.Lock()
		defer failCloseSender.mu.Unlock()
		return failCloseSender.called
	})
	if !ok {
		t.Fatal("sender was never called")
	}

	cancel()

	done := make(chan struct{})
	go func() {
		w.Stop()
		close(done)
	}()

	select {
	case <-done:
		// success - worker handled MarkFailed error gracefully
	case <-time.After(10 * time.Second):
		t.Fatal("Stop() did not return within 10 seconds")
	}
}

func TestWorkerResetStuckMessagesError(t *testing.T) {
	// Covers the Start() error branch when ResetStuckMessages fails
	// by closing the store before calling Start().
	st := newTestStore(t)
	ms := &mockSender{}
	w := NewWorker(st, ms, 1, 10*time.Millisecond)

	// Close the store so ResetStuckMessages will fail.
	st.Close()

	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx) // Should log warning but not panic.

	cancel()

	done := make(chan struct{})
	go func() {
		w.Stop()
		close(done)
	}()

	select {
	case <-done:
		// success - Start handled the error gracefully
	case <-time.After(5 * time.Second):
		t.Fatal("Stop() did not return within 5 seconds")
	}
}

func TestWorkerDispatchCancelDuringChannelSend(t *testing.T) {
	// Covers the second ctx.Done() case in dispatch's channel-send select
	// (line 75-77). We use a sender that blocks forever, filling the
	// channel, then cancel the context while dispatch is trying to send
	// another message to the full channel.
	st := newTestStore(t)

	// Use a sender that blocks until context is done, keeping workers busy
	// so the channel fills up.
	blockCh := make(chan struct{})
	sender := &blockingMockSender{
		onSend: func() {
			// Block until test signals
			<-blockCh
		},
	}

	// 1 worker with channel capacity 2 (workers*2=2).
	// Worker will be blocked by sender, so at most 1 message is consumed
	// from channel. If we queue enough messages, the channel will fill up.
	w := NewWorker(st, sender, 1, 10*time.Millisecond)

	// Enqueue many messages so dispatch has more to send than the channel
	// can hold.
	for i := 0; i < 10; i++ {
		_, err := st.EnqueueMessage(nil,
			"dispatch@example.com", "Dispatch Test",
			"text", "html", 3)
		if err != nil {
			t.Fatalf("enqueue %d: %v", i, err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)

	// Wait for the worker to pick up a message (sender will block).
	ok := pollUntil(10*time.Second, 50*time.Millisecond, func() bool {
		sender.mu.Lock()
		defer sender.mu.Unlock()
		return sender.called
	})
	if !ok {
		t.Fatal("worker never picked up a message")
	}

	// Now wait a bit for dispatch to fill the channel and block on send.
	time.Sleep(3 * time.Second)

	// Cancel the context. This should unblock the dispatch channel-send select.
	cancel()

	// Unblock the sender so the worker goroutine can exit.
	close(blockCh)

	done := make(chan struct{})
	go func() {
		w.Stop()
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(10 * time.Second):
		t.Fatal("Stop() did not return within 10 seconds")
	}

	st.Close()
}

// failThenCloseSender returns an error from Send and closes the store,
// so that the subsequent MarkFailed call in the worker fails.
type failThenCloseSender struct {
	mu      sync.Mutex
	called  bool
	store   *store.Store
	sendErr error
}

var _ mail.Sender = (*failThenCloseSender)(nil)

func (f *failThenCloseSender) Send(_, _, _, _ string) error {
	f.mu.Lock()
	f.called = true
	f.mu.Unlock()
	f.store.Close()
	return f.sendErr
}
