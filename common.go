// Copyright 2026 The go-rar Authors
// SPDX-License-Identifier: Apache-2.0

package rar

// Common carries the RFC 9396 §2 baseline members shared by every
// authorization_details element. All five members are OPTIONAL per
// the spec; consumers populate only what their type definition
// requires. Field order mirrors the spec's enumeration in §2 so that
// MarshalJSON output matches the byte order of every example in
// RFC 9396 §2–§9 (see the "type member always first; §2 common
// members in spec order" sub-decision in the design notes).
//
// The struct is intended for embedding into per-type carriers — see
// CommonType for the §2-only case and the registry types for the
// extension case. Validation lives in Validate(), not on the fields
// themselves; consult that method for the RFC 9396 rules that apply.
type Common struct {
	// Locations enumerates the resource servers (or other locations)
	// the authorization applies to. Each entry is a URI per RFC 9396
	// §2; URI well-formedness is checked only via opt-in Validate(),
	// never at marshal time by default.
	Locations []string `json:"locations,omitempty"`

	// Actions enumerates the operations the client requests at the
	// resource (e.g. "read", "write"). Free-form strings per RFC 9396
	// §2; the spec defers semantics to the type definition.
	Actions []string `json:"actions,omitempty"`

	// Datatypes enumerates the kinds of data the client requests
	// access to (e.g. "contacts", "photos"). Free-form strings per
	// RFC 9396 §2; semantics are type-defined.
	Datatypes []string `json:"datatypes,omitempty"`

	// Identifier identifies a specific resource the authorization
	// applies to, in a form the authorization server recognizes
	// (RFC 9396 §2). Free-form; the library does not validate its
	// shape — consumers do.
	Identifier string `json:"identifier,omitempty"`

	// Privileges enumerates levels or roles the authorization grants
	// at the resource (e.g. "admin", "reader"). Free-form strings per
	// RFC 9396 §2; semantics are type-defined.
	Privileges []string `json:"privileges,omitempty"`
}
