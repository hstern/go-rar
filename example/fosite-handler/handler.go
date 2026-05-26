// Copyright 2026 The go-rar Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ory/fosite"

	"github.com/hstern/go-rar"
)

// detailsHandler is the RFC 9396 plug-in for fosite. It is registered
// alongside fosite's built-in AuthorizeExplicitGrantHandler and
// CoreValidator (introspection); the three handlers together carry the
// authorization_details slice across all three OAuth touchpoints the
// spec covers:
//
//  1. /authorize: HandleAuthorizeEndpointRequest reads the form value,
//     decodes it via rar.DecodeForm, validates via rar.ValidateAll,
//     and stashes the slice into the *sessionWithDetails. fosite then
//     persists the request (including the same session pointer) under
//     the authorize code in CoreStorage.
//  2. /token: PopulateTokenEndpointResponse runs after fosite's
//     built-in code-to-token exchange has re-attached the persisted
//     session to the access request. It pulls the slice off the
//     session and surfaces it as the "authorization_details" JSON
//     member on the token response (RFC 9396 §7).
//  3. /introspect: the same slice flows out via sessionWithDetails'
//     GetExtraClaims method, picked up by fosite's
//     WriteIntrospectionResponse (RFC 9396 §9).
//
// detailsHandler returns ErrUnknownRequest from HandleTokenEndpointRequest
// and false from CanHandleTokenEndpointRequest because it does not own
// the request — fosite's AuthorizeExplicitGrantHandler does. fosite's
// dispatch loop calls every registered handler's
// PopulateTokenEndpointResponse regardless of ownership, which is the
// hook this type uses to layer authorization_details onto the
// already-populated response.
type detailsHandler struct{}

// Compile-time confirmation that detailsHandler satisfies both fosite
// handler shapes it needs to plug into.
var (
	_ fosite.AuthorizeEndpointHandler = (*detailsHandler)(nil)
	_ fosite.TokenEndpointHandler     = (*detailsHandler)(nil)
)

// HandleAuthorizeEndpointRequest decodes the authorization_details
// form value and stores the decoded slice on the session for later legs
// to pick up. It does not "handle" the response in the fosite sense —
// it never calls SetResponseTypeHandled or AddParameter on the
// responder — so the built-in AuthorizeExplicitGrantHandler still owns
// issuing the authorize code.
func (detailsHandler) HandleAuthorizeEndpointRequest(_ context.Context, ar fosite.AuthorizeRequester, _ fosite.AuthorizeResponder) error {
	formValue := ar.GetRequestForm().Get("authorization_details")
	if formValue == "" {
		// No authorization_details on this request — that is
		// well-formed under RFC 9396 (the parameter is optional), so
		// just leave the session's slice nil.
		return nil
	}

	details, err := rar.DecodeForm(formValue)
	if err != nil {
		return fosite.ErrInvalidRequest.WithHintf("authorization_details could not be decoded: %s", err.Error()).WithWrap(err)
	}
	if err := rar.ValidateAll(details); err != nil {
		return fosite.ErrInvalidRequest.WithHintf("authorization_details failed validation: %s", err.Error()).WithWrap(err)
	}

	sess, ok := ar.GetSession().(*sessionWithDetails)
	if !ok {
		// The caller wired a session type the handler does not
		// recognize — this is a programmer error in the AS setup, not
		// a client error, so return a server error rather than an
		// invalid_request.
		return fosite.ErrServerError.WithHintf("session type %T does not carry RFC 9396 authorization_details", ar.GetSession())
	}
	sess.AuthorizationDetails = details

	fmt.Printf("  [AS] /authorize stashed %d authorization_detail(s) onto session; first type=%q\n",
		len(details), details[0].Type())
	return nil
}

// PopulateTokenEndpointResponse reads the authorization_details slice
// off the session that fosite re-attached during the code-to-token
// exchange and writes it as a JSON member on the token response. The
// json.RawMessage wrapper carries the spec-ordered bytes that rar's
// MarshalJSON produced so that fosite's json.Marshal pass on
// AccessResponse.ToMap() copies them through verbatim.
func (detailsHandler) PopulateTokenEndpointResponse(_ context.Context, requester fosite.AccessRequester, responder fosite.AccessResponder) error {
	if !requester.GetGrantTypes().ExactOne("authorization_code") {
		return fosite.ErrUnknownRequest
	}

	sess, ok := requester.GetSession().(*sessionWithDetails)
	if !ok || len(sess.AuthorizationDetails) == 0 {
		// No details to surface — either the client did not send any
		// at /authorize, or the session was wired with a different
		// type. Either way, nothing to add to the response.
		return nil
	}

	encoded, err := json.Marshal(sess.AuthorizationDetails)
	if err != nil {
		return fosite.ErrServerError.WithWrap(err)
	}
	responder.SetExtra("authorization_details", json.RawMessage(encoded))
	fmt.Printf("  [AS] /token populated authorization_details with %d detail(s)\n",
		len(sess.AuthorizationDetails))
	return nil
}

// HandleTokenEndpointRequest is a no-op for this handler: the built-in
// AuthorizeExplicitGrantHandler validates the authorize code and
// rehydrates the session. Returning ErrUnknownRequest is the fosite
// idiom for "I am not responsible for this request".
func (detailsHandler) HandleTokenEndpointRequest(_ context.Context, _ fosite.AccessRequester) error {
	return fosite.ErrUnknownRequest
}

// CanSkipClientAuth returns false because the built-in handler that
// actually owns this request (AuthorizeExplicitGrantHandler) also
// returns false — there is no scenario where this handler is called
// without an authenticated client.
func (detailsHandler) CanSkipClientAuth(_ context.Context, _ fosite.AccessRequester) bool {
	return false
}

// CanHandleTokenEndpointRequest returns false so fosite's request
// dispatch loop never calls HandleTokenEndpointRequest on this
// handler. PopulateTokenEndpointResponse is dispatched regardless,
// which is the hook the handler actually uses.
func (detailsHandler) CanHandleTokenEndpointRequest(_ context.Context, _ fosite.AccessRequester) bool {
	return false
}
