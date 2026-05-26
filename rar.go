// Copyright 2026 The go-rar Authors
// SPDX-License-Identifier: Apache-2.0

// Package rar implements RFC 9396 Rich Authorization Requests —
// the typed encoder/decoder/validator for the OAuth 2.0
// authorization_details parameter.
//
// The package is in pre-release scaffolding state; the public surface
// will be filled in as subsequent phases land (§2 baseline fields,
// JSON and form codec, validation, conformance fixtures). The stable
// surface at this point is [SpecVersion] and the [AuthorizationDetail]
// sealed interface together with the [ValidationError] error type and
// the [ErrTypeReserved] sentinel.
package rar

// SpecVersion identifies the RFC this package implements. RFCs have no
// minor or patch numbers; errata to RFC 9396 are absorbed into
// Go-minor releases of this module without changing the value of this
// constant.
const SpecVersion = "RFC 9396"

// AuthorizationDetail is any element of the RFC 9396
// authorization_details JSON array — the typed Go form of one entry
// in the discriminated union.
//
// The interface is sealed: implementations must live inside this
// package. The sanctioned way for downstream code to add a new
// concrete type is RegisterType (landing in a later commit), which
// installs an unmarshal constructor in the dispatch table. Sealing
// keeps the wire-shape contract under the package's control —
// every value an unmarshal can produce is one this package knows how
// to marshal and validate consistently.
//
// Built-in implementations planned for this surface:
//
//   - CommonType — the §2-only carrier registered under the
//     "common" discriminator.
//   - UnknownType — the forward-compatibility carrier returned
//     for any type value not present in the dispatch table at
//     unmarshal time.
//
// Concrete consumer types registered via RegisterType also satisfy
// AuthorizationDetail; the sealed marker is satisfied by embedding
// one of the built-in carriers (typically [Common], via [CommonType]
// or a custom struct that embeds Common).
type AuthorizationDetail interface {
	// Type returns the spec's `type` discriminator value for this
	// element. Per RFC 9396 §2, every authorization_details element
	// MUST carry a non-empty `type` string; concrete implementations
	// surface that string here.
	Type() string

	// Common returns a pointer to the shared §2 baseline members
	// (locations, actions, datatypes, identifier, privileges) for
	// implementations that carry them. Implementations that do NOT
	// embed the §2 baseline — e.g. a type-specific struct whose wire
	// shape replaces rather than extends the baseline — MAY return
	// nil. Callers that want to read the §2 fields must therefore
	// nil-check the result.
	Common() *Common

	// Validate runs the spec's well-formedness checks for this
	// element and returns a *ValidationError on the first violation,
	// or nil if the element is well-formed. Validate is opt-in:
	// neither UnmarshalJSON nor MarshalJSON calls it by default,
	// matching the library's lenient-unmarshal / strict-on-opt-in
	// posture (see the package's design notes).
	Validate() error

	// sealed is an unexported marker that prevents code outside this
	// package from satisfying AuthorizationDetail directly. New
	// concrete types are registered through RegisterType, which
	// returns values whose Go type is defined inside this package.
	sealed()
}
