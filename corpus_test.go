// Copyright 2026 The go-rar Authors
// SPDX-License-Identifier: Apache-2.0

// Package corpus test — phase 4 acceptance gate.
//
// This file is the single iterable artifact that demonstrates the
// library's RFC 9396 §2 validation surface is complete: every spec
// MUST that the library enforces as a Validate rule appears here with
// both a passing positive fixture and a failing negative fixture, and
// every negative fixture is shown to fail-fast under StrictMarshal.
//
// Rules covered (one happy + one negative each):
//
//   - "type-required"           RFC 9396 §2: `type` member must be present and non-empty
//   - "locations-uri"           RFC 9396 §2 + RFC 3986: locations entries are absolute URIs
//   - "actions-element-empty"   library opinion: empty action string is a producer bug
//   - "datatypes-element-empty" library opinion: empty datatypes string is a producer bug
//   - "privileges-element-empty" library opinion: empty privileges string is a producer bug
//
// Structural invariants restated from phase 3 (codec contracts, not
// Validate rules — covered as separate dedicated tests so that the
// phase-4 corpus also pins the codec-side spec obligations):
//
//   - `type` member is first in marshal output (byte-stability).
//   - Unknown wire fields are silently dropped on unmarshal (Postel's law).
//
// Existing per-rule coverage in validate_test.go (RAR-16/17) and
// strict_test.go (RAR-18) is intentionally NOT replaced; the corpus
// here organizes the gate-relevant cases as one structure so the
// phase-4 reviewer can read "every MUST has happy + negative + strict
// fail-fast" off a single table without re-deriving it from the
// scattered tests.

package rar

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
)

// ruleCase pairs a rule name with a happy-path and negative-path
// fixture exercising that rule. The corpus iterates over the slice
// and asserts, in three separate test functions:
//
//   - Validate() returns nil on happy.
//   - Validate() returns *ValidationError{Rule: rule} on negative.
//   - With StrictMarshal(true), json.Marshal(negative) returns the
//     same *ValidationError.
//   - With StrictMarshal(false) (default), json.Marshal(negative)
//     succeeds and produces some JSON.
type ruleCase struct {
	rule     string
	happy    *CommonType
	negative *CommonType
}

// validationCorpus is the gate artifact: one entry per RFC 9396 §2
// MUST that the library enforces as a Validate rule. Adding a new
// rule means adding a row here; the phase-4 reviewer reads down this
// slice to confirm gate satisfaction.
var validationCorpus = []ruleCase{
	{
		rule:     "type-required",
		happy:    &CommonType{TypeName: "common"},
		negative: &CommonType{TypeName: ""},
	},
	{
		rule: "locations-uri",
		happy: &CommonType{
			TypeName:       "common",
			commonBaseline: Common{Locations: []string{"https://rs.example.com/x"}},
		},
		negative: &CommonType{
			TypeName:       "common",
			commonBaseline: Common{Locations: []string{"not-a-uri"}},
		},
	},
	{
		rule: "actions-element-empty",
		happy: &CommonType{
			TypeName:       "common",
			commonBaseline: Common{Actions: []string{"read"}},
		},
		negative: &CommonType{
			TypeName:       "common",
			commonBaseline: Common{Actions: []string{"read", ""}},
		},
	},
	{
		rule: "datatypes-element-empty",
		happy: &CommonType{
			TypeName:       "common",
			commonBaseline: Common{Datatypes: []string{"contacts"}},
		},
		negative: &CommonType{
			TypeName:       "common",
			commonBaseline: Common{Datatypes: []string{""}},
		},
	},
	{
		rule: "privileges-element-empty",
		happy: &CommonType{
			TypeName:       "common",
			commonBaseline: Common{Privileges: []string{"admin"}},
		},
		negative: &CommonType{
			TypeName:       "common",
			commonBaseline: Common{Privileges: []string{""}},
		},
	},
}

// TestValidationCorpus_HappyPaths walks the corpus and asserts that
// every happy fixture validates clean. A regression that tightens a
// rule beyond what the spec actually requires would surface here as
// a previously-valid fixture starting to fail.
func TestValidationCorpus_HappyPaths(t *testing.T) {
	for _, tc := range validationCorpus {
		t.Run(tc.rule, func(t *testing.T) {
			if err := tc.happy.Validate(); err != nil {
				t.Fatalf("Validate() on happy fixture = %v; want nil", err)
			}
		})
	}
}

// TestValidationCorpus_NegativePaths walks the corpus and asserts that
// every negative fixture fails Validate with a *ValidationError whose
// Rule matches the row's rule name. The umbrella errors.Is(err, Err)
// match is also asserted because every ValidationError must wrap the
// package-wide sentinel (see errors.go).
//
// This is the "every spec MUST is covered" half of the phase-4 gate:
// a new rule added to validate.go without a corresponding corpus row
// would still pass its own dedicated test but would not show up here,
// which is the cue for the reviewer.
func TestValidationCorpus_NegativePaths(t *testing.T) {
	for _, tc := range validationCorpus {
		t.Run(tc.rule, func(t *testing.T) {
			err := tc.negative.Validate()
			if err == nil {
				t.Fatalf("Validate() on negative fixture = nil; want *ValidationError with Rule %q", tc.rule)
			}

			var verr *ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("errors.As(err, &*ValidationError) = false; got %T", err)
			}
			if verr.Rule != tc.rule {
				t.Errorf("Rule = %q; want %q", verr.Rule, tc.rule)
			}
			if !errors.Is(err, Err) {
				t.Error("errors.Is(err, Err) = false; want true (every ValidationError wraps Err)")
			}
		})
	}
}

// TestValidationCorpus_StrictMarshalFailsFast is the "strict marshal
// correctly fails-fast on every negative" half of the phase-4 gate.
// For each negative fixture: flip StrictMarshal on, marshal through
// the stdlib json.Marshal entry point (NOT the receiver's MarshalJSON
// directly — the stdlib path is the contract consumers actually use),
// and confirm the recovered *ValidationError carries the row's rule.
//
// The withStrictMarshal helper (defined in strict_test.go from
// RAR-18) handles the package-global cleanup so this test composes
// with -shuffle and -count without leaking state into siblings.
func TestValidationCorpus_StrictMarshalFailsFast(t *testing.T) {
	withStrictMarshal(t, true)

	for _, tc := range validationCorpus {
		t.Run(tc.rule, func(t *testing.T) {
			b, err := json.Marshal(tc.negative)
			if err == nil {
				t.Fatalf("json.Marshal(negative) under strict = (%q, nil); want *ValidationError", string(b))
			}

			var verr *ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("errors.As(err, &*ValidationError) = false; got %T (%v)", err, err)
			}
			if verr.Rule != tc.rule {
				t.Errorf("Rule = %q; want %q", verr.Rule, tc.rule)
			}
		})
	}
}

// TestValidationCorpus_LenientMarshalSucceeds pins the contract that
// the default lenient posture survives every rule-violating fixture
// in the corpus: json.Marshal succeeds on each negative and produces
// some JSON. The byte content is intentionally NOT asserted — the
// point is that lenient marshal is the documented default and the
// outbound path stays usable even on inputs that would fail Validate.
// A regression flipping the default to strict, or wiring strict into
// a path that should be lenient, surfaces here.
func TestValidationCorpus_LenientMarshalSucceeds(t *testing.T) {
	// Explicitly normalize to off via the helper rather than relying
	// on whatever a sibling test left the package global at — the
	// race detector is fine either way but -shuffle could otherwise
	// make this test's behavior depend on test ordering.
	withStrictMarshal(t, false)

	for _, tc := range validationCorpus {
		t.Run(tc.rule, func(t *testing.T) {
			b, err := json.Marshal(tc.negative)
			if err != nil {
				t.Fatalf("json.Marshal(negative) under lenient default = %v; want nil error", err)
			}
			if len(b) == 0 {
				t.Errorf("json.Marshal(negative) = empty bytes; want some JSON")
			}
		})
	}
}

// TestStructural_TypeFirstInMarshalOutput restates the phase-3
// byte-stability contract (codec.go's MarshalJSON puts `type` first;
// see design-decisions.md and the MarshalJSON godoc) as part of the
// phase-4 corpus. The phase gate's "every spec MUST has coverage"
// criterion includes the §2-§9 example wire-shape obligation that
// the discriminator leads — this test pins that obligation regardless
// of how the codec is later refactored.
func TestStructural_TypeFirstInMarshalOutput(t *testing.T) {
	cases := []struct {
		name string
		in   *CommonType
	}{
		{
			name: "type only",
			in:   &CommonType{TypeName: "common"},
		},
		{
			name: "type plus locations",
			in: &CommonType{
				TypeName:       "common",
				commonBaseline: Common{Locations: []string{"https://rs.example.com/x"}},
			},
		},
		{
			name: "type plus every baseline member populated",
			in: &CommonType{
				TypeName: "common",
				commonBaseline: Common{
					Locations:  []string{"https://rs.example.com/x"},
					Actions:    []string{"read", "write"},
					Datatypes:  []string{"contacts"},
					Identifier: "rs-1",
					Privileges: []string{"admin"},
				},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := json.Marshal(tc.in)
			if err != nil {
				t.Fatalf("json.Marshal() = %v; want nil", err)
			}
			if !bytes.HasPrefix(b, []byte(`{"type":`)) {
				t.Errorf("marshal output = %q; want it to start with %q", string(b), `{"type":`)
			}
		})
	}
}

// TestStructural_UnknownFieldsDroppedOnUnmarshal restates the
// lenient-unmarshal (Postel's law) contract from phase 3 — see the
// UnmarshalJSON godoc on CommonType in codec.go. A wire payload
// carrying members outside the §2 baseline parses clean; the unknown
// members are silently dropped. The "dropped" assertion is verified
// by remarshaling the parsed value and confirming the unknown
// members do not appear in the round-trip output.
func TestStructural_UnknownFieldsDroppedOnUnmarshal(t *testing.T) {
	raw := []byte(`{"type":"common","locations":["https://x"],"this_field":"ignored","extra":{"obj":"too"}}`)

	d, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse() = %v; want nil", err)
	}
	c, ok := d.(*CommonType)
	if !ok {
		t.Fatalf("Parse() returned %T; want *CommonType", d)
	}
	if c.TypeName != "common" {
		t.Errorf("TypeName = %q; want %q", c.TypeName, "common")
	}
	if got, want := c.Locations, []string{"https://x"}; len(got) != len(want) || got[0] != want[0] {
		t.Errorf("Locations = %v; want %v", got, want)
	}

	// The "unknown members are dropped" guarantee is observable
	// through round-trip: a remarshal must not reintroduce them.
	out, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("json.Marshal(parsed) = %v; want nil", err)
	}
	if bytes.Contains(out, []byte("this_field")) {
		t.Errorf("round-trip output = %q; must not contain dropped member %q", string(out), "this_field")
	}
	if bytes.Contains(out, []byte("extra")) {
		t.Errorf("round-trip output = %q; must not contain dropped member %q", string(out), "extra")
	}
}
