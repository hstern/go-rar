// Copyright 2026 The go-rar Authors
// SPDX-License-Identifier: Apache-2.0

// Package rar implements RFC 9396 Rich Authorization Requests —
// the typed encoder/decoder/validator for the OAuth 2.0
// authorization_details parameter.
//
// # Quickstart
//
// Parse a wire payload, then opt in to validation:
//
//	const payload = `[{"type":"common","actions":["read","write"],"locations":["https://api.example.com/v1/data"]}]`
//
//	details, err := rar.ParseArray([]byte(payload))
//	if err != nil { /* malformed JSON or missing type discriminator */ }
//	if err := rar.ValidateAll(details); err != nil { /* well-formedness violation */ }
//
// See [Parse], [ParseArray], [EncodeForm], and [DecodeForm] for the
// codec surface; see [ValidateAll] and [SetStrictMarshal] for the
// validation surface.
//
// # Surface map
//
// The public API is intentionally small:
//
//   - [SpecVersion] — the RFC this build implements.
//   - [AuthorizationDetail] — the sealed interface satisfied by every
//     element in the discriminated union, with the [AuthorizationDetails]
//     slice alias for the array form.
//   - [Common] — the RFC 9396 §2 baseline members shared by every
//     element ([Common.Locations], [Common.Actions], [Common.Datatypes],
//     [Common.Identifier], [Common.Privileges]).
//   - [CommonType] — the §2-only built-in, registered under the
//     "common" discriminator.
//   - [UnknownType] — the forward-compatibility carrier for any
//     discriminator the registry does not recognize (round-trips
//     verbatim).
//   - [RegisterType] — installs a constructor for an additional
//     discriminator.
//   - [Parse] / [ParseArray] — decode the wire shape into typed values.
//   - [EncodeForm] / [DecodeForm] — convert to and from the
//     authorization-endpoint form parameter value.
//   - [ValidateAll] / [SetStrictMarshal] — opt-in validation.
//   - [Err], [ErrTypeReserved], [ValidationError] — the error surface;
//     every error this package returns matches errors.Is(err, [Err]).
//
// # Design posture
//
// Lenient unmarshal, strict-on-opt-in marshal: the codec accepts
// whatever well-formed JSON the wire delivers and defers
// well-formedness checks to [AuthorizationDetail.Validate] (per
// element) or [ValidateAll] (across an array). The [SetStrictMarshal]
// toggle additionally runs [CommonType.Validate] inside
// [CommonType.MarshalJSON]. Unknown discriminators are NOT an error —
// they land in [UnknownType] and round-trip byte-stably, which is how
// the sealed-interface model stays forward-compatible with the
// IANA-registered type space the RFC defines as open.
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
// concrete type is [RegisterType], which installs an unmarshal
// constructor in the dispatch table. Sealing keeps the wire-shape
// contract under the package's control — every value an unmarshal
// can produce is one this package knows how to marshal and validate
// consistently.
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

// AuthorizationDetails is the wire-shape alias for the JSON array of
// authorization_details elements defined by RFC 9396 §2 — the same
// `authorization_details` parameter that appears in the authorization
// request, the token request, and the access-token introspection
// response, expressed in Go as a slice of the singular element type.
//
// It exists purely as a readability convenience for function
// signatures. A future codec entry point will read as
//
//	func ParseArray(raw json.RawMessage) (AuthorizationDetails, error)
//
// which is easier to scan than the bare slice type while carrying
// the exact same meaning.
//
// AuthorizationDetails is a type alias (note the `=` in the
// declaration), not a defined type. Consequences:
//
//   - AuthorizationDetails and []AuthorizationDetail are the same
//     Go type and are interchangeable without conversion at any
//     call site, in any direction.
//   - Methods cannot be attached to AuthorizationDetails. Helpers
//     that act on a slice of elements are written as ordinary
//     functions taking AuthorizationDetails (equivalently,
//     []AuthorizationDetail) — see the validation and codec
//     surfaces in subsequent files.
//
// The alias-not-defined-type choice is deliberate: consumers that
// already hold a []AuthorizationDetail (for example, a slice they
// built by hand for a test, or one returned from a third-party
// helper) can pass it straight to functions that declare
// AuthorizationDetails, and vice versa, with no wrapping noise.
type AuthorizationDetails = []AuthorizationDetail
