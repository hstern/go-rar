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
