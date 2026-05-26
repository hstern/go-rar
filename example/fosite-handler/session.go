// Copyright 2026 The go-rar Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"

	"github.com/ory/fosite"

	"github.com/hstern/go-rar"
)

// sessionWithDetails extends fosite's default session with the RFC 9396
// authorization_details slice that the request carried at the
// authorization-endpoint leg. fosite's session pointer is what survives
// across legs: the AuthorizeExplicitGrantHandler stores the sanitized
// Requester (carrying the same *sessionWithDetails pointer) under the
// authorize code, and re-attaches that session to the token-endpoint
// requester before PopulateTokenEndpointResponse runs. Our custom
// TokenEndpointHandler then reads the details back off the session and
// echoes them into the token response (RFC 9396 §7) and the
// introspection response (RFC 9396 §9).
//
// Two interface contracts have to keep working for that thread to hold
// together: fosite.Session (so the type can replace DefaultSession in
// the request pipeline) and fosite.ExtraClaimsSession (so the
// introspection writer surfaces extra fields, per the implementation
// in fosite/introspection_response_writer.go).
type sessionWithDetails struct {
	*fosite.DefaultSession

	// AuthorizationDetails is the slice the client sent on /authorize.
	// Nil means "client did not send an authorization_details parameter";
	// the example treats that as a hard error, but a real AS might
	// also fall through to scope-only authorization.
	AuthorizationDetails rar.AuthorizationDetails
}

// Clone returns an independent copy of the session. fosite calls
// Session.Clone() at several points (introspection in particular) and
// the contract is that the returned value can be mutated without
// affecting the receiver. The embedded DefaultSession knows how to
// deep-copy itself; the only field this type adds is an
// AuthorizationDetails slice, and a fresh slice header over the same
// element pointers is enough because each element is itself only ever
// produced by rar's decoder (which allocates fresh values per call)
// and never mutated after that.
func (s *sessionWithDetails) Clone() fosite.Session {
	if s == nil {
		return nil
	}
	inner, ok := s.DefaultSession.Clone().(*fosite.DefaultSession)
	if !ok {
		// DefaultSession.Clone always returns *DefaultSession (see
		// fosite/session.go), so this branch is structurally
		// unreachable. Panic is the right response for a contract
		// violation that the type system cannot express; the demo
		// would otherwise silently hand back a session missing the
		// embedded DefaultSession pointer.
		panic("fosite.DefaultSession.Clone returned a non-*DefaultSession")
	}
	cloned := &sessionWithDetails{DefaultSession: inner}
	if s.AuthorizationDetails != nil {
		cloned.AuthorizationDetails = append(rar.AuthorizationDetails(nil), s.AuthorizationDetails...)
	}
	return cloned
}

// GetExtraClaims implements fosite.ExtraClaimsSession so that the
// introspection-response writer (fosite.WriteIntrospectionResponse)
// surfaces authorization_details alongside the standard RFC 7662
// fields. The writer iterates the returned map and skips a fixed set
// of reserved names (exp, client_id, scope, iat, sub, aud, username);
// authorization_details is not on that list and so passes through.
//
// json.RawMessage carries the marshaled details rather than the
// AuthorizationDetails slice directly because fosite's writer
// json.Marshal()s the whole response map in one shot; using
// RawMessage means rar's per-element MarshalJSON has already produced
// the spec-ordered bytes (type first, common members in spec order)
// and json.Marshal will copy them through verbatim instead of
// re-marshaling a Go slice and risking a re-order.
func (s *sessionWithDetails) GetExtraClaims() map[string]any {
	extras := s.DefaultSession.GetExtraClaims()
	if len(s.AuthorizationDetails) == 0 {
		return extras
	}
	encoded, err := json.Marshal(s.AuthorizationDetails)
	if err != nil {
		// The marshal path on rar's built-in types cannot fail in
		// practice — the only error path is a custom MarshalJSON on a
		// registered type, and this example registers none. Panicking
		// here keeps the demo binary simple; a production integration
		// would log + omit the field instead.
		panic(err)
	}
	extras["authorization_details"] = json.RawMessage(encoded)
	return extras
}
