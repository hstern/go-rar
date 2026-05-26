// Copyright 2026 The go-rar Authors
// SPDX-License-Identifier: Apache-2.0

package rar

// Baseline is a public type alias for [Common], intended for
// embedding inside [Extension] and consumer-defined types. The alias
// exists to dodge the field-vs-method name collision that arises
// when a type embeds Common and also satisfies the
// [AuthorizationDetail.Common] method: Go derives the embedded field
// name from the unqualified type name, so embedding `Common` would
// make the field name "Common" — colliding with the method. Embedding
// `Baseline` names the field "Baseline" instead, leaving the method
// name free.
//
// Because this is a Go type alias (not a defined type), Baseline IS
// Common — same fields, same methods, same JSON-promotion behavior.
// The two names are fully interchangeable; consumers can write
// either `rar.Baseline{...}` or `rar.Common{...}` as the composite
// literal and they are identical.
type Baseline = Common

// Extension is the embeddable base struct that lets consumer-defined
// types satisfy [AuthorizationDetail] from outside the rar package.
//
// The AuthorizationDetail interface is sealed via an unexported
// sealed() marker method that consumers cannot implement directly.
// Embedding Extension into a consumer struct transitively grants the
// sealed() method (and the other AuthorizationDetail methods), so
// the consumer type satisfies the interface without breaking the
// seal — the library still controls every concrete type the codec
// produces, and consumers gain typed extension only through
// registration via [RegisterType].
//
// # Consumer pattern
//
// Embed Extension, add type-specific fields, override [Extension.Validate]
// for type-specific rules, and write MarshalJSON / UnmarshalJSON
// following the pattern in [CommonType] (anonymous struct in spec order:
// type first, then §2 baseline, then type-specific members in declared
// order):
//
//	type PaymentInitiation struct {
//	    rar.Extension
//	    InstructedAmount Amount `json:"instructedAmount,omitempty"`
//	    CreditorName     string `json:"creditorName,omitempty"`
//	}
//
//	func (p *PaymentInitiation) Validate() error {
//	    if err := p.Extension.Validate(); err != nil { return err }
//	    if p.CreditorName == "" {
//	        return &rar.ValidationError{
//	            Rule: "creditorName-required", Type: p.TypeName,
//	            Reason: "creditorName must be non-empty",
//	        }
//	    }
//	    return nil
//	}
//
//	func (p *PaymentInitiation) MarshalJSON() ([]byte, error) { ... }
//	func (p *PaymentInitiation) UnmarshalJSON(b []byte) error { ... }
//
//	rar.RegisterType("payment_initiation", func() rar.AuthorizationDetail {
//	    return &PaymentInitiation{Extension: rar.Extension{TypeName: "payment_initiation"}}
//	})
//
// See extension_external_test.go for the complete pattern exercised
// from an out-of-package test.
//
// # Direct use
//
// Extension is also usable directly without embedding, in which case
// it behaves identically to [CommonType] — both ship the §2-only
// carrier shape; CommonType is the built-in registered under "common"
// at package init, while Extension is reserved for consumer
// registrations under any other discriminator.
type Extension struct {
	// TypeName is the `type` discriminator value for this element.
	// RFC 9396 §2 requires the member to be a non-empty string;
	// consumer types embedding Extension typically set it inside the
	// registered constructor passed to [RegisterType].
	TypeName string

	// Baseline is the embedded §2 baseline (aliased to Common — see
	// the alias declaration above for why the alias is needed rather
	// than embedding Common directly). Embedding lifts the baseline
	// members onto the receiver's wire shape so they marshal as
	// top-level object members alongside `type`, matching every spec
	// example.
	Baseline
}

// Type returns the wire `type` discriminator the element carries.
// Implements [AuthorizationDetail].
func (e *Extension) Type() string { return e.TypeName }

// Common returns a pointer to the embedded §2 baseline struct.
// Implements [AuthorizationDetail]. The returned pointer aliases
// the embedded field, so writes through it mutate the receiver.
func (e *Extension) Common() *Common { return &e.Baseline }

// Validate runs the RFC 9396 §2 well-formedness checks for the
// baseline: type-required, locations URI syntax, and non-empty
// element rules for actions / datatypes / privileges. Consumer
// types embedding Extension should call this from their own
// Validate to inherit the baseline rules and then add type-specific
// checks. Implements [AuthorizationDetail].
func (e *Extension) Validate() error {
	if e.TypeName == "" {
		return &ValidationError{
			Rule:   "type-required",
			Reason: "type member must be a non-empty string",
		}
	}
	if err := validateLocations(e.TypeName, e.Locations); err != nil {
		return err
	}
	if err := validateNonEmptyStrings(e.TypeName, "actions", e.Actions); err != nil {
		return err
	}
	if err := validateNonEmptyStrings(e.TypeName, "datatypes", e.Datatypes); err != nil {
		return err
	}
	if err := validateNonEmptyStrings(e.TypeName, "privileges", e.Privileges); err != nil {
		return err
	}
	return nil
}

// sealed satisfies the unexported marker on [AuthorizationDetail].
// Embedded into consumer structs to grant them the marker without
// exporting it — this is what lets out-of-package types satisfy the
// otherwise-sealed interface while keeping the library in control of
// every concrete type the codec produces.
func (e *Extension) sealed() {}

// MarshalJSON emits the §2 baseline in spec order, preserving
// nil-vs-length-zero on slice fields (see [CommonType.MarshalJSON]
// for the same logic). When [SetStrictMarshal] is true, validates
// first and returns the [*ValidationError] on failure.
//
// Consumer types embedding Extension typically override this method
// to handle their type-specific fields; the override should write
// `type` first, then the §2 baseline in spec order, then
// type-specific members in declared order. See the package
// documentation for the consumer pattern.
func (e *Extension) MarshalJSON() ([]byte, error) {
	if strictMarshal() {
		if err := e.Validate(); err != nil {
			return nil, err
		}
	}
	return marshalCommonShape(e.TypeName, &e.Baseline)
}

// UnmarshalJSON decodes the §2 baseline plus the type discriminator.
// Type-specific fields are silently dropped at this level; consumer
// types embedding Extension typically override UnmarshalJSON to
// populate their type-specific fields, following the pattern in
// [CommonType.UnmarshalJSON].
func (e *Extension) UnmarshalJSON(b []byte) error {
	return unmarshalCommonShape(b, &e.TypeName, &e.Baseline)
}

var _ AuthorizationDetail = (*Extension)(nil)
