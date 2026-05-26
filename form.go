// Copyright 2026 The go-rar Authors
// SPDX-License-Identifier: Apache-2.0

package rar

import (
	"encoding/json"
	"fmt"
)

// EncodeForm produces the value string for the `authorization_details`
// form parameter carried on the OAuth 2.0 authorization endpoint
// request (RFC 9396 §2). The returned string is the JSON-encoded
// array; the caller wires it into a [net/url.Values],
// [net/http.Request].PostForm, or any equivalent form-encoder, and
// THAT layer performs the URL-encoding step as part of the
// `application/x-www-form-urlencoded` serialization.
//
// The library deliberately stops at the JSON layer because
// URL-encoding has several legal forms (`+` vs `%20` in the body,
// canonicalization choices on the query string) and ownership of
// that choice belongs to the form encoder, not to the typed codec.
// stdlib `url.Values{}.Encode()` will percent-encode the value
// returned here correctly:
//
//	v := url.Values{}
//	s, err := rar.EncodeForm(details)
//	if err != nil { ... }
//	v.Set("authorization_details", s)
//	body := v.Encode() // "authorization_details=%5B%7B%22type%22%3A...%7D%5D"
//
// EncodeForm is the inverse of [DecodeForm]: for every well-formed
// AuthorizationDetails value, DecodeForm(EncodeForm(d)) returns a
// slice byte-stable-equal to d under the library's marshal contract
// (see the byte-stability discussion on [CommonType.MarshalJSON]).
//
// Empty-input behavior. Both a nil slice and a length-zero slice
// produce the JSON empty array `[]`. The library normalizes the nil
// case (stdlib `encoding/json` would otherwise emit `null` for a nil
// slice) because RFC 9396 §2 defines `authorization_details` as a
// JSON array and `null` is not interchangeable with an empty array
// on the wire. Some authorization servers reject empty
// `authorization_details` values; the library does NOT enforce that
// (Postel's law on outbound), but callers can detect the case by
// checking `len(details) == 0` before calling.
//
// Scope. EncodeForm is for the form-encoded transport — the
// authorization endpoint (`GET /authorize`) and the Pushed
// Authorization Request endpoint (RFC 9126) when the latter is
// carried as `application/x-www-form-urlencoded` (its default
// transport). For JSON-body transports — the token response
// (RFC 9396 §7), the introspection response (RFC 9396 §9), and PAR
// when sent as JSON — callers use the JSON codec directly:
// `json.Marshal(details)` and [ParseArray].
func EncodeForm(details AuthorizationDetails) (string, error) {
	// Normalize nil to an empty slice so stdlib `encoding/json`
	// emits `[]` instead of `null`. RFC 9396 §2 defines the wire
	// shape as a JSON array; emitting `null` would be a wire-format
	// surprise for the receiver and would not round-trip through
	// DecodeForm (which expects a JSON array, not a JSON null).
	if details == nil {
		details = AuthorizationDetails{}
	}
	b, err := json.Marshal(details)
	if err != nil {
		return "", fmt.Errorf("%w: encode authorization_details form value: %v", Err, err)
	}
	return string(b), nil
}

// DecodeForm parses the value of the `authorization_details` form
// parameter (RFC 9396 §2) into the typed slice. The input is the
// JSON-encoded array AFTER the form decoder has URL-decoded it;
// stdlib `r.PostFormValue("authorization_details")` and
// `url.Values.Get("authorization_details")` both yield the right
// shape.
//
// DecodeForm uses the same dispatch path as [Parse] and [ParseArray]:
//
//   - Each array element is dispatched through the type registry.
//     Built-in `common` and consumer-registered types land in their
//     concrete carriers; unrecognized discriminators land in
//     [*UnknownType] (forward compatibility — NOT an error).
//   - A missing or empty `type` on any element returns a
//     [*ValidationError] with Rule "type-required" (RFC 9396 §2 MUST).
//   - JSON-level parse failures (the input is not a JSON array, or
//     an element is not a JSON object) return an error wrapped under
//     [Err].
//
// Scope. DecodeForm is for the form-encoded transport. The
// JSON-body transports (token response, introspection response, PAR
// sent as JSON) carry the `authorization_details` member alongside
// other fields; for those, extract the member as
// [json.RawMessage] and call [ParseArray] directly. See the test
// fixtures in `form_test.go` for the extraction pattern.
func DecodeForm(value string) (AuthorizationDetails, error) {
	return ParseArray(json.RawMessage(value))
}
