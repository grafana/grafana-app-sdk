# Inline secure values

Run this local example from the repository root:

```sh
go run ./examples/resource/secure
```

It prints direct redacted JSON, JSONCodec output using a synthetic value,
and the OpenAPI schema. It does not contact a server.

Declare secure keys on a kind in your app's CUE manifest:

```cue
versions: v1: kinds: [{
    kind: "Connection"
    schema: spec: endpoint: string
    secure: [
        {key: "apiKey", description: "API key from the service"},
        {key: "token"},
    ]
}]
```

Generation adds an optional top-level `secure` map to the Go and TypeScript
resource types and their schemas. Declared keys carry their descriptions;
undeclared keys are rejected by the served OpenAPI schema. CRDs retain the
named properties and descriptions, with Kubernetes pruning undeclared keys.
Add `{key: "*", description: "Service credentials"}` to the declaration to accept
any key. Explicit keys can be combined with the wildcard in any order and retain
their individual descriptions in served OpenAPI. Wildcard CRDs use a typed map
and preserve arbitrary keys. Each value still has to match `InlineSecureValue`.

`secure` is not a REST subresource. Each value supplies exactly one of `create`, `name`, or `remove`,
with an optional `description` only when `create` is supplied:

```json
{"secure":{"apiKey":{"create":"example-only-value"},"token":{"name":"existing-secret-reference"}}}
```

Go's `resource.RawSecureValue` redacts direct JSON/YAML encoding and formatting.
The regular `resource.JSONCodec` writes plaintext `create` values for transport:

```go
codec := resource.NewJSONCodec()
err := codec.Write(writer, obj)
```

Encoding does not consume the source values, so they remain available for retries.
Generated codecs for kinds declaring secure values also preserve plaintext
`create` values. `PassthroughJSONCodec` and direct `encoding/json` calls retain
the redacting behavior.

The SDK supplies types, serialization, and schemas. Resolving references and
processing create/remove operations still requires Grafana's secure-value
handling on the server; an ordinary Kubernetes CRD does not perform that work.
