// Copyright 2026 The go-rar Authors
// SPDX-License-Identifier: Apache-2.0

// Three-flow walks a single authorization_details value through the
// three OAuth 2.0 touchpoints RFC 9396 defines — the authorization
// request, the token response, and the access-token introspection
// response — using only the standard library and the rar package.
//
// The point of the example is to make the round-trip story tangible.
// Every step prints both the bytes that crossed the wire and the
// decoded Go-value shape, so a reader can compare what the wire looked
// like to what the parser returned, and see for themselves that the
// same detail flows unchanged through all three legs (which is the
// whole reason the rar library uses one type set end-to-end rather
// than a per-flow trio).
//
// The "authorization server" is three net/http/httptest handlers
// stitched together in-process. There is no PKCE, no consent UI, no
// signing, no PAR, no real session store, no token introspection
// authentication, and no client credentials check; the AS exists
// purely to give the client somewhere to send each leg's request and
// to give the example somewhere to call rar.DecodeForm /
// rar.ParseArray on the receive side. The companion README spells out
// what is and is not in scope.
//
// On any error the program calls panic — the example is a demo, not a
// production binary. Wiring slog or structured error handling would
// add noise without adding pedagogical value at this scope.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	"github.com/hstern/go-rar"
)

func main() {
	// Step 1: build the detail in memory. This is what the client
	// already knows it wants before any wire traffic happens.
	detail := buildDetail()
	fmt.Println("== step 1: build the authorization_details value ==")
	printDetailShape(detail)
	fmt.Println()

	// Spin up the fake AS. The three handlers share no state beyond
	// the synthetic code/token strings; each leg re-receives the
	// detail on the wire, demonstrating that the library round-trips
	// it without help from server-side memory.
	srv := httptest.NewServer(newAS())
	defer srv.Close()

	// Step 2: authorization request leg. EncodeForm produces the
	// JSON-encoded array; url.Values.Encode percent-encodes it into a
	// query string the way an OAuth client would for /authorize.
	fmt.Println("== step 2: authorization request (GET /authorize) ==")
	code := authorizationRequest(srv.URL, rar.AuthorizationDetails{detail})
	fmt.Printf("client received code: %q\n\n", code)

	// Step 3: token response leg. The client POSTs an
	// application/x-www-form-urlencoded body carrying the code and
	// the same authorization_details value (per RFC 9396 §7, a client
	// MAY re-send the details to bind them to the token request).
	// The AS responds with a JSON token-response body that itself
	// carries authorization_details as a JSON member.
	fmt.Println("== step 3: token response (POST /token) ==")
	token := tokenRequest(srv.URL, code, rar.AuthorizationDetails{detail})
	fmt.Printf("client received access_token: %q\n\n", token)

	// Step 4: introspection leg. The client POSTs the access token to
	// /introspect; the AS responds with the introspection JSON body,
	// which again carries the same authorization_details. ValidateAll
	// runs on the parsed slice so the example exercises the validate
	// surface end-to-end as well as the codec surface.
	fmt.Println("== step 4: introspection (POST /introspect) ==")
	introspectionRequest(srv.URL, token)

	fmt.Println("== done — same detail crossed all three touchpoints ==")
}

// buildDetail constructs the synthetic detail used in every leg. A
// *CommonType (the rar library's only built-in §2-only carrier) is
// enough to demonstrate the round-trip because the spec's wire shape
// for a baseline element is exactly the §2 fields plus the type
// discriminator; type-specific extensions would change the values
// being serialized but not the flow of the example.
func buildDetail() *rar.CommonType {
	d := &rar.CommonType{TypeName: "common"}
	d.Locations = []string{"https://api.example.com/v1/data"}
	d.Actions = []string{"read", "write"}
	d.Datatypes = []string{"contacts", "photos"}
	return d
}

// printDetailShape prints the Go-value view of one detail. Pairing
// this with the wire-side prints elsewhere in main is the load-bearing
// pedagogy of the example: a reader sees both halves of the
// "bytes on the wire" / "values out of the parser" picture.
func printDetailShape(d *rar.CommonType) {
	fmt.Printf("  go value: %T{TypeName:%q, Locations:%v, Actions:%v, Datatypes:%v}\n",
		d, d.TypeName, d.Locations, d.Actions, d.Datatypes)
}

// authorizationRequest performs the /authorize leg. It encodes the
// details, assembles the URL, prints the exact URL string that will
// hit the wire, calls the AS, and returns the synthetic code the AS
// hands back.
func authorizationRequest(base string, details rar.AuthorizationDetails) string {
	formValue, err := rar.EncodeForm(details)
	if err != nil {
		panic(err)
	}
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", "demo-client")
	q.Set("authorization_details", formValue)
	reqURL := base + "/authorize?" + q.Encode()
	fmt.Printf("  wire URL: %s\n", reqURL)

	resp, err := http.Get(reqURL) //nolint:noctx // demo program, no surrounding context
	if err != nil {
		panic(err)
	}
	defer func() { _ = resp.Body.Close() }() //nolint:errcheck // demo program; Close errs are not actionable after a successful read
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	fmt.Printf("  wire response body: %s\n", body)

	// The /authorize handler returns the code as the response body
	// (no redirect — a real AS would redirect to the client's
	// redirect_uri, but that flow shape is out of scope here).
	return strings.TrimSpace(string(body))
}

// tokenRequest performs the /token leg. It builds the
// application/x-www-form-urlencoded body, prints the exact bytes that
// will hit the wire, calls the AS, parses the JSON response, extracts
// the authorization_details JSON member, hands it to rar.ParseArray,
// and returns the access token.
func tokenRequest(base, code string, details rar.AuthorizationDetails) string {
	formValue, err := rar.EncodeForm(details)
	if err != nil {
		panic(err)
	}
	body := url.Values{}
	body.Set("grant_type", "authorization_code")
	body.Set("code", code)
	body.Set("authorization_details", formValue)
	encoded := body.Encode()
	fmt.Printf("  wire request body: %s\n", encoded)

	resp, err := http.Post(base+"/token", "application/x-www-form-urlencoded", strings.NewReader(encoded)) //nolint:noctx // demo program
	if err != nil {
		panic(err)
	}
	defer func() { _ = resp.Body.Close() }() //nolint:errcheck // demo program; Close errs are not actionable after a successful read
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	fmt.Printf("  wire response body: %s\n", raw)

	// json.RawMessage on authorization_details means the library
	// receives the exact bytes the AS wrote, not a re-marshal of a
	// map[string]any (which would reorder keys). This is the whole
	// reason rar uses json.RawMessage for extension carriers — see
	// the library's design notes.
	var tokenResp struct {
		AccessToken          string          `json:"access_token"`
		TokenType            string          `json:"token_type"`
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

// introspectionRequest performs the /introspect leg. Same shape as
// tokenRequest's response side, plus a ValidateAll call to exercise
// the validation surface on the parsed slice.
func introspectionRequest(base, token string) {
	body := url.Values{}
	body.Set("token", token)
	encoded := body.Encode()
	fmt.Printf("  wire request body: %s\n", encoded)

	resp, err := http.Post(base+"/introspect", "application/x-www-form-urlencoded", strings.NewReader(encoded)) //nolint:noctx // demo program
	if err != nil {
		panic(err)
	}
	defer func() { _ = resp.Body.Close() }() //nolint:errcheck // demo program; Close errs are not actionable after a successful read
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	fmt.Printf("  wire response body: %s\n", raw)

	// Field order is sized-bytes-first (RawMessage's slice header is
	// larger than the bool) to satisfy govet's fieldalignment check.
	// On the wire the order is the JSON-tag order, not the struct
	// declaration order, so packing has no visibility outside Go.
	var introResp struct {
		AuthorizationDetails json.RawMessage `json:"authorization_details"`
		Active               bool            `json:"active"`
	}
	if err = json.Unmarshal(raw, &introResp); err != nil {
		panic(err)
	}

	parsed, err := rar.ParseArray(introResp.AuthorizationDetails)
	if err != nil {
		panic(err)
	}
	if err := rar.ValidateAll(parsed); err != nil {
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

// newAS returns the in-process fake authorization server. Each handler
// echoes the authorization_details value it received back into the
// response, so the example demonstrates that the same value flows
// across all three legs without server-side persistence.
func newAS() http.Handler {
	mux := http.NewServeMux()

	// /authorize prints what it decoded so the reader sees the
	// server-side view of the form parameter, and returns a synthetic
	// code as the response body (a real AS would redirect, but that
	// would complicate the example without adding pedagogical value).
	mux.HandleFunc("GET /authorize", func(w http.ResponseWriter, r *http.Request) {
		formValue := r.URL.Query().Get("authorization_details")
		details, err := rar.DecodeForm(formValue)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := rar.ValidateAll(details); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		fmt.Printf("  [AS] /authorize received %d detail(s); first type=%q\n",
			len(details), details[0].Type())
		w.Header().Set("Content-Type", "text/plain")
		if _, err := fmt.Fprint(w, "demo-auth-code"); err != nil {
			panic(err)
		}
	})

	// /token re-echoes the authorization_details the client sent. A
	// real AS would re-issue from its own consent-grant record; the
	// re-echo keeps the example self-contained without weakening the
	// round-trip story (the bytes still cross the wire twice).
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		formValue := r.PostForm.Get("authorization_details")
		details, err := rar.DecodeForm(formValue)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		fmt.Printf("  [AS] /token received code=%q + %d detail(s)\n",
			r.PostForm.Get("code"), len(details))

		// Marshal the details back into JSON for the response body.
		// json.Marshal on the slice routes through each element's
		// MarshalJSON (CommonType.MarshalJSON for the demo detail),
		// which is the same path rar.EncodeForm uses for the
		// authorization-endpoint leg — the response is byte-stable
		// against the request the client sent.
		detailsJSON, err := json.Marshal(details)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		resp := struct {
			AccessToken          string          `json:"access_token"`
			TokenType            string          `json:"token_type"`
			AuthorizationDetails json.RawMessage `json:"authorization_details"`
		}{
			AccessToken:          "demo-access-token",
			TokenType:            "Bearer",
			AuthorizationDetails: detailsJSON,
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			panic(err)
		}
	})

	// /introspect synthesizes an introspection response by rebuilding
	// the same detail the demo started with. A real AS would look up
	// the token in its store; the synthesis keeps the example
	// self-contained.
	mux.HandleFunc("POST /introspect", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		fmt.Printf("  [AS] /introspect received token=%q\n", r.PostForm.Get("token"))

		details := rar.AuthorizationDetails{buildDetail()}
		detailsJSON, err := json.Marshal(details)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Field order is sized-bytes-first per govet's fieldalignment;
		// the wire shape is the JSON-tag order, independent of packing.
		resp := struct {
			AuthorizationDetails json.RawMessage `json:"authorization_details"`
			Active               bool            `json:"active"`
		}{
			AuthorizationDetails: detailsJSON,
			Active:               true,
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			panic(err)
		}
	})

	return mux
}
