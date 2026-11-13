# CMH parameter normalization

This is an offline, request-processing-only plugin artifact. It exports only the `outbound` stage hook and config validation; it does not declare a task driver, provider relay routes, models, channel types, credentials, fetch targets, persistent data, usage facts or pricing rules. It is not automatically embedded, registered or enabled by adding this directory. This change performs no live upload, instance registration, activation or channel binding.

## Conservative scope

All switches default to `false`; the default channel list is empty and therefore does nothing. A request is changed only when **all** of these checks pass:

- Stage is `outbound`
- Format is exactly `openai`
- Client path is exactly `/v1/chat/completions`
- The actual numeric channel ID is in `channelIds`
- `ctx.model` equals `expectedModel` exactly
- `ctx.upstreamModel` equals `expectedUpstreamModel` exactly

Missing channel/upstream context, an empty channel list or any mismatch returns `continue` without effects. Channel type is never inferred from model names, supplier identity or the channel type number. Instance-level model filters can further narrow scope but do not replace these checks.

## Configuration

Example with synthetic identifiers only:

```json
{
  "channelIds": "7,9",
  "expectedModel": "public-chat",
  "expectedUpstreamModel": "mapped-chat",
  "removeTemperature": false,
  "removeTopP": false,
  "convertDeveloperToSystem": false
}
```

`channelIds` is a comma-separated **string**, not a JSON number array. Each ID must be a positive canonical decimal safe integer, without whitespace, signs, leading zeros, decimal points, exponents, empty entries or duplicates. There are at most 100 IDs and 2048 characters. The string representation intentionally allows validation to reject ambiguous numeric spellings that a JSON number parser would otherwise normalize.

Both expected names are required and nonempty when channels are configured. Names are exact strings of at most 128 characters, without whitespace, control characters or `*`; no trim, case folding, wildcard expansion or upstream-model fallback is performed. With empty channels, omitted names are valid and the instance remains a no-op. Unknown fields and non-boolean switches are rejected. The host schema and `validateConfig` reject malformed saves; a stale malformed config encountered on an eligible format/path is rejected with a fixed sanitized message, without echoing its values.

| Switch | Enabled behavior |
| --- | --- |
| `removeTemperature` | Remove an existing top-level `temperature`, including explicit zero |
| `removeTopP` | Remove an existing top-level `top_p`, including explicit zero |
| `convertDeveloperToSystem` | Replace only `/messages/<index>/role` when its value is exactly `developer` |

Disabled switches leave their fields untouched. Message content, names, tool fields, unknown fields, explicit zero/false values and inline media survive role conversion because it never replaces or copies the message object. Inputs are read-only and no request data is stored between calls. At most **512 patch operations in total** are emitted, including parameter removals. A request needing 513 or more is rejected with `normalize.too_many_ops`, with no partial effects.

The plugin never changes max-token fields, `n`, `model`, streaming/usage/billing fields or headers. It does not read or derive response usage, prices, reservation or settlement data. It is not a publication/model whitelist gate.

## Host limitations

The hook runs before channel parameter overrides: those overrides can reintroduce a removed field or override a converted role. The host does not run `outbound` on body pass-through channels, task submissions or upstream multipart bodies. The plugin does not normalize Responses, Claude, Gemini, legacy completions, audio or any other endpoint. A vendor-specific body labeled `openai` still requires administrator verification that it is truly Chat Completions shaped. There is no live-provider compatibility or billing test in this offline change.

Normal host failures remain fail-closed: the plugin does not declare `optional: true`. Explicit rejections stop the request without trying another channel. Deployment requires a separate authorized upload/version activation and a disabled-by-default instance, followed by exact channel/model configuration and explicit switch enablement. No such deployment is part of this change.

## Offline verification

From the repository root, using a built gateway CLI:

```powershell
& "$env:TEMP/cmh-gateway-ui-acceptance.exe" plugin lint plugins/tasks/cmh-param-normalize/plugin.js
& "$env:TEMP/cmh-gateway-ui-acceptance.exe" plugin test plugins/tasks/cmh-param-normalize/plugin.js --fixture plugins/tasks/cmh-param-normalize/golden.json
go test ./ext/domestic -run TestParamNormalizePluginHostContract -count=1
```

The golden fixture includes disabled/unmatched scopes, exact matching, explicit zero/false preservation, media-safe developer leaf changes, malformed config, aggregate 512/513-operation boundaries and sequential calls after matching/rejected requests. The consolidated Go integration test uses the real `pkg/jsplugin.ReplayFixture`, `CallTransform`, host config validation and `pkg/requestpatch` decoding/application. It verifies transform-only metadata and original request preservation without a database, network, credentials or registration.

Metadata wording was reviewed separately from lint: bilingual names, summary and setting titles are equivalent short phrases without trailing punctuation; boolean titles name the enabled action. There are no usage-schema/price descriptions. This repository intentionally keeps this guide under `ext/docs/plugins`, not under `plugins`; no marketplace README/changelog publication claim is made.
