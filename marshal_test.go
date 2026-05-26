// Copyright 2026 The go-rar Authors
// SPDX-License-Identifier: Apache-2.0

package rar

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestCommonTypeMarshalJSON_RoundTrip(t *testing.T) {
	// Each payload is already in the spec's member order (`type` first,
	// then locations / actions / datatypes / identifier / privileges);
	// Parse should accept it and MarshalJSON should return byte-
	// identical output. This is the RFC 9396 implementer-pin #1
	// invariant — the wire-fidelity contract every published example
	// in RFC 9396 §2–§9 relies on.
	cases := []struct {
		name string
		in   string
	}{
		{
			name: "bare minimum",
			in:   `{"type":"common"}`,
		},
		{
			name: "locations and actions",
			in:   `{"type":"common","locations":["https://example.com/x"],"actions":["read","write"]}`,
		},
		{
			name: "identifier only",
			in:   `{"type":"common","identifier":"abc-123"}`,
		},
		{
			name: "all baseline fields",
			in:   `{"type":"common","locations":["a"],"actions":["read"],"datatypes":["x"],"identifier":"i","privileges":["p"]}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(json.RawMessage(tc.in))
			if err != nil {
				t.Fatalf("Parse(%q) = %v; want nil", tc.in, err)
			}
			out, err := json.Marshal(got)
			if err != nil {
				t.Fatalf("json.Marshal(...) = %v; want nil", err)
			}
			if !bytes.Equal(out, []byte(tc.in)) {
				t.Errorf("round-trip mismatch:\n got %s\nwant %s", out, tc.in)
			}
		})
	}
}

func TestCommonTypeMarshalJSON_TypeFirst(t *testing.T) {
	// The wire-fidelity invariant: `type` must always be the first
	// member of the emitted object, even when the receiver carries
	// other baseline fields. Without this guarantee, output diffs
	// against the spec's published examples drift.
	ct := &CommonType{
		TypeName: "common",
		commonBaseline: Common{
			Locations:  []string{"https://example.com/x"},
			Actions:    []string{"read"},
			Privileges: []string{"admin"},
		},
	}
	out, err := json.Marshal(ct)
	if err != nil {
		t.Fatalf("json.Marshal(...) = %v; want nil", err)
	}
	if !bytes.HasPrefix(out, []byte(`{"type":`)) {
		t.Errorf("output does not lead with `type`: %s", out)
	}
	want := `{"type":"common","locations":["https://example.com/x"],"actions":["read"],"privileges":["admin"]}`
	if string(out) != want {
		t.Errorf("output mismatch:\n got %s\nwant %s", out, want)
	}
}

func TestCommonTypeMarshalJSON_SpecMemberOrder(t *testing.T) {
	// All five baseline fields set — verify they appear in spec order:
	// locations, actions, datatypes, identifier, privileges. The
	// declared-order property of stdlib encoding/json (which the
	// anonymous struct in MarshalJSON exploits) guarantees this; the
	// test pins the guarantee against accidental field reorderings in
	// the marshal helper.
	ct := &CommonType{
		TypeName: "common",
		commonBaseline: Common{
			Locations:  []string{"l"},
			Actions:    []string{"a"},
			Datatypes:  []string{"d"},
			Identifier: "i",
			Privileges: []string{"p"},
		},
	}
	out, err := json.Marshal(ct)
	if err != nil {
		t.Fatalf("json.Marshal(...) = %v; want nil", err)
	}
	want := `{"type":"common","locations":["l"],"actions":["a"],"datatypes":["d"],"identifier":"i","privileges":["p"]}`
	if string(out) != want {
		t.Errorf("output mismatch:\n got %s\nwant %s", out, want)
	}
}

func TestCommonTypeMarshalJSON_OmitEmpty(t *testing.T) {
	// Absent (nil / zero) baseline fields must be elided so the bare-
	// minimum `{"type":"common"}` shape is reachable. The test
	// constructs a CommonType with only TypeName set and asserts no
	// baseline keys leak into the output.
	ct := &CommonType{TypeName: "common"}
	out, err := json.Marshal(ct)
	if err != nil {
		t.Fatalf("json.Marshal(...) = %v; want nil", err)
	}
	if string(out) != `{"type":"common"}` {
		t.Errorf("output mismatch: got %s, want %s", out, `{"type":"common"}`)
	}
}

func TestCommonTypeMarshalJSON_EmptyArrayElided(t *testing.T) {
	// Document and pin the empty-array limitation: a CommonType whose
	// Locations is a length-zero non-nil slice marshals with the
	// member elided. This is the consequence of `omitempty` on a
	// []string — stdlib treats len==0 as empty regardless of
	// nil-vs-non-nil. The behavior is called out in CommonType's
	// MarshalJSON godoc and in design-decisions.md sub-decision §3;
	// the test guards against an accidental fix that would change the
	// wire shape without updating the docs.
	ct := &CommonType{
		TypeName: "common",
		commonBaseline: Common{
			Locations: []string{},
		},
	}
	out, err := json.Marshal(ct)
	if err != nil {
		t.Fatalf("json.Marshal(...) = %v; want nil", err)
	}
	if string(out) != `{"type":"common"}` {
		t.Errorf("output mismatch: got %s, want %s (omitempty elides empty slices)", out, `{"type":"common"}`)
	}
}

func TestCommonTypeMarshalJSON_DispatchesFromInterface(t *testing.T) {
	// json.Marshal on an AuthorizationDetail interface value must
	// dispatch to the concrete type's MarshalJSON. The test pins
	// "stdlib dispatch works" because a future refactor that moves
	// MarshalJSON off *CommonType (e.g. to a wrapper type) would
	// silently break interface-typed callers — they'd fall back to
	// stdlib struct marshaling and emit fields in declared-Go order,
	// not spec order.
	var ad AuthorizationDetail = &CommonType{
		TypeName: "common",
		commonBaseline: Common{
			Identifier: "id",
		},
	}
	out, err := json.Marshal(ad)
	if err != nil {
		t.Fatalf("json.Marshal(ad) = %v; want nil", err)
	}
	ct, ok := ad.(*CommonType)
	if !ok {
		t.Fatalf("ad type = %T; want *CommonType", ad)
	}
	direct, err := json.Marshal(ct)
	if err != nil {
		t.Fatalf("json.Marshal(*CommonType) = %v; want nil", err)
	}
	if !bytes.Equal(out, direct) {
		t.Errorf("interface vs concrete marshal mismatch:\n interface: %s\n concrete:  %s", out, direct)
	}
	if !bytes.HasPrefix(out, []byte(`{"type":`)) {
		t.Errorf("interface-dispatched output does not lead with `type`: %s", out)
	}
}

func TestUnknownTypeMarshalJSON_RoundTrip(t *testing.T) {
	resetRegistryForTest(t)

	// Parse an unrecognized type, capture the bytes Parse landed in
	// Raw, then Marshal and assert byte-equality. This is the round-
	// trip guarantee UnknownType exists to provide — a consumer
	// forwarding an unknown element through the library must observe
	// no key reordering, no whitespace normalization, no member
	// dropping.
	in := json.RawMessage(`{"type":"customer_information","actions":["read"],"x":1,"nested":{"k":"v"}}`)
	got, err := Parse(in)
	if err != nil {
		t.Fatalf("Parse(...) = %v; want nil", err)
	}
	ut, ok := got.(*UnknownType)
	if !ok {
		t.Fatalf("Parse(...) returned %T; want *UnknownType", got)
	}
	out, err := json.Marshal(ut)
	if err != nil {
		t.Fatalf("json.Marshal(ut) = %v; want nil", err)
	}
	if !bytes.Equal(out, in) {
		t.Errorf("round-trip mismatch:\n got %s\nwant %s", out, in)
	}
}

func TestUnknownTypeMarshalJSON_BareSynthesizesType(t *testing.T) {
	// Hand-constructed UnknownType (TypeName set, Raw never populated)
	// must marshal to a minimal `{"type":"<TypeName>"}` object. This
	// is the producer-side use case: a consumer needs to forward an
	// element with a type it doesn't carry its own struct for, sets
	// TypeName, and expects the library to produce well-formed JSON.
	u := &UnknownType{TypeName: "foo"}
	out, err := json.Marshal(u)
	if err != nil {
		t.Fatalf("json.Marshal(u) = %v; want nil", err)
	}
	if string(out) != `{"type":"foo"}` {
		t.Errorf("output mismatch: got %s, want %s", out, `{"type":"foo"}`)
	}
}

func TestUnknownTypeMarshalJSON_BareEmptyTypeNameStillWellFormed(t *testing.T) {
	// Edge case: TypeName empty AND Raw empty. The synthesizer emits
	// `{"type":""}` — well-formed JSON, technically out-of-spec
	// (RFC 9396 §2 requires `type` non-empty), but consistent with
	// MarshalJSON's lenient-marshal posture: validation is opt-in,
	// not enforced here.
	u := &UnknownType{}
	out, err := json.Marshal(u)
	if err != nil {
		t.Fatalf("json.Marshal(u) = %v; want nil", err)
	}
	if string(out) != `{"type":""}` {
		t.Errorf("output mismatch: got %s, want %s", out, `{"type":""}`)
	}
}

func TestUnknownTypeMarshalJSON_DispatchesFromInterface(t *testing.T) {
	resetRegistryForTest(t)

	in := json.RawMessage(`{"type":"customer_information","actions":["read"]}`)
	got, err := Parse(in)
	if err != nil {
		t.Fatalf("Parse(...) = %v; want nil", err)
	}
	out, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("json.Marshal(ad) = %v; want nil", err)
	}
	if !bytes.Equal(out, in) {
		t.Errorf("interface-dispatched UnknownType marshal mismatch:\n got %s\nwant %s", out, in)
	}
}

func TestParseArray_RoundTripReassembled(t *testing.T) {
	resetRegistryForTest(t)

	// ParseArray → marshal each element → reassemble as a JSON array.
	// This is the workflow phase-5 conformance tests will hammer on:
	// the full request-side wire representation flows through
	// Parse/Marshal without losing the spec's member order or the
	// per-element byte-stability. The fixture mixes a CommonType and
	// an UnknownType so both Marshal paths exercise.
	in := json.RawMessage(`[{"type":"common","actions":["read"]},{"type":"customer_information","x":1}]`)
	got, err := ParseArray(in)
	if err != nil {
		t.Fatalf("ParseArray(...) = %v; want nil", err)
	}
	out, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("json.Marshal([]AuthorizationDetail) = %v; want nil", err)
	}
	if !bytes.Equal(out, in) {
		t.Errorf("array round-trip mismatch:\n got %s\nwant %s", out, in)
	}
}

func TestCommonTypeMarshalJSON_NoTrailingWhitespace(t *testing.T) {
	// Stdlib json.Marshal does not append a trailing newline (that's
	// json.Encoder's behavior). Pin the property here so a future
	// switch to a custom marshal path that uses an Encoder doesn't
	// silently break byte-stability on consumers comparing produced
	// bytes against fixed expected bytes.
	ct := &CommonType{TypeName: "common"}
	out, err := json.Marshal(ct)
	if err != nil {
		t.Fatalf("json.Marshal(...) = %v; want nil", err)
	}
	if strings.HasSuffix(string(out), "\n") {
		t.Errorf("output has trailing newline: %q", out)
	}
}
