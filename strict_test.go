// Copyright 2026 The go-rar Authors
// SPDX-License-Identifier: Apache-2.0

package rar

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// withStrictMarshal sets the package-wide strict-marshal toggle for
// the duration of the test and schedules a cleanup that restores the
// prior value. Tests that exercise the strict path MUST use this
// helper rather than calling [SetStrictMarshal] directly, because
// [strictMarshalEnabled] is package-global state: a leaked `true`
// from one test would silently corrupt every subsequent test that
// assumes the lenient default. The helper mirrors the
// `resetRegistryForTest` pattern in registry_test.go.
//
// The atomic-backed swap is race-free at the language level, but the
// cleanup ordering still matters: t.Cleanup runs LIFO, so nested
// subtests that flip the toggle independently each restore their own
// prior in reverse order.
func withStrictMarshal(t *testing.T, strict bool) {
	t.Helper()
	prev := SetStrictMarshal(strict)
	t.Cleanup(func() { SetStrictMarshal(prev) })
}

// TestStrictMarshal_DefaultIsOff pins the package's documented
// default. The lenient default matters because every other test in
// the suite that exercises MarshalJSON on out-of-spec data
// (TestUnknownTypeMarshalJSON_BareEmptyTypeNameStillWellFormed in
// codec_test.go is the load-bearing example) relies on it; a regress
// to a strict default would silently turn those tests red. The test
// also exercises the lenient path with an explicitly invalid
// CommonType to confirm marshal still succeeds.
func TestStrictMarshal_DefaultIsOff(t *testing.T) {
	if got := strictMarshal(); got {
		t.Fatalf("strictMarshal() at package init = true; want false (lenient default)")
	}

	// A CommonType with an empty TypeName would fail Validate
	// (type-required), but the lenient default must still produce
	// JSON.
	c := &CommonType{}
	b, err := c.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON() under lenient default = %v; want nil error", err)
	}
	if string(b) != `{"type":""}` {
		t.Errorf("MarshalJSON() = %q; want %q", string(b), `{"type":""}`)
	}
}

// TestStrictMarshal_ToggleSwapReturnsPrior pins the "returns the
// previous value" contract on [SetStrictMarshal]. The test is the
// single place that calls SetStrictMarshal directly without going
// through withStrictMarshal, because withStrictMarshal swallows the
// return value to enforce the cleanup convention.
func TestStrictMarshal_ToggleSwapReturnsPrior(t *testing.T) {
	// Capture and restore manually — we need the raw return values.
	original := SetStrictMarshal(false) // normalize to false
	t.Cleanup(func() { SetStrictMarshal(original) })

	if got := SetStrictMarshal(true); got != false {
		t.Errorf("SetStrictMarshal(true) = %v; want false (the prior value)", got)
	}
	if got := SetStrictMarshal(false); got != true {
		t.Errorf("SetStrictMarshal(false) = %v; want true (the prior value)", got)
	}
	if got := SetStrictMarshal(false); got != false {
		t.Errorf("SetStrictMarshal(false) (idempotent) = %v; want false", got)
	}
}

// TestStrictMarshal_OnFailsValidation_TypeRequired exercises the
// happy negative path: with strict on, a CommonType missing the §2
// required `type` member must fail MarshalJSON with a
// *ValidationError carrying Rule "type-required". The test asserts on
// the structured error via errors.As (not on a string message) so a
// future Reason rewording doesn't break the assertion.
func TestStrictMarshal_OnFailsValidation_TypeRequired(t *testing.T) {
	withStrictMarshal(t, true)

	c := &CommonType{} // empty TypeName triggers type-required
	b, err := c.MarshalJSON()
	if err == nil {
		t.Fatalf("MarshalJSON() under strict = (%q, nil); want *ValidationError", string(b))
	}
	if b != nil {
		t.Errorf("MarshalJSON() bytes = %q; want nil on error", string(b))
	}

	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("errors.As(err, &*ValidationError) = false; got %T", err)
	}
	if verr.Rule != "type-required" {
		t.Errorf("Rule = %q; want %q", verr.Rule, "type-required")
	}
	if !errors.Is(err, Err) {
		t.Error("errors.Is(err, Err) = false; want true (every ValidationError wraps Err)")
	}
}

// TestStrictMarshal_OnFailsValidation_LocationsURI exercises the
// other §2 rule the CommonType.Validate chain emits — bad locations
// — to confirm strict MarshalJSON surfaces non-type-required
// validation errors too. Without this coverage a regression could
// hard-code the type-required short-circuit and leave the other
// rules silently lenient.
func TestStrictMarshal_OnFailsValidation_LocationsURI(t *testing.T) {
	withStrictMarshal(t, true)

	c := &CommonType{
		TypeName:       "common",
		commonBaseline: Common{Locations: []string{"not-a-uri"}},
	}
	b, err := c.MarshalJSON()
	if err == nil {
		t.Fatalf("MarshalJSON() under strict = (%q, nil); want *ValidationError", string(b))
	}

	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("errors.As(err, &*ValidationError) = false; got %T", err)
	}
	if verr.Rule != "locations-uri" {
		t.Errorf("Rule = %q; want %q", verr.Rule, "locations-uri")
	}
}

// TestStrictMarshal_OnPassesValidation_ProducesJSON confirms the
// strict-on happy path: a fully-valid CommonType marshals to the
// same bytes as the lenient path. This is the contract that lets a
// consumer flip the toggle in init without rewriting downstream
// byte-comparison assertions.
func TestStrictMarshal_OnPassesValidation_ProducesJSON(t *testing.T) {
	c := &CommonType{
		TypeName: "common",
		commonBaseline: Common{
			Locations: []string{"https://example.com/x"},
			Actions:   []string{"read", "write"},
		},
	}

	// Capture the lenient-path bytes first while strict is still off
	// (the test relies on the default; TestStrictMarshal_DefaultIsOff
	// pins that contract).
	lenient, err := c.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON() lenient = %v; want nil", err)
	}

	withStrictMarshal(t, true)

	strict, err := c.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON() strict on valid input = %v; want nil", err)
	}
	if !bytes.Equal(strict, lenient) {
		t.Errorf("strict bytes = %q; want byte-identical to lenient %q", string(strict), string(lenient))
	}
}

// TestStrictMarshal_UnknownTypeExempt pins the documented exemption
// on [SetStrictMarshal]: the forward-compatibility carrier marshals
// successfully even under strict, because [UnknownType.Validate]
// returns nil unconditionally. A regression that wired the strict
// check into UnknownType.MarshalJSON would still pass this test
// (because Validate is a no-op), but the test pins the semantic
// promise — UnknownType is a pass-through and strict-marshal does
// not change that.
func TestStrictMarshal_UnknownTypeExempt(t *testing.T) {
	withStrictMarshal(t, true)

	cases := []struct {
		name string
		u    *UnknownType
	}{
		{name: "empty TypeName + nil Raw (synthesize path)", u: &UnknownType{}},
		{
			name: "populated Raw round-trip path",
			u: &UnknownType{
				TypeName: "org.example.unknown",
				Raw:      json.RawMessage(`{"type":"org.example.unknown","x":1}`),
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tc.u.MarshalJSON(); err != nil {
				t.Errorf("UnknownType.MarshalJSON() under strict = %v; want nil", err)
			}
		})
	}
}

// TestStrictMarshal_ConcurrentReadsSafe pins the atomic-backed-flag
// contract documented on [SetStrictMarshal]. The test is here to
// keep the race detector quiet on a pathological consumer that flips
// the flag while a marshal goroutine reads it; the test is not an
// endorsement of doing so (the godoc says don't), but the atomic
// gives correctness instead of corruption for free. Run under
// `go test -race` to exercise the assertion.
func TestStrictMarshal_ConcurrentReadsSafe(t *testing.T) {
	withStrictMarshal(t, false)

	const readers = 8
	const iters = 200

	// Two WaitGroups so the flipper can be joined separately from the
	// readers — joining everything into one WaitGroup would deadlock
	// because the flipper runs until stopFlips is set, and stopFlips
	// cannot be set until the readers have finished, which requires
	// a separate Wait on the reader group.
	var readersWG sync.WaitGroup
	var flipperWG sync.WaitGroup
	var stopFlips atomic.Bool

	for range readers {
		readersWG.Go(func() {
			c := &CommonType{TypeName: "common"}
			for range iters {
				// Each call reads strictMarshalEnabled internally via
				// strictMarshal(); the race detector watches this
				// against the flipper's Swap.
				if _, err := c.MarshalJSON(); err != nil {
					// strict==true would surface the strict error,
					// but TypeName is non-empty so Validate passes
					// either way — any error here is a bug.
					t.Errorf("MarshalJSON() under racing toggle = %v; want nil", err)
				}
			}
		})
	}

	flipperWG.Go(func() {
		for !stopFlips.Load() {
			SetStrictMarshal(true)
			SetStrictMarshal(false)
		}
	})

	readersWG.Wait()
	stopFlips.Store(true)
	flipperWG.Wait()
	// withStrictMarshal's cleanup restores the original value.
}

// TestStrictMarshal_PropagatesThroughJSONStdlibMarshal pins the
// contract that json.Marshal propagates a MarshalJSON error to its
// caller verbatim — strict-on must work the same whether the consumer
// calls c.MarshalJSON() directly or goes through json.Marshal(c).
// Without this coverage a regression could special-case the receiver
// type and accidentally lose the error through the stdlib path.
func TestStrictMarshal_PropagatesThroughJSONStdlibMarshal(t *testing.T) {
	withStrictMarshal(t, true)

	c := &CommonType{} // empty TypeName
	_, err := json.Marshal(c)
	if err == nil {
		t.Fatal("json.Marshal(badAuthDetail) under strict = nil; want validation error")
	}

	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("errors.As(err, &*ValidationError) = false; got %T (%v)", err, err)
	}
	if verr.Rule != "type-required" {
		t.Errorf("Rule = %q; want %q", verr.Rule, "type-required")
	}
}

// TestStrictMarshal_PropagatesThroughEncodeForm pins the same
// contract via the form-encoding entry point. EncodeForm delegates to
// json.Marshal on the slice, which in turn calls each element's
// MarshalJSON; the strict error must propagate end-to-end. The form
// path is independent enough from the JSON path (different entry
// point, slice marshaling, the nil-vs-empty normalization in
// EncodeForm) that a regression in either layer could swallow the
// error.
//
// EncodeForm wraps the inner json.Marshal error with %v rather than
// %w (see form.go), so the structured *ValidationError is not
// recoverable via errors.As at the EncodeForm boundary; what IS
// preserved is the umbrella sentinel [Err] via the outer %w wrap,
// plus the rule name visible in the error string. The test asserts on
// those two channels — non-nil error, errors.Is(err, Err), and the
// rule name in the message — which is the contract a consumer can
// rely on today.
func TestStrictMarshal_PropagatesThroughEncodeForm(t *testing.T) {
	withStrictMarshal(t, true)

	details := AuthorizationDetails{
		&CommonType{TypeName: "common"}, // good
		&CommonType{},                   // bad: missing type
	}
	s, err := EncodeForm(details)
	if err == nil {
		t.Fatalf("EncodeForm(badDetails) under strict = (%q, nil); want validation error", s)
	}
	if !errors.Is(err, Err) {
		t.Errorf("errors.Is(err, Err) = false; want true (every form error wraps Err)")
	}
	if !strings.Contains(err.Error(), "type-required") {
		t.Errorf("err.Error() = %q; want substring %q", err.Error(), "type-required")
	}
}
