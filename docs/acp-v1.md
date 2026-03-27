# ACP v1

ACP v1 is Gastown's runtime contract for agent CLIs that can speak JSON-RPC over
standard input/output. It turns runtime support into a protocol implementation
instead of a bespoke integration.

## Transport

- JSON-RPC 2.0
- newline-delimited JSON messages
- stdin for requests/notifications to the runtime
- stdout for responses/notifications from the runtime
- stderr is reserved for debug output and is not part of the protocol

Use [gt acp verify](../internal/cmd/acp.go) to probe an implementation from a
workspace or against a raw command.

## Lifecycle

ACP v1 currently standardizes this sequence:

1. `initialize`
2. `session/new`
3. `session/prompt`
4. `session/update` notifications while work is in progress
5. optional `session/set_mode` or `_ping` heartbeat traffic
6. `session/cancel` when the orchestrator needs to stop a turn

## Required Methods

### `initialize`

Request:

```json
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1}}
```

Response:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "protocolVersion": 1,
    "capabilities": {}
  }
}
```

The runtime must return `protocolVersion >= 1`.

### `session/new`

Request:

```json
{"jsonrpc":"2.0","id":2,"method":"session/new"}
```

Response:

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "result": {
    "sessionId": "abc123",
    "modes": {
      "currentModeId": "default",
      "availableModes": [
        {"id":"default","name":"Default"}
      ]
    }
  }
}
```

`sessionId` is required.

### `session/prompt`

Request:

```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "method": "session/prompt",
  "params": {
    "sessionId": "abc123",
    "prompt": [
      {"type":"text","text":"health check"}
    ]
  }
}
```

The runtime may answer with a normal JSON-RPC result and/or emit
`session/update` notifications while work is in flight.

## Notifications

### `session/update`

This is the progress/event channel from the runtime back to Gastown. The exact
payload is runtime-defined, but it should be JSON and should include the
`sessionId` when possible.

### `session/cancel`

Gastown may send this notification to stop the active turn for a session.

### `session/set_mode`

Optional. Gastown uses this as the preferred heartbeat when the runtime exposes
session modes. If unsupported, the proxy falls back to `_ping` or disables
heartbeats.

## Runtime Classes

Gastown classifies runtimes into three buckets:

| Class | Contract | Typical fit |
|------|----------|-------------|
| `acp` | ACP v1 over JSON-RPC/stdin | Native multi-turn runtimes with session IDs |
| `hooks` | CLI + hook/plugin integration | CLIs that can receive Gastown context and tool policy via files/hooks |
| `shim` | tmux-only orchestration | Any terminal CLI without native integration |

ACP is the preferred platform contract. Hooks and shim support remain available
for runtimes that are not yet ACP-capable.

## Schemas

- [`initialize.request.schema.json`](schemas/acp/initialize.request.schema.json)
- [`initialize.response.schema.json`](schemas/acp/initialize.response.schema.json)
- [`session-new.response.schema.json`](schemas/acp/session-new.response.schema.json)
- [`session-prompt.request.schema.json`](schemas/acp/session-prompt.request.schema.json)
- [`session-update.notification.schema.json`](schemas/acp/session-update.notification.schema.json)
