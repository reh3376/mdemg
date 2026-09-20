package api

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"testing"
)

// RETRIEVE-CALLER-CANCEL-001 — pin tests for the caller-cancel classifier
// and its status-code routing through writeInternalError.

func TestIsCallerCancelled_ContextCanceled(t *testing.T) {
	if !isCallerCancelled(context.Canceled) {
		t.Error("context.Canceled should classify as caller-cancelled")
	}
}

func TestIsCallerCancelled_DeadlineExceededIsNot(t *testing.T) {
	// Server-owned timeout: NOT a caller cancellation.
	if isCallerCancelled(context.DeadlineExceeded) {
		t.Error("context.DeadlineExceeded is server-side; must NOT classify as caller-cancelled")
	}
}

func TestIsCallerCancelled_WrappedCanceled(t *testing.T) {
	wrapped := fmt.Errorf("retrieval failed: %w", context.Canceled)
	if !isCallerCancelled(wrapped) {
		t.Error("wrapped context.Canceled must classify as caller-cancelled")
	}
}

func TestIsCallerCancelled_RealErrorIsNot(t *testing.T) {
	if isCallerCancelled(errors.New("neo4j connection refused")) {
		t.Error("real error must NOT classify as caller-cancelled")
	}
}

func TestIsCallerCancelled_NilIsNot(t *testing.T) {
	if isCallerCancelled(nil) {
		t.Error("nil error must NOT classify as caller-cancelled")
	}
}

func TestWriteInternalError_CallerCancelReturns499(t *testing.T) {
	rr := httptest.NewRecorder()
	writeInternalError(rr, context.Canceled, "retrieve")
	if rr.Code != httpStatusClientClosedRequest {
		t.Errorf("caller-cancel must return 499, got %d", rr.Code)
	}
	// 499 is outside the ^5 alert regex — this is the whole point of the sprint.
	if rr.Code >= 500 && rr.Code < 600 {
		t.Errorf("caller-cancel status %d must NOT be in the 5xx range (SLO-alert regex)", rr.Code)
	}
}

func TestWriteInternalError_DeadlineExceededReturns500(t *testing.T) {
	// Server budget expired = real server error; alert SHOULD fire on this class.
	rr := httptest.NewRecorder()
	writeInternalError(rr, context.DeadlineExceeded, "retrieve")
	if rr.Code != 500 {
		t.Errorf("server-side deadline must return 500 (real server error), got %d", rr.Code)
	}
}

func TestWriteInternalError_RealErrorReturns500(t *testing.T) {
	rr := httptest.NewRecorder()
	writeInternalError(rr, errors.New("db: connection refused"), "retrieve")
	if rr.Code != 500 {
		t.Errorf("real error must return 500, got %d", rr.Code)
	}
}

// HITL-ERROR-VISIBLE-001 pins for the ClientVisibleError opt-in on
// sanitizeError. Preserves the "generic errors stay generic" contract
// while letting sinks surface named policy-error text.

// TestSanitizeError_GenericErrorStaysGeneric — regression pin. Any plain
// error is still wrapped as the opaque "internal error during X" — the
// security contract (don't leak internal detail by default) MUST hold.
func TestSanitizeError_GenericErrorStaysGeneric(t *testing.T) {
	msg := sanitizeError(errors.New("db: connection refused"), "retrieve")
	if msg != "internal error during retrieve" {
		t.Errorf("generic error leaked detail: got %q", msg)
	}
}

// TestSanitizeError_CallerCancelPreserved — pre-existing "request cancelled
// during X" contract survives the ClientVisibleError check ordering.
func TestSanitizeError_CallerCancelPreserved(t *testing.T) {
	msg := sanitizeError(context.Canceled, "retrieve")
	if msg != "request cancelled during retrieve" {
		t.Errorf("caller-cancel path drifted: got %q", msg)
	}
}

// testClientVisibleError is a minimal type implementing ClientVisibleError
// for the pin test. Kept intra-package so we don't leak the interface as
// an exported hook.
type testClientVisibleError struct{ msg string }

func (e *testClientVisibleError) Error() string         { return e.msg }
func (e *testClientVisibleError) ClientVisible() string { return e.msg }

// TestSanitizeError_ClientVisibleErrorSurfacesMessage — errors implementing
// ClientVisibleError have their message returned verbatim to the client.
// The security contract is preserved because opt-in requires the error type
// to explicitly implement the interface.
func TestSanitizeError_ClientVisibleErrorSurfacesMessage(t *testing.T) {
	sinkMsg := "human_class_queue: auto-grader rejected — human-verifiability class is operator-only by construction"
	err := &testClientVisibleError{msg: sinkMsg}
	got := sanitizeError(err, "review reinforcement apply")
	if got != sinkMsg {
		t.Errorf("client-visible error was sanitized: got %q, want %q", got, sinkMsg)
	}
}

// TestSanitizeError_ClientVisibleErrorViaErrorsAs — verifies the interface
// check uses errors.As so wrapped errors are still surfaced. Sinks that
// wrap errAutograderRejected via fmt.Errorf("...%w", err) must still
// surface the underlying named message.
func TestSanitizeError_ClientVisibleErrorViaErrorsAs(t *testing.T) {
	inner := &testClientVisibleError{msg: "policy violation: X"}
	wrapped := fmt.Errorf("outer wrap: %w", inner)
	got := sanitizeError(wrapped, "test-op")
	if got != inner.msg {
		t.Errorf("errors.As unwrap failed: got %q, want %q", got, inner.msg)
	}
}
