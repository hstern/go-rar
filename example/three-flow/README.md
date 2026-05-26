# `example/three-flow`

## What this is

A synthetic round-trip of a single RFC 9396 `authorization_details`
value through the three OAuth 2.0 touchpoints the spec covers — the
authorization request (`GET /authorize`), the token response
(`POST /token`), and the access-token introspection response
(`POST /introspect`) — built from `net/http/httptest` and the `rar`
package and nothing else. The example pairs each leg with prints of
both the bytes that crossed the wire and the decoded Go-value shape,
so a reader can see for themselves that the same detail flows
unchanged across all three legs.

It is **not** a production-grade authorization server. The in-process
handlers exist only to give the client somewhere to call; see
"What's omitted and why" below for the full list of shortcuts.

For the library's quickstart and per-flow API pointers see the
[top-level `README.md`](../../README.md).

## Run it

```sh
cd example/three-flow
go run .
```

Expected output (host port varies per run; everything else is
deterministic):

```
== step 1: build the authorization_details value ==
  go value: *rar.CommonType{TypeName:"common", Locations:[https://api.example.com/v1/data], Actions:[read write], Datatypes:[contacts photos]}

== step 2: authorization request (GET /authorize) ==
  wire URL: http://127.0.0.1:PORT/authorize?authorization_details=%5B%7B%22type%22%3A%22common%22%2C%22locations%22%3A%5B%22https%3A%2F%2Fapi.example.com%2Fv1%2Fdata%22%5D%2C%22actions%22%3A%5B%22read%22%2C%22write%22%5D%2C%22datatypes%22%3A%5B%22contacts%22%2C%22photos%22%5D%7D%5D&client_id=demo-client&response_type=code
  [AS] /authorize received 1 detail(s); first type="common"
  wire response body: demo-auth-code
client received code: "demo-auth-code"

== step 3: token response (POST /token) ==
  wire request body: authorization_details=%5B%7B%22type%22%3A%22common%22%2C%22locations%22%3A%5B%22https%3A%2F%2Fapi.example.com%2Fv1%2Fdata%22%5D%2C%22actions%22%3A%5B%22read%22%2C%22write%22%5D%2C%22datatypes%22%3A%5B%22contacts%22%2C%22photos%22%5D%7D%5D&code=demo-auth-code&grant_type=authorization_code
  [AS] /token received code="demo-auth-code" + 1 detail(s)
  wire response body: {"access_token":"demo-access-token","token_type":"Bearer","authorization_details":[{"type":"common","locations":["https://api.example.com/v1/data"],"actions":["read","write"],"datatypes":["contacts","photos"]}]}

  parsed back: 1 detail(s)
  go value: *rar.CommonType{TypeName:"common", Locations:[https://api.example.com/v1/data], Actions:[read write], Datatypes:[contacts photos]}
client received access_token: "demo-access-token"

== step 4: introspection (POST /introspect) ==
  wire request body: token=demo-access-token
  [AS] /introspect received token="demo-access-token"
  wire response body: {"authorization_details":[{"type":"common","locations":["https://api.example.com/v1/data"],"actions":["read","write"],"datatypes":["contacts","photos"]}],"active":true}

  parsed back: 1 detail(s) (validated)
  go value: *rar.CommonType{TypeName:"common", Locations:[https://api.example.com/v1/data], Actions:[read write], Datatypes:[contacts photos]}

== done — same detail crossed all three touchpoints ==
```

## Module layout

`example/three-flow` is its **own Go module** (`go.mod` in this
directory), separate from the library's root module. This keeps the
example's dependency graph out of the library's `go.mod` / `go.sum`
— the library ships zero non-test dependencies, and a co-located main
package would otherwise pull example-only dependencies into the
library's lockfile if any were ever added.

The committed `go.mod` carries

```
replace github.com/hstern/go-rar => ../..
```

so that `go run .` from this directory builds against the library
source in the enclosing repository, not against the published
`v0.1.0` tag. The replace is deliberately kept in the committed file
so a contributor cloning a fresh checkout can run the example against
their working copy without any extra step.

## What's omitted and why

The example is the smallest thing that demonstrates the three-flow
round-trip end-to-end. Anything not load-bearing for that
demonstration is omitted:

- **No PKCE.** RFC 7636 lives orthogonal to RFC 9396; threading
  PKCE through would add code-verifier / code-challenge plumbing to
  every leg without changing what the `rar` library does on the wire.
- **No consent UI.** The `/authorize` handler synthesizes a code on
  the spot rather than rendering a consent screen and waiting for a
  user decision. A real AS would; the wire shape on the
  authorization_details parameter is unchanged either way.
- **No request / response signing.** No JAR (RFC 9101), no DPoP, no
  mTLS sender-constraining. RFC 9396 itself is silent on signing —
  it defines the parameter shape, not the transport security.
- **No PAR.** RFC 9126 Pushed Authorization Requests would replace
  the query-string `/authorize` call with a server-side push of the
  same form-encoded body. The `authorization_details` value crosses
  the wire identically; only the envelope changes.
- **No real client authentication, token store, or introspection
  authentication.** The `/token` and `/introspect` handlers accept
  any request and echo back the demo detail. A real AS would
  authenticate the client, look up the code/token in a store, and
  return only the details bound to the grant.
- **No third-party authorization-server framework.** The handlers
  are stdlib `net/http` mux entries. A realistic integration against
  a published AS framework is the scope of a follow-up example.

The point of this example is the `rar` library's round-trip
guarantees; the point of a realistic AS-framework example is
ergonomic fit. Two examples, two scopes.

