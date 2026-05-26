# `example/fosite-handler`

## What this is

A worked integration of `go-rar` against
[`github.com/ory/fosite`](https://github.com/ory/fosite) — a published
OAuth 2.0 framework — showing how an
[RFC 9396](https://www.rfc-editor.org/rfc/rfc9396.html)
`authorization_details` slice threads through a real authorization-code
flow end to end: the authorization request (`/oauth2/auth`), the token
response (`/oauth2/token`), and the access-token introspection response
(`/oauth2/introspect`).

The integration is three files:

- **[`session.go`](session.go)** — a `sessionWithDetails` type that
  embeds fosite's `DefaultSession` and adds an `AuthorizationDetails`
  field. It also implements `fosite.ExtraClaimsSession.GetExtraClaims`
  so fosite's introspection-response writer surfaces the details
  alongside the standard RFC 7662 fields.
- **[`handler.go`](handler.go)** — a `detailsHandler` type that
  implements both `fosite.AuthorizeEndpointHandler` (decoding the form
  parameter on `/oauth2/auth`) and `fosite.TokenEndpointHandler`
  (re-emitting the slice on `/oauth2/token`). It plugs in alongside
  fosite's built-in handlers via `compose.Compose`.
- **[`main.go`](main.go)** — wires the AS via `compose.Compose`,
  registers one synthetic confidential client, spins up the AS and a
  callback receiver in-process, and drives the three-leg round-trip
  programmatically. Same pedagogic discipline as
  [`example/three-flow`](../three-flow/README.md): every leg prints
  both the bytes that crossed the wire and the decoded Go-value shape.

This is **not** a production-grade authorization server. The shortcuts
the example takes are listed under
[What's omitted and why](#whats-omitted-and-why) below.

For the library's quickstart and per-flow API pointers see the
[top-level `README.md`](../../README.md). For the stdlib-only variant
of the same round-trip (no fosite, just `net/http/httptest`) see
[`example/three-flow`](../three-flow/README.md).

## Run it

```sh
cd example/fosite-handler
go run .
```

Expected output (host port, auth code, and access token vary per run;
everything else is deterministic):

```
== step 1: build the authorization_details value ==
  go value: *rar.CommonType{TypeName:"common", Locations:[https://api.example.com/v1/data], Actions:[read write], Datatypes:[contacts photos]}

== step 2: authorization request (GET /oauth2/auth) ==
  wire URL: http://127.0.0.1:PORT/oauth2/auth?authorization_details=%5B%7B%22type%22%3A%22common%22%2C%22locations%22%3A%5B%22https%3A%2F%2Fapi.example.com%2Fv1%2Fdata%22%5D%2C%22actions%22%3A%5B%22read%22%2C%22write%22%5D%2C%22datatypes%22%3A%5B%22contacts%22%2C%22photos%22%5D%7D%5D&client_id=demo-client&redirect_uri=http%3A%2F%2F127.0.0.1%3APORT%2Fcallback&response_type=code&scope=demo&state=demo-state
  [AS] /authorize stashed 1 authorization_detail(s) onto session; first type="common"
  client callback body: demo client received code=ory_ac_<...>
client received code: "ory_ac_<...>"

== step 3: token response (POST /oauth2/token) ==
  wire request body: code=ory_ac_<...>&grant_type=authorization_code&redirect_uri=http%3A%2F%2F127.0.0.1%3APORT%2Fcallback
  [AS] /token populated authorization_details with 1 detail(s)
  wire response body: {"access_token":"ory_at_<...>","authorization_details":[{"type":"common","locations":["https://api.example.com/v1/data"],"actions":["read","write"],"datatypes":["contacts","photos"]}],"expires_in":3600,"scope":"demo","token_type":"bearer"}
  parsed back: 1 detail(s)
  go value: *rar.CommonType{TypeName:"common", Locations:[https://api.example.com/v1/data], Actions:[read write], Datatypes:[contacts photos]}
client received access_token: "ory_at_<...>"

== step 4: introspection (POST /oauth2/introspect) ==
  wire request body: token=ory_at_<...>
  wire response body: {"active":true,"authorization_details":[{"type":"common","locations":["https://api.example.com/v1/data"],"actions":["read","write"],"datatypes":["contacts","photos"]}],"client_id":"demo-client","exp":<unix>,"iat":<unix>,"scope":"demo","sub":"demo-user"}

  parsed back: 1 detail(s) (validated)
  go value: *rar.CommonType{TypeName:"common", Locations:[https://api.example.com/v1/data], Actions:[read write], Datatypes:[contacts photos]}

== done — same detail crossed all three touchpoints ==
```

## Module layout

`example/fosite-handler` is its **own Go module** (`go.mod` in this
directory), separate from the library's root module and from
`example/three-flow`'s module. This keeps fosite (and its transitive
graph — OpenTelemetry, koanf, viper, gRPC, …) out of the library's
`go.mod` / `go.sum`: the library itself still ships zero non-test
runtime dependencies. The example is the only place fosite is allowed
to enter the dependency graph.

The committed `go.mod` carries

```
replace github.com/hstern/go-rar => ../..
```

so that `go run .` from this directory builds against the library
source in the enclosing repository, not against a published tag. The
replace is deliberately kept in the committed file so a contributor
cloning a fresh checkout can run the example against their working
copy without any extra step — same convention as
[`example/three-flow`](../three-flow/README.md#module-layout).

## How the integration works

The three legs share state through fosite's session pointer:

1. **`/oauth2/auth`** — `authorizeHTTPHandler` (in `main.go`) builds
   an empty `*sessionWithDetails` and calls
   `provider.NewAuthorizeRequest`. fosite's request-handler dispatch
   loop calls `detailsHandler.HandleAuthorizeEndpointRequest`, which
   reads the `authorization_details` form value, decodes it via
   `rar.DecodeForm`, validates via `rar.ValidateAll`, and stashes the
   slice onto the session. fosite then persists the request
   (including the same session pointer) under the auth code via
   `CoreStorage.CreateAuthorizeCodeSession`. The handler issues a 302
   to the registered redirect_uri with the auth code in the query
   string.

2. **`/oauth2/token`** — `tokenHTTPHandler` builds another empty
   `*sessionWithDetails` and calls `provider.NewAccessRequest`.
   fosite's built-in `AuthorizeExplicitGrantHandler` validates the
   auth code, retrieves the persisted request, and re-attaches its
   session (the one carrying our details) to the access request via
   `SetSession`. `provider.NewAccessResponse` then runs every
   registered `PopulateTokenEndpointResponse`; `detailsHandler`'s
   re-marshals the slice and sets it as the `authorization_details`
   extra on the access response, which fosite's response writer
   emits as a top-level JSON member.

3. **`/oauth2/introspect`** — `introspectHTTPHandler` builds yet
   another empty session and calls `provider.NewIntrospectionRequest`.
   fosite's `CoreValidator` (the introspector) hydrates the request's
   session from storage; the session is again our `*sessionWithDetails`
   carrying the slice.
   `provider.WriteIntrospectionResponse` then iterates
   `GetExtraClaims()` on the session — which `sessionWithDetails`
   implements to return `{"authorization_details": <marshaled
   slice>}` — and surfaces those entries alongside the standard
   RFC 7662 fields.

The integration touches zero fosite source: it composes only against
fosite's public interfaces (`AuthorizeEndpointHandler`,
`TokenEndpointHandler`, `Session`, `ExtraClaimsSession`).

## What's omitted and why

The example is the smallest thing that demonstrates the round-trip
through a real OAuth 2.0 library end-to-end. Anything not load-bearing
for that demonstration is omitted:

- **No PKCE.** RFC 7636 lives orthogonal to RFC 9396; threading PKCE
  through would add code-verifier / code-challenge plumbing to every
  leg without changing what `go-rar` does on the wire. A real
  integration against a public client would add PKCE — fosite ships a
  PKCE factory (`compose.OAuth2PKCEFactory`) that drops in
  alongside the existing factory list.
- **No consent UI.** `authorizeHTTPHandler` auto-approves every
  requested scope. A real AS would render a consent screen and wait
  for a user decision; the `authorization_details` parameter would
  flow through the consent step unchanged.
- **No real client / user database.** One hardcoded confidential
  client (`demo-client` / `demo-secret`); the synthesized session
  carries a fixed `sub=demo-user`. The user's existence is implicit in
  the session; there is no user lookup or password check.
- **Plain-text client secret comparator.** fosite's default
  client-secret hasher is bcrypt; the demo replaces it with a literal
  byte comparator (`plaintextHasher` in `main.go`) so the example can
  carry the secret as a plain string without precomputing a hash per
  build. A production AS would never substitute this for bcrypt.
- **No HTTPS.** The AS and the callback receiver are both
  `httptest.Server` instances, which fosite explicitly permits under
  `localhost` host suffixes.
- **No persistence.** `storage.NewMemoryStore` lives only for the
  lifetime of the process. A production AS would back the same
  storage interfaces with a real database.
- **No request / response signing.** No JAR (RFC 9101), no DPoP, no
  mTLS sender-constraining. RFC 9396 itself is silent on signing.
- **No PAR.** RFC 9126 Pushed Authorization Requests would replace
  the query-string `/oauth2/auth` call with a server-side push of the
  same form-encoded body. The `authorization_details` value crosses
  the wire identically; only the envelope changes.

## Sealed-interface limitation

The library's `AuthorizationDetail` interface is sealed to the `rar`
package: consumers cannot today supply their own concrete Go types
from outside the package. The example consequently uses only
`*rar.CommonType` end-to-end, even where a richer integration would
want a `type:"payment_initiation"` value to deserialize into a
consumer-defined `PaymentInitiation` struct with its own type-specific
fields.

For now, consumers needing typed access to extension fields parse the
`*UnknownType.Raw` bytes themselves. Lifting the sealing for
downstream-defined types is tracked as a post-`v0.1.0` follow-on; once
landed, the integration shown here gains a typed extension story
without changing the handler glue.
