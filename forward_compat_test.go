// Copyright 2026 The go-rar Authors
// SPDX-License-Identifier: Apache-2.0

package rar

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
)

// The tests in this file pin the forward-compatibility contract that
// the rest of the library leans on: a payload whose `type` value the
// running binary does not recognize parses cleanly, round-trips
// byte-for-byte through *UnknownType, and never produces an
// ErrTypeReserved (the sentinel is reserved for RegisterType
// collisions, not for unmarshal-path "I don't know this type" cases).
//
// The conformance suite in conformance_test.go exercises the same
// path incidentally — every RFC 9396 example fixture uses an
// unregistered type discriminator like "customer_information", so
// every parsed element flows through *UnknownType. The tests here
// make the contract explicit using synthesized payloads (not spec
// fixtures), so a regression in the forward-compat path produces a
// targeted failure rather than a confusing diff against a §3 example.
//
// Synthesized payloads use the IETF `org.example.` reverse-DNS
// convention to signal "this is a placeholder, not a registered
// type". The library has zero special handling for the prefix; it is
// chosen purely for readability and to avoid collision with any
// real IANA registration.

// TestForwardCompat_UnknownTypeRoundTrip pins the headline guarantee:
// an unregistered `type` value parses into *UnknownType with the
// discriminator surfaced on TypeName, the full original bytes
// captured verbatim on Raw, and a subsequent MarshalJSON producing a
// byte-identical payload. Type-specific members beyond the §2
// baseline (customField, nested) are included so the test confirms
// Raw actually preserves them rather than silently dropping anything
// the library does not recognize as a baseline field.
func TestForwardCompat_UnknownTypeRoundTrip(t *testing.T) {
	payload := []byte(`{"type":"org.example.future_thing","locations":["https://example.com/x"],"actions":["read"],"customField":42,"nested":{"a":1,"b":["c","d"]}}`)

	d, err := Parse(payload)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	u, ok := d.(*UnknownType)
	if !ok {
		t.Fatalf("Parse returned %T; want *UnknownType", d)
	}

	if got, want := u.TypeName, "org.example.future_thing"; got != want {
		t.Errorf("TypeName = %q; want %q", got, want)
	}
	if got, want := u.Type(), "org.example.future_thing"; got != want {
		t.Errorf("Type() = %q; want %q", got, want)
	}
	if c := u.Common(); c != nil {
		t.Errorf("Common() = %+v; want nil (UnknownType exposes no §2 baseline)", c)
	}
	if vErr := u.Validate(); vErr != nil {
		t.Errorf("Validate() = %v; want nil (UnknownType is opaque, library has no rules)", vErr)
	}

	if !bytes.Equal(u.Raw, payload) {
		t.Errorf("Raw not byte-identical to input.\n got: %s\nwant: %s", u.Raw, payload)
	}

	got, err := json.Marshal(u)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("re-marshal not byte-identical to input.\n got: %s\nwant: %s", got, payload)
	}
}

// TestForwardCompat_DoesNotFireErrTypeReserved locks the invariant
// that the unknown-type unmarshal path is silent. A reader skimming
// the code might assume "unrecognized type" must produce some kind of
// error, and ErrTypeReserved is the closest-named sentinel; this test
// pins that Parse returns neither it nor any other error for an
// unregistered discriminator.
func TestForwardCompat_DoesNotFireErrTypeReserved(t *testing.T) {
	_, err := Parse([]byte(`{"type":"org.example.unrecognized"}`))
	if errors.Is(err, ErrTypeReserved) {
		t.Errorf("Parse of unrecognized type erroneously returned ErrTypeReserved; the sentinel is reserved for RegisterType collisions, not unmarshal-path unknowns")
	}
	if err != nil {
		t.Errorf("Parse of unrecognized type should not error; got: %v", err)
	}
}

// TestForwardCompat_ArrayWithMixedKnownAndUnknown exercises the
// ParseArray path with a mix of one known ("common") and two
// unknown discriminators. The known element must land on
// *CommonType with §2 fields populated; the unknowns must land on
// *UnknownType with their discriminators surfaced. Re-marshaling the
// whole slice must reproduce the input bytes exactly — the per-
// element round-trip composes into a slice-level round-trip without
// reordering elements or whitespace.
func TestForwardCompat_ArrayWithMixedKnownAndUnknown(t *testing.T) {
	arr := []byte(`[{"type":"common","actions":["read"]},{"type":"org.example.alpha","x":1},{"type":"org.example.beta","y":[1,2,3]}]`)

	got, err := ParseArray(arr)
	if err != nil {
		t.Fatalf("ParseArray: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len(got) = %d; want 3", len(got))
	}

	c, ok := got[0].(*CommonType)
	if !ok {
		t.Fatalf("got[0] type = %T; want *CommonType", got[0])
	}
	if c.TypeName != "common" {
		t.Errorf("got[0].TypeName = %q; want %q", c.TypeName, "common")
	}
	if len(c.Actions) != 1 || c.Actions[0] != "read" {
		t.Errorf("got[0].Actions = %v; want [read]", c.Actions)
	}

	for i, want := range []string{"org.example.alpha", "org.example.beta"} {
		idx := i + 1
		u, ok := got[idx].(*UnknownType)
		if !ok {
			t.Errorf("got[%d] type = %T; want *UnknownType", idx, got[idx])
			continue
		}
		if u.TypeName != want {
			t.Errorf("got[%d].TypeName = %q; want %q", idx, u.TypeName, want)
		}
	}

	out, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("Marshal slice: %v", err)
	}
	if !bytes.Equal(out, arr) {
		t.Errorf("re-marshaled slice not byte-identical to input.\n got: %s\nwant: %s", out, arr)
	}
}

// TestForwardCompat_RegistryFallthroughDoesNotPollute is the subtle
// invariant pin: the fallback to *UnknownType when Parse encounters an
// unrecognized discriminator MUST NOT install that discriminator into
// the dispatch table as a "shortcut" for future parses. If a future
// refactor ever started doing so, the registry would grow without
// bound at runtime and consumers' RegisterType calls would suddenly
// observe spurious ErrTypeReserved-shaped collisions against names
// they never registered. This test catches that regression by reading
// the registry directly through the package-private lookup helper.
func TestForwardCompat_RegistryFallthroughDoesNotPollute(t *testing.T) {
	const unknownName = "org.example.never_registered"

	if ctor := lookup(unknownName); ctor != nil {
		t.Fatalf("precondition: %q already in registry before parse; got ctor %p", unknownName, ctor)
	}

	if _, err := Parse([]byte(`{"type":"` + unknownName + `"}`)); err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if ctor := lookup(unknownName); ctor != nil {
		t.Errorf("unrecognized type %q ended up in registry after parse; got ctor %p", unknownName, ctor)
	}
}

// TestForwardCompat_EmptyRawCarrierCannotRoundTrip clarifies the
// limit of the forward-compat round-trip guarantee. A bare
// &UnknownType{TypeName: "x"} (Raw never populated, typically because
// a consumer hand-constructed the carrier instead of going through
// Parse) marshals to a synthesized minimal `{"type":"x"}` object per
// the codec — well-formed JSON satisfying the spec's MUST on `type`,
// but obviously not byte-identical to any prior input. A bare
// &UnknownType{} (TypeName also empty) marshals to `{"type":""}`,
// which is intentionally still well-formed JSON; validation of the
// `type` value is the consumer's job, not MarshalJSON's. This test
// pins both behaviors so a future refactor cannot quietly change
// them.
//
// The overlap with TestUnknownTypeMarshalJSON_BareSynthesizesType /
// TestUnknownTypeMarshalJSON_BareEmptyTypeNameStillWellFormed in
// marshal_test.go is intentional: the conformance phase organizes
// the invariants under "forward compatibility" naming, while the
// marshal phase had them under "marshal". The duplication is the
// cost of having an organized conformance suite — either file going
// red signals a regression in the same underlying contract.
func TestForwardCompat_EmptyRawCarrierCannotRoundTrip(t *testing.T) {
	t.Run("bare TypeName only", func(t *testing.T) {
		u := &UnknownType{TypeName: "x"}
		got, err := json.Marshal(u)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		if want := []byte(`{"type":"x"}`); !bytes.Equal(got, want) {
			t.Errorf("bare TypeName marshal = %s; want %s", got, want)
		}
	})

	t.Run("empty everything", func(t *testing.T) {
		u := &UnknownType{}
		got, err := json.Marshal(u)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		if want := []byte(`{"type":""}`); !bytes.Equal(got, want) {
			t.Errorf("empty Unknown marshal = %s; want %s", got, want)
		}
	})
}
