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
