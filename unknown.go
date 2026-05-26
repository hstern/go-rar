// Copyright 2026 The go-rar Authors
// SPDX-License-Identifier: Apache-2.0

package rar

import "encoding/json"

// UnknownType is the forward-compatibility carrier returned by the
// codec for any authorization_details element whose `type` member
// (RFC 9396 §2) is not present in the package's dispatch table at
// unmarshal time.
//
// RFC 9396 deliberately defines no built-in `type` values: the entire
// discriminator vocabulary is delegated to extension specifications
// and the IANA registry. A correct decoder therefore cannot assume
// foreknowledge of every type it will encounter — tomorrow's
// registration is, by construction, unknown to today's binary. The
// library's response is to round-trip the unrecognized element
// verbatim through UnknownType rather than fail the parse. Producers
// that registered the type via RegisterType get a typed value;
// everyone else gets the raw bytes preserved exactly as they arrived.
//
// This is the sealed-interface posture's safety net. Because
// AuthorizationDetail is sealed (see [AuthorizationDetail]), an
// external package cannot supply its own concrete carrier for an
// unregistered type — but the wire still needs to flow through. The
// alternative — returning an error on every unknown `type` — would
// strand any consumer whose authorization server emits a type that
// post-dates the consumer's build, which the spec's open registry
// model effectively guarantees will happen.
//
// Field semantics:
//
//   - TypeName is the value of the element's `type` member, surfaced
//     so callers can route on it without re-parsing Raw. It is also
//     what [UnknownType.Type] returns.
//
//   - Raw holds the entire JSON object the unmarshal observed,
//     INCLUDING the `type` member — not just the members the library
//     did not recognize. Keeping the full object (rather than the
//     residual after stripping `type`) is what permits byte-stable
//     round-trip: a later MarshalJSON can emit Raw verbatim and the
//     output is identical to the input. Open-extension fields are
//     [json.RawMessage] precisely so a re-marshal does not reorder
//     keys; UnknownType extends that guarantee to the whole element.
//
// Marshal and unmarshal wiring for UnknownType lives in the codec
// (a later commit). This file declares only the type and its
// AuthorizationDetail-satisfying methods; the dispatch that produces
// an *UnknownType from an unrecognized `type` member, and the inverse
// that writes Raw back out, are codec concerns.
//
// Implementations notes for [AuthorizationDetail]:
//
//   - [UnknownType.Common] returns nil. UnknownType holds opaque
//     bytes, not the parsed §2 baseline — populating a *Common would
//     require speculatively decoding fields the library has no
//     contract for. Callers that want §2 members from an UnknownType
//     must unmarshal Raw into their own struct. The nil-Common
//     possibility is already called out on AuthorizationDetail.Common,
//     so callers reading §2 fields off an AuthorizationDetail interface
//     value MUST nil-check the result regardless of the concrete type.
//
//   - [UnknownType.Validate] returns nil. The library cannot judge
//     well-formedness of a type whose definition it does not know;
//     the §2 baseline rules are about field presence and shape, and
//     UnknownType's Raw may carry type-specific structure the spec
//     does not constrain. Trusting the source here matches the
//     library's lenient-unmarshal / opt-in-validate posture. A
//     consumer that wants stricter handling can inspect TypeName and
//     reject unknowns at the application layer.
type UnknownType struct {
	// TypeName is the `type` discriminator value as it appeared on
	// the wire. RFC 9396 §2 requires this member to be a non-empty
	// string; the codec populates it from the unmarshaled object.
	TypeName string

	// Raw is the complete JSON object for this authorization_details
	// element, including the `type` member. It is preserved verbatim
	// so the element can be re-marshaled byte-for-byte identically
	// to its input — the round-trip guarantee that makes UnknownType
	// safe to flow through a consumer that does not understand the
	// type.
	Raw json.RawMessage
}

// Type returns the wire `type` discriminator the element carried.
// Implements [AuthorizationDetail].
func (u *UnknownType) Type() string { return u.TypeName }

// Common returns nil. See the type-level documentation for the
// rationale — UnknownType holds opaque bytes, not the parsed §2
// baseline. Implements [AuthorizationDetail].
func (u *UnknownType) Common() *Common { return nil }

// Validate returns nil unconditionally — the library cannot judge
// well-formedness of a type it does not natively understand, so it
// trusts the source. RFC 9396 §10 (IANA considerations) defines the
// `type` value space as open: new discriminators are registered
// independently of any library release, and their per-type
// well-formedness rules live in those extension specifications, not
// here. Applying the §2 baseline rules to an UnknownType would also
// be unsafe — an extension type's wire shape may legitimately rename,
// repurpose, or omit the §2 fields, and the library has no contract
// telling it which is the case.
//
// Consumers that want stricter handling of unrecognized types can
// inspect TypeName after parse and reject what they don't recognize
// at the application layer, or register a type-specific constructor
// via RegisterType so the payload dispatches to a carrier whose
// Validate enforces the right rules. Implements [AuthorizationDetail].
func (u *UnknownType) Validate() error { return nil }

// sealed satisfies the unexported marker on [AuthorizationDetail],
// keeping UnknownType inside the package's sealed-interface posture.
func (u *UnknownType) sealed() {}

var _ AuthorizationDetail = (*UnknownType)(nil)
