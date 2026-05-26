// Copyright 2026 The go-rar Authors
// SPDX-License-Identifier: Apache-2.0

package rar

import (
	"bytes"
	"encoding/json"
	"sort"
	"testing"

	"github.com/hstern/go-rar/internal/specfixtures"
)

// extractAuthorizationDetailsFromFixture returns the raw bytes of the
// authorization_details member from an OAuth envelope payload (token
// response, introspection response). For payloads that are already a
// bare authorization_details array, it returns the input unchanged.
//
// The dispatch is gated on the first byte: '{' selects the envelope
// path, '[' selects the bare-array path. This is more explicit (and
// produces a clearer failure mode on a malformed fixture) than relying
// on stdlib json.Unmarshal of an envelope struct to fail against a
// bare array. The helper sibling in form_test.go takes a different
// signature (name + body, returns json.RawMessage) and is reused
// across form-encoding cases; the conformance suite walks the spec
// corpus end-to-end and needs its own narrower extractor.
func extractAuthorizationDetailsFromFixture(t *testing.T, fixture []byte) []byte {
	t.Helper()
	if len(fixture) == 0 {
		t.Fatalf("fixture is empty")
	}
	switch fixture[0] {
	case '{':
		var env struct {
			AuthorizationDetails json.RawMessage `json:"authorization_details"`
		}
		if err := json.Unmarshal(fixture, &env); err != nil {
			t.Fatalf("unmarshal envelope: %v", err)
		}
		if len(env.AuthorizationDetails) == 0 {
			t.Fatalf("envelope has no authorization_details member")
		}
		return env.AuthorizationDetails
	case '[':
		return fixture
	default:
		t.Fatalf("unrecognized fixture shape: leading byte %q", fixture[0])
		return nil
	}
}

// fixtureNames returns the keys of [specfixtures.All] in sorted order
// so subtest output is deterministic across runs. Go map iteration is
// randomized; sorting here keeps `go test -v` output diff-friendly.
func fixtureNames(fx map[string][]byte) []string {
	names := make([]string, 0, len(fx))
	for name := range fx {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// TestConformance_RoundTripByteStable asserts that every RFC 9396
// spec fixture round-trips byte-identically through
// [ParseArray] → [ValidateAll] → [json.Marshal]. This is the
// "wire-shape fidelity" guarantee the library exists to provide.
//
// The four spec-derived fixtures use `type` values
// (customer_information, account_information, payment_initiation) that
// are NOT registered — only "common" is built-in — so every parsed
// element on those payloads is an [*UnknownType] whose MarshalJSON
// emits the captured Raw bytes verbatim. That makes the spec-derived
// half of this test the canonical exercise of the forward-
// compatibility path against real spec payloads. A registered-type
// round-trip is exercised separately by codec / marshal tests using
// synthesized `type:"common"` payloads.
//
// The library-internal `empty_array` fixture parses to a *CommonType*
// (not an UnknownType) — its `type` is `common`, which IS registered.
// That makes the empty-array case the canonical exercise of the
// CommonType hand-written marshal path: the byte-stability assertion
// here catches a regression in either the nil-vs-empty preservation
// on unmarshal or the per-field elision rule on marshal.
func TestConformance_RoundTripByteStable(t *testing.T) {
	t.Parallel()
	fx := specfixtures.All()
	for _, name := range fixtureNames(fx) {
		fixture := fx[name]
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			arr := extractAuthorizationDetailsFromFixture(t, fixture)

			parsed, err := ParseArray(arr)
			if err != nil {
				t.Fatalf("ParseArray: %v", err)
			}
			if verr := ValidateAll(parsed); verr != nil {
				t.Fatalf("ValidateAll: %v", verr)
			}

			out, err := json.Marshal(parsed)
			if err != nil {
				t.Fatalf("json.Marshal: %v", err)
			}
			if !bytes.Equal(arr, out) {
				t.Fatalf("byte-stability lost\n  want: %s\n  got:  %s", arr, out)
			}
		})
	}
}

// TestConformance_ValidationFlows pins per-element invariants that the
// spec corpus must satisfy: every element advertises a non-empty
// discriminator via [AuthorizationDetail.Type], and per-element
// [AuthorizationDetail.Validate] returns nil. The first invariant
// catches a regression where a forward-compat carrier loses its
// discriminator on round-trip; the second matches the "spec example
// MUST = library Validate nil" rule the codec relies on for
// fail-fast outbound (strict-marshal opt-in).
func TestConformance_ValidationFlows(t *testing.T) {
	t.Parallel()
	fx := specfixtures.All()
	for _, name := range fixtureNames(fx) {
		fixture := fx[name]
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			arr := extractAuthorizationDetailsFromFixture(t, fixture)
			parsed, err := ParseArray(arr)
			if err != nil {
				t.Fatalf("ParseArray: %v", err)
			}
			for i, d := range parsed {
				if got := d.Type(); got == "" {
					t.Errorf("element %d: Type() returned empty string", i)
				}
				if verr := d.Validate(); verr != nil {
					t.Errorf("element %d: Validate: %v", i, verr)
				}
			}
		})
	}
}

// TestConformance_CompactInputAssumption pins that every fixture is
// already stored in stdlib-compact JSON form. The byte-stability
// assertion in [TestConformance_RoundTripByteStable] silently depends
// on this: stdlib [json.Marshal] always emits compact output, so a
// fixture stored with stray whitespace would fail round-trip for the
// wrong reason (whitespace mismatch, not a real codec regression).
// The internal/specfixtures package owns the canonical version of this
// invariant; pinning it again here keeps the conformance suite
// self-contained — a regression in fixture storage surfaces at the
// site that depends on it, not just upstream.
func TestConformance_CompactInputAssumption(t *testing.T) {
	t.Parallel()
	fx := specfixtures.All()
	for _, name := range fixtureNames(fx) {
		fixture := fx[name]
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			if err := json.Compact(&buf, fixture); err != nil {
				t.Fatalf("json.Compact: %v", err)
			}
			if !bytes.Equal(buf.Bytes(), fixture) {
				t.Fatalf("fixture is not in compact form\n  stored:  %s\n  compact: %s", fixture, buf.Bytes())
			}
		})
	}
}
