// Copyright 2026 The go-rar Authors
// SPDX-License-Identifier: Apache-2.0

package rar

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestCommonType_Validate_TypeRequired_Happy(t *testing.T) {
	c := &CommonType{TypeName: "common"}
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate() error = %v; want nil", err)
	}
}

func TestCommonType_Validate_TypeRequired_Negative(t *testing.T) {
	c := &CommonType{} // empty TypeName

	err := c.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil; want *ValidationError")
	}

	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("errors.As(err, &*ValidationError) = false; got %T", err)
	}
	if verr.Rule != "type-required" {
		t.Errorf("Rule = %q; want %q", verr.Rule, "type-required")
	}
	if verr.Type != "" {
		t.Errorf("Type = %q; want empty string", verr.Type)
	}
	if verr.Reason == "" {
		t.Error("Reason is empty; want a human-readable explanation")
	}

	// Categorical umbrella match: every validation error wraps Err.
	if !errors.Is(err, Err) {
		t.Error("errors.Is(err, Err) = false; want true")
	}
}

func TestUnknownType_Validate_AlwaysNil(t *testing.T) {
	cases := []struct {
		name string
		u    *UnknownType
	}{
		{
			name: "zero-value",
			u:    &UnknownType{},
		},
		{
			name: "populated",
			u: &UnknownType{
				TypeName: "org.example.unknown",
				Raw:      json.RawMessage(`{"type":"org.example.unknown","x":1}`),
			},
		},
		{
			name: "garbage-raw",
			u: &UnknownType{
				TypeName: "org.example.unknown",
				// Intentionally not valid JSON — Validate must not
				// inspect Raw at all for an unknown type.
				Raw: json.RawMessage(`not even close to JSON`),
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.u.Validate(); err != nil {
				t.Fatalf("Validate() error = %v; want nil", err)
			}
		})
	}
}

func TestValidateAll_HappyPath(t *testing.T) {
	details := AuthorizationDetails{
		&CommonType{TypeName: "common"},
		&CommonType{TypeName: "common"},
		&CommonType{TypeName: "common"},
	}
	if err := ValidateAll(details); err != nil {
		t.Fatalf("ValidateAll(...) error = %v; want nil", err)
	}
}

func TestValidateAll_EmptyAndNilSlices(t *testing.T) {
	if err := ValidateAll(nil); err != nil {
		t.Errorf("ValidateAll(nil) error = %v; want nil", err)
	}
	if err := ValidateAll(AuthorizationDetails{}); err != nil {
		t.Errorf("ValidateAll(empty) error = %v; want nil", err)
	}
}

func TestValidateAll_MultipleViolations(t *testing.T) {
	details := AuthorizationDetails{
		&CommonType{TypeName: "common"}, // good
		&CommonType{},                   // bad: missing type
		&CommonType{},                   // bad: missing type
	}

	err := ValidateAll(details)
	if err == nil {
		t.Fatal("ValidateAll(...) error = nil; want joined error")
	}

	type multiUnwrap interface{ Unwrap() []error }
	m, ok := err.(multiUnwrap)
	if !ok {
		t.Fatalf("joined error %T does not implement Unwrap() []error", err)
	}
	leaves := m.Unwrap()
	if got, want := len(leaves), 2; got != want {
		t.Fatalf("len(Unwrap()) = %d; want %d", got, want)
	}

	// Each leaf must carry an element-index prefix and unwrap to a
	// *ValidationError with the type-required rule.
	wantPrefixes := []string{"element 1:", "element 2:"}
	for i, leaf := range leaves {
		if !strings.Contains(leaf.Error(), wantPrefixes[i]) {
			t.Errorf("leaf %d = %q; want prefix %q", i, leaf.Error(), wantPrefixes[i])
		}
		var verr *ValidationError
		if !errors.As(leaf, &verr) {
			t.Errorf("leaf %d: errors.As(&*ValidationError) = false; got %T", i, leaf)
			continue
		}
		if verr.Rule != "type-required" {
			t.Errorf("leaf %d: Rule = %q; want %q", i, verr.Rule, "type-required")
		}
	}
}

// TestCommonType_Validate_Table exercises every rule CommonType.Validate
// can emit, both happy and negative, in one place. Each case asserts
// either nil or a specific Rule on the recovered [*ValidationError]; for
// the locations-uri cases that name a specific offending index in the
// Reason, the case also asserts on a Reason substring so the index is
// pinned (otherwise a future refactor could silently swap the offending
// entry without breaking the test).
func TestCommonType_Validate_Table(t *testing.T) {
	cases := []struct {
		name          string
		in            *CommonType
		wantRule      string // "" means want nil error
		wantReasonSub string // optional substring assertion on Reason
	}{
		{
			name: "locations-uri happy https+urn",
			in: &CommonType{
				TypeName:       "common",
				commonBaseline: Common{Locations: []string{"https://example.com/x", "urn:foo:bar"}},
			},
		},
		{
			name: "locations-uri happy http",
			in: &CommonType{
				TypeName:       "common",
				commonBaseline: Common{Locations: []string{"http://example.com"}},
			},
		},
		{
			name: "locations-uri missing scheme",
			in: &CommonType{
				TypeName:       "common",
				commonBaseline: Common{Locations: []string{"example.com/x"}},
			},
			wantRule:      "locations-uri",
			wantReasonSub: `locations[0] "example.com/x"`,
		},
		{
			name: "locations-uri empty entry",
			in: &CommonType{
				TypeName:       "common",
				commonBaseline: Common{Locations: []string{""}},
			},
			wantRule:      "locations-uri",
			wantReasonSub: "locations[0]",
		},
		{
			name: "locations-uri relative path",
			in: &CommonType{
				TypeName:       "common",
				commonBaseline: Common{Locations: []string{"/just/a/path"}},
			},
			wantRule:      "locations-uri",
			wantReasonSub: "locations[0]",
		},
		{
			name: "locations-uri valid then invalid; index 1 reported",
			in: &CommonType{
				TypeName:       "common",
				commonBaseline: Common{Locations: []string{"https://ok", "broken"}},
			},
			wantRule:      "locations-uri",
			wantReasonSub: `locations[1] "broken"`,
		},
		{
			name: "locations-uri unparseable hits parse-error path",
			in: &CommonType{
				TypeName:       "common",
				commonBaseline: Common{Locations: []string{"://no-scheme"}},
			},
			wantRule:      "locations-uri",
			wantReasonSub: "missing protocol scheme",
		},
		{
			name: "actions-element-empty at index 1",
			in: &CommonType{
				TypeName:       "common",
				commonBaseline: Common{Actions: []string{"read", "", "write"}},
			},
			wantRule:      "actions-element-empty",
			wantReasonSub: "actions[1]",
		},
		{
			name: "datatypes-element-empty at index 1",
			in: &CommonType{
				TypeName:       "common",
				commonBaseline: Common{Datatypes: []string{"contacts", ""}},
			},
			wantRule:      "datatypes-element-empty",
			wantReasonSub: "datatypes[1]",
		},
		{
			name: "privileges-element-empty at index 0",
			in: &CommonType{
				TypeName:       "common",
				commonBaseline: Common{Privileges: []string{""}},
			},
			wantRule:      "privileges-element-empty",
			wantReasonSub: "privileges[0]",
		},
		{
			name: "actions happy",
			in: &CommonType{
				TypeName:       "common",
				commonBaseline: Common{Actions: []string{"read", "write"}},
			},
		},
		{
			name: "identifier empty is allowed (free-form, no rule)",
			in: &CommonType{
				TypeName:       "common",
				commonBaseline: Common{Identifier: ""},
			},
		},
		{
			name: "identifier free-form value is allowed",
			in: &CommonType{
				TypeName:       "common",
				commonBaseline: Common{Identifier: "anything"},
			},
		},
		{
			name: "first-failure wins: empty TypeName beats bad locations and empty actions",
			in: &CommonType{
				// TypeName intentionally empty so the type-required
				// rule fires before locations/actions get inspected.
				commonBaseline: Common{
					Locations: []string{"not-a-uri"},
					Actions:   []string{""},
				},
			},
			wantRule: "type-required",
		},
		{
			name: "nil/empty arrays validate clean",
			in: &CommonType{
				TypeName: "common",
				// Locations, Actions, Datatypes, Privileges all nil —
				// no rule fires for absent fields.
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.in.Validate()

			if tc.wantRule == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v; want nil", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("Validate() error = nil; want *ValidationError with Rule %q", tc.wantRule)
			}
			var verr *ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("errors.As(err, &*ValidationError) = false; got %T", err)
			}
			if verr.Rule != tc.wantRule {
				t.Errorf("Rule = %q; want %q", verr.Rule, tc.wantRule)
			}
			if tc.wantReasonSub != "" && !strings.Contains(verr.Reason, tc.wantReasonSub) {
				t.Errorf("Reason = %q; want substring %q", verr.Reason, tc.wantReasonSub)
			}
			// Categorical umbrella match holds for every validation error.
			if !errors.Is(err, Err) {
				t.Error("errors.Is(err, Err) = false; want true")
			}
		})
	}
}

// TestValidateLocations_Direct exercises the helper without going
// through CommonType.Validate, so a future refactor of the wiring
// does not silently lose helper-level coverage. The helper's contract
// (rule name, type-name pass-through, index in Reason) is part of the
// shared validation infrastructure that future per-type Validate
// implementations will rely on.
func TestValidateLocations_Direct(t *testing.T) {
	t.Run("nil slice is nil error", func(t *testing.T) {
		if err := validateLocations("payment_initiation", nil); err != nil {
			t.Fatalf("validateLocations(nil) = %v; want nil", err)
		}
	})

	t.Run("typeName flows through to ValidationError.Type", func(t *testing.T) {
		err := validateLocations("payment_initiation", []string{"not-a-uri"})
		var verr *ValidationError
		if !errors.As(err, &verr) {
			t.Fatalf("errors.As = false; got %T", err)
		}
		if verr.Rule != "locations-uri" {
			t.Errorf("Rule = %q; want %q", verr.Rule, "locations-uri")
		}
		if verr.Type != "payment_initiation" {
			t.Errorf("Type = %q; want %q", verr.Type, "payment_initiation")
		}
	})

	t.Run("reports first failing index", func(t *testing.T) {
		err := validateLocations("x", []string{"https://ok1", "https://ok2", "bad"})
		var verr *ValidationError
		if !errors.As(err, &verr) {
			t.Fatalf("errors.As = false; got %T", err)
		}
		if !strings.Contains(verr.Reason, "locations[2]") {
			t.Errorf("Reason = %q; want substring %q", verr.Reason, "locations[2]")
		}
	})
}

// TestValidateNonEmptyStrings_Direct exercises the helper without
// going through CommonType.Validate; see TestValidateLocations_Direct
// for the rationale.
func TestValidateNonEmptyStrings_Direct(t *testing.T) {
	t.Run("nil slice is nil error", func(t *testing.T) {
		if err := validateNonEmptyStrings("common", "actions", nil); err != nil {
			t.Fatalf("validateNonEmptyStrings(nil) = %v; want nil", err)
		}
	})

	t.Run("field name flows into Rule and Reason", func(t *testing.T) {
		err := validateNonEmptyStrings("common", "datatypes", []string{"ok", ""})
		var verr *ValidationError
		if !errors.As(err, &verr) {
			t.Fatalf("errors.As = false; got %T", err)
		}
		if verr.Rule != "datatypes-element-empty" {
			t.Errorf("Rule = %q; want %q", verr.Rule, "datatypes-element-empty")
		}
		if !strings.Contains(verr.Reason, "datatypes[1]") {
			t.Errorf("Reason = %q; want substring %q", verr.Reason, "datatypes[1]")
		}
		if verr.Type != "common" {
			t.Errorf("Type = %q; want %q", verr.Type, "common")
		}
	})

	t.Run("all non-empty is nil error", func(t *testing.T) {
		if err := validateNonEmptyStrings("common", "privileges", []string{"a", "b", "c"}); err != nil {
			t.Fatalf("validateNonEmptyStrings(all non-empty) = %v; want nil", err)
		}
	})
}

func TestValidateAll_PreservesValidationErrorAccess(t *testing.T) {
	details := AuthorizationDetails{
		&CommonType{TypeName: "common"},
		&CommonType{}, // bad
	}

	err := ValidateAll(details)
	if err == nil {
		t.Fatal("ValidateAll(...) error = nil; want joined error")
	}

	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("errors.As(joined, &*ValidationError) = false; got %T", err)
	}
	if verr.Rule != "type-required" {
		t.Errorf("Rule = %q; want %q", verr.Rule, "type-required")
	}

	// Umbrella sentinel still matches through the joined chain.
	if !errors.Is(err, Err) {
		t.Error("errors.Is(joined, Err) = false; want true")
	}
}
