// Copyright 2026 The go-rar Authors
// SPDX-License-Identifier: Apache-2.0

package rar

import (
	"encoding/json"
	"fmt"
)

// Parse decodes a single authorization_details element into the
// concrete [AuthorizationDetail] the registry dispatches to from the
// wire `type` discriminator.
//
// The raw bytes MUST be a JSON object (RFC 9396 §2). The object's
// `type` member selects the concrete carrier:
//
//   - If the registry has a constructor for the `type` value (either
//     a built-in or a consumer registration installed via
//     [RegisterType]), Parse calls it and delegates the field decode
//     to the returned value's UnmarshalJSON. The returned interface
//     wraps that concrete type.
//
//   - If no constructor matches, Parse returns an [*UnknownType]
//     carrying the discriminator and the entire JSON object verbatim
//     in its Raw field. This is the forward-compatibility path the
//     library's sealed-interface posture relies on (see [UnknownType]
//     for the rationale) — unknown types are NOT an error.
//
//   - If the JSON does not parse, Parse returns a wrapped stdlib
//     [encoding/json] error matching errors.Is(err, [Err]).
//
//   - If the `type` member is missing or empty, Parse returns a
//     [*ValidationError] with Rule "type-required" (which also
//     matches errors.Is(err, [Err])).
//
// Parse is the discriminator-aware entry point for a single element;
// callers parsing the wire array should use [ParseArray] instead.
func Parse(raw json.RawMessage) (AuthorizationDetail, error) {
	// Peek the discriminator without committing to a typed struct.
	// A minimal probe is cheap and avoids double-decoding the typed
	// fields — the registry-selected ctor's UnmarshalJSON does that
	// once below on the original bytes.
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, fmt.Errorf("%w: parse authorization_detail: %v", Err, err)
	}
	if probe.Type == "" {
		return nil, &ValidationError{
			Rule:   "type-required",
			Reason: "type member missing or empty",
		}
	}

	// Default to the forward-compat carrier; the registry hit
	// overrides when the discriminator is known.
	var dst AuthorizationDetail = &UnknownType{}
	if ctor := lookup(probe.Type); ctor != nil {
		dst = ctor()
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return nil, fmt.Errorf("%w: parse %q: %v", Err, probe.Type, err)
	}
	return dst, nil
}

// ParseArray decodes the JSON array form of the authorization_details
// parameter (RFC 9396 §2). Each element is dispatched through [Parse]
// in order.
//
// Per-element errors halt the parse — ParseArray does not partial-
// return. The first element that fails is identified by its zero-based
// index in the wrapping error chain; the underlying [*ValidationError]
// (or wrapped stdlib JSON error) is reachable with errors.As.
//
// A successful parse returns an [AuthorizationDetails] slice of the
// same length as the input array, with each entry holding the concrete
// carrier the registry selected (or an [*UnknownType] for unrecognized
// discriminators).
func ParseArray(raw json.RawMessage) (AuthorizationDetails, error) {
	var elems []json.RawMessage
	if err := json.Unmarshal(raw, &elems); err != nil {
		return nil, fmt.Errorf("%w: parse authorization_details array: %v", Err, err)
	}
	out := make(AuthorizationDetails, 0, len(elems))
	for i, e := range elems {
		d, err := Parse(e)
		if err != nil {
			return nil, fmt.Errorf("element %d: %w", i, err)
		}
		out = append(out, d)
	}
	return out, nil
}

// UnmarshalJSON populates the receiver from the wire object: the
// `type` discriminator lands in TypeName, and the RFC 9396 §2 baseline
// members land in the embedded [Common]. Members outside that set are
// silently dropped — the lenient-unmarshal posture (Postel's law)
// means a payload with extra fields parses cleanly, leaving any
// strict-shape enforcement to the opt-in [CommonType.Validate].
//
// UnmarshalJSON does NOT enforce the "type non-empty" rule. That check
// lives in [Parse] (which is called before dispatch reaches here) and
// in the validation phase via the "type-required" rule on
// [ValidationError]; UnmarshalJSON's job is to populate fields from
// whatever bytes the wire produced, not to gate on them.
//
// The aux struct mirrors CommonType's wire shape — `type` plus the
// five §2 baseline members — and is the standard pattern for
// implementing UnmarshalJSON on a type that embeds another struct
// without triggering infinite recursion (calling json.Unmarshal on a
// *CommonType from inside *CommonType.UnmarshalJSON would re-enter
// this method).
func (c *CommonType) UnmarshalJSON(b []byte) error {
	var aux struct {
		Type       string   `json:"type"`
		Locations  []string `json:"locations,omitempty"`
		Actions    []string `json:"actions,omitempty"`
		Datatypes  []string `json:"datatypes,omitempty"`
		Identifier string   `json:"identifier,omitempty"`
		Privileges []string `json:"privileges,omitempty"`
	}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	c.TypeName = aux.Type
	c.Locations = aux.Locations
	c.Actions = aux.Actions
	c.Datatypes = aux.Datatypes
	c.Identifier = aux.Identifier
	c.Privileges = aux.Privileges
	return nil
}

// UnmarshalJSON captures the entire JSON object verbatim into Raw and
// surfaces the `type` discriminator on TypeName. The verbatim capture
// is what makes [UnknownType] safe to round-trip — the inverse
// MarshalJSON (landing in a later commit) emits Raw byte-for-byte, so
// an unrecognized type flows through the library without reordering
// keys, dropping members, or normalizing whitespace.
//
// The input slice is copied before storing in Raw because the stdlib
// JSON decoder may reuse its input buffer; aliasing it directly would
// expose the receiver to corruption when the caller reuses the
// buffer.
func (u *UnknownType) UnmarshalJSON(b []byte) error {
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(b, &probe); err != nil {
		return err
	}
	u.TypeName = probe.Type
	raw := make(json.RawMessage, len(b))
	copy(raw, b)
	u.Raw = raw
	return nil
}
