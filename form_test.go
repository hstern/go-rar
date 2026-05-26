// Copyright 2026 The go-rar Authors
// SPDX-License-Identifier: Apache-2.0

package rar

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

// Spec fixtures.
//
// These are the RFC 9396 example payloads from the four sections that
// carry `authorization_details` on the wire:
//
//   - §2 baseline: a single authorization_details element with the
//     §2 baseline members only.
//   - §4 multiple: an array of two elements, the second carrying
//     type-specific extension members (`instructedAmount` etc.).
//   - §7 token response: a full OAuth 2.0 token response body with
//     `authorization_details` as one of several members.
//   - §9 introspection response: a full RFC 7662 introspection
//     response body with `authorization_details` as one of several
//     members.
//
// The fixtures are inlined as Go string literals for this phase;
// phase 5 (RAR-20) will move them to `internal/specfixtures/` and
// load them via `go:embed`. The string content here MUST stay
// byte-identical to that future fixture corpus so the round-trip
// tests in `conformance_test.go` continue to pass after the move.

// §2 baseline — the canonical single-element example. Pretty-
// printed exactly as in the RFC. Note this is one element, not the
// JSON array; the test wraps it in `[...]` before dispatching to
// ParseArray.
const specFixtureBaselineElement = `{
  "type": "customer_information",
  "locations": ["https://example.com/customers"],
  "actions":   ["read", "write"],
  "datatypes": ["contacts", "photos"]
}`

// §4 multiple — two-element array, second element carries
// payment_initiation-style extension fields.
const specFixtureMultiple = `[
  {
    "type": "account_information",
    "actions": ["list_accounts", "read_balances", "read_transactions"],
    "locations": ["https://example.com/accounts"]
  },
  {
    "type": "payment_initiation",
    "actions": ["initiate", "status", "cancel"],
    "locations": ["https://example.com/payments"],
    "instructedAmount": {"currency": "EUR", "amount": "123.50"},
    "creditorName":    "Merchant A",
    "creditorAccount": {"iban": "DE02100100109307118603"},
    "remittanceInformationUnstructured": "Ref Number Merchant"
  }
]`

// §7 token response — full OAuth response body with
// authorization_details as one member alongside access_token etc.
const specFixtureTokenResponse = `{
  "access_token": "2YotnFZFEjr1zCsicMWpAA",
  "token_type":   "Bearer",
  "expires_in":   3600,
  "refresh_token": "tGzv3JOkF0XG5Qx2TlKWIA",
  "authorization_details": [
    {
      "type": "payment_initiation",
      "actions": ["initiate"],
      "locations": ["https://example.com/payments"],
      "instructedAmount": {"currency": "EUR", "amount": "123.50"},
      "creditorName": "Merchant A"
    }
  ]
}`

// §9 introspection response — full RFC 7662 body with
// authorization_details as one member.
const specFixtureIntrospection = `{
  "active": true,
  "sub":    "24400320",
  "aud":    "s6BhdRkqt3",
  "iss":    "https://server.example.com/",
  "exp":    1419356238,
  "authorization_details": [
    {
      "type": "account_information",
      "actions": ["read"],
      "locations": ["https://example.com/accounts"]
    }
  ]
}`

// mustRoundTripJSON drives a fixture through the JSON codec and
// asserts second-cycle byte stability. The spec figures use
// pretty-printed JSON with whitespace; the library emits compact
// JSON. First-cycle byte equality therefore does NOT hold (and
// MUST NOT — compactness is a feature). Second-cycle byte equality
// IS the contract: once the library has produced output, that
// output is its own fixed point.
//
// This is the round-trip helper described in the RFC 9396 phase 3
// gate. Every spec fixture funnels through it so a regression in any
// codec component (marshal ordering, unknown-type capture, common-
// field omitempty behavior) surfaces here.
func mustRoundTripJSON(t *testing.T, name string, raw json.RawMessage) AuthorizationDetails {
	t.Helper()
	parsed, err := ParseArray(raw)
	if err != nil {
		t.Fatalf("%s: parse: %v", name, err)
	}
	out, err := json.Marshal(parsed)
	if err != nil {
		t.Fatalf("%s: marshal: %v", name, err)
	}
	parsed2, err := ParseArray(out)
	if err != nil {
		t.Fatalf("%s: reparse: %v", name, err)
	}
	out2, err := json.Marshal(parsed2)
	if err != nil {
		t.Fatalf("%s: remarshal: %v", name, err)
	}
	if !bytes.Equal(out, out2) {
		t.Errorf("%s: second-cycle byte drift\nfirst:  %s\nsecond: %s", name, out, out2)
	}
	return parsed
}

// mustRoundTripForm drives the parsed slice through EncodeForm and
// DecodeForm and asserts structural equality. The library's marshal
// contract guarantees second-cycle byte stability on JSON, so the
// form layer (which is a thin wrapper over JSON) inherits the same
// guarantee — verified independently here.
func mustRoundTripForm(t *testing.T, name string, details AuthorizationDetails) {
	t.Helper()
	encoded, err := EncodeForm(details)
	if err != nil {
		t.Fatalf("%s: EncodeForm: %v", name, err)
	}
	decoded, err := DecodeForm(encoded)
	if err != nil {
		t.Fatalf("%s: DecodeForm: %v", name, err)
	}
	if len(decoded) != len(details) {
		t.Fatalf("%s: round-trip length drift: got %d, want %d", name, len(decoded), len(details))
	}
	for i := range details {
		if got, want := decoded[i].Type(), details[i].Type(); got != want {
			t.Errorf("%s: element %d Type() drift: got %q, want %q", name, i, got, want)
		}
	}

	// Second-cycle byte stability on the form layer specifically.
	encoded2, err := EncodeForm(decoded)
	if err != nil {
		t.Fatalf("%s: EncodeForm second cycle: %v", name, err)
	}
	if encoded != encoded2 {
		t.Errorf("%s: form second-cycle byte drift\nfirst:  %s\nsecond: %s", name, encoded, encoded2)
	}
}

// extractAuthorizationDetails pulls the `authorization_details`
// member out of a JSON-body fixture (token response, introspection
// response) as raw bytes ready for ParseArray. The library does NOT
// provide a typed token-response or introspection-response struct
// — the wire shape of those bodies is OAuth-flow territory, not
// RFC 9396 territory — so consumers extract the member themselves.
// This helper mirrors what a real consumer would write.
func extractAuthorizationDetails(t *testing.T, name string, body []byte) json.RawMessage {
	t.Helper()
	var envelope struct {
		AuthorizationDetails json.RawMessage `json:"authorization_details"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("%s: extract authorization_details: %v", name, err)
	}
	if len(envelope.AuthorizationDetails) == 0 {
		t.Fatalf("%s: authorization_details member absent or empty", name)
	}
	return envelope.AuthorizationDetails
}

// TestSpecFixtures_RoundTrip is the phase 3 acceptance gate. Every
// spec figure that carries authorization_details — §2 baseline, §4
// multiple, §7 token response, §9 introspection — round-trips
// byte-stably through both the JSON codec and the form codec.
func TestSpecFixtures_RoundTrip(t *testing.T) {
	t.Run("RFC9396_§2_baseline", func(t *testing.T) {
		// The §2 baseline figure shows ONE element; the wire shape is
		// always an array. Wrap before dispatching.
		array := json.RawMessage("[" + specFixtureBaselineElement + "]")
		parsed := mustRoundTripJSON(t, "§2 baseline", array)
		mustRoundTripForm(t, "§2 baseline", parsed)

		if got, want := len(parsed), 1; got != want {
			t.Fatalf("§2 baseline: parsed len = %d, want %d", got, want)
		}
		if got, want := parsed[0].Type(), "customer_information"; got != want {
			t.Errorf("§2 baseline: Type = %q, want %q", got, want)
		}
		// "customer_information" is not a built-in (RFC 9396 ships no
		// built-in types except `common`); it lands in UnknownType.
		if _, ok := parsed[0].(*UnknownType); !ok {
			t.Errorf("§2 baseline: element type = %T, want *UnknownType (no registered type)", parsed[0])
		}
	})

	t.Run("RFC9396_§4_multiple", func(t *testing.T) {
		parsed := mustRoundTripJSON(t, "§4 multiple", json.RawMessage(specFixtureMultiple))
		mustRoundTripForm(t, "§4 multiple", parsed)

		if got, want := len(parsed), 2; got != want {
			t.Fatalf("§4 multiple: parsed len = %d, want %d", got, want)
		}
		wantTypes := []string{"account_information", "payment_initiation"}
		for i, want := range wantTypes {
			if got := parsed[i].Type(); got != want {
				t.Errorf("§4 multiple: parsed[%d].Type = %q, want %q", i, got, want)
			}
		}

		// The payment_initiation element carries type-specific
		// extension members (instructedAmount, creditorName, ...).
		// Verify they survive the verbatim-capture round-trip by
		// checking the marshaled output of the second element
		// contains them.
		out, err := json.Marshal(parsed[1])
		if err != nil {
			t.Fatalf("§4 multiple: marshal element 1: %v", err)
		}
		for _, member := range []string{
			`"instructedAmount"`,
			`"creditorName"`,
			`"creditorAccount"`,
			`"remittanceInformationUnstructured"`,
		} {
			if !bytes.Contains(out, []byte(member)) {
				t.Errorf("§4 multiple: marshaled element 1 missing %s\ngot: %s", member, out)
			}
		}
	})

	t.Run("RFC9396_§7_token_response", func(t *testing.T) {
		// Token-response fixture is a full OAuth body, not a bare
		// array. Extract the authorization_details member as raw
		// JSON, then dispatch through ParseArray. This is the
		// pattern documented in DecodeForm's godoc for JSON-body
		// transports.
		raw := extractAuthorizationDetails(t, "§7 token response", []byte(specFixtureTokenResponse))
		parsed := mustRoundTripJSON(t, "§7 token response", raw)
		mustRoundTripForm(t, "§7 token response", parsed)

		if got, want := len(parsed), 1; got != want {
			t.Fatalf("§7 token response: parsed len = %d, want %d", got, want)
		}
		if got, want := parsed[0].Type(), "payment_initiation"; got != want {
			t.Errorf("§7 token response: Type = %q, want %q", got, want)
		}
	})

	t.Run("RFC9396_§9_introspection", func(t *testing.T) {
		// Introspection fixture is a full RFC 7662 body; same
		// extraction pattern as §7.
		raw := extractAuthorizationDetails(t, "§9 introspection", []byte(specFixtureIntrospection))
		parsed := mustRoundTripJSON(t, "§9 introspection", raw)
		mustRoundTripForm(t, "§9 introspection", parsed)

		if got, want := len(parsed), 1; got != want {
			t.Fatalf("§9 introspection: parsed len = %d, want %d", got, want)
		}
		if got, want := parsed[0].Type(), "account_information"; got != want {
			t.Errorf("§9 introspection: Type = %q, want %q", got, want)
		}
	})
}

// TestEncodeForm_NilProducesEmptyArray pins the nil-normalization
// choice documented on EncodeForm. A nil AuthorizationDetails MUST
// encode as `[]`, not the stdlib default `null`, because RFC 9396 §2
// defines the wire shape as a JSON array and the receiver would
// reject `null` as a malformed value.
func TestEncodeForm_NilProducesEmptyArray(t *testing.T) {
	out, err := EncodeForm(nil)
	if err != nil {
		t.Fatalf("EncodeForm(nil) error = %v; want nil", err)
	}
	if out != "[]" {
		t.Errorf("EncodeForm(nil) = %q; want %q", out, "[]")
	}
}

// TestEncodeForm_EmptySliceProducesEmptyArray pins the empty-slice
// case — same wire output as nil, separately verified because the
// two travel different codepaths through stdlib encoding/json.
func TestEncodeForm_EmptySliceProducesEmptyArray(t *testing.T) {
	out, err := EncodeForm(AuthorizationDetails{})
	if err != nil {
		t.Fatalf("EncodeForm([]) error = %v; want nil", err)
	}
	if out != "[]" {
		t.Errorf("EncodeForm([]) = %q; want %q", out, "[]")
	}
}

// TestDecodeForm_EmptyStringIsError pins that the form decoder
// rejects the empty string. An empty form value is not valid JSON
// and definitely not a JSON array; the right behavior is a clear
// error rather than silently returning an empty slice.
func TestDecodeForm_EmptyStringIsError(t *testing.T) {
	_, err := DecodeForm("")
	if err == nil {
		t.Fatalf("DecodeForm(\"\") error = nil; want non-nil")
	}
	if !errors.Is(err, Err) {
		t.Errorf("DecodeForm(\"\"): errors.Is(err, Err) = false; want true (err = %v)", err)
	}
}

// TestDecodeForm_NotJSONIsError covers the malformed-input path.
// The wrapped error matches errors.Is(err, Err) so callers can
// branch on "did this come from rar?" without inspecting the
// underlying encoding/json error.
func TestDecodeForm_NotJSONIsError(t *testing.T) {
	_, err := DecodeForm("not json")
	if err == nil {
		t.Fatalf("DecodeForm(\"not json\") error = nil; want non-nil")
	}
	if !errors.Is(err, Err) {
		t.Errorf("DecodeForm(\"not json\"): errors.Is(err, Err) = false; want true (err = %v)", err)
	}
}

// TestDecodeForm_EmptyArray pins the round-trip of the empty-array
// wire value. `[]` is a valid (if semantically borderline) RFC 9396
// payload; the library decodes it to a zero-length slice without
// error. The empty-array form is what EncodeForm produces for nil
// and empty input, so this closes the round-trip loop on those.
func TestDecodeForm_EmptyArray(t *testing.T) {
	got, err := DecodeForm("[]")
	if err != nil {
		t.Fatalf("DecodeForm(\"[]\") error = %v; want nil", err)
	}
	if len(got) != 0 {
		t.Errorf("DecodeForm(\"[]\") len = %d; want 0", len(got))
	}
}

// TestDecodeForm_MissingTypeMember exercises the dispatch path's
// "type-required" rule: an element without a `type` member is a
// RFC 9396 §2 MUST violation, surfaced as a *ValidationError
// wrapped under Err.
func TestDecodeForm_MissingTypeMember(t *testing.T) {
	_, err := DecodeForm(`[{"locations":["x"]}]`)
	if err == nil {
		t.Fatalf("DecodeForm(no-type) error = nil; want *ValidationError")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("DecodeForm(no-type): errors.As(*ValidationError) = false; err = %v", err)
	}
	if verr.Rule != "type-required" {
		t.Errorf("DecodeForm(no-type): Rule = %q; want %q", verr.Rule, "type-required")
	}
}

// TestEncodeForm_DecodeForm_RoundTrip is the inverse-operation
// contract: for every well-formed AuthorizationDetails slice,
// DecodeForm(EncodeForm(d)) returns a value with the same length
// and element discriminators as the input. This is the core
// guarantee EncodeForm's godoc advertises.
func TestEncodeForm_DecodeForm_RoundTrip(t *testing.T) {
	original, err := ParseArray(json.RawMessage(specFixtureMultiple))
	if err != nil {
		t.Fatalf("ParseArray(spec §4): %v", err)
	}

	encoded, err := EncodeForm(original)
	if err != nil {
		t.Fatalf("EncodeForm: %v", err)
	}
	decoded, err := DecodeForm(encoded)
	if err != nil {
		t.Fatalf("DecodeForm: %v", err)
	}

	if len(decoded) != len(original) {
		t.Fatalf("round-trip length: got %d, want %d", len(decoded), len(original))
	}
	for i := range original {
		if got, want := decoded[i].Type(), original[i].Type(); got != want {
			t.Errorf("element %d Type drift: got %q, want %q", i, got, want)
		}
	}

	// The §4 fixture's payment_initiation element lands in
	// UnknownType (no built-in registration); verify the verbatim
	// Raw capture survives the form round-trip.
	want, ok := original[1].(*UnknownType)
	if !ok {
		t.Fatalf("original[1] = %T; want *UnknownType", original[1])
	}
	got, ok := decoded[1].(*UnknownType)
	if !ok {
		t.Fatalf("decoded[1] = %T; want *UnknownType", decoded[1])
	}
	if !reflect.DeepEqual(want.TypeName, got.TypeName) {
		t.Errorf("TypeName drift: got %q, want %q", got.TypeName, want.TypeName)
	}
	// Raw bytes are byte-stable on second cycle, not first (the
	// fixture is pretty-printed, the library emits compact). The
	// JSON round-trip helper already covered the byte-stability
	// claim; here we just confirm the Raw bytes are non-empty.
	if len(got.Raw) == 0 {
		t.Errorf("decoded[1].Raw is empty; want non-empty (verbatim capture)")
	}
}

// TestEncodeForm_IntegratesWithURLValues is documentation-by-test:
// the EncodeForm output is meant to slot into a stdlib url.Values
// without further escaping. This test wires up the typical caller
// pattern and sanity-checks the resulting body.
//
// This is not a strict assertion test — it exercises the
// integration story documented in EncodeForm's godoc so that a
// regression in how the form layer composes with url.Values would
// surface.
func TestEncodeForm_IntegratesWithURLValues(t *testing.T) {
	details, err := ParseArray(json.RawMessage(specFixtureMultiple))
	if err != nil {
		t.Fatalf("ParseArray: %v", err)
	}

	s, err := EncodeForm(details)
	if err != nil {
		t.Fatalf("EncodeForm: %v", err)
	}
	v := url.Values{}
	v.Set("authorization_details", s)
	encoded := v.Encode()

	if !strings.HasPrefix(encoded, "authorization_details=") {
		t.Errorf("encoded body should start with authorization_details=; got %q", encoded)
	}

	// Round-trip through url.Values to confirm the URL-encoded form
	// decodes cleanly back to the value EncodeForm produced.
	parsed, err := url.ParseQuery(encoded)
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	got := parsed.Get("authorization_details")
	if got != s {
		t.Errorf("url.Values round-trip drift:\ngot:  %s\nwant: %s", got, s)
	}

	// And DecodeForm the recovered value back to the typed slice.
	back, err := DecodeForm(got)
	if err != nil {
		t.Fatalf("DecodeForm(url-recovered): %v", err)
	}
	if len(back) != len(details) {
		t.Errorf("post-url round-trip length: got %d, want %d", len(back), len(details))
	}
}
