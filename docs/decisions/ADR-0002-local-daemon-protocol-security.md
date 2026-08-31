---
id: ADR-0002
status: accepted
date: 2026-08-31
supersedes: none
---

# ADR-0002: Local Daemon Protocol and Security Boundary

## Context

Phase 1 introduced the dependency-free `internal/evidence` module. It accepts an explicit Git selection and evidence limits, then returns a bounded preview of changed Go declarations. Phase 2 is intended to expose that module to a later Pi extension through the smallest local daemon seam. No daemon behavior exists yet.

This decision freezes the transport and security contract before implementation because its discovery files, authentication scheme, payloads, limits, and error semantics become compatibility-sensitive as soon as the Pi extension depends on them.

Verified platform constraints are:

- Pi extensions may use `fetch` and Node.js built-ins, but run with the user's permissions and are not an operating-system sandbox: <https://pi.dev/docs/latest/extensions> and <https://pi.dev/docs/latest/security>.
- Go's `os.UserConfigDir` resolves to `$HOME/Library/Application Support` on Darwin: <https://pkg.go.dev/os#UserConfigDir>.
- Go's standard library provides bounded HTTP request bodies, server timeouts, cryptographic randomness, constant-time comparison, and Darwin advisory file locks: <https://pkg.go.dev/net/http>, <https://pkg.go.dev/crypto/rand>, <https://pkg.go.dev/crypto/subtle>, and <https://go.dev/src/syscall/syscall_darwin.go>.
- Loopback clients should use an IP literal and an operating-system-assigned ephemeral port. `localhost` can resolve unpredictably and is not the selected binding contract: <https://www.rfc-editor.org/rfc/rfc8252.html#section-7.3>.

### Domain terms

- **Runtime Descriptor**: non-secret discovery metadata for one running daemon instance.
- **Instance Token**: a random, per-start secret proving that a local client could read the daemon's protected runtime state. It is not a Pi API key, model-provider credential, user identity, or durable session credential.
- **Evidence Preview Request**: one explicit repository and commit-range or working-tree selection submitted to the daemon.
- **Evidence Preview**: the bounded, inspectable Phase 1 result. It is not an evaluation, learning record, or authorization decision.

### Threat model

The Phase 2 boundary must protect against:

- accidental non-loopback network exposure;
- browser-originated and DNS-rebinding-style requests;
- local processes that cannot read the current user's protected runtime files;
- malformed, oversized, or stalled HTTP requests;
- accidental disclosure through error messages, URLs, or logs.

It does not protect against:

- root or a malicious process already running as the same operating-system user;
- a compromised Pi extension that the user has trusted and installed;
- operating-system-level source access by that same user;
- remote clients, multi-user service hosting, or a network attacker outside loopback;
- prompt injection in repository content, because Phase 2 invokes no model.

The Instance Token is therefore local client authentication, not a filesystem sandbox, repository allowlist, or user-account authorization system.

## Decision

This ADR was accepted and Phase 2 implementation was explicitly authorized on 2026-08-31.

### 1. Process and transport

- Phase 2 provides a foreground `pi-learnloop daemon` process. It does not daemonize itself and does not install `launchd`, login items, autostart, or a background service.
- The process creates one IPv4 TCP listener using `tcp4` and `127.0.0.1:0`. The operating system selects the port.
- It must not bind `localhost`, `0.0.0.0`, an IPv6 address, or any non-loopback address.
- The protocol is HTTP/1.1 with JSON payloads. Plain HTTP is acceptable only for this exact loopback boundary; any remote or non-loopback mode requires a new ADR and transport security review.
- Phase 2 exposes request/response endpoints only. It does not add SSE, WebSocket, polling jobs, durable queues, or background evaluation.
- Only one daemon instance may own the runtime state. A non-blocking exclusive advisory lock on `daemon.lock` enforces this on the initial macOS target without adding a dependency.

### 2. Runtime discovery and lifecycle

The state directory is:

```text
os.UserConfigDir()/pi-learnloop/runtime
```

On macOS this normally resolves to:

```text
~/Library/Application Support/pi-learnloop/runtime
```

The directory must have mode `0700`. `daemon.json`, `daemon.token`, and `daemon.lock` must have mode `0600`. The daemon fails closed for symlinks, unexpected ownership, or group/other access on security-sensitive runtime paths. It does not repair an ambiguous path silently.

The non-secret `daemon.json` Runtime Descriptor has this schema:

```json
{
  "schema_version": 1,
  "protocol_version": 1,
  "instance_id": "<16 random bytes, base64url without padding>",
  "pid": 12345,
  "base_url": "http://127.0.0.1:49152",
  "started_at": "2026-08-31T12:00:00Z"
}
```

`daemon.token` contains exactly one 43-character, unpadded base64url value derived from 32 bytes generated by `crypto/rand`, with no trailing newline. The token rotates on every daemon start and never appears in `daemon.json`.

Startup order is:

1. Resolve and validate the protected state directory.
2. Acquire `daemon.lock` exclusively without waiting.
3. Generate a new instance ID and Instance Token.
4. Bind `127.0.0.1:0`.
5. Atomically publish `daemon.token`, then `daemon.json`, using same-directory temporary files, restrictive modes, file sync, rename, and directory sync.
6. Begin serving requests.

The later Pi client discovers the daemon in this order:

1. Read and validate `daemon.json`, including the schema version, protocol version, exact `http://127.0.0.1:<port>` form, and numeric port.
2. Call unauthenticated `GET /v1/status` and verify the returned instance ID.
3. Read `daemon.token` only after descriptor and status validation.
4. Send the authenticated Evidence Preview Request.

If connection, instance-ID, or authentication state changes during discovery, the client may reread discovery state once and then fails with a recoverable error. It must not retry indefinitely or send the token to a URL that did not pass exact loopback validation. The client must bypass environment-configured HTTP proxies for this request.

On `SIGINT` or `SIGTERM`, the daemon stops accepting new work, allows at most five seconds for graceful shutdown, and removes its descriptor and token only when the descriptor still names its own instance ID. After a crash, the operating system releases the lock; the next valid instance overwrites stale token and descriptor files. The empty lock file may remain.

### 3. Authentication and browser-request defenses

`GET /v1/status` is intentionally unauthenticated and returns only:

```json
{
  "protocol_version": 1,
  "instance_id": "<current instance ID>",
  "status": "ready"
}
```

Every product endpoint requires this header:

```text
Authorization: PiLearnLoop <instance-token>
```

The custom scheme avoids implying OAuth bearer-token compliance. The token must never be accepted in a URL, query parameter, cookie, or JSON body. The server compares the expected and provided token bytes in constant time and returns the same generic response for missing and incorrect tokens:

```text
HTTP/1.1 401 Unauthorized
WWW-Authenticate: PiLearnLoop
Cache-Control: no-store
```

For every request, the server also:

- requires the peer address to parse as IPv4 loopback;
- requires `Host` to equal the exact advertised `127.0.0.1:<port>` authority;
- rejects any non-empty `Origin` header;
- emits no CORS allow headers and does not enable `OPTIONS` as a product method;
- adds `Cache-Control: no-store` to status, success, and error responses;
- never logs the authorization header, Instance Token, request body, source excerpts, or full response body.

These checks reduce browser and DNS-rebinding exposure, but do not change the same-user limitation in the threat model.

### 4. Versioned endpoints and request schema

Phase 2 exposes exactly two endpoints:

```text
GET  /v1/status
POST /v1/evidence-previews
```

All other paths return `404`; unsupported methods return `405` with an `Allow` header. `POST /v1/evidence-previews` accepts only `Content-Type: application/json` and the authentication contract above.

Commit-range request:

```json
{
  "repository": "/absolute/path/to/repository",
  "selection": {
    "kind": "commit_range",
    "base": "<Git revision>",
    "head": "<Git revision>"
  }
}
```

Working-tree request:

```json
{
  "repository": "/absolute/path/to/repository",
  "selection": {
    "kind": "working_tree",
    "base": "<Git revision>"
  }
}
```

Request decoding is strict:

- `repository` must be an absolute path and at most 4096 UTF-8 bytes;
- `base` and `head` are each at most 256 UTF-8 bytes;
- `head` is required for `commit_range` and forbidden for `working_tree`;
- unknown fields, duplicate object keys, and any trailing JSON value or non-whitespace content are rejected;
- the request body is limited to 16 KiB before decoding;
- the v1 request has no client-controlled evidence-limit field.

The daemon canonicalizes the path and delegates repository, revision, and containment validation to `internal/evidence`. It does not copy Git or Go-analysis rules into the HTTP adapter. Possession of the Instance Token does not restrict the client to one configured repository; the later Pi adapter is responsible for submitting its trusted `ctx.cwd`, and Phase 2 does not claim a repository sandbox.

### 5. Success schema and evidence limits

The server owns fixed v1 evidence caps:

```text
maximum files:             20
maximum declarations:    100
maximum aggregate excerpt bytes: 131072 (128 KiB)
```

The adapter always passes these explicit values to `internal/evidence`. Clients cannot raise or lower them through the public protocol. A successful response echoes the applied limits and contains the Phase 1 result:

```json
{
  "protocol_version": 1,
  "applied_limits": {
    "max_files": 20,
    "max_declarations": 100,
    "max_excerpt_bytes": 131072
  },
  "preview": {
    "repository_root": "/canonical/repository/root",
    "base_revision": "<resolved revision>",
    "head_revision": "<resolved revision or WORKTREE>",
    "files": [
      {
        "path": "internal/example/example.go",
        "status": "added",
        "changed_lines": [{"start": 1, "end": 8}],
        "declarations": [
          {
            "kind": "function",
            "name": "Example",
            "receiver": "",
            "identity": "Example",
            "start_line": 1,
            "end_line": 8,
            "changed_lines": [{"start": 1, "end": 8}],
            "excerpt": "package example\n...",
            "excerpt_truncated": false
          }
        ],
        "omissions": []
      }
    ],
    "truncation": {
      "truncated": false,
      "omitted_files": 0,
      "omitted_declarations": 0,
      "omitted_excerpt_bytes": 0
    }
  }
}
```

The v1 JSON mapping preserves the Phase 1 vocabulary:

- file status: `added | modified | deleted`;
- declaration kind: `function | method | type | interface | variable | constant`;
- omission reason: `deleted_file | deleted_only_hunk | outside_declaration`;
- paths are repository-relative with slash separators;
- line ranges are one-based and inclusive;
- `receiver` is an empty string for non-method declarations;
- collection fields encode as `[]`, not `null`;
- excerpts are valid UTF-8 and may be explicitly truncated.

### 6. Error contract

Errors use one envelope and safe, client-facing text:

```json
{
  "protocol_version": 1,
  "error": {
    "code": "invalid_request",
    "message": "request body is invalid"
  }
}
```

The stable v1 mapping is:

| Condition | HTTP | Code |
| --- | ---: | --- |
| Invalid JSON, schema, selection, or path shape | 400 | `invalid_request` |
| Missing or incorrect Instance Token | 401 | `unauthorized` |
| Origin, host, peer, or repository-containment rejection | 403 | `forbidden` |
| Unknown route | 404 | `not_found` |
| Unsupported method | 405 | `method_not_allowed` |
| Source changed or became unreadable during analysis | 409 | `source_unavailable` |
| Request body exceeds 16 KiB | 413 | `request_too_large` |
| Non-JSON content type | 415 | `unsupported_media_type` |
| Not a supported Git repository | 422 | `invalid_repository` |
| Revision cannot be resolved | 422 | `invalid_revision` |
| Changed Go source cannot be parsed | 422 | `invalid_source` |
| Evidence analysis exceeds its deadline | 504 | `deadline_exceeded` |
| Git or unexpected internal failure | 500 | `analysis_failed` or `internal_error` |

The adapter translates existing `internal/evidence` error codes; it does not change the module's public error vocabulary. Responses must not expose raw Git stderr, stack traces, tokens, unrelated local paths, or source excerpts in error messages.

### 7. Resource bounds and shutdown

The initial fixed server bounds are:

```text
maximum request body:   16 KiB
maximum request headers: 8 KiB
read-header timeout:     2 seconds
request-body read limit: 5 seconds
evidence deadline:      30 seconds
write timeout:          35 seconds
idle timeout:           30 seconds
graceful shutdown:       5 seconds
```

The request context is propagated through the adapter into `internal/evidence`. Phase 2 adds no compression, configurable public overrides, telemetry, request-body logging, or unbounded retry behavior.

### 8. Compatibility rules

- `daemon.json` has `schema_version: 1`; HTTP paths and bodies have `protocol_version: 1`.
- One daemon process serves one protocol major.
- Clients must reject unsupported descriptor or protocol versions before reading or sending the Instance Token.
- Within v1, responses may gain optional fields and clients must ignore unknown response fields.
- Requests remain strict; adding required fields, changing field meaning, changing fixed limit semantics, or adding enum values requires `/v2` and `protocol_version: 2`.
- The first implementation must not expose undocumented configuration flags that make the binding, authentication, discovery path, or security limits weaker.

## Alternatives

### Unix domain socket

A Unix socket would place discovery and access control primarily at the filesystem boundary. It was not selected because Pi's `fetch` API does not naturally address Unix sockets; a custom Node transport would make the first adapter more complex and less representative of the later client. It remains a credible future alternative if the HTTP seam proves unsafe or operationally awkward.

### Child process over stdin/stdout

Spawning the Go process per command would avoid a listening socket and discovery files. It was not selected because it conflicts with the project's later durable local-job and recovery direction and couples process ownership to the extension lifecycle. Phase 2 nevertheless remains foreground-only and does not introduce service installation.

### Fixed loopback port

A fixed port simplifies discovery but creates collisions, stale-address assumptions, and port-squatting risk. An operating-system-assigned port plus protected Runtime Descriptor was selected.

### Ephemeral loopback port without authentication

Port randomness alone is not authentication and does not adequately address browser or local-process requests. The split descriptor and Instance Token were selected.

### Store the token inside the Runtime Descriptor

A single file is simpler, but mixes shareable discovery metadata with the secret and increases accidental logging or diagnostic exposure. Separate files make the secret boundary explicit.

### HTTPS on loopback

HTTPS would add certificate issuance, storage, and local identity validation without protecting against a malicious process under the same user in the defined boundary. It was not selected for v1. It must be reconsidered before any non-loopback transport.

## Consequences

- The HTTP layer remains a thin adapter; all Git selection and Go evidence behavior stays inside `internal/evidence`.
- The later Pi extension must implement the ordered descriptor/status/token discovery flow and disable proxies for local calls.
- Security-sensitive defaults, fixed evidence caps, error codes, and JSON fields become compatibility commitments only after this ADR is accepted and implemented.
- Browser-origin and accidental-network exposure are narrowed without claiming protection from root, same-user malware, or a compromised trusted extension.
- The initial single-instance lock is macOS-specific. Linux or Windows support requires a portability decision and platform tests rather than an unverified claim.
- No external Go dependency is required for the proposed Phase 2 design.
- Phase 2 implementation must include adversarial integration tests for binding, authentication, origin/host checks, file permissions, stale discovery, size/time limits, cancellation, and shutdown.
- This ADR does not authorize the daemon, Pi extension, SSE, SQLite, durable jobs, evaluator calls, autostart, telemetry, remote access, or any business behavior.
