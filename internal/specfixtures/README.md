# specfixtures

Embedded JSON example payloads from RFC 9396, used by the library's
conformance test suite. Internal package — not importable from
outside this module.

The on-disk `.json` files are the **compact canonical form** of each
example: the exact byte shape that `encoding/json.Compact` emits, and
the same byte shape that the library's `MarshalJSON` emits. This lets
conformance tests assert `bytes.Equal(fixture, marshaled)` directly,
with no normalization step on either side. A package-level sanity
test (`TestAllFixturesAreValidCompactJSON`) pins that invariant so a
contributor pretty-printing a file by hand will see the test fail.

The pretty-printed forms below are the same payloads as published in
the RFC — they're reproduced here only for reviewer convenience. The
authoritative bytes are the compact `.json` files.

## Fixtures

### `baseline.json` — RFC 9396 §2

A single `type`-only–plus–common-members detail, wrapped in a
one-element array. The wire shape is always an array, even with a
single detail.

```json
[
  {
    "type": "customer_information",
    "locations": ["https://example.com/customers"],
    "actions": ["read", "write"],
    "datatypes": ["contacts", "photos"]
  }
]
```

### `multiple.json` — RFC 9396 §4

A two-element array. The second element carries type-specific members
(`instructedAmount`, `creditorName`, `creditorAccount`,
`remittanceInformationUnstructured`) alongside the common members,
exercising the open-extension path.

```json
[
  {
    "type": "account_information",
    "actions": ["list_accounts", "read_balances", "read_transactions"],
    "locations": ["https://example.com/accounts"]
  },
  {
    "type": "payment_initiation",
    "actions": ["initiate", "status", "cancel"],
    "locations": ["https://example.com/payments"],
    "instructedAmount": {"currency": "EUR", "amount": "123.50"},
    "creditorName": "Merchant A",
    "creditorAccount": {"iban": "DE02100100109307118603"},
    "remittanceInformationUnstructured": "Ref Number Merchant"
  }
]
```

### `token_response.json` — RFC 9396 §7

The full OAuth token-response envelope. Stored whole (not just the
`authorization_details` array) so the conformance test can extract
the array, round-trip it, and confirm it slots back into the envelope
identically.

```json
{
  "access_token": "2YotnFZFEjr1zCsicMWpAA",
  "token_type": "Bearer",
  "expires_in": 3600,
  "refresh_token": "tGzv3JOkF0XG5Qx2TlKWIA",
  "authorization_details": [
    {
      "type": "payment_initiation",
      "actions": ["initiate"],
      "locations": ["https://example.com/payments"],
      "instructedAmount": {"currency": "EUR", "amount": "123.50"},
      "creditorName": "Merchant A"
    }
  ]
}
```

### `empty_array.json` — library-internal (not from RFC 9396)

A single `common`-type detail carrying an explicit empty `actions`
array. **This fixture is not drawn from RFC 9396** — the spec's
published examples never use an explicit empty array (members are
either omitted or non-empty). The fixture exists to exercise the
present-but-empty round-trip invariant introduced alongside the
hand-written `*CommonType.MarshalJSON` path: a payload with
`"actions": []` must survive `Parse` → `Marshal` byte-stably, with
the empty array preserved rather than elided. Sits in the conformance
corpus so the same round-trip and validation machinery hits this case
uniformly with the spec-derived fixtures.

```json
[
  {
    "type": "common",
    "actions": []
  }
]
```

### `introspection.json` — RFC 9396 §9

The full OAuth introspection-response envelope. Same rationale as
`token_response.json` — the envelope is part of the fixture so the
conformance test exercises the realistic extract-and-reembed shape.

```json
{
  "active": true,
  "sub": "24400320",
  "aud": "s6BhdRkqt3",
  "iss": "https://server.example.com/",
  "exp": 1419356238,
  "authorization_details": [
    {
      "type": "account_information",
      "actions": ["read"],
      "locations": ["https://example.com/accounts"]
    }
  ]
}
```

## Adding a fixture

1. Write the `.json` file in its compact canonical form. The easiest
   recipe: paste the pretty-printed payload through
   `jq -c . | tr -d '\n'`, or run it through `json.Compact` and write
   the result. Verify with `go test ./internal/specfixtures/` — the
   sanity test will refuse anything pretty-printed.
2. Add a `//go:embed` line and a package-level `var` in
   `specfixtures.go`.
3. Add an entry to `All()`.
4. Document the new fixture in this README (compact bytes are
   authoritative; pretty form is reviewer convenience).
