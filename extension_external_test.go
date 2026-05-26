// Copyright 2026 The go-rar Authors
// SPDX-License-Identifier: Apache-2.0

package rar_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/hstern/go-rar"
)

// This file is the regression bar for the embeddable [rar.Extension]
// base: it exercises the consumer pattern from an external test
// package (note `package rar_test`, not `package rar`), proving that
// a struct embedding rar.Extension satisfies the otherwise-sealed
// [rar.AuthorizationDetail] interface from outside the library. If
// the seal regresses — say, by replacing the embedded sealed() with
// a typed method that the embedded field cannot grant from outside
// the package — the var _ assertion below stops compiling.
//
// The discriminator name "rar.example/payment_initiation" is
// intentionally namespaced so the global registry mutation this test
// performs cannot collide with any production registration a parallel
// test or downstream consumer might install. (The in-package
// extension_test.go uses "payment_initiation" via the
// resetRegistryForTest helper, which is unreachable from rar_test;
// using a distinct namespaced name here avoids cross-test
// interference without needing the helper.)

// paymentInitiationType is the out-of-package consumer pattern: a
// struct embedding rar.Extension to inherit the sealed() marker and
// the §2 baseline validation rules, plus type-specific fields and
// overrides of MarshalJSON / UnmarshalJSON / Validate.
type paymentInitiationType struct {
	rar.Extension
	CreditorName string `json:"creditorName,omitempty"`
	CreditorIBAN string `json:"creditorIban,omitempty"`
}

// Validate inherits the §2 baseline rules from rar.Extension and adds
// the type-specific creditorName-required check. The composition is
// the headline reason rar.Extension exists: consumers get the §2
// validation for free and add only what their type's spec figure
// requires.
func (p *paymentInitiationType) Validate() error {
	if err := p.Extension.Validate(); err != nil {
		return err
	}
	if p.CreditorName == "" {
		return &rar.ValidationError{
			Rule:   "creditorName-required",
			Type:   p.TypeName,
			Reason: "creditorName must be non-empty",
		}
	}
	return nil
}

// MarshalJSON emits members in spec order: type, then §2 baseline,
// then type-specific in declared order. This is the consumer-side
// version of the same pattern [rar.CommonType] uses internally.
func (p *paymentInitiationType) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type         string   `json:"type"`
		Locations    []string `json:"locations,omitempty"`
		Actions      []string `json:"actions,omitempty"`
		Datatypes    []string `json:"datatypes,omitempty"`
		Identifier   string   `json:"identifier,omitempty"`
		Privileges   []string `json:"privileges,omitempty"`
		CreditorName string   `json:"creditorName,omitempty"`
		CreditorIBAN string   `json:"creditorIban,omitempty"`
	}{
		Type:         p.TypeName,
		Locations:    p.Locations,
		Actions:      p.Actions,
		Datatypes:    p.Datatypes,
		Identifier:   p.Identifier,
		Privileges:   p.Privileges,
		CreditorName: p.CreditorName,
		CreditorIBAN: p.CreditorIBAN,
	})
}

// UnmarshalJSON populates the receiver from the wire object. Same
// anonymous-struct shape as MarshalJSON; the aux struct breaks the
// would-be infinite recursion that would otherwise occur if we called
// json.Unmarshal on a *paymentInitiationType from inside its own
// UnmarshalJSON.
func (p *paymentInitiationType) UnmarshalJSON(b []byte) error {
	var aux struct {
		Type         string   `json:"type"`
		Locations    []string `json:"locations,omitempty"`
		Actions      []string `json:"actions,omitempty"`
		Datatypes    []string `json:"datatypes,omitempty"`
		Identifier   string   `json:"identifier,omitempty"`
		Privileges   []string `json:"privileges,omitempty"`
		CreditorName string   `json:"creditorName,omitempty"`
		CreditorIBAN string   `json:"creditorIban,omitempty"`
	}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	p.TypeName = aux.Type
	p.Locations = aux.Locations
	p.Actions = aux.Actions
	p.Datatypes = aux.Datatypes
	p.Identifier = aux.Identifier
	p.Privileges = aux.Privileges
	p.CreditorName = aux.CreditorName
	p.CreditorIBAN = aux.CreditorIBAN
	return nil
}

// Compile-time assertion that the out-of-package type satisfies the
// sealed interface via embedded rar.Extension. This is THE regression
// bar — if the seal can no longer be inherited via embedding, this
// line stops compiling and the consumer-extension API is broken.
var _ rar.AuthorizationDetail = (*paymentInitiationType)(nil)

// externalExtensionTypeName is the namespaced discriminator used by
// every test in this file. The namespace prefix ("rar.example/")
// prevents collision with any production registration a parallel
// test or downstream consumer might install.
const externalExtensionTypeName = "rar.example/payment_initiation"

// registerExternalExtensionType installs the test's constructor into
// the package-global registry. The registration is intentionally NOT
// torn down — the rar package's reset-for-test helper is unexported,
// and the namespaced discriminator makes the leak benign for any
// subsequent test in this binary. (A re-registration in another test
// would silently overwrite per [rar.RegisterType]'s documented
// replace-on-rewrite contract, which is acceptable for a test-only
// name nobody else owns.)
func registerExternalExtensionType(t *testing.T) {
	t.Helper()
	err := rar.RegisterType(externalExtensionTypeName, func() rar.AuthorizationDetail {
		return &paymentInitiationType{Extension: rar.Extension{TypeName: externalExtensionTypeName}}
	})
	if err != nil {
		t.Fatalf("RegisterType(%q, _) = %v; want nil", externalExtensionTypeName, err)
	}
}

// externalExtensionFixture is the wire payload used by the
// round-trip and dispatch tests. Members are written in the order
// MarshalJSON emits them so the byte-stable comparison holds.
var externalExtensionFixture = []byte(
	`{"type":"rar.example/payment_initiation","locations":["https://example.com/payments"],"actions":["initiate"],"creditorName":"Merchant A","creditorIban":"DE02100100109307118603"}`,
)

// TestExternalExtension_RegisterAndRoundTrip is the headline path:
// register the consumer type, parse a payload, assert the result is
// the consumer-defined concrete type, and marshal back byte-stably.
// If any of the four steps fails, the consumer-extension API is
// broken.
func TestExternalExtension_RegisterAndRoundTrip(t *testing.T) {
	registerExternalExtensionType(t)

	parsed, err := rar.Parse(externalExtensionFixture)
	if err != nil {
		t.Fatalf("rar.Parse = %v; want nil", err)
	}

	got, ok := parsed.(*paymentInitiationType)
	if !ok {
		t.Fatalf("rar.Parse returned %T; want *paymentInitiationType", parsed)
	}

	if got.TypeName != externalExtensionTypeName {
		t.Errorf("TypeName = %q; want %q", got.TypeName, externalExtensionTypeName)
	}
	if got.CreditorName != "Merchant A" {
		t.Errorf("CreditorName = %q; want %q", got.CreditorName, "Merchant A")
	}
	if got.CreditorIBAN != "DE02100100109307118603" {
		t.Errorf("CreditorIBAN = %q; want %q", got.CreditorIBAN, "DE02100100109307118603")
	}
	wantLocations := []string{"https://example.com/payments"}
	if !equalStringSliceExt(got.Locations, wantLocations) {
		t.Errorf("Locations = %v; want %v", got.Locations, wantLocations)
	}
	wantActions := []string{"initiate"}
	if !equalStringSliceExt(got.Actions, wantActions) {
		t.Errorf("Actions = %v; want %v", got.Actions, wantActions)
	}

	roundTripped, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("json.Marshal(got) = %v; want nil", err)
	}
	if !bytes.Equal(roundTripped, externalExtensionFixture) {
		t.Errorf("round-trip not byte-stable\n got: %s\nwant: %s", roundTripped, externalExtensionFixture)
	}
}

// TestExternalExtension_TypeSpecificValidation pins the consumer's
// override path: a missing CreditorName returns the consumer's
// "creditorName-required" rule, and a populated value passes the
// composed §2-baseline + type-specific check.
func TestExternalExtension_TypeSpecificValidation(t *testing.T) {
	registerExternalExtensionType(t)

	missing := &paymentInitiationType{
		Extension: rar.Extension{TypeName: externalExtensionTypeName},
		// CreditorName intentionally empty.
	}
	err := missing.Validate()
	if err == nil {
		t.Fatalf("Validate() = nil error; want creditorName-required violation")
	}
	var verr *rar.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("errors.As(err, *ValidationError) = false; got %T", err)
	}
	if verr.Rule != "creditorName-required" {
		t.Errorf("verr.Rule = %q; want %q", verr.Rule, "creditorName-required")
	}
	if verr.Type != externalExtensionTypeName {
		t.Errorf("verr.Type = %q; want %q", verr.Type, externalExtensionTypeName)
	}

	populated := &paymentInitiationType{
		Extension:    rar.Extension{TypeName: externalExtensionTypeName},
		CreditorName: "Merchant A",
	}
	if err := populated.Validate(); err != nil {
		t.Errorf("Validate() = %v; want nil for populated value", err)
	}
}

// TestExternalExtension_BaselineValidation pins the inherited path:
// an empty TypeName returns the §2 "type-required" rule via the
// embedded rar.Extension.Validate, proving the composition works
// even when the consumer's own Validate forwards to the embedded one
// first.
func TestExternalExtension_BaselineValidation(t *testing.T) {
	registerExternalExtensionType(t)

	emptyType := &paymentInitiationType{
		// Extension{} leaves TypeName empty; CreditorName is also
		// empty but the §2 baseline check fires first.
	}
	err := emptyType.Validate()
	if err == nil {
		t.Fatalf("Validate() = nil error; want type-required violation")
	}
	var verr *rar.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("errors.As(err, *ValidationError) = false; got %T", err)
	}
	if verr.Rule != "type-required" {
		t.Errorf("verr.Rule = %q; want %q (baseline rule should fire before type-specific)", verr.Rule, "type-required")
	}
}

// TestExternalExtension_ParseArrayDispatch exercises the mixed-
// dispatch path: an array containing one registered consumer type
// and one unregistered type. The registered element lands on the
// consumer type; the unregistered element falls through to
// *rar.UnknownType. Both coexist on the same wire path without
// either disturbing the other's dispatch.
func TestExternalExtension_ParseArrayDispatch(t *testing.T) {
	registerExternalExtensionType(t)

	const arrayPayload = `[` +
		`{"type":"rar.example/payment_initiation","creditorName":"Merchant A"},` +
		`{"type":"rar.example/unregistered","arbitrary":"value"}` +
		`]`

	details, err := rar.ParseArray([]byte(arrayPayload))
	if err != nil {
		t.Fatalf("rar.ParseArray = %v; want nil", err)
	}
	if got, want := len(details), 2; got != want {
		t.Fatalf("rar.ParseArray returned %d elements; want %d", got, want)
	}

	// Element 0 — registered, lands on the consumer type.
	pi, ok := details[0].(*paymentInitiationType)
	if !ok {
		t.Fatalf("details[0] is %T; want *paymentInitiationType", details[0])
	}
	if pi.TypeName != externalExtensionTypeName {
		t.Errorf("details[0].TypeName = %q; want %q", pi.TypeName, externalExtensionTypeName)
	}
	if pi.CreditorName != "Merchant A" {
		t.Errorf("details[0].CreditorName = %q; want %q", pi.CreditorName, "Merchant A")
	}

	// Element 1 — unregistered, falls through to *UnknownType.
	unk, ok := details[1].(*rar.UnknownType)
	if !ok {
		t.Fatalf("details[1] is %T; want *rar.UnknownType", details[1])
	}
	if unk.TypeName != "rar.example/unregistered" {
		t.Errorf("details[1].TypeName = %q; want %q", unk.TypeName, "rar.example/unregistered")
	}
}

// equalStringSliceExt is a tiny local helper to avoid pulling reflect
// or slices.Equal into a single comparison site. Named with the -Ext
// suffix to avoid colliding with the in-package equalStringSlice
// helper in extension_test.go (both files would otherwise see each
// other's symbols if they shared a package; here they do not, but the
// naming keeps the intent clear).
func equalStringSliceExt(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
