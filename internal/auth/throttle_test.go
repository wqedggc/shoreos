package auth

import (
	"testing"
	"time"
)

func TestLoginThrottleBlocksOnlyAfterConfiguredFailures(t *testing.T) {
	now := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	throttle := newLoginThrottle(2, 10*time.Minute, func() time.Time { return now })

	if !throttle.Allow("shore") {
		t.Fatal("a new login key should be allowed")
	}
	throttle.RecordFailure("shore")
	if !throttle.Allow("shore") {
		t.Fatal("one failed login should not block the account")
	}
	throttle.RecordFailure("shore")
	if throttle.Allow("shore") {
		t.Fatal("configured failure limit should block more attempts")
	}

	now = now.Add(10 * time.Minute)
	if !throttle.Allow("shore") {
		t.Fatal("attempts outside the configured window should expire")
	}
}

func TestLoginThrottleResetClearsFailures(t *testing.T) {
	throttle := newLoginThrottle(1, time.Minute, time.Now)
	throttle.RecordFailure("shore")
	if throttle.Allow("shore") {
		t.Fatal("failure should block at a one-attempt limit")
	}
	throttle.Reset("shore")
	if !throttle.Allow("shore") {
		t.Fatal("successful login reset should allow a later attempt")
	}
}

func TestLoginThrottleCanLimitSuccessfulRegistrationAttempts(t *testing.T) {
	throttle := newLoginThrottle(1, time.Minute, time.Now)
	if !throttle.Allow("127.0.0.1") {
		t.Fatal("first registration attempt should be allowed")
	}
	throttle.RecordAttempt("127.0.0.1")
	if throttle.Allow("127.0.0.1") {
		t.Fatal("recorded registration attempt should count toward the limit")
	}
}
