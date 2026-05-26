// Copyright 2026 The go-rar Authors
// SPDX-License-Identifier: Apache-2.0

// fosite-handler walks one RFC 9396 authorization_details value
// through a fosite-backed OAuth 2.0 authorization server end to end —
// the authorization request (/oauth2/auth), the token response
// (/oauth2/token), and the access-token introspection response
// (/oauth2/introspect) — using the real fosite handler pipeline rather
// than the bespoke httptest-only handlers in example/three-flow.
//
// The point of the example is to show what plugging go-rar into a
// published OAuth 2.0 library looks like in practice: a custom session
// type that carries the decoded slice, a one-file
// AuthorizeEndpointHandler + TokenEndpointHandler that registers
// alongside fosite's built-ins, and zero changes to fosite itself.
// fosite already serializes Session.Extra through every leg of the
// auth-code grant; the integration is mostly about defining the right
// session shape and the right handler hook.
//
// The AS is wired with compose.OAuth2AuthorizeExplicitFactory for the
// authorize-code grant and compose.OAuth2TokenIntrospectionFactory for
// RFC 7662 introspection. There is no consent UI (the /oauth2/auth
// handler synthesizes the authorize response immediately), no PKCE
// (RFC 7636 is orthogonal to RFC 9396 — see README for the discussion),
// no real client/user database (one hardcoded confidential client), no
// HTTPS (httptest.Server is plain HTTP under localhost, which fosite
// permits for that hostname suffix), and no persistence
// (storage.NewMemoryStore lives only for the lifetime of the process).
//
// On any error the program panics — the example is a demo, not a
// production binary. Wiring slog or structured error handling would
// add noise without changing the pedagogy.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"time"

	"github.com/ory/fosite"
	"github.com/ory/fosite/compose"
	"github.com/ory/fosite/storage"

	"github.com/hstern/go-rar"
)

const (
	clientID     = "demo-client"
	clientSecret = "demo-secret"
	// redirectPort is fixed so the client's redirect_uri matches what
	// the AS has registered for the demo client. The actual httptest
	// server still picks a random port, but the redirect_uri lives in
	// client config rather than in the AS's listening URL.
	callbackPath = "/callback"
)

func main() {
	// Step 1: build the detail in memory. Same shape used by
	// example/three-flow so a reader can compare side by side.
	detail := buildDetail()
	fmt.Println("== step 1: build the authorization_details value ==")
	printDetailShape(detail)
	fmt.Println()

	// Spin up the fosite-backed AS plus the demo client's redirect
	// receiver. callbackURL is fixed at registration time; the AS's
	// own URL is whatever httptest assigns.
	asSrv, callbackSrv, codeCh := newServers()
	defer asSrv.Close()
	defer callbackSrv.Close()

	registeredCallback := callbackSrv.URL + callbackPath

	// Step 2: authorization request leg. The client points the user
	// agent at /oauth2/auth with authorization_details as a query
	// parameter; the AS reads the parameter, stashes the decoded
	// slice on the session (via detailsHandler in handler.go), and
	// then issues a 302 to the redirect_uri carrying the auth code.
	fmt.Println("== step 2: authorization request (GET /oauth2/auth) ==")
	code := authorizationRequest(asSrv.URL, registeredCallback, rar.AuthorizationDetails{detail}, codeCh)
	fmt.Printf("client received code: %q\n\n", code)

	// Step 3: token response leg. The client exchanges the code for
	// an access token at /oauth2/token. fosite's
	// AuthorizeExplicitGrantHandler validates the code and re-attaches
	// the session that was persisted under it; detailsHandler's
	// PopulateTokenEndpointResponse then surfaces the slice as the
	// authorization_details JSON member on the response body.
	fmt.Println("== step 3: token response (POST /oauth2/token) ==")
	accessToken := tokenRequest(asSrv.URL, registeredCallback, code)
	fmt.Printf("client received access_token: %q\n\n", accessToken)

	// Step 4: introspection leg. The client (acting as the RS in this
	// flow) POSTs the access token to /oauth2/introspect; fosite
	// hydrates the session from storage, the introspection-response
	// writer calls sessionWithDetails.GetExtraClaims (via the
	// fosite.ExtraClaimsSession interface), and the slice flows back
	// out as the authorization_details JSON member on the
	// introspection response.
	fmt.Println("== step 4: introspection (POST /oauth2/introspect) ==")
	introspectionRequest(asSrv.URL, accessToken)

	fmt.Println("== done — same detail crossed all three touchpoints ==")
}

// buildDetail constructs the synthetic detail used in every leg. Same
// shape as example/three-flow so the two demos are visually
// comparable.
func buildDetail() *rar.CommonType {
	d := &rar.CommonType{TypeName: "common"}
	d.Locations = []string{"https://api.example.com/v1/data"}
	d.Actions = []string{"read", "write"}
	d.Datatypes = []string{"contacts", "photos"}
	return d
}

// printDetailShape prints the Go-value view of one detail.
func printDetailShape(d *rar.CommonType) {
	fmt.Printf("  go value: %T{TypeName:%q, Locations:%v, Actions:%v, Datatypes:%v}\n",
		d, d.TypeName, d.Locations, d.Actions, d.Datatypes)
}

// authorizationRequest performs the /oauth2/auth leg. It builds the
// authorize URL with authorization_details as a query parameter,
// dispatches the request through an HTTP client that does NOT follow
// redirects (so the test code can inspect the Location header), and
// reads the auth code out of the redirect's query string.
//
// In a real browser flow the user agent would follow the redirect to
// the client's /callback, which would then invoke the client's
// code-exchange logic. The demo collapses that into a synchronous
// "wait for the callback handler to write the code into a channel"
// step, since the redirect target is also a local httptest server.
func authorizationRequest(asURL, redirectURI string, details rar.AuthorizationDetails, codeCh <-chan string) string {
	formValue, err := rar.EncodeForm(details)
	if err != nil {
		panic(err)
	}
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("scope", "demo")
	q.Set("state", "demo-state")
	q.Set("authorization_details", formValue)
	reqURL := asURL + "/oauth2/auth?" + q.Encode()
	fmt.Printf("  wire URL: %s\n", reqURL)

	// Default Client follows redirects automatically, which is exactly
	// the browser-mimicking behavior the demo wants here: the AS
	// issues 302 -> callbackSrv/callback?code=... and the callback
	// handler pushes the code onto codeCh.
	resp, err := http.Get(reqURL) //nolint:noctx // demo program, no surrounding context
	if err != nil {
		panic(err)
	}
	defer func() { _ = resp.Body.Close() }() //nolint:errcheck // demo program; Close errs are not actionable after a successful read
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	if resp.StatusCode != http.StatusOK {
		panic(fmt.Sprintf("authorize redirect chain ended with status %d: %s", resp.StatusCode, body))
	}
	fmt.Printf("  client callback body: %s\n", strings.TrimSpace(string(body)))

	select {
	case code := <-codeCh:
		return code
	case <-time.After(5 * time.Second):
		panic("timed out waiting for authorize code at callback")
	}
}

// tokenRequest performs the /oauth2/token leg. It builds the
// application/x-www-form-urlencoded body with grant_type +
// authorization_code, dispatches with client BasicAuth (since fosite
// requires client auth at the token endpoint for confidential
// clients), parses the JSON token response, surfaces both the wire
// bytes and the decoded Go shape of the authorization_details member,
// and returns the access token.
func tokenRequest(asURL, redirectURI, code string) string {
	body := url.Values{}
	body.Set("grant_type", "authorization_code")
	body.Set("code", code)
	body.Set("redirect_uri", redirectURI)
	encoded := body.Encode()
	fmt.Printf("  wire request body: %s\n", encoded)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, asURL+"/oauth2/token", strings.NewReader(encoded))
	if err != nil {
		panic(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(clientID, clientSecret)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		panic(err)
	}
	defer func() { _ = resp.Body.Close() }() //nolint:errcheck // demo program; Close errs are not actionable after a successful read
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	fmt.Printf("  wire response body: %s\n", raw)
	if resp.StatusCode != http.StatusOK {
		panic(fmt.Sprintf("/oauth2/token returned status %d: %s", resp.StatusCode, raw))
	}

	// json.RawMessage on authorization_details means the library
	// receives the exact bytes the AS wrote, not a re-marshal of a
	// map[string]any (which would reorder keys). Same rationale as in
	// example/three-flow.
	//
	// Only the two fields the demo actually consumes are kept on the
	// struct — the rest of the token response (token_type, scope,
	// expires_in, …) is silently discarded by the json decoder. The
	// wire response body is printed verbatim above so the reader can
	// still inspect the full shape.
	var tokenResp struct {
		AccessToken          string          `json:"access_token"`
		AuthorizationDetails json.RawMessage `json:"authorization_details"`
	}
	if err = json.Unmarshal(raw, &tokenResp); err != nil {
		panic(err)
	}

	parsed, err := rar.ParseArray(tokenResp.AuthorizationDetails)
	if err != nil {
		panic(err)
	}
	fmt.Printf("  parsed back: %d detail(s)\n", len(parsed))
	for _, d := range parsed {
		c, ok := d.(*rar.CommonType)
		if !ok {
			panic(fmt.Sprintf("expected *rar.CommonType, got %T", d))
		}
		printDetailShape(c)
	}
	return tokenResp.AccessToken
}

// introspectionRequest performs the /oauth2/introspect leg. Same shape
// as tokenRequest's response side: parse the JSON, hand the
// authorization_details RawMessage to rar.ParseArray, run ValidateAll
// to exercise the validation surface end-to-end.
func introspectionRequest(asURL, token string) {
	body := url.Values{}
	body.Set("token", token)
	encoded := body.Encode()
	fmt.Printf("  wire request body: %s\n", encoded)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, asURL+"/oauth2/introspect", strings.NewReader(encoded))
	if err != nil {
		panic(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(clientID, clientSecret)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		panic(err)
	}
	defer func() { _ = resp.Body.Close() }() //nolint:errcheck // demo program; Close errs are not actionable after a successful read
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	fmt.Printf("  wire response body: %s\n", raw)
	if resp.StatusCode != http.StatusOK {
		panic(fmt.Sprintf("/oauth2/introspect returned status %d: %s", resp.StatusCode, raw))
	}

	// Field order is sized-bytes-first per govet fieldalignment.
	var introResp struct {
		AuthorizationDetails json.RawMessage `json:"authorization_details"`
		Active               bool            `json:"active"`
	}
	if err = json.Unmarshal(raw, &introResp); err != nil {
		panic(err)
	}
	if !introResp.Active {
		panic("introspection returned active=false")
	}

	parsed, err := rar.ParseArray(introResp.AuthorizationDetails)
	if err != nil {
		panic(err)
	}
	if err = rar.ValidateAll(parsed); err != nil {
		panic(err)
	}
	fmt.Printf("  parsed back: %d detail(s) (validated)\n", len(parsed))
	for _, d := range parsed {
		c, ok := d.(*rar.CommonType)
		if !ok {
			panic(fmt.Sprintf("expected *rar.CommonType, got %T", d))
		}
		printDetailShape(c)
	}
	fmt.Println()
}

// newServers wires the fosite-backed AS plus a tiny "client callback"
// HTTP server that captures the auth code from the AS's redirect.
// Returning the channel alongside the servers lets main wait for the
// code without needing any shared state.
//
// The two servers must be started in this order: the callback server
// first (so its URL is known and can be set as the client's
// redirect_uri), then the AS (which is wired with a client whose
// redirect_uri points at the callback). The AS server is constructed
// last but registered into the client config first because the
// registration is path-only — the host:port goes into the callback.
func newServers() (asSrv, callbackSrv *httptest.Server, codeCh <-chan string) {
	ch := make(chan string, 1)

	// Start the callback server first so the redirect URI is known.
	callbackSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "missing code", http.StatusBadRequest)
			return
		}
		select {
		case ch <- code:
		default:
			// Already received one code on this run; multiple
			// callbacks in the same demo would be a bug, so panic
			// loudly rather than silently dropping.
			panic("callback received second code")
		}
		// The wire response body the client sees. main prints this so
		// the reader can confirm the redirect chain ended at the
		// callback rather than somewhere else.
		_, _ = fmt.Fprintf(w, "demo client received code=%s\n", code) //nolint:errcheck // demo program; write errors are not actionable
	}))
	registeredCallback := callbackSrv.URL + callbackPath

	provider := newProvider(registeredCallback)

	mux := http.NewServeMux()
	mux.Handle("GET /oauth2/auth", authorizeHTTPHandler(provider))
	mux.Handle("POST /oauth2/token", tokenHTTPHandler(provider))
	mux.Handle("POST /oauth2/introspect", introspectHTTPHandler(provider))
	asSrv = httptest.NewServer(mux)

	return asSrv, callbackSrv, ch
}

// newProvider builds the fosite OAuth2Provider with the handler set
// the example needs: authorize-code grant, token introspection, and
// the RFC 9396 detailsHandler stitched in as both an authorize-endpoint
// handler and a token-endpoint handler.
func newProvider(registeredCallback string) fosite.OAuth2Provider {
	cfg := &fosite.Config{
		AccessTokenLifespan:        time.Hour,
		AuthorizeCodeLifespan:      10 * time.Minute,
		GlobalSecret:               []byte("demo-secret-32-bytes-long-padxxx"),
		SendDebugMessagesToClients: true,
	}

	// fosite's default client-secret hasher is bcrypt, so a "secret"
	// in the in-memory client would normally be a bcrypt hash. The
	// plaintextHasher below replaces that with a literal byte
	// comparison so the demo can carry the secret as a plain string
	// without precomputing a hash per build. A production AS would
	// never substitute this for the default bcrypt hasher.
	cfg.ClientSecretsHasher = &plaintextHasher{}

	store := storage.NewMemoryStore()
	store.Clients[clientID] = &fosite.DefaultClient{
		ID:            clientID,
		Secret:        []byte(clientSecret),
		RedirectURIs:  []string{registeredCallback},
		ResponseTypes: []string{"code"},
		GrantTypes:    []string{"authorization_code"},
		Scopes:        []string{"demo"},
	}

	strategy := compose.NewOAuth2HMACStrategy(cfg)
	provider := compose.Compose(
		cfg, store, strategy,
		compose.OAuth2AuthorizeExplicitFactory,
		compose.OAuth2TokenIntrospectionFactory,
		// Register the rar bridge twice — once as an authorize
		// handler (so HandleAuthorizeEndpointRequest fires on
		// /oauth2/auth) and once as a token handler (so
		// PopulateTokenEndpointResponse fires on /oauth2/token).
		// compose.Compose runs the factory's return value through a
		// type-switch and registers it under every interface it
		// satisfies, so one factory call covers both.
		func(_ fosite.Configurator, _ any, _ any) any {
			return detailsHandler{}
		},
	)
	return provider
}

// authorizeHTTPHandler builds the /oauth2/auth handler. It:
//  1. Calls provider.NewAuthorizeRequest, which parses the form and
//     runs every registered AuthorizeEndpointHandler's validation
//     path (in particular ours, which decodes authorization_details).
//  2. Synthesizes an "authenticated user" by attaching a
//     *sessionWithDetails to the request and granting all requested
//     scopes (no consent UI).
//  3. Calls provider.NewAuthorizeResponse to invoke the response-side
//     hooks (the built-in AuthorizeExplicitGrantHandler issues the
//     code; ours has already stashed the details on the session).
//  4. Calls provider.WriteAuthorizeResponse to emit the redirect.
func authorizeHTTPHandler(provider fosite.OAuth2Provider) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Build the session early — detailsHandler reads it from the
		// requester during NewAuthorizeRequest's handler dispatch.
		// The pointer ends up persisted under the authorize code so
		// the token endpoint can read the details back later.
		sess := &sessionWithDetails{DefaultSession: &fosite.DefaultSession{Subject: "demo-user"}}

		ar, err := provider.NewAuthorizeRequest(ctx, r)
		if err != nil {
			provider.WriteAuthorizeError(ctx, w, ar, err)
			return
		}

		// In a real AS this is where the consent screen would render.
		// The demo grants every requested scope.
		for _, scope := range ar.GetRequestedScopes() {
			ar.GrantScope(scope)
		}

		// fosite's NewAuthorizeResponse takes the session as a
		// separate argument and attaches it to the request itself —
		// after this call ar.GetSession() returns sess.
		ar.SetSession(sess)
		resp, err := provider.NewAuthorizeResponse(ctx, ar, sess)
		if err != nil {
			provider.WriteAuthorizeError(ctx, w, ar, err)
			return
		}
		provider.WriteAuthorizeResponse(ctx, w, ar, resp)
	})
}

// tokenHTTPHandler builds the /oauth2/token handler. It mirrors
// fosite's documented hand-rolled token endpoint: NewAccessRequest
// (which validates the auth code, authenticates the client, and
// rehydrates the session), NewAccessResponse (which runs every
// registered PopulateTokenEndpointResponse), WriteAccessResponse
// (which json.Marshals the response body).
func tokenHTTPHandler(provider fosite.OAuth2Provider) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Provide an empty *sessionWithDetails — fosite's
		// AuthorizeExplicitGrantHandler will overwrite it with the
		// one persisted under the auth code via SetSession, but the
		// type must match for the type assertion in
		// detailsHandler.PopulateTokenEndpointResponse to succeed.
		sess := &sessionWithDetails{DefaultSession: new(fosite.DefaultSession)}

		ar, err := provider.NewAccessRequest(ctx, r, sess)
		if err != nil {
			provider.WriteAccessError(ctx, w, ar, err)
			return
		}

		resp, err := provider.NewAccessResponse(ctx, ar)
		if err != nil {
			provider.WriteAccessError(ctx, w, ar, err)
			return
		}
		provider.WriteAccessResponse(ctx, w, ar, resp)
	})
}

// introspectHTTPHandler builds the /oauth2/introspect handler. Same
// shape as the token endpoint but using
// NewIntrospectionRequest / WriteIntrospectionResponse. The
// introspection-response writer iterates
// sessionWithDetails.GetExtraClaims and surfaces
// authorization_details on the response body.
func introspectHTTPHandler(provider fosite.OAuth2Provider) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		sess := &sessionWithDetails{DefaultSession: new(fosite.DefaultSession)}
		ir, err := provider.NewIntrospectionRequest(ctx, r, sess)
		if err != nil {
			provider.WriteIntrospectionError(ctx, w, err)
			return
		}
		provider.WriteIntrospectionResponse(ctx, w, ir)
	})
}

// plaintextHasher is a fosite.Hasher that does literal byte
// comparison instead of bcrypt. It exists so the demo can authenticate
// the demo client with a plain-text "demo-secret" rather than carrying
// a precomputed bcrypt hash in source. A production AS would never
// substitute this for the default bcrypt hasher.
type plaintextHasher struct{}

func (plaintextHasher) Compare(_ context.Context, hash, data []byte) error {
	if bytes.Equal(hash, data) {
		return nil
	}
	return errors.New("plaintext hash mismatch")
}

func (plaintextHasher) Hash(_ context.Context, data []byte) ([]byte, error) {
	return data, nil
}
