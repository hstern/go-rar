// Copyright 2026 The go-rar Authors
// SPDX-License-Identifier: Apache-2.0

package rar

import (
	"errors"
	"fmt"
)

// Err is the top-level umbrella sentinel for every error this
// package returns. The per-rule [ValidationError] struct values and
// the [ErrTypeReserved] sentinel all match errors.Is(err, rar.Err).
//
// Use this when handling errors from multiple sources to branch on
// "did this come from rar?" without naming each leaf sentinel:
//
//	if errors.Is(err, rar.Err) {
//	    // handle as a go-rar codec/validation failure
//	} else {
//	    // unrelated to go-rar
//	}
var Err = errors.New("rar")

// ErrTypeReserved is returned by [RegisterType] when a caller
// attempts to register a constructor for a `type` value that the
// library has already populated as a built-in.
//
// RFC 9396 deliberately defines no built-in type values of its own;
// the only name this library reserves is "common", the §2-only
// carrier registered at package init. Every other type — including
// IANA-published values like "payment_initiation" — is open for
// consumer registration. Callers that want to detect the collision
// case can branch with errors.Is:
//
//	if err := rar.RegisterType("common", ctor); errors.Is(err, rar.ErrTypeReserved) {
//	    // expected — "common" is the built-in
//	}
//
// ErrTypeReserved wraps [Err] so categorical errors.Is(err, rar.Err)
// also matches.
var ErrTypeReserved = fmt.Errorf("%w: type name is reserved for a built-in", Err)

// ValidationError reports a single RFC 9396 well-formedness rule
// violation found by [AuthorizationDetail.Validate] (or by the
// codec when [SetStrictMarshal] is enabled).
//
// Fields:
//
//   - Rule names the violated rule using a short stable identifier
//     suitable for programmatic dispatch — for example
//     "type-required", "locations-uri", "actions-non-empty". Rule
//     identifiers are part of the library's public contract; new
//     identifiers may be added in a Go-minor release but existing
//     ones do not change spelling.
//
//   - Type is the offending element's discriminator value (the
//     spec's `type` member). For the "type-required" rule, when the
//     wire shape did not carry a discriminator at all, Type is the
//     empty string.
//
//   - Reason is a human-readable explanation suitable for log lines
//     or developer-facing error messages. It is NOT part of the
//     programmatic contract; do not match on its text.
//
// Recover the structured fields with errors.As:
//
//	var verr *rar.ValidationError
//	if errors.As(err, &verr) {
//	    log.Printf("rule=%s type=%q reason=%s", verr.Rule, verr.Type, verr.Reason)
//	}
//
// ValidationError also matches the package umbrella, so
// errors.Is(err, rar.Err) is true for any ValidationError.
type ValidationError struct {
	// Rule is the short stable identifier of the violated rule.
	Rule string
	// Type is the offending element's `type` discriminator value,
	// or the empty string when the discriminator itself is missing.
	Type string
	// Reason is a human-readable detail. Not part of the
	// programmatic contract; safe to change wording across releases.
	Reason string
}

// Error implements the error interface. The format is stable enough
// to grep for in logs but the Reason text is not part of the
// programmatic contract — branch on Rule (or use errors.As to recover
// the struct) rather than matching on the formatted string.
func (e *ValidationError) Error() string {
	switch {
	case e == nil:
		// Defensive: a nil *ValidationError is not a valid error
		// value, but Go's typed-nil pitfall means it can reach
		// Error() if a caller forgets the nil-check pattern.
		// Return a recognizable placeholder rather than panicking.
		return "rar: <nil ValidationError>"
	case e.Type == "":
		return fmt.Sprintf("rar: %s: %s", e.Rule, e.Reason)
	default:
		return fmt.Sprintf("rar: %s: type=%q: %s", e.Rule, e.Type, e.Reason)
	}
}

// Is reports whether the target is the package umbrella [Err] or
// another *ValidationError. Pointer identity is not required: any
// non-nil *ValidationError matches another *ValidationError target
// for categorical "is this a validation error?" checks. To branch on
// a specific rule, compare the Rule field after recovering the
// struct with errors.As.
func (e *ValidationError) Is(target error) bool {
	if target == Err {
		return true
	}
	_, ok := target.(*ValidationError)
	return ok
}
