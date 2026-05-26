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
//
// Each fixture is the verbatim payload from the RFC, modulo
// whitespace canonicalization. The library never modifies these —
// they're the conformance source of truth.
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
	}
}
