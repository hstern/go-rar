# go-rar

A Go library implementing
[RFC 9396 — OAuth 2.0 Rich Authorization Requests][rfc9396].

[rfc9396]: https://www.rfc-editor.org/rfc/rfc9396.html

> **Status: pre-release.** The public API surface is feature-complete
> against RFC 9396 §2–§9; the `v0.1.0` tag is next. The module path
> `github.com/hstern/go-rar` is stable from the first commit.

## What it is

`go-rar` is the Go ecosystem's reference implementation of the RFC 9396
`authorization_details` parameter — the discriminated-union JSON value
OAuth 2.0 uses to express fine-grained authorization.

The library handles:

- **The §2 `common` baseline** — `locations`, `actions`, `datatypes`,
  `identifier`, `privileges`. RFC 9396 deliberately defines no built-in
  `type` values, so `common` is the only carrier shipped out of the
  box; every IANA-published or consumer-defined type comes in via
  `RegisterType`.
- **JSON codec** — discriminator-driven `Unmarshal` dispatch plus
  spec-order, byte-stable `Marshal` output (`type` always first).
- **Authorization-endpoint form encoding** — `EncodeForm` /
  `DecodeForm` for the `application/x-www-form-urlencoded` case where
  the array is carried as a single URL-encoded JSON string.
- **Validation** — opt-in `Validate()` per type, returning a
  `*ValidationError` with `Rule`, `Type`, and `Reason` fields.
  `ValidateAll` joins per-element errors via `errors.Join`.
- **Forward compatibility** — unknown `type` values parse into an
  `UnknownType` carrier that preserves the wire bytes verbatim, so the
  authorization detail round-trips even when the library can't fully
  interpret it.
- **Extension** — `RegisterType(name, ctor)` lets downstream code add
  a constructor for any custom or IANA-published type. Built-in names
  are protected by `ErrTypeReserved`.

## Install

```sh
go get github.com/hstern/go-rar@latest
```

Requires Go 1.26 or newer.

## Quickstart

```go
package main

import (
    "fmt"
    "log"

    "github.com/hstern/go-rar"
)

func main() {
    const payload = `[{"type":"common","actions":["read","write"],"locations":["https://api.example.com/v1/data"]}]`

    details, err := rar.ParseArray([]byte(payload))
    if err != nil {
        log.Fatal(err)
    }
    if err := rar.ValidateAll(details); err != nil {
        log.Fatal(err)
    }
    fmt.Printf("parsed %d authorization detail(s); first type = %q\n", len(details), details[0].Type())
}
```

Parse, then validate, then use. `ParseArray` is lenient — members
outside the §2 baseline are silently dropped per the spec's
MUST-ignore-unknown convention, and a `type` value the library has no
constructor for lands in `*UnknownType` rather than erroring.
Validation is opt-in via `ValidateAll`; `MarshalJSON` stays lenient by
default and can be flipped to fail-fast via `SetStrictMarshal(true)`.

## Per-flow examples

RFC 9396 carries the same `authorization_details` JSON shape through
three OAuth touchpoints. The library has one set of types for all
three; only the envelope changes.

### Authorization request — `EncodeForm`

```go
c := &rar.CommonType{TypeName: "common"}
c.Actions = []string{"read"}
c.Locations = []string{"https://api.example.com/v1/data"}
details := rar.AuthorizationDetails{c}

value, err := rar.EncodeForm(details)
if err != nil {
    log.Fatal(err)
}

v := url.Values{}
v.Set("authorization_details", value)
v.Set("response_type", "code")
v.Set("client_id", "client123")
// Wire into the redirect URL: https://as.example.com/authorize?<v.Encode()>
```

**Composing a `CommonType`.** The embedded §2 baseline is reached
through the promoted field selectors (`c.Actions`, `c.Locations`, …) or
through `c.Common()`. The embedded field itself is unexported, so a
composite literal naming the embedded field does not compile from
outside the package; assign the §2 fields after construction as above.

`EncodeForm` returns the JSON-encoded array; the form encoder
(`url.Values.Encode()`, `http.Request.PostForm`, …) owns the
percent-encoding step.

### Token response — `ParseArray` from a JSON body

```go
var resp struct {
    AccessToken          string          `json:"access_token"`
    TokenType            string          `json:"token_type"`
    ExpiresIn            int             `json:"expires_in"`
    AuthorizationDetails json.RawMessage `json:"authorization_details"`
}
if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
    log.Fatal(err)
}
details, err := rar.ParseArray(resp.AuthorizationDetails)
if err != nil {
    log.Fatal(err)
}
_ = details
```

Decode the envelope yourself; hand the `json.RawMessage` for
`authorization_details` to `ParseArray`. Using `json.RawMessage` rather
than `[]any` or `map[string]any` preserves the exact wire bytes for any
type the library does not natively recognize (those land in
`*UnknownType.Raw` for downstream handling).

### Introspection response — same shape, different envelope

```go
var resp struct {
    Active               bool            `json:"active"`
    Scope                string          `json:"scope,omitempty"`
    ClientID             string          `json:"client_id,omitempty"`
    AuthorizationDetails json.RawMessage `json:"authorization_details,omitempty"`
}
if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
    log.Fatal(err)
}
details, err := rar.ParseArray(resp.AuthorizationDetails)
if err != nil {
    log.Fatal(err)
}
_ = details
```

Per RFC 9396 §9 the introspection response carries
`authorization_details` with the same JSON shape as the token response.
Three flows, one library type.

## How this fits with OAuth 2.0

RFC 9396 defines the value of a single OAuth parameter
(`authorization_details`); it does not specify any new endpoints, grant
types, or HTTP flows. The same JSON shape this library encodes is
carried unchanged through three OAuth touchpoints:

- the authorization request (URL form-encoded at the authorization
  endpoint, or as a JSON member of a Pushed Authorization Request),
- the token response (as a JSON member alongside `access_token`), and
- the token introspection response (as a JSON member describing the
  authorization the access token represents).

`go-rar` is the codec for that shape; binding it into an authorization
server, resource server, or client library is the OAuth library's job.
The library plugs in alongside any stdlib `net/http` server or any
existing OAuth 2.0 client/server stack.

For a worked integration against a published OAuth 2.0 framework
showing the three-flow round-trip end-to-end through a real
authorization-code grant, see
[`example/fosite-handler/`](example/fosite-handler/).

## Extensibility

`RegisterType(name, ctor)` lets downstream code register custom
constructors for the `type` discriminator. Built-in: only `common`
(the §2-only carrier). Any other `type` value falls through to
`*UnknownType`, which preserves the wire bytes verbatim so the detail
round-trips even when the library cannot natively interpret it.

Consumer-defined typed extensions land via the embeddable `Extension`
base. A consumer struct that embeds `rar.Extension` satisfies the
sealed `AuthorizationDetail` interface from outside the package (the
sealed marker is inherited via embedding without being exported),
inherits the §2 baseline validation rules, and adds its own
type-specific fields plus a `RegisterType` call:

```go
type PaymentInitiation struct {
    rar.Extension
    CreditorName string `json:"creditorName,omitempty"`
}

// Override Validate to add type-specific rules; the embedded
// Extension.Validate covers the §2 baseline.
func (p *PaymentInitiation) Validate() error { /* ... */ }

// Override MarshalJSON / UnmarshalJSON to handle the type-specific
// fields in spec order (type first, then §2 baseline, then your
// members in declared order). The library's CommonType is the
// template.
func (p *PaymentInitiation) MarshalJSON() ([]byte, error) { /* ... */ }
func (p *PaymentInitiation) UnmarshalJSON(b []byte) error { /* ... */ }

rar.RegisterType("payment_initiation", func() rar.AuthorizationDetail {
    return &PaymentInitiation{Extension: rar.Extension{TypeName: "payment_initiation"}}
})
```

See [`extension_external_test.go`](extension_external_test.go) for the
complete worked example exercised from an out-of-package test, and
[`extension_test.go`](extension_test.go) for the equivalent in-package
pattern that mirrors the spec's §4 `payment_initiation` figure.

## Design

The library's design rationale will be summarized in `DESIGN.md` ahead
of the `v0.1.0` tag. The headline decisions:

- Sealed `AuthorizationDetail` interface with per-type concrete
  Go types — no `map[string]any`, no zero-valued union-fields struct.
- `encoding/json` stdlib with custom `UnmarshalJSON` dispatch on the
  `type` discriminator.
- **Lenient on unmarshal, strict on marshal.** Postel's law: decode
  whatever the wire gave us, validate at the marshal boundary only
  when `SetStrictMarshal(true)` is set.
- Byte-stable output: `type` first, then the §2 common members in
  spec order, then any type-specific members in declared order.
- Open-extension fields are `json.RawMessage`, not `map[string]any` —
  interop scenarios pin exact JSON bytes, and `map` reorders keys.

## Stability

- **Pre-`v0.1.0`**: API surface may change without notice.
- **`v0.1.x`**: additive changes only within the `v0.x` line.
- **`v1.0.0`** ships when (a) at least one external OAuth consumer has
  integrated and stayed integrated for a release cycle, and (b) at
  least one IANA-published `type` value has been exercised end-to-end
  (typically via consumer `RegisterType` calls).
- **`v2.0+`**: future-major handling follows the `go-jose` branch
  pattern (versioned `go.mod` module path, prior majors preserved on
  dedicated branches).

## Compatibility

- **Spec version**: `const SpecVersion = "RFC 9396"`. RFCs have no
  minor or patch numbers; errata are absorbed into Go-minor releases
  without changing the constant.
- **Go version**: 1.26+.
- **Dependencies**: standard library only, at runtime. Test
  dependencies: none. Standard library only.
- **Library SemVer** is independent of the spec version. Major-version
  handling follows the `go-jose` branch pattern (no versioned
  subdirectories — `vN` lives in `go.mod` on a `vN` branch).

## Contributing

Contributor conventions are in [`AGENTS.md`](AGENTS.md): commit message
style, code review expectations, the per-file SPDX header, and the
local pre-PR checks the CI also runs.

Bugs and feature ideas are welcome via the project's issue tracker.

## License

Apache-2.0. See [`LICENSE`](LICENSE) for the full text.

Every source file carries an SPDX identifier:

```go
// Copyright 2026 The go-rar Authors
// SPDX-License-Identifier: Apache-2.0
```
