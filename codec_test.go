// Copyright 2026 The go-rar Authors
// SPDX-License-Identifier: Apache-2.0

package rar

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestParse_CommonTypeHappyPath(t *testing.T) {
	raw := json.RawMessage(`{"type":"common","locations":["https://example/x"],"actions":["read"]}`)

	got, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse(...) error = %v; want nil", err)
	}
	ct, ok := got.(*CommonType)
	if !ok {
		t.Fatalf("Parse(...) returned %T; want *CommonType", got)
	}
	if ct.TypeName != "common" {
		t.Errorf("TypeName = %q; want %q", ct.TypeName, "common")
	}
	if want := []string{"https://example/x"}; !slices.Equal(ct.Locations, want) {
		t.Errorf("Locations = %#v; want %#v", ct.Locations, want)
	}
	if want := []string{"read"}; !slices.Equal(ct.Actions, want) {
		t.Errorf("Actions = %#v; want %#v", ct.Actions, want)
	}
}

func TestParse_PostelsLawExtras(t *testing.T) {
	raw := json.RawMessage(
		`{"type":"common","locations":["a"],"unknown_field":"ignored","extra_obj":{"k":1}}`,
	)

	got, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse(...) error = %v; want nil (lenient unmarshal)", err)
	}
	ct, ok := got.(*CommonType)
	if !ok {
		t.Fatalf("Parse(...) returned %T; want *CommonType", got)
	}
	if ct.TypeName != "common" {
		t.Errorf("TypeName = %q; want %q", ct.TypeName, "common")
	}
	if want := []string{"a"}; !slices.Equal(ct.Locations, want) {
		t.Errorf("Locations = %#v; want %#v", ct.Locations, want)
	}
}

func TestParse_EmptyArrayPreserved(t *testing.T) {
	// RFC 9396 fixtures distinguish absent (no member) from present-
	// but-empty ([]). Sub-decision §3 in design-decisions.md requires
	// the round-trip to keep that distinction. Verify stdlib
	// encoding/json does, in fact, give us a non-nil zero-length slice
	// for the empty-array case on unmarshal — if a future Go release
	// changes that, this test fails loudly and we revisit.
	raw := json.RawMessage(`{"type":"common","locations":[]}`)

	got, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse(...) error = %v; want nil", err)
	}
	ct, ok := got.(*CommonType)
	if !ok {
		t.Fatalf("Parse(...) returned %T; want *CommonType", got)
	}
	if ct.Locations == nil {
		t.Fatalf("Locations is nil; want non-nil empty slice (present-but-empty distinct from absent)")
	}
	if len(ct.Locations) != 0 {
		t.Errorf("len(Locations) = %d; want 0", len(ct.Locations))
	}
}

func TestParse_UnknownTypeFallsThrough(t *testing.T) {
	resetRegistryForTest(t)

	raw := json.RawMessage(`{"type":"customer_information","actions":["read"]}`)

	got, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse(...) error = %v; want nil (unknown is not an error)", err)
	}
	ut, ok := got.(*UnknownType)
	if !ok {
		t.Fatalf("Parse(...) returned %T; want *UnknownType", got)
	}
	if ut.TypeName != "customer_information" {
		t.Errorf("TypeName = %q; want %q", ut.TypeName, "customer_information")
	}
	if !bytes.Equal(ut.Raw, raw) {
		t.Errorf("Raw = %s; want %s", string(ut.Raw), string(raw))
	}
}

func TestParse_MissingType(t *testing.T) {
	raw := json.RawMessage(`{"locations":["x"]}`)

	got, err := Parse(raw)
	if err == nil {
		t.Fatalf("Parse(...) = %#v, nil error; want *ValidationError", got)
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("errors.As(err, *ValidationError) = false; want true (err=%v)", err)
	}
	if verr.Rule != "type-required" {
		t.Errorf("Rule = %q; want %q", verr.Rule, "type-required")
	}
	if !errors.Is(err, Err) {
		t.Errorf("errors.Is(err, Err) = false; want true (umbrella sentinel)")
	}
}

func TestParse_EmptyType(t *testing.T) {
	raw := json.RawMessage(`{"type":"","actions":["read"]}`)

	got, err := Parse(raw)
	if err == nil {
		t.Fatalf("Parse(...) = %#v, nil error; want *ValidationError", got)
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("errors.As(err, *ValidationError) = false; want true (err=%v)", err)
	}
	if verr.Rule != "type-required" {
		t.Errorf("Rule = %q; want %q", verr.Rule, "type-required")
	}
}

func TestParse_InvalidJSON(t *testing.T) {
	raw := json.RawMessage(`{not json}`)

	_, err := Parse(raw)
	if err == nil {
		t.Fatalf("Parse(...) = nil error; want wrapped JSON error")
	}
	if !errors.Is(err, Err) {
		t.Errorf("errors.Is(err, Err) = false; want true (categorical wrap)")
	}
}

func TestParseArray_MixedElements(t *testing.T) {
	resetRegistryForTest(t)

	raw := json.RawMessage(`[{"type":"common"},{"type":"customer_information","actions":["read"]}]`)

	got, err := ParseArray(raw)
	if err != nil {
		t.Fatalf("ParseArray(...) error = %v; want nil", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d; want 2", len(got))
	}
	if _, ok := got[0].(*CommonType); !ok {
		t.Errorf("got[0] = %T; want *CommonType", got[0])
	}
	if _, ok := got[1].(*UnknownType); !ok {
		t.Errorf("got[1] = %T; want *UnknownType", got[1])
	}
}

func TestParseArray_ElementErrorHalts(t *testing.T) {
	raw := json.RawMessage(`[{"type":"common"},{"locations":["x"]}]`)

	got, err := ParseArray(raw)
	if err == nil {
		t.Fatalf("ParseArray(...) = %#v, nil error; want element-1 error", got)
	}
	if got != nil {
		t.Errorf("ParseArray on element error returned slice %#v; want nil (no partial returns)", got)
	}
	if !strings.Contains(err.Error(), "element 1") {
		t.Errorf("err = %v; want message containing %q", err, "element 1")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("errors.As(err, *ValidationError) = false; want true (err=%v)", err)
	}
	if verr.Rule != "type-required" {
		t.Errorf("Rule = %q; want %q", verr.Rule, "type-required")
	}
}

func TestParseArray_InvalidJSON(t *testing.T) {
	raw := json.RawMessage(`not an array`)

	_, err := ParseArray(raw)
	if err == nil {
		t.Fatalf("ParseArray(...) = nil error; want wrapped JSON error")
	}
	if !errors.Is(err, Err) {
		t.Errorf("errors.Is(err, Err) = false; want true (umbrella sentinel)")
	}
}

func TestParseArray_EmptyArray(t *testing.T) {
	raw := json.RawMessage(`[]`)

	got, err := ParseArray(raw)
	if err != nil {
		t.Fatalf("ParseArray([]) error = %v; want nil", err)
	}
	if len(got) != 0 {
		t.Errorf("len(got) = %d; want 0", len(got))
	}
}

func TestParse_RegisteredTypeDispatches(t *testing.T) {
	resetRegistryForTest(t)

	// Register a consumer constructor for a non-built-in name; the
	// registry should hand Parse this ctor and the resulting value
	// should land as the type the ctor produced (here *CommonType).
	const typeName = "test_type"
	if err := RegisterType(typeName, func() AuthorizationDetail {
		return &CommonType{}
	}); err != nil {
		t.Fatalf("RegisterType(%q) = %v; want nil", typeName, err)
	}

	raw := json.RawMessage(`{"type":"test_type","actions":["write"]}`)
	got, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse(...) error = %v; want nil", err)
	}
	ct, ok := got.(*CommonType)
	if !ok {
		t.Fatalf("Parse(...) returned %T; want *CommonType (registered ctor)", got)
	}
	if ct.TypeName != "test_type" {
		t.Errorf("TypeName = %q; want %q (populated from wire `type`)", ct.TypeName, "test_type")
	}
	if want := []string{"write"}; !slices.Equal(ct.Actions, want) {
		t.Errorf("Actions = %#v; want %#v", ct.Actions, want)
	}
}

func TestCommonTypeUnmarshalJSON_DropsExtras(t *testing.T) {
	// Direct UnmarshalJSON (bypassing Parse) is lenient by design —
	// payloads with extra members parse cleanly, the lenient-unmarshal
	// posture (Postel's law). Validation of the §2-required `type`
	// member is a separate concern handled by Parse and Validate.
	raw := []byte(`{"type":"common","identifier":"id-7","privileges":["admin"],"junk":42}`)

	var ct CommonType
	if err := json.Unmarshal(raw, &ct); err != nil {
		t.Fatalf("json.Unmarshal(_, *CommonType) = %v; want nil", err)
	}
	if ct.TypeName != "common" {
		t.Errorf("TypeName = %q; want %q", ct.TypeName, "common")
	}
	if ct.Identifier != "id-7" {
		t.Errorf("Identifier = %q; want %q", ct.Identifier, "id-7")
	}
	if want := []string{"admin"}; !slices.Equal(ct.Privileges, want) {
		t.Errorf("Privileges = %#v; want %#v", ct.Privileges, want)
	}
}

func TestCommonTypeUnmarshalJSON_EmptyTypeOK(t *testing.T) {
	// The unmarshal does NOT enforce the type-required rule — Parse
	// and Validate own that check. A direct json.Unmarshal of a
	// payload missing `type` must therefore succeed with TypeName="".
	raw := []byte(`{"locations":["x"]}`)

	var ct CommonType
	if err := json.Unmarshal(raw, &ct); err != nil {
		t.Fatalf("json.Unmarshal(_, *CommonType) = %v; want nil (lenient at parse boundary)", err)
	}
	if ct.TypeName != "" {
		t.Errorf("TypeName = %q; want empty (unmarshal does not gate)", ct.TypeName)
	}
}

func TestUnknownTypeUnmarshalJSON_CapturesVerbatim(t *testing.T) {
	raw := []byte(`{"type":"x","a":1,"b":"two"}`)

	var u UnknownType
	if err := json.Unmarshal(raw, &u); err != nil {
		t.Fatalf("json.Unmarshal(_, *UnknownType) = %v; want nil", err)
	}
	if u.TypeName != "x" {
		t.Errorf("TypeName = %q; want %q", u.TypeName, "x")
	}
	if !bytes.Equal(u.Raw, raw) {
		t.Errorf("Raw = %s; want %s", string(u.Raw), string(raw))
	}
}

func TestUnknownTypeUnmarshalJSON_CopiesInput(t *testing.T) {
	// The stdlib JSON decoder may reuse its input buffer; the
	// implementation must copy bytes before storing in Raw. Mutate the
	// source post-unmarshal and confirm Raw is unaffected — that's the
	// behavior the copy guarantees.
	src := []byte(`{"type":"x"}`)
	buf := make([]byte, len(src))
	copy(buf, src)

	var u UnknownType
	if err := json.Unmarshal(buf, &u); err != nil {
		t.Fatalf("json.Unmarshal(...) = %v; want nil", err)
	}
	// Corrupt the input buffer; Raw must be unchanged.
	for i := range buf {
		buf[i] = 'Z'
	}
	if !bytes.Equal(u.Raw, src) {
		t.Errorf("Raw = %s; want %s (input copy must be defensive)", string(u.Raw), string(src))
	}
}
