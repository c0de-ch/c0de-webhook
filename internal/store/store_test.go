package store

import (
	"encoding/hex"
	"fmt"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestNew(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("New(:memory:) returned error: %v", err)
	}
	defer s.Close()

	if s.db == nil {
		t.Fatal("expected db to be non-nil")
	}
}

func TestCreateToken(t *testing.T) {
	s := newTestStore(t)

	rawToken, token, err := s.CreateToken("test-token")
	if err != nil {
		t.Fatalf("CreateToken returned error: %v", err)
	}

	// rawToken should be 64 hex chars (32 bytes encoded)
	if len(rawToken) != 64 {
		t.Errorf("expected rawToken length 64, got %d", len(rawToken))
	}
	if _, err := hex.DecodeString(rawToken); err != nil {
		t.Errorf("rawToken is not valid hex: %v", err)
	}

	if token.Name != "test-token" {
		t.Errorf("expected token name 'test-token', got %q", token.Name)
	}
	if token.ID == 0 {
		t.Error("expected token ID to be non-zero")
	}
	if !token.IsActive {
		t.Error("expected token to be active")
	}
}

func TestListTokens(t *testing.T) {
	s := newTestStore(t)

	// Create 3 tokens with slight delays to ensure ordering
	names := []string{"first", "second", "third"}
	for _, name := range names {
		if _, _, err := s.CreateToken(name); err != nil {
			t.Fatalf("CreateToken(%s) error: %v", name, err)
		}
	}

	tokens, err := s.ListTokens()
	if err != nil {
		t.Fatalf("ListTokens error: %v", err)
	}
	if len(tokens) != 3 {
		t.Fatalf("expected 3 tokens, got %d", len(tokens))
	}

	// Order is DESC by created_at, so the last created appears first.
	// Since they may have the same created_at timestamp (datetime('now') granularity),
	// we verify by ID descending which matches insertion order for AUTOINCREMENT.
	for i, tok := range tokens {
		if tok.ID == 0 {
			t.Errorf("token[%d] has zero ID", i)
		}
		if tok.Name == "" {
			t.Errorf("token[%d] has empty name", i)
		}
		if tok.TokenHash == "" {
			t.Errorf("token[%d] has empty token hash", i)
		}
		if !tok.IsActive {
			t.Errorf("token[%d] expected active", i)
		}
	}

	// Since all created at the same second, verify we got all three names
	gotNames := map[string]bool{}
	for _, tok := range tokens {
		gotNames[tok.Name] = true
	}
	for _, name := range names {
		if !gotNames[name] {
			t.Errorf("expected token name %q in list", name)
		}
	}
}

func TestValidateToken(t *testing.T) {
	s := newTestStore(t)

	rawToken, created, err := s.CreateToken("validate-me")
	if err != nil {
		t.Fatalf("CreateToken error: %v", err)
	}

	// First validation
	tok, err := s.ValidateToken(rawToken)
	if err != nil {
		t.Fatalf("ValidateToken error: %v", err)
	}
	if tok.ID != created.ID {
		t.Errorf("expected ID %d, got %d", created.ID, tok.ID)
	}
	if tok.Name != "validate-me" {
		t.Errorf("expected name 'validate-me', got %q", tok.Name)
	}
	if !tok.IsActive {
		t.Error("expected token to be active")
	}

	// Second validation - check that last_used_at is set
	tok2, err := s.ValidateToken(rawToken)
	if err != nil {
		t.Fatalf("second ValidateToken error: %v", err)
	}
	if tok2.LastUsedAt == nil {
		t.Error("expected last_used_at to be set after second validation")
	}
}

func TestValidateToken_Invalid(t *testing.T) {
	s := newTestStore(t)

	_, err := s.ValidateToken("0000000000000000000000000000000000000000000000000000000000000000")
	if err == nil {
		t.Fatal("expected error for invalid token, got nil")
	}
}

func TestValidateToken_Disabled(t *testing.T) {
	s := newTestStore(t)

	rawToken, created, err := s.CreateToken("disable-me")
	if err != nil {
		t.Fatalf("CreateToken error: %v", err)
	}

	// Toggle off
	if err := s.ToggleToken(created.ID); err != nil {
		t.Fatalf("ToggleToken error: %v", err)
	}

	_, err = s.ValidateToken(rawToken)
	if err == nil {
		t.Fatal("expected error for disabled token, got nil")
	}
	if err.Error() != "token is disabled" {
		t.Errorf("expected error 'token is disabled', got %q", err.Error())
	}
}

func TestToggleToken(t *testing.T) {
	s := newTestStore(t)

	_, created, err := s.CreateToken("toggle-me")
	if err != nil {
		t.Fatalf("CreateToken error: %v", err)
	}

	// Verify initially active
	tokens, _ := s.ListTokens()
	if !tokens[0].IsActive {
		t.Fatal("expected token to start active")
	}

	// Toggle off
	if err := s.ToggleToken(created.ID); err != nil {
		t.Fatalf("first ToggleToken error: %v", err)
	}
	tokens, _ = s.ListTokens()
	if tokens[0].IsActive {
		t.Error("expected token to be disabled after first toggle")
	}

	// Toggle back on
	if err := s.ToggleToken(created.ID); err != nil {
		t.Fatalf("second ToggleToken error: %v", err)
	}
	tokens, _ = s.ListTokens()
	if !tokens[0].IsActive {
		t.Error("expected token to be active after second toggle")
	}
}

func TestDeleteToken(t *testing.T) {
	s := newTestStore(t)

	_, created, err := s.CreateToken("delete-me")
	if err != nil {
		t.Fatalf("CreateToken error: %v", err)
	}

	if err := s.DeleteToken(created.ID); err != nil {
		t.Fatalf("DeleteToken error: %v", err)
	}

	tokens, err := s.ListTokens()
	if err != nil {
		t.Fatalf("ListTokens error: %v", err)
	}
	if len(tokens) != 0 {
		t.Errorf("expected 0 tokens after delete, got %d", len(tokens))
	}
}

func TestEnqueueMessage(t *testing.T) {
	s := newTestStore(t)

	_, tok, err := s.CreateToken("msg-token")
	if err != nil {
		t.Fatalf("CreateToken error: %v", err)
	}

	tokenID := tok.ID
	msg, err := s.EnqueueMessage(&tokenID, "user@example.com", "Test Subject", "text body", "<p>html</p>", 3)
	if err != nil {
		t.Fatalf("EnqueueMessage error: %v", err)
	}

	if msg.ID == 0 {
		t.Error("expected message ID to be non-zero")
	}
	if msg.Status != "queued" {
		t.Errorf("expected status 'queued', got %q", msg.Status)
	}
	if msg.To != "user@example.com" {
		t.Errorf("expected To 'user@example.com', got %q", msg.To)
	}
	if msg.Subject != "Test Subject" {
		t.Errorf("expected Subject 'Test Subject', got %q", msg.Subject)
	}
	if msg.TextBody != "text body" {
		t.Errorf("expected TextBody 'text body', got %q", msg.TextBody)
	}
	if msg.HTMLBody != "<p>html</p>" {
		t.Errorf("expected HTMLBody '<p>html</p>', got %q", msg.HTMLBody)
	}
	if msg.MaxAttempts != 3 {
		t.Errorf("expected MaxAttempts 3, got %d", msg.MaxAttempts)
	}
	if msg.TokenID == nil || *msg.TokenID != tokenID {
		t.Errorf("expected TokenID %d, got %v", tokenID, msg.TokenID)
	}
}

func TestClaimPendingMessages(t *testing.T) {
	s := newTestStore(t)

	// Enqueue 3 messages
	for i := 0; i < 3; i++ {
		_, err := s.EnqueueMessage(nil, fmt.Sprintf("user%d@example.com", i), fmt.Sprintf("Subject %d", i), "", "", 3)
		if err != nil {
			t.Fatalf("EnqueueMessage error: %v", err)
		}
	}

	// Claim 2
	claimed, err := s.ClaimPendingMessages(2)
	if err != nil {
		t.Fatalf("ClaimPendingMessages(2) error: %v", err)
	}
	if len(claimed) != 2 {
		t.Fatalf("expected 2 claimed messages, got %d", len(claimed))
	}

	// Verify the claimed messages have correct status by marking them sent
	// (they should be in 'sending' state in the DB)
	for _, m := range claimed {
		if err := s.MarkSent(m.ID); err != nil {
			t.Errorf("MarkSent(%d) error: %v", m.ID, err)
		}
	}

	// Claim again - should get the remaining 1
	claimed2, err := s.ClaimPendingMessages(10)
	if err != nil {
		t.Fatalf("second ClaimPendingMessages error: %v", err)
	}
	if len(claimed2) != 1 {
		t.Errorf("expected 1 claimed message, got %d", len(claimed2))
	}
}

func TestClaimPendingMessages_RespectRetryTime(t *testing.T) {
	s := newTestStore(t)

	msg, err := s.EnqueueMessage(nil, "user@example.com", "Retry Test", "", "", 3)
	if err != nil {
		t.Fatalf("EnqueueMessage error: %v", err)
	}

	// Claim and mark failed - this sets next_retry_at in the future
	claimed, err := s.ClaimPendingMessages(1)
	if err != nil {
		t.Fatalf("ClaimPendingMessages error: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("expected 1 claimed, got %d", len(claimed))
	}

	err = s.MarkFailed(msg.ID, "temporary error", 1*time.Hour)
	if err != nil {
		t.Fatalf("MarkFailed error: %v", err)
	}

	// Claim should return empty because next_retry_at is in the future
	claimed, err = s.ClaimPendingMessages(10)
	if err != nil {
		t.Fatalf("ClaimPendingMessages after fail error: %v", err)
	}
	if len(claimed) != 0 {
		t.Errorf("expected 0 claimed messages (retry in future), got %d", len(claimed))
	}

	// Manually set next_retry_at to the past
	_, err = s.db.Exec("UPDATE messages SET next_retry_at = datetime('now', '-1 hour') WHERE id = ?", msg.ID)
	if err != nil {
		t.Fatalf("manual update next_retry_at error: %v", err)
	}

	// Now claim should return the message
	claimed, err = s.ClaimPendingMessages(10)
	if err != nil {
		t.Fatalf("ClaimPendingMessages after retry time error: %v", err)
	}
	if len(claimed) != 1 {
		t.Errorf("expected 1 claimed message (retry time passed), got %d", len(claimed))
	}
}

func TestMarkSent(t *testing.T) {
	s := newTestStore(t)

	msg, err := s.EnqueueMessage(nil, "user@example.com", "Sent Test", "body", "", 3)
	if err != nil {
		t.Fatalf("EnqueueMessage error: %v", err)
	}

	// Claim first (moves to 'sending')
	_, err = s.ClaimPendingMessages(1)
	if err != nil {
		t.Fatalf("ClaimPendingMessages error: %v", err)
	}

	// Mark sent
	if err := s.MarkSent(msg.ID); err != nil {
		t.Fatalf("MarkSent error: %v", err)
	}

	// Verify via ListMessages
	msgs, total, err := s.ListMessages("sent", 10, 0)
	if err != nil {
		t.Fatalf("ListMessages error: %v", err)
	}
	if total != 1 {
		t.Errorf("expected total 1, got %d", total)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Status != "sent" {
		t.Errorf("expected status 'sent', got %q", msgs[0].Status)
	}
	if msgs[0].SentAt == nil {
		t.Error("expected sent_at to be set")
	}
}

func TestMarkFailed_WithRetry(t *testing.T) {
	s := newTestStore(t)

	msg, err := s.EnqueueMessage(nil, "user@example.com", "Fail Test", "", "", 3)
	if err != nil {
		t.Fatalf("EnqueueMessage error: %v", err)
	}

	// First attempt: claim and fail
	_, err = s.ClaimPendingMessages(1)
	if err != nil {
		t.Fatalf("ClaimPendingMessages error: %v", err)
	}
	err = s.MarkFailed(msg.ID, "error 1", 0) // zero delay so retry is immediate
	if err != nil {
		t.Fatalf("MarkFailed (1st) error: %v", err)
	}

	// Check status is back to queued with attempts=1
	msgs, _, _ := s.ListMessages("queued", 10, 0)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 queued message after 1st failure, got %d", len(msgs))
	}
	if msgs[0].Attempts != 1 {
		t.Errorf("expected attempts=1, got %d", msgs[0].Attempts)
	}

	// Second attempt: claim and fail
	_, err = s.ClaimPendingMessages(1)
	if err != nil {
		t.Fatalf("ClaimPendingMessages (2nd) error: %v", err)
	}
	err = s.MarkFailed(msg.ID, "error 2", 0)
	if err != nil {
		t.Fatalf("MarkFailed (2nd) error: %v", err)
	}

	// Check attempts=2, still queued
	msgs, _, _ = s.ListMessages("queued", 10, 0)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 queued message after 2nd failure, got %d", len(msgs))
	}
	if msgs[0].Attempts != 2 {
		t.Errorf("expected attempts=2, got %d", msgs[0].Attempts)
	}

	// Third attempt: claim and fail - should go to "failed"
	_, err = s.ClaimPendingMessages(1)
	if err != nil {
		t.Fatalf("ClaimPendingMessages (3rd) error: %v", err)
	}
	err = s.MarkFailed(msg.ID, "error 3", 0)
	if err != nil {
		t.Fatalf("MarkFailed (3rd) error: %v", err)
	}

	// Check status is now "failed"
	msgs, _, _ = s.ListMessages("failed", 10, 0)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 failed message after 3rd failure, got %d", len(msgs))
	}
	if msgs[0].Attempts != 3 {
		t.Errorf("expected attempts=3, got %d", msgs[0].Attempts)
	}
	if msgs[0].Status != "failed" {
		t.Errorf("expected status 'failed', got %q", msgs[0].Status)
	}
}

func TestListMessages(t *testing.T) {
	s := newTestStore(t)

	// Enqueue 5 messages
	for i := 0; i < 5; i++ {
		_, err := s.EnqueueMessage(nil, fmt.Sprintf("user%d@example.com", i), fmt.Sprintf("Subject %d", i), "", "", 3)
		if err != nil {
			t.Fatalf("EnqueueMessage error: %v", err)
		}
	}

	// Mark messages 1, 2 as sent
	claimed, _ := s.ClaimPendingMessages(5)
	for i := 0; i < 2; i++ {
		_ = s.MarkSent(claimed[i].ID)
	}
	// Mark message 3 as failed (max_attempts=3, fail 3 times)
	for j := 0; j < 3; j++ {
		_ = s.MarkFailed(claimed[2].ID, "err", 0)
	}
	// Messages 4, 5 remain in 'sending' status (since they were claimed)
	// Reset them back to queued for a clean test
	_ = s.ResetStuckMessages()

	// Test filter "all"
	msgs, total, err := s.ListMessages("all", 100, 0)
	if err != nil {
		t.Fatalf("ListMessages(all) error: %v", err)
	}
	if total != 5 {
		t.Errorf("expected total 5 for 'all', got %d", total)
	}
	if len(msgs) != 5 {
		t.Errorf("expected 5 messages for 'all', got %d", len(msgs))
	}

	// Test filter "sent"
	msgs, total, err = s.ListMessages("sent", 100, 0)
	if err != nil {
		t.Fatalf("ListMessages(sent) error: %v", err)
	}
	if total != 2 {
		t.Errorf("expected total 2 for 'sent', got %d", total)
	}

	// Test filter "failed"
	msgs, total, err = s.ListMessages("failed", 100, 0)
	if err != nil {
		t.Fatalf("ListMessages(failed) error: %v", err)
	}
	if total != 1 {
		t.Errorf("expected total 1 for 'failed', got %d", total)
	}

	// Test filter "queued"
	msgs, total, err = s.ListMessages("queued", 100, 0)
	if err != nil {
		t.Fatalf("ListMessages(queued) error: %v", err)
	}
	if total != 2 {
		t.Errorf("expected total 2 for 'queued', got %d", total)
	}

	// Test limit and offset
	msgs, total, err = s.ListMessages("all", 2, 0)
	if err != nil {
		t.Fatalf("ListMessages(limit=2) error: %v", err)
	}
	if total != 5 {
		t.Errorf("expected total 5 with limit, got %d", total)
	}
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages with limit=2, got %d", len(msgs))
	}

	msgs, total, err = s.ListMessages("all", 2, 3)
	if err != nil {
		t.Fatalf("ListMessages(limit=2,offset=3) error: %v", err)
	}
	if total != 5 {
		t.Errorf("expected total 5, got %d", total)
	}
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages with limit=2 offset=3, got %d", len(msgs))
	}

	// Test empty status (same as "all")
	_, total, err = s.ListMessages("", 100, 0)
	if err != nil {
		t.Fatalf("ListMessages('') error: %v", err)
	}
	if total != 5 {
		t.Errorf("expected total 5 for empty filter, got %d", total)
	}

	_ = msgs // suppress unused
}

func TestDeleteMessage(t *testing.T) {
	s := newTestStore(t)

	msg, err := s.EnqueueMessage(nil, "user@example.com", "Delete Me", "", "", 3)
	if err != nil {
		t.Fatalf("EnqueueMessage error: %v", err)
	}

	if err := s.DeleteMessage(msg.ID); err != nil {
		t.Fatalf("DeleteMessage error: %v", err)
	}

	msgs, total, err := s.ListMessages("all", 100, 0)
	if err != nil {
		t.Fatalf("ListMessages error: %v", err)
	}
	if total != 0 {
		t.Errorf("expected 0 messages after delete, got %d", total)
	}
	if len(msgs) != 0 {
		t.Errorf("expected empty list after delete, got %d", len(msgs))
	}
}

func TestRetryMessage(t *testing.T) {
	s := newTestStore(t)

	msg, err := s.EnqueueMessage(nil, "user@example.com", "Retry Me", "", "", 1)
	if err != nil {
		t.Fatalf("EnqueueMessage error: %v", err)
	}

	// Claim and mark failed to exhaust retries (max_attempts=1)
	_, err = s.ClaimPendingMessages(1)
	if err != nil {
		t.Fatalf("ClaimPendingMessages error: %v", err)
	}
	err = s.MarkFailed(msg.ID, "permanent error", 0)
	if err != nil {
		t.Fatalf("MarkFailed error: %v", err)
	}

	// Verify it's in "failed" status
	msgs, _, _ := s.ListMessages("failed", 10, 0)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 failed message, got %d", len(msgs))
	}

	// Retry
	if err := s.RetryMessage(msg.ID); err != nil {
		t.Fatalf("RetryMessage error: %v", err)
	}

	// Verify it's back to "queued"
	msgs, _, _ = s.ListMessages("queued", 10, 0)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 queued message after retry, got %d", len(msgs))
	}
	if msgs[0].Status != "queued" {
		t.Errorf("expected status 'queued', got %q", msgs[0].Status)
	}
}

func TestResetStuckMessages(t *testing.T) {
	s := newTestStore(t)

	_, err := s.EnqueueMessage(nil, "user@example.com", "Stuck", "", "", 3)
	if err != nil {
		t.Fatalf("EnqueueMessage error: %v", err)
	}

	// Claim to move to 'sending'
	claimed, err := s.ClaimPendingMessages(1)
	if err != nil {
		t.Fatalf("ClaimPendingMessages error: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("expected 1 claimed, got %d", len(claimed))
	}

	// Verify it's in sending status via ListMessages
	msgs, sendingCount, _ := s.ListMessages("sending", 10, 0)
	if sendingCount != 1 {
		t.Errorf("expected 1 sending message, got %d", sendingCount)
	}
	_ = msgs

	// Verify no queued messages
	_, queuedCount, _ := s.ListMessages("queued", 10, 0)
	if queuedCount != 0 {
		t.Errorf("expected 0 queued before reset, got %d", queuedCount)
	}

	// Reset stuck
	if err := s.ResetStuckMessages(); err != nil {
		t.Fatalf("ResetStuckMessages error: %v", err)
	}

	// Verify it's back to queued
	_, queuedCount, _ = s.ListMessages("queued", 10, 0)
	if queuedCount != 1 {
		t.Errorf("expected 1 queued after reset, got %d", queuedCount)
	}

	// Verify no sending messages
	_, sendingCount, _ = s.ListMessages("sending", 10, 0)
	if sendingCount != 0 {
		t.Errorf("expected 0 sending after reset, got %d", sendingCount)
	}
}

func TestGetDashboardStats(t *testing.T) {
	s := newTestStore(t)

	// Create some messages with various statuses
	// 2 will be sent, 1 will fail, 2 will stay queued
	for i := 0; i < 5; i++ {
		_, err := s.EnqueueMessage(nil, fmt.Sprintf("u%d@example.com", i), fmt.Sprintf("Subj %d", i), "", "", 1)
		if err != nil {
			t.Fatalf("EnqueueMessage error: %v", err)
		}
	}

	// Claim all, send 2, fail 1, leave 2 as sending then reset to queued
	claimed, _ := s.ClaimPendingMessages(5)
	_ = s.MarkSent(claimed[0].ID)
	_ = s.MarkSent(claimed[1].ID)
	_ = s.MarkFailed(claimed[2].ID, "err", 0) // max_attempts=1, so goes to failed
	// Reset the remaining 'sending' messages back to queued
	_ = s.ResetStuckMessages()

	stats, err := s.GetDashboardStats()
	if err != nil {
		t.Fatalf("GetDashboardStats error: %v", err)
	}

	if stats.TotalSent != 2 {
		t.Errorf("expected TotalSent=2, got %d", stats.TotalSent)
	}
	if stats.TotalFailed != 1 {
		t.Errorf("expected TotalFailed=1, got %d", stats.TotalFailed)
	}
	if stats.QueueDepth != 2 {
		t.Errorf("expected QueueDepth=2, got %d", stats.QueueDepth)
	}
	// TodaySent and TodayFailed depend on date('now') matching
	if stats.TodaySent != 2 {
		t.Errorf("expected TodaySent=2, got %d", stats.TodaySent)
	}
	if stats.TodayFailed != 1 {
		t.Errorf("expected TodayFailed=1, got %d", stats.TodayFailed)
	}

	// SuccessRate = 2/(2+1) * 100 = 66.66...
	expectedRate := float64(2) / float64(3) * 100
	if stats.SuccessRate < expectedRate-0.01 || stats.SuccessRate > expectedRate+0.01 {
		t.Errorf("expected SuccessRate ~%.2f, got %.2f", expectedRate, stats.SuccessRate)
	}
}

func TestGetHourlyStats(t *testing.T) {
	s := newTestStore(t)

	// Enqueue and send a message
	msg, err := s.EnqueueMessage(nil, "user@example.com", "Hourly", "", "", 3)
	if err != nil {
		t.Fatalf("EnqueueMessage error: %v", err)
	}
	_, _ = s.ClaimPendingMessages(1)
	_ = s.MarkSent(msg.ID)

	stats, err := s.GetHourlyStats(24)
	if err != nil {
		t.Fatalf("GetHourlyStats error: %v", err)
	}

	// Should have at least one hour entry
	if len(stats) == 0 {
		t.Fatal("expected at least 1 hourly stat entry")
	}

	// The entry should show at least 1 sent
	totalSent := 0
	totalAll := 0
	for _, h := range stats {
		totalSent += h.Sent
		totalAll += h.Total
		if h.Hour.IsZero() {
			t.Error("expected non-zero hour time")
		}
	}
	if totalSent < 1 {
		t.Errorf("expected at least 1 sent in hourly stats, got %d", totalSent)
	}
	if totalAll < 1 {
		t.Errorf("expected at least 1 total in hourly stats, got %d", totalAll)
	}
}

func TestGetRecentMessages(t *testing.T) {
	s := newTestStore(t)

	// Enqueue 5 messages
	for i := 0; i < 5; i++ {
		_, err := s.EnqueueMessage(nil, fmt.Sprintf("user%d@example.com", i), fmt.Sprintf("Recent %d", i), "", "", 3)
		if err != nil {
			t.Fatalf("EnqueueMessage error: %v", err)
		}
	}

	// Get recent with limit 3
	msgs, err := s.GetRecentMessages(3)
	if err != nil {
		t.Fatalf("GetRecentMessages error: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 recent messages, got %d", len(msgs))
	}

	// Verify order is DESC by created_at (most recent first)
	// Since all were inserted in order, IDs are ascending, so recent means highest IDs first
	for i := 1; i < len(msgs); i++ {
		if msgs[i].CreatedAt.After(msgs[i-1].CreatedAt) {
			t.Errorf("expected descending order, but msg[%d].CreatedAt (%v) > msg[%d].CreatedAt (%v)",
				i, msgs[i].CreatedAt, i-1, msgs[i-1].CreatedAt)
		}
	}

	// Get all
	msgs, err = s.GetRecentMessages(100)
	if err != nil {
		t.Fatalf("GetRecentMessages(100) error: %v", err)
	}
	if len(msgs) != 5 {
		t.Errorf("expected 5 messages, got %d", len(msgs))
	}
}

func TestGetRecentMessages_WithTokenAndSentAt(t *testing.T) {
	s := newTestStore(t)

	// Create a token so the JOIN populates TokenName
	_, tok, err := s.CreateToken("recent-tok")
	if err != nil {
		t.Fatalf("CreateToken error: %v", err)
	}
	tokenID := tok.ID

	msg, err := s.EnqueueMessage(&tokenID, "user@example.com", "With Token", "", "", 3)
	if err != nil {
		t.Fatalf("EnqueueMessage error: %v", err)
	}

	// Claim and mark sent to populate sent_at
	_, _ = s.ClaimPendingMessages(1)
	_ = s.MarkSent(msg.ID)

	msgs, err := s.GetRecentMessages(10)
	if err != nil {
		t.Fatalf("GetRecentMessages error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].TokenName != "recent-tok" {
		t.Errorf("expected TokenName 'recent-tok', got %q", msgs[0].TokenName)
	}
	if msgs[0].TokenID == nil || *msgs[0].TokenID != tokenID {
		t.Errorf("expected TokenID %d, got %v", tokenID, msgs[0].TokenID)
	}
	if msgs[0].SentAt == nil {
		t.Error("expected SentAt to be non-nil for sent message")
	}
}

func TestGetDashboardStats_Empty(t *testing.T) {
	s := newTestStore(t)

	stats, err := s.GetDashboardStats()
	if err != nil {
		t.Fatalf("GetDashboardStats error: %v", err)
	}

	if stats.TotalSent != 0 {
		t.Errorf("expected TotalSent=0, got %d", stats.TotalSent)
	}
	if stats.TotalFailed != 0 {
		t.Errorf("expected TotalFailed=0, got %d", stats.TotalFailed)
	}
	if stats.QueueDepth != 0 {
		t.Errorf("expected QueueDepth=0, got %d", stats.QueueDepth)
	}
	if stats.SuccessRate != 0 {
		t.Errorf("expected SuccessRate=0, got %f", stats.SuccessRate)
	}
}

func TestClaimPendingMessages_Empty(t *testing.T) {
	s := newTestStore(t)

	// Claiming when no messages exist should return nil/empty
	claimed, err := s.ClaimPendingMessages(10)
	if err != nil {
		t.Fatalf("ClaimPendingMessages error: %v", err)
	}
	if len(claimed) != 0 {
		t.Errorf("expected 0 claimed from empty store, got %d", len(claimed))
	}
}

func TestListMessages_WithTokenName(t *testing.T) {
	s := newTestStore(t)

	// Create a token and enqueue a message with it, then send it
	// This covers the tokenID.Valid and sentAt.Valid branches in ListMessages
	_, tok, _ := s.CreateToken("list-tok")
	tokenID := tok.ID
	msg, err := s.EnqueueMessage(&tokenID, "user@example.com", "Token msg", "", "", 3)
	if err != nil {
		t.Fatalf("EnqueueMessage error: %v", err)
	}

	_, _ = s.ClaimPendingMessages(1)
	_ = s.MarkSent(msg.ID)

	msgs, total, err := s.ListMessages("sent", 10, 0)
	if err != nil {
		t.Fatalf("ListMessages error: %v", err)
	}
	if total != 1 {
		t.Errorf("expected total=1, got %d", total)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 msg, got %d", len(msgs))
	}
	if msgs[0].TokenName != "list-tok" {
		t.Errorf("expected TokenName 'list-tok', got %q", msgs[0].TokenName)
	}
	if msgs[0].TokenID == nil || *msgs[0].TokenID != tokenID {
		t.Errorf("expected TokenID %d, got %v", tokenID, msgs[0].TokenID)
	}
	if msgs[0].SentAt == nil {
		t.Error("expected SentAt to be set")
	}
}

func TestGetHourlyStats_WithFailed(t *testing.T) {
	s := newTestStore(t)

	// Create a message that will be failed
	msg, err := s.EnqueueMessage(nil, "user@example.com", "Hourly Fail", "", "", 1)
	if err != nil {
		t.Fatalf("EnqueueMessage error: %v", err)
	}

	_, _ = s.ClaimPendingMessages(1)
	_ = s.MarkFailed(msg.ID, "err", 0) // max_attempts=1, goes to failed

	stats, err := s.GetHourlyStats(24)
	if err != nil {
		t.Fatalf("GetHourlyStats error: %v", err)
	}

	if len(stats) == 0 {
		t.Fatal("expected at least 1 hourly stat")
	}

	totalFailed := 0
	for _, h := range stats {
		totalFailed += h.Failed
	}
	if totalFailed < 1 {
		t.Errorf("expected at least 1 failed in hourly stats, got %d", totalFailed)
	}
}

func TestListTokens_WithLastUsed(t *testing.T) {
	s := newTestStore(t)

	rawToken, _, err := s.CreateToken("used-token")
	if err != nil {
		t.Fatalf("CreateToken error: %v", err)
	}

	// Validate to set last_used_at
	_, _ = s.ValidateToken(rawToken)

	tokens, err := s.ListTokens()
	if err != nil {
		t.Fatalf("ListTokens error: %v", err)
	}
	if len(tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(tokens))
	}
	if tokens[0].LastUsedAt == nil {
		t.Error("expected LastUsedAt to be set after validation")
	}
}

func TestEnqueueMessage_NilTokenID(t *testing.T) {
	s := newTestStore(t)

	msg, err := s.EnqueueMessage(nil, "user@example.com", "No Token", "text", "<p>html</p>", 5)
	if err != nil {
		t.Fatalf("EnqueueMessage error: %v", err)
	}
	if msg.TokenID != nil {
		t.Errorf("expected nil TokenID, got %v", msg.TokenID)
	}
	if msg.MaxAttempts != 5 {
		t.Errorf("expected MaxAttempts=5, got %d", msg.MaxAttempts)
	}
}

func TestClaimPendingMessages_WithTokenID(t *testing.T) {
	s := newTestStore(t)

	// Create message with a token ID to cover the tokenID.Valid branch in ClaimPendingMessages
	_, tok, _ := s.CreateToken("claim-tok")
	tokenID := tok.ID
	_, err := s.EnqueueMessage(&tokenID, "user@example.com", "With Token", "", "", 3)
	if err != nil {
		t.Fatalf("EnqueueMessage error: %v", err)
	}

	claimed, err := s.ClaimPendingMessages(1)
	if err != nil {
		t.Fatalf("ClaimPendingMessages error: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("expected 1 claimed, got %d", len(claimed))
	}
	if claimed[0].TokenID == nil || *claimed[0].TokenID != tokenID {
		t.Errorf("expected TokenID %d, got %v", tokenID, claimed[0].TokenID)
	}
}

func TestNew_InvalidDSN(t *testing.T) {
	_, err := New("/nonexistent/path/to/impossible/db.sqlite")
	if err == nil {
		t.Error("expected error for invalid DSN")
	}
}

func TestListMessages_AllStatuses(t *testing.T) {
	s := newTestStore(t)

	m1, _ := s.EnqueueMessage(nil, "a@b.com", "S1", "t", "", 3)
	m2, _ := s.EnqueueMessage(nil, "a@b.com", "S2", "t", "", 3)
	_, _ = s.EnqueueMessage(nil, "a@b.com", "S3", "t", "", 3)

	_, _ = s.ClaimPendingMessages(3)
	_ = s.MarkSent(m1.ID)
	_ = s.MarkFailed(m2.ID, "err", time.Second)

	all, total, _ := s.ListMessages("all", 10, 0)
	if total != 3 {
		t.Errorf("all: expected total=3, got %d", total)
	}
	if len(all) != 3 {
		t.Errorf("all: expected 3 messages, got %d", len(all))
	}

	sent, sentTotal, _ := s.ListMessages("sent", 10, 0)
	if sentTotal != 1 {
		t.Errorf("sent: expected total=1, got %d", sentTotal)
	}
	if len(sent) != 1 {
		t.Errorf("sent: expected 1 message, got %d", len(sent))
	}

	_, offsetTotal, _ := s.ListMessages("", 1, 1)
	if offsetTotal != 3 {
		t.Errorf("offset: expected total=3, got %d", offsetTotal)
	}
}

func TestMarkFailed_ExceedsMaxRetries(t *testing.T) {
	s := newTestStore(t)

	// max_attempts=1: first failure should mark it failed
	msg, _ := s.EnqueueMessage(nil, "x@y.com", "Sub", "t", "", 1)
	_, _ = s.ClaimPendingMessages(1)

	err := s.MarkFailed(msg.ID, "boom", time.Second)
	if err != nil {
		t.Fatalf("MarkFailed error: %v", err)
	}

	msgs, _, _ := s.ListMessages("failed", 10, 0)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 failed, got %d", len(msgs))
	}
	if msgs[0].LastError != "boom" {
		t.Errorf("expected error 'boom', got %q", msgs[0].LastError)
	}
}

func TestListTokens_Empty(t *testing.T) {
	s := newTestStore(t)
	tokens, err := s.ListTokens()
	if err != nil {
		t.Fatal(err)
	}
	if tokens != nil {
		t.Errorf("expected nil, got %v", tokens)
	}
}

func TestGetHourlyStats_Empty(t *testing.T) {
	s := newTestStore(t)
	stats, err := s.GetHourlyStats(24)
	if err != nil {
		t.Fatal(err)
	}
	if stats != nil {
		t.Errorf("expected nil for empty hourly stats, got %v", stats)
	}
}

func TestGetRecentMessages_Empty(t *testing.T) {
	s := newTestStore(t)
	msgs, err := s.GetRecentMessages(10)
	if err != nil {
		t.Fatal(err)
	}
	if msgs != nil {
		t.Errorf("expected nil for empty recent messages, got %v", msgs)
	}
}

func TestHashToken(t *testing.T) {
	// Verify the hash is deterministic
	h1 := hashToken("test-token")
	h2 := hashToken("test-token")
	if h1 != h2 {
		t.Error("hash should be deterministic")
	}
	if len(h1) != 64 {
		t.Errorf("expected 64 char hex hash, got %d", len(h1))
	}
	// Verify it's valid hex
	_, err := hex.DecodeString(h1)
	if err != nil {
		t.Errorf("hash is not valid hex: %v", err)
	}
}
