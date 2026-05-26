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
// The dispatch table (landing in a later commit) registers CommonType
// under the literal `type` discriminator string "common"; any payload
// the codec parses whose `type` is "common" lands here. Payloads with
// any other `type` value either dispatch to a consumer-registered
// constructor or fall through to [UnknownType]. RegisterType, the
// dispatch table itself, and the constructor wiring are codec
// concerns; this file declares only the type and its
// AuthorizationDetail-satisfying methods.
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
//     [CommonType.Validate] enforces that rule once the validation
//     phase lands.
//
//   - The embedded baseline supplies the §2 fields. See [Common] for
//     per-field semantics and the marshal-order guarantees.
type CommonType struct {
	// TypeName is the `type` discriminator value for this element.
	// RFC 9396 §2 requires the member to be a non-empty string; the
	// codec populates it from the unmarshaled object, and the
	// validation phase (a later commit) rejects an empty value via
	// the type-required rule.
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

// Validate is a stub awaiting the validation phase. It currently
// returns nil. A later commit replaces this body with the RFC 9396
// §2 well-formedness check — at minimum the type-required rule
// (non-empty TypeName) plus the opt-in URI syntax check on
// Locations entries. Implements [AuthorizationDetail].
func (c *CommonType) Validate() error { return nil }

// sealed satisfies the unexported marker on [AuthorizationDetail],
// keeping CommonType inside the package's sealed-interface posture.
func (c *CommonType) sealed() {}

var _ AuthorizationDetail = (*CommonType)(nil)
