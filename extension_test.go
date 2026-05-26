// Copyright 2026 The go-rar Authors
// SPDX-License-Identifier: Apache-2.0

package rar

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/hstern/go-rar/internal/specfixtures"
)

// This file is the phase-5 acceptance gate for the extension API:
// it exercises [RegisterType] end-to-end against a test-local
// concrete type modeled on the IANA-published `payment_initiation`
// shape from RFC 9396 §4 (UK Open Banking / ETSI OAuth profiles).
//
// The shape is intentionally NOT exported from the library — RFC 9396
// ships no built-in `type` values and the library reserves only the
// "common" §2-only carrier; every other type, including the IANA-
// registered ones, lives in consumer code and reaches the codec
// through [RegisterType]. paymentInitiationType below is a worked
// example of that consumer pattern, kept in a test file so the
// example lives next to the test that drives it.

// paymentBaseline aliases [Common] so the embedded-field name on
// paymentInitiationType does not collide with the [Common] method of
// [AuthorizationDetail]. This mirrors the commonBaseline alias in
// common_type.go and is the standard pattern for any consumer type
// that wants to embed the §2 baseline while still satisfying the
// sealed interface.
type paymentBaseline = Common

// amount is the §4 example's instructedAmount sub-object —
// {"currency": "EUR", "amount": "123.50"}. Modeled as a plain struct
// because the spec figure shows a fixed two-member shape; consumers
// adding their own variants would extend this in their own code.
type amount struct {
	Currency string `json:"currency,omitempty"`
	Amount   string `json:"amount,omitempty"`
}

// account is the §4 example's creditorAccount sub-object — a single
// IBAN string. Same modeling rationale as [amount].
type account struct {
	IBAN string `json:"iban,omitempty"`
}

// paymentInitiationType is a test-local example of how a downstream
// consumer would model an IANA-published `type` value beyond the §2
// baseline the library ships. The shape mirrors the second element
// of the multiple-details example in RFC 9396 §4:
//
//	{
//	  "type": "payment_initiation",
//	  "actions": ["initiate", "status", "cancel"],
//	  "locations": ["https://example.com/payments"],
//	  "instructedAmount": {"currency": "EUR", "amount": "123.50"},
//	  "creditorName": "Merchant A",
//	  "creditorAccount": {"iban": "DE02100100109307118603"},
//	  "remittanceInformationUnstructured": "Ref Number Merchant"
//	}
//
// Defined in a test file (and unexported) so it lives next to the
// test that exercises it. The library does NOT publish this type;
// consumers needing payment_initiation handling write their own.
type paymentInitiationType struct {
	// TypeName carries the wire `type` discriminator, populated by
	// UnmarshalJSON below. Consumers that hand-build a value set this
	// to "payment_initiation".
	TypeName string

	// paymentBaseline is the embedded §2 baseline — same pattern as
	// CommonType's commonBaseline. Wire-shape members
	// (locations/actions/datatypes/identifier/privileges) sit at the
	// top level alongside `type` and the type-specific members below.
	paymentBaseline

	// Type-specific members, in spec-figure declaration order.
	InstructedAmount                  amount  `json:"instructedAmount,omitzero"`
	CreditorName                      string  `json:"creditorName,omitempty"`
	CreditorAccount                   account `json:"creditorAccount,omitzero"`
	RemittanceInformationUnstructured string  `json:"remittanceInformationUnstructured,omitempty"`
}

// Type returns the wire `type` discriminator. Implements
// [AuthorizationDetail].
func (p *paymentInitiationType) Type() string { return p.TypeName }

// Common returns a pointer to the embedded §2 baseline so callers can
// reach the shared members via the sealed interface. Implements
// [AuthorizationDetail].
func (p *paymentInitiationType) Common() *Common { return &p.paymentBaseline }

// Validate is a no-op on this worked example. A real consumer
// implementation would call [CommonType]-style baseline checks plus
// type-specific rules (e.g. currency code shape, IBAN structure);
// keeping it empty here lets the extension-end-to-end tests focus on
// the wiring rather than re-testing the §2 rules already exercised
// by validate_test.go.
func (p *paymentInitiationType) Validate() error { return nil }

// sealed satisfies the unexported marker on [AuthorizationDetail].
// Outside the rar package this would be unreachable; inside the
// package tests can implement it directly, which is what makes the
// extension-pattern example exercisable from a _test.go file.
func (p *paymentInitiationType) sealed() {}

// Compile-time interface assertion — mirrors the package's pattern of
// pinning every concrete carrier to the sealed interface so a
// regression in the interface shape breaks here at build time.
var _ AuthorizationDetail = (*paymentInitiationType)(nil)

// MarshalJSON emits members in the order the §4 spec figure uses for
// the payment_initiation element: `type` first, then `actions` then
// `locations` (the order the figure literally writes them — RFC 9396
// does not mandate wire-level ordering, and the §4 figure deviates
// from the §2 enumeration order [locations, actions, datatypes,
// identifier, privileges] used by [CommonType.MarshalJSON]), then any
// remaining §2 baseline members in spec-§2 order, then the type-
// specific members in declared order. The anonymous-struct pattern is
// identical to [CommonType.MarshalJSON] — Go's encoding/json walks
// fields in declaration order, so the wire ordering comes from the
// struct's source layout for free. Pinning the §4-figure order is
// what lets the round-trip test compare bytes against
// [specfixtures.Multiple] directly.
func (p *paymentInitiationType) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type                              string   `json:"type"`
		Actions                           []string `json:"actions,omitempty"`
		Locations                         []string `json:"locations,omitempty"`
		Datatypes                         []string `json:"datatypes,omitempty"`
		Identifier                        string   `json:"identifier,omitempty"`
		Privileges                        []string `json:"privileges,omitempty"`
		InstructedAmount                  amount   `json:"instructedAmount,omitzero"`
		CreditorName                      string   `json:"creditorName,omitempty"`
		CreditorAccount                   account  `json:"creditorAccount,omitzero"`
		RemittanceInformationUnstructured string   `json:"remittanceInformationUnstructured,omitempty"`
	}{
		Type:                              p.TypeName,
		Actions:                           p.Actions,
		Locations:                         p.Locations,
		Datatypes:                         p.Datatypes,
		Identifier:                        p.Identifier,
		Privileges:                        p.Privileges,
		InstructedAmount:                  p.InstructedAmount,
		CreditorName:                      p.CreditorName,
		CreditorAccount:                   p.CreditorAccount,
		RemittanceInformationUnstructured: p.RemittanceInformationUnstructured,
	})
}

// UnmarshalJSON populates the receiver from the wire object. Same
// anonymous-struct shape as [paymentInitiationType.MarshalJSON]; the
// aux struct breaks the would-be infinite recursion that would
// otherwise occur if we called json.Unmarshal on a
// *paymentInitiationType from inside its own UnmarshalJSON.
func (p *paymentInitiationType) UnmarshalJSON(b []byte) error {
	var aux struct {
		Type                              string   `json:"type"`
		Locations                         []string `json:"locations,omitempty"`
		Actions                           []string `json:"actions,omitempty"`
		Datatypes                         []string `json:"datatypes,omitempty"`
		Identifier                        string   `json:"identifier,omitempty"`
		Privileges                        []string `json:"privileges,omitempty"`
		InstructedAmount                  amount   `json:"instructedAmount,omitzero"`
		CreditorName                      string   `json:"creditorName,omitempty"`
		CreditorAccount                   account  `json:"creditorAccount,omitzero"`
		RemittanceInformationUnstructured string   `json:"remittanceInformationUnstructured,omitempty"`
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
	p.InstructedAmount = aux.InstructedAmount
	p.CreditorName = aux.CreditorName
	p.CreditorAccount = aux.CreditorAccount
	p.RemittanceInformationUnstructured = aux.RemittanceInformationUnstructured
	return nil
}

// paymentInitiationFixture is the second element of the §4 multiple-
// details example — the exact bytes the spec figure carries on the
// wire. Kept as a package-level literal so multiple tests can share
// it without re-extracting from [specfixtures.Multiple].
var paymentInitiationFixture = []byte(`{"type":"payment_initiation","actions":["initiate","status","cancel"],"locations":["https://example.com/payments"],"instructedAmount":{"currency":"EUR","amount":"123.50"},"creditorName":"Merchant A","creditorAccount":{"iban":"DE02100100109307118603"},"remittanceInformationUnstructured":"Ref Number Merchant"}`)

// TestExtension_RegisterAndParseRoundTrip is the headline path: a
// consumer registers a constructor for an IANA-published `type`
// value, parses a payload carrying that discriminator, and gets back
// the consumer-defined concrete type with every field populated. The
// produced value re-marshals byte-stably so the registered shape
// participates in the same round-trip guarantee the spec-fixture
// tests pin on [CommonType] and [UnknownType].
func TestExtension_RegisterAndParseRoundTrip(t *testing.T) {
	resetRegistryForTest(t)

	if err := RegisterType("payment_initiation", func() AuthorizationDetail { return &paymentInitiationType{} }); err != nil {
		t.Fatalf("RegisterType(\"payment_initiation\", _) = %v; want nil", err)
	}

	parsed, err := Parse(paymentInitiationFixture)
	if err != nil {
		t.Fatalf("Parse(paymentInitiationFixture) = %v; want nil", err)
	}

	got, ok := parsed.(*paymentInitiationType)
	if !ok {
		t.Fatalf("Parse returned %T; want *paymentInitiationType", parsed)
	}

	if got.TypeName != "payment_initiation" {
		t.Errorf("TypeName = %q; want %q", got.TypeName, "payment_initiation")
	}
	wantActions := []string{"initiate", "status", "cancel"}
	if !equalStringSlice(got.Actions, wantActions) {
		t.Errorf("Actions = %v; want %v", got.Actions, wantActions)
	}
	wantLocations := []string{"https://example.com/payments"}
	if !equalStringSlice(got.Locations, wantLocations) {
		t.Errorf("Locations = %v; want %v", got.Locations, wantLocations)
	}
	wantAmount := amount{Currency: "EUR", Amount: "123.50"}
	if got.InstructedAmount != wantAmount {
		t.Errorf("InstructedAmount = %+v; want %+v", got.InstructedAmount, wantAmount)
	}
	if got.CreditorName != "Merchant A" {
		t.Errorf("CreditorName = %q; want %q", got.CreditorName, "Merchant A")
	}
	wantAccount := account{IBAN: "DE02100100109307118603"}
	if got.CreditorAccount != wantAccount {
		t.Errorf("CreditorAccount = %+v; want %+v", got.CreditorAccount, wantAccount)
	}
	if got.RemittanceInformationUnstructured != "Ref Number Merchant" {
		t.Errorf("RemittanceInformationUnstructured = %q; want %q", got.RemittanceInformationUnstructured, "Ref Number Merchant")
	}

	if vErr := got.Validate(); vErr != nil {
		t.Errorf("Validate() = %v; want nil", vErr)
	}

	roundTripped, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("json.Marshal(got) = %v; want nil", err)
	}
	if !bytes.Equal(roundTripped, paymentInitiationFixture) {
		t.Errorf("round-trip not byte-stable\n got: %s\nwant: %s", roundTripped, paymentInitiationFixture)
	}
}

// TestExtension_RegisterTypeCommonCollisionReturnsErrTypeReserved
// pins the reserved-name contract from the registry: attempting to
// register a constructor under "common" returns [ErrTypeReserved]
// (which also matches errors.Is against the package umbrella [Err]),
// and the registry itself is NOT mutated — the built-in entry stays
// pointed at *CommonType so the codec still dispatches correctly.
func TestExtension_RegisterTypeCommonCollisionReturnsErrTypeReserved(t *testing.T) {
	resetRegistryForTest(t)

	// Use a deliberately wrong constructor (returns *paymentInitiationType
	// under the "common" name) — if the rejection were silently dropped
	// and the registration went through anyway, the lookup below would
	// surface the swap.
	err := RegisterType("common", func() AuthorizationDetail { return &paymentInitiationType{} })
	if err == nil {
		t.Fatalf("RegisterType(\"common\", _) = nil error; want ErrTypeReserved")
	}
	if !errors.Is(err, ErrTypeReserved) {
		t.Errorf("errors.Is(err, ErrTypeReserved) = false; want true (err=%v)", err)
	}
	if !errors.Is(err, Err) {
		t.Errorf("errors.Is(err, Err) = false; want true (err=%v)", err)
	}

	// The registry must be unchanged: the built-in "common" entry
	// still returns a *CommonType, not the would-be replacement.
	ctor := lookup("common")
	if ctor == nil {
		t.Fatalf("lookup(\"common\") = nil; want the built-in constructor intact")
	}
	val := ctor()
	if _, ok := val.(*CommonType); !ok {
		t.Fatalf("after rejected RegisterType(\"common\"), lookup returns %T; want *CommonType", val)
	}
}

// otherTestType is a second test-local AuthorizationDetail used only
// by TestExtension_ReregisterReplacesPriorCtor — it exists to give
// the replacement constructor a *different* concrete Go type than
// the first constructor's return value, so the test can distinguish
// "ctor B replaced ctor A" from "ctor A's return shape happened to
// match" via a type assertion.
type otherTestType struct {
	TypeName string
	commonBaseline
}

func (o *otherTestType) Type() string    { return o.TypeName }
func (o *otherTestType) Common() *Common { return &o.commonBaseline }
func (o *otherTestType) Validate() error { return nil }
func (o *otherTestType) sealed()         {}

// UnmarshalJSON is the minimal shape the dispatch needs: read the
// discriminator into TypeName and discard everything else (this type
// is only used to prove ctor identity, not to round-trip).
func (o *otherTestType) UnmarshalJSON(b []byte) error {
	var aux struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	o.TypeName = aux.Type
	return nil
}

var _ AuthorizationDetail = (*otherTestType)(nil)

// TestExtension_ReregisterReplacesPriorCtor pins the documented
// "re-registering a previously consumer-registered name silently
// replaces the prior constructor" behavior from [RegisterType]'s
// godoc. The behavior is convenient for tests that swap a
// constructor in and out; init-time consumers should register each
// name at most once per process. Exercising the swap here keeps the
// contract from regressing.
func TestExtension_ReregisterReplacesPriorCtor(t *testing.T) {
	resetRegistryForTest(t)

	// ctor A returns *paymentInitiationType.
	if err := RegisterType("payment_initiation", func() AuthorizationDetail { return &paymentInitiationType{} }); err != nil {
		t.Fatalf("first RegisterType = %v; want nil", err)
	}
	// ctor B returns a *different* test-local concrete type.
	if err := RegisterType("payment_initiation", func() AuthorizationDetail { return &otherTestType{} }); err != nil {
		t.Fatalf("second RegisterType = %v; want nil", err)
	}

	// A payload with type:"payment_initiation" must now land on the
	// ctor-B type. Parse takes any object whose `type` member is
	// present, so a minimal object is enough to drive the dispatch.
	parsed, err := Parse([]byte(`{"type":"payment_initiation"}`))
	if err != nil {
		t.Fatalf("Parse = %v; want nil", err)
	}
	if _, ok := parsed.(*otherTestType); !ok {
		t.Fatalf("after re-registration, Parse returned %T; want *otherTestType", parsed)
	}
}

// TestExtension_ParseArrayWithRegisteredAndUnregistered exercises the
// mixed-dispatch path against the full §4 fixture: the first element
// (account_information) is intentionally NOT registered, so it falls
// through to *UnknownType; the second element (payment_initiation) IS
// registered, so it lands on the consumer type. The whole array
// re-marshals byte-stably against the fixture, proving that the
// extension carrier and the forward-compat carrier coexist on the
// same wire path without either disturbing the other's round-trip.
func TestExtension_ParseArrayWithRegisteredAndUnregistered(t *testing.T) {
	resetRegistryForTest(t)

	if err := RegisterType("payment_initiation", func() AuthorizationDetail { return &paymentInitiationType{} }); err != nil {
		t.Fatalf("RegisterType(\"payment_initiation\", _) = %v; want nil", err)
	}

	details, err := ParseArray(specfixtures.Multiple)
	if err != nil {
		t.Fatalf("ParseArray(specfixtures.Multiple) = %v; want nil", err)
	}
	if got, want := len(details), 2; got != want {
		t.Fatalf("ParseArray returned %d elements; want %d", got, want)
	}

	// Element 0 — account_information is unregistered, so it falls
	// through to the forward-compat carrier.
	unk, ok := details[0].(*UnknownType)
	if !ok {
		t.Fatalf("details[0] is %T; want *UnknownType", details[0])
	}
	if unk.TypeName != "account_information" {
		t.Errorf("details[0].TypeName = %q; want %q", unk.TypeName, "account_information")
	}

	// Element 1 — payment_initiation IS registered, so it lands on
	// the consumer type.
	pi, ok := details[1].(*paymentInitiationType)
	if !ok {
		t.Fatalf("details[1] is %T; want *paymentInitiationType", details[1])
	}
	if pi.TypeName != "payment_initiation" {
		t.Errorf("details[1].TypeName = %q; want %q", pi.TypeName, "payment_initiation")
	}

	// The whole array must re-marshal byte-stably. json.Marshal on a
	// []AuthorizationDetail walks the slice and calls each element's
	// MarshalJSON — *UnknownType emits its captured Raw verbatim,
	// *paymentInitiationType emits the spec-ordered struct above,
	// and the result is byte-identical to the fixture.
	roundTripped, err := json.Marshal(details)
	if err != nil {
		t.Fatalf("json.Marshal(details) = %v; want nil", err)
	}
	if !bytes.Equal(roundTripped, specfixtures.Multiple) {
		t.Errorf("round-trip not byte-stable\n got: %s\nwant: %s", roundTripped, specfixtures.Multiple)
	}
}

// equalStringSlice is a tiny local helper to avoid pulling reflect
// or slices.Equal into a single comparison site. Returns true when
// both slices have the same length and element-wise equality.
func equalStringSlice(a, b []string) bool {
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
