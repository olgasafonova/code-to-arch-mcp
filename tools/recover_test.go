package tools

import (
	"strings"
	"testing"
)

// TestRecoverPanic_AssignsErrorWithCorrelationID is a regression test for HG-1.
// The dispatcher's deferred recover MUST reassign the named `retErr` return so
// a panicking handler surfaces as a structured error to the MCP caller, and
// the panic value MUST stay server-side — only a correlation ID reaches the
// wire. The previous shape used `fmt.Errorf("%s panicked: %v", spec.Name, r)`
// which echoed the panic value to the agent.
func TestRecoverPanic_AssignsErrorWithCorrelationID(t *testing.T) {
	h := &HandlerRegistry{logger: testLogger()}

	err := func() (err error) {
		defer h.recoverPanic("test_tool", &err)
		panic("secret panic value")
	}()

	if err == nil {
		t.Fatal("expected non-nil error after panic, got nil — HG-1 silent fake-success regression")
	}
	msg := err.Error()
	if !strings.Contains(msg, "test_tool: internal error") {
		t.Errorf("error message missing tool name and 'internal error': %q", msg)
	}
	if !strings.Contains(msg, "correlation_id=") {
		t.Errorf("error message missing correlation_id: %q", msg)
	}
	if strings.Contains(msg, "secret panic value") {
		t.Errorf("error message leaked panic value: %q", msg)
	}
}

func TestRecoverPanic_NoPanicNoError(t *testing.T) {
	h := &HandlerRegistry{logger: testLogger()}

	err := func() (err error) {
		defer h.recoverPanic("test_tool", &err)
		return nil
	}()

	if err != nil {
		t.Errorf("expected no error when no panic, got: %v", err)
	}
}

func TestRecoverPanic_PreservesExistingError(t *testing.T) {
	h := &HandlerRegistry{logger: testLogger()}

	sentinel := errStub("real error")
	err := func() (err error) {
		defer h.recoverPanic("test_tool", &err)
		return sentinel
	}()

	if err != sentinel {
		t.Errorf("recoverPanic clobbered existing error: got %v, want %v", err, sentinel)
	}
}

func TestNewCorrelationID_DistinctAndShaped(t *testing.T) {
	a := newCorrelationID()
	b := newCorrelationID()
	if a == b {
		t.Errorf("expected distinct correlation IDs, got same: %q", a)
	}
	if len(a) != 16 {
		t.Errorf("expected 16-char hex ID, got len %d: %q", len(a), a)
	}
}

type errStub string

func (e errStub) Error() string { return string(e) }
