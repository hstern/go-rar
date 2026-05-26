// Copyright 2026 The go-rar Authors
// SPDX-License-Identifier: Apache-2.0

package rar

// commonBaseline is a file-local type alias for [Common] used as the
// embedded-field name on [CommonType]. The alias exists solely to
// pick the embedded field's name: Go derives the field name of an
// embedded type from its unqualified type name, so embedding Common
// directly would make the field name "Common" — which would collide
// with the [AuthorizationDetail] interface's Common() method and
// prevent CommonType from satisfying the interface. Embedding the
// alias instead names the field commonBaseline, leaving the method
// name free.
//
// Because this is a Go type alias (not a defined type), commonBaseline
// IS Common — same methods, same JSON marshaling promotion rules —
// so the wire shape is identical to what plain embedding would have
// produced. The alias is unexported because consumers reach the §2
// fields via the promoted selectors (c.Locations, c.Actions, …) or
// via the [CommonType.Common] method; nothing outside this file needs
// to name the alias.
type commonBaseline = Common

// CommonType is the one built-in [AuthorizationDetail] implementation
// the library ships. It carries any authorization_details element
// whose only members are the RFC 9396 §2 baseline — the elements that
// do not extend the discriminated union with type-specific fields.
//
// The dispatch table registers CommonType under the literal `type`
// discriminator string "common"; any payload the codec parses whose
// `type` is "common" lands here. Payloads with any other `type` value
// either dispatch to a consumer-registered constructor or fall through
// to [UnknownType]. [RegisterType], the dispatch table itself, and the
// constructor wiring are codec concerns; this file declares only the
// type and its [AuthorizationDetail]-satisfying methods.
//
// *CommonType is functionally equivalent to a directly-used
// *[Extension]; the library ships both because CommonType is the
// registered "common" discriminator while Extension is the embeddable
// base for consumer-defined types that need to satisfy the sealed
// [AuthorizationDetail] interface from outside the rar package.
//
// CommonType embeds the §2 baseline (via the [commonBaseline] alias —
// see that type's documentation for why the alias is necessary)
// rather than holding a *Common field, so the §2 fields sit directly
// on the wire shape. Locations, Actions, Datatypes, Identifier, and
// Privileges marshal alongside `type` as top-level object members,
// matching every example in RFC 9396 §2–§9. Embedding by value also
// keeps the zero value usable: a freshly constructed CommonType has
// empty §2 fields and a writable TypeName, which is exactly what the
// dispatch constructor needs.
//
// Field semantics:
//
//   - TypeName carries the spec's `type` discriminator string. For
//     payloads parsed via the codec, the dispatch registry fills
//     TypeName from the wire `type` value before returning the
//     constructed *CommonType. Producers building a CommonType by hand
//     are responsible for setting TypeName to a non-empty string
//     (RFC 9396 §2 makes the `type` member required and non-empty);
//     [CommonType.Validate] enforces that rule via the
//     "type-required" check.
//
//   - The embedded baseline supplies the §2 fields. See [Common] for
//     per-field semantics and the marshal-order guarantees.
type CommonType struct {
	// TypeName is the `type` discriminator value for this element.
	// RFC 9396 §2 requires the member to be a non-empty string; the
	// codec populates it from the unmarshaled object, and
	// [CommonType.Validate] rejects an empty value via the
	// "type-required" rule.
	TypeName string

	// commonBaseline is the embedded §2 baseline (aliased to Common —
	// see the alias declaration above for why the alias is needed
	// rather than embedding Common directly). Embedding lifts the
	// baseline members onto CommonType's wire shape so they marshal
	// as top-level object members alongside `type`, matching every
	// spec example.
	commonBaseline
}

// Type returns the wire `type` discriminator the element carries.
// Implements [AuthorizationDetail].
func (c *CommonType) Type() string { return c.TypeName }

// Common returns a pointer to the embedded §2 baseline struct.
// Implements [AuthorizationDetail]. The returned pointer aliases the
// embedded field, so writes through it mutate the receiver.
func (c *CommonType) Common() *Common { return &c.commonBaseline }

// Validate enforces the RFC 9396 §2 well-formedness rules that apply
// categorically to every authorization_details element, regardless of
// type-specific shape. Implements [AuthorizationDetail].
//
// Rules enforced, in evaluation order:
//
//   - "type-required" — TypeName must be a non-empty string. RFC 9396
//     §2 makes the `type` member required and non-empty; an element
//     missing the discriminator cannot be dispatched and is rejected
//     here as a structural error rather than a per-type concern.
//
//   - "locations-uri" — every entry of [Common.Locations] must be a
//     syntactically valid absolute URI (a parseable reference carrying
//     a non-empty scheme). RFC 9396 §2 defines locations as URIs
//     identifying the resource the authorization applies to; relative
//     references and empty strings cannot stand alone. Delegated to
//     [validateLocations]; see that helper for the exact two-step
//     check and why empty entries are caught here rather than via the
//     non-empty-string helper.
//
//   - "actions-element-empty", "datatypes-element-empty",
//     "privileges-element-empty" — none of the three §2 array-of-
//     strings members may contain an empty-string element. RFC 9396 §2
//     does not literally forbid it, but an empty value in a
//     permission-like field is meaningless and almost always a
//     producer bug; the library surfaces it through opt-in Validate
//     rather than at marshal time. Delegated to
//     [validateNonEmptyStrings]; see that helper for the rationale.
//
// [Common.Identifier] is intentionally not validated. RFC 9396 §2
// defines it as a free-form identifier the authorization server
// recognizes; the library has no view on what shapes are meaningful
// and leaves any structural check to the consumer.
//
// Validate returns the first violation as a [*ValidationError]; on a
// well-formed value it returns nil. Per-element validation is opt-in
// via [ValidateAll], which aggregates across elements; within a single
// element, the first failing rule wins so that callers get a
// deterministic, structurally-meaningful answer (a missing
// discriminator is reported before a bad locations entry, etc.).
func (c *CommonType) Validate() error {
	if c.TypeName == "" {
		return &ValidationError{
			Rule:   "type-required",
			Type:   "",
			Reason: "type member must be a non-empty string",
		}
	}
	if err := validateLocations(c.TypeName, c.Locations); err != nil {
		return err
	}
	if err := validateNonEmptyStrings(c.TypeName, "actions", c.Actions); err != nil {
		return err
	}
	if err := validateNonEmptyStrings(c.TypeName, "datatypes", c.Datatypes); err != nil {
		return err
	}
	if err := validateNonEmptyStrings(c.TypeName, "privileges", c.Privileges); err != nil {
		return err
	}
	return nil
}

// sealed satisfies the unexported marker on [AuthorizationDetail],
// keeping CommonType inside the package's sealed-interface posture.
func (c *CommonType) sealed() {}

var _ AuthorizationDetail = (*CommonType)(nil)
