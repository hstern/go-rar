// Copyright 2026 The go-rar Authors
// SPDX-License-Identifier: Apache-2.0

package rar

import (
	"errors"
	"fmt"
	"net/url"
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

// validateLocations runs the RFC 9396 §2 URI well-formedness check on
// each entry of a [Common.Locations] slice. Per §2, every location is a
// URI identifying the resource the authorization applies to; the
// library treats "URI" the way RFC 3986 does — a syntactically valid
// reference that carries a scheme (i.e. an absolute URI), so that the
// authorization server can route requests to the named resource server
// without out-of-band base-URI context.
//
// The check is two-step per element:
//
//  1. [net/url.Parse] must succeed. URL.Parse is permissive — it
//     accepts both absolute URIs and relative references — so a
//     parse error here signals a value that is not even a valid
//     reference, which RFC 9396 §2 does not permit.
//  2. The parsed reference must carry a non-empty Scheme. RFC 3986
//     §4.3 defines an absolute URI as one with a scheme; without it
//     a value like "/some/path" or "example.com/x" is a relative
//     reference and cannot stand alone as a location identifier.
//
// An empty string ("") fails the second check (the empty value
// trivially has no scheme) and is reported with the same
// "locations-uri" rule rather than a separate empty-element rule:
// empty entries inside Locations are never legal URIs and the unified
// rule keeps the helper composition clean. As a corollary, callers do
// not need to (and should not) run [validateNonEmptyStrings] over
// Locations — this helper subsumes that check for the URI case.
//
// A nil or empty slice iterates zero times and returns nil: the
// "locations URIs MUST be absolute" rule only applies to entries that
// are present.
//
// The first failing entry produces a [*ValidationError] with Rule
// "locations-uri", Type set to the caller-supplied typeName (so that
// downstream log filters can group violations by discriminator), and
// Reason naming the offending index together with the parse error or
// the missing-scheme detail. Subsequent entries are not inspected —
// per-element validation is opt-in via [ValidateAll], not per-field.
//
// validateLocations is unexported by design. It is shared infrastructure
// for per-type Validate() implementations (CommonType today, future
// registered types tomorrow); it is not part of the library's public
// surface and may grow optional knobs in a Go-minor release without
// breaking external callers.
func validateLocations(typeName string, locations []string) error {
	for i, loc := range locations {
		u, err := url.Parse(loc)
		if err != nil {
			return &ValidationError{
				Rule:   "locations-uri",
				Type:   typeName,
				Reason: fmt.Sprintf("locations[%d] %q: %v", i, loc, err),
			}
		}
		if u.Scheme == "" {
			return &ValidationError{
				Rule:   "locations-uri",
				Type:   typeName,
				Reason: fmt.Sprintf("locations[%d] %q is not an absolute URI (missing scheme)", i, loc),
			}
		}
	}
	return nil
}

// validateNonEmptyStrings rejects empty-string elements in any of the
// RFC 9396 §2 array-of-strings members — actions, datatypes, and
// privileges. The spec text in §2 does not literally say each element
// must be non-empty, but an empty string in a permission-like field
// carries no meaning and is almost always a producer bug: a missing
// value the producer mis-encoded as "" rather than omitting the entry
// or omitting the whole array. The library's opt-in Validate posture
// (Postel's law on the wire, strictness on demand) is the right place
// to surface that bug; the marshal path remains lenient so existing
// integrations are not broken by an upgrade.
//
// The caller supplies the field's spec-level name ("actions",
// "datatypes", or "privileges") as the field argument; that name is
// used both to construct the Rule identifier ("<field>-element-empty")
// and in the Reason text. Using a single helper across the three §2
// arrays keeps the rule taxonomy parallel — one suffix, three field
// prefixes — rather than fanning out into three near-identical helpers.
//
// A nil or empty slice iterates zero times and returns nil. The first
// empty element returns a [*ValidationError] with Type set to the
// caller-supplied typeName (so violations can be grouped by
// discriminator) and Reason naming the offending index; subsequent
// elements are not inspected.
//
// validateNonEmptyStrings is unexported for the same reasons given on
// [validateLocations] — shared infrastructure for per-type Validate()
// implementations, not part of the library's public surface.
func validateNonEmptyStrings(typeName, field string, ss []string) error {
	for i, s := range ss {
		if s == "" {
			return &ValidationError{
				Rule:   field + "-element-empty",
				Type:   typeName,
				Reason: fmt.Sprintf("%s[%d] is empty", field, i),
			}
		}
	}
	return nil
}
