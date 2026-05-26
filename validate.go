// Copyright 2026 The go-rar Authors
// SPDX-License-Identifier: Apache-2.0

package rar

import (
	"errors"
	"fmt"
)

// ValidateAll runs [AuthorizationDetail.Validate] on each element of
// details and joins the per-element errors via [errors.Join]. If every
// element is well-formed, ValidateAll returns nil. A nil or empty
// slice also returns nil — there is nothing to validate, and no rule
// in RFC 9396 §2 says an empty array is itself ill-formed (the spec's
// only constraint on the outer array is that it MUST be present and
// MUST be a JSON array when the parameter is sent at all; whether an
// empty array is meaningful is a per-flow concern handled by the
// caller, not by per-element validation).
//
// The joined error preserves each per-element [*ValidationError] as a
// leaf in the error chain. Each leaf is wrapped in an "element N:"
// message that records the slice index of the failing element, so
// consumers can localize the violation without re-iterating the
// slice. Callers can still recover the structured rule data with
// [errors.As]:
//
//	var verr *rar.ValidationError
//	if errors.As(err, &verr) {
//	    // verr.Rule, verr.Type, verr.Reason are filled in; the
//	    // first *ValidationError in the chain is returned, which
//	    // for an errors.Join result is the first failing element.
//	}
//
// For the full set of violations, walk the chain via the multi-Unwrap
// pattern that [errors.Join] supports:
//
//	type multi interface{ Unwrap() []error }
//	if m, ok := err.(multi); ok {
//	    for _, leaf := range m.Unwrap() {
//	        // leaf is one element-N wrapper; errors.As on each leaf
//	        // recovers its *ValidationError.
//	    }
//	}
//
// The "validate every element, then join" shape — rather than
// returning on the first failure — is deliberate. When a consumer
// explicitly asks for validation (Postel's law on outbound is opt-in
// here), the useful answer is every violation in one shot, not a
// drip-feed that forces re-running validation after each fix.
func ValidateAll(details AuthorizationDetails) error {
	var errs []error
	for i, d := range details {
		if err := d.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("element %d: %w", i, err))
		}
	}
	return errors.Join(errs...)
}
