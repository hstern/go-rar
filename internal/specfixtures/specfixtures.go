// Copyright 2026 The go-rar Authors
// SPDX-License-Identifier: Apache-2.0

// Package specfixtures embeds the verbatim JSON example payloads
// from RFC 9396 §2–§9 so phase-5 conformance tests can iterate
// them as a unit.
//
// The fixtures are stored in their compact canonical form (what
// json.Compact would produce), matching the byte shape the library's
// MarshalJSON emits. This lets conformance tests assert
// bytes.Equal(fixture, marshaled) directly, with no preprocessing
// step on either side.
//
// Fixtures available:
//
//   - Baseline:      single §2-style detail wrapped in a one-element
//     array — the smallest legal authorization_details value.
//   - Multiple:      §4 two-element array, second element carrying
//     type-specific members (instructedAmount, creditorName, etc.).
//   - TokenResponse: §7 full token-response envelope, including the
//     surrounding OAuth members (access_token, token_type, ...).
//   - Introspection: §9 full introspection-response envelope.
//   - EmptyArray:    library-internal fixture (NOT from RFC 9396 —
//     the spec never uses explicit empty arrays in its examples)
//     pinning the post-fix invariant that a CommonType carrying a
//     present-but-empty array (`"actions":[]`) round-trips byte-
//     stably through Parse / Marshal. Sits in the conformance corpus
//     so the round-trip assertion machinery exercises it uniformly
//     with the spec-derived fixtures.
//
// Each spec-derived fixture is the verbatim payload from the RFC,
// modulo whitespace canonicalization. The library never modifies
// these — they're the conformance source of truth.
package specfixtures

import _ "embed"

// Baseline is the RFC 9396 §2 single-detail example, wrapped in the
// required one-element array.
//
//go:embed baseline.json
var Baseline []byte

// Multiple is the RFC 9396 §4 two-element array example, exercising
// type-specific members on the second element.
//
//go:embed multiple.json
var Multiple []byte

// TokenResponse is the RFC 9396 §7 full token-response envelope,
// stored whole so conformance tests can extract authorization_details
// and verify it reembeds identically.
//
//go:embed token_response.json
var TokenResponse []byte

// Introspection is the RFC 9396 §9 full introspection-response
// envelope, stored whole for the same reason as TokenResponse.
//
//go:embed introspection.json
var Introspection []byte

// EmptyArray is a library-internal fixture (not drawn from RFC 9396)
// pinning the explicit-empty-array round-trip invariant introduced
// alongside the hand-written `*CommonType.MarshalJSON` path. RFC 9396's
// published examples never use `"actions":[]` (members are either
// omitted or non-empty), so the spec-derived fixtures cannot exercise
// the present-but-empty case; this fixture fills that gap so the
// conformance round-trip suite catches a regression in either the
// unmarshal nil-vs-empty preservation or the marshal-side per-field
// elision rule.
//
//go:embed empty_array.json
var EmptyArray []byte

// All returns the full set of fixtures keyed by a stable name.
//
// Consumers (the phase-5 conformance test suite) typically iterate
// this map to run the same assertions across every fixture. Go map
// iteration order is randomized, so callers that need deterministic
// ordering should sort the keys before iterating.
func All() map[string][]byte {
	return map[string][]byte{
		"baseline":       Baseline,
		"multiple":       Multiple,
		"token_response": TokenResponse,
		"introspection":  Introspection,
		"empty_array":    EmptyArray,
	}
}
