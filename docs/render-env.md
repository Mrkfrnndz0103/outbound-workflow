# Render Environment Variables

Generated from `render.yaml` and workflow code env lookups.

Regenerate with:

```powershell
go run ./scripts/generate_render_env_doc.go
```

Auto-update on local `.env` / `.env.example` / `render.yaml` changes:

```powershell
powershell -ExecutionPolicy Bypass -File ./scripts/watch_render_env_doc.ps1
```

## go-bot-seatalk-bot

- Build command: `go build -o bin/seatalk-bot ./cmd/seatalk-bot`
- Workflow source: `cmd/seatalk-bot`

### Render Vars (`render.yaml`)

| Key | Management | Value |
| --- | --- | --- |
| `BOT_COMMAND_PREFIX` | `value` (managed) | `/` |
| `BOT_CONFIG_SHEET_ID` | `value` (managed) | `1pLN46ZKWJIsidswMeoxhZwoacuFMR08sCaTFG6mLytc` |
| `BOT_CONFIG_SYNC_BASE_URL` | `value` (managed) | `https://openapi.seatalk.io` |
| `BOT_CONFIG_SYNC_HTTP_TIMEOUT_SECONDS` | `value` (managed) | `10` |
| `BOT_CONFIG_TAB` | `value` (managed) | `bot_config` |
| `SEATALK_APP_ID` | `sync: false` (unmanaged/secret) | - |
| `SEATALK_APP_SECRET` | `sync: false` (unmanaged/secret) | - |
| `SEATALK_BASE_URL` | `value` (managed) | `https://openapi.seatalk.io` |
| `SEATALK_HTTP_TIMEOUT_SECONDS` | `value` (managed) | `10` |
| `SEATALK_MODE` | `value` (managed) | `bot` |
| `SEATALK_SIGNING_SECRET` | `sync: false` (unmanaged/secret) | - |
| `WF1_GOOGLE_CREDENTIALS_FILE` | `value` (managed) | `""` |
| `WF1_GOOGLE_CREDENTIALS_JSON` | `sync: false` (unmanaged/secret) | - |
| `WF21_GOOGLE_CREDENTIALS_FILE` | `value` (managed) | `""` |
| `WF21_GOOGLE_CREDENTIALS_JSON` | `sync: false` (unmanaged/secret) | - |
| `WF2_GOOGLE_CREDENTIALS_FILE` | `value` (managed) | `""` |
| `WF2_GOOGLE_CREDENTIALS_JSON` | `sync: false` (unmanaged/secret) | - |
| `WORKFLOWS_FILE` | `value` (managed) | `workflows.yaml` |
| `WORKFLOW_DEFAULT_TIMEOUT_SECONDS` | `value` (managed) | `120` |

### Code Scan (Env Keys)

- Detected keys (prefix-filtered for this service): ``
- Missing from `render.yaml`: none

## go-bot-workflow-mm-lh-provided

- Build command: `go build -o bin/workflow-mm-lh-provided ./workflows/wf1-mm-lh-provided/cmd`
- Workflow source: `workflows/wf1-mm-lh-provided/cmd`

### Render Vars (`render.yaml`)

| Key | Management | Value |
| --- | --- | --- |
| `BOT_CONFIG_SHEET_ID` | `value` (managed) | `1pLN46ZKWJIsidswMeoxhZwoacuFMR08sCaTFG6mLytc` |
| `BOT_CONFIG_TAB` | `value` (managed) | `bot_config` |
| `SEATALK_SYSTEM_WEBHOOK_URL` | `sync: false` (unmanaged/secret) | - |
| `WF1_AT_ALL` | `value` (managed) | `false` |
| `WF1_BOOTSTRAP_SEND_EXISTING` | `value` (managed) | `false` |
| `WF1_CONTINUOUS` | `value` (managed) | `true` |
| `WF1_DEBUG_LOG_SKIPS` | `value` (managed) | `false` |
| `WF1_DRY_RUN` | `value` (managed) | `false` |
| `WF1_ENABLE_HEALTH_SERVER` | `value` (managed) | `true` |
| `WF1_FORCE_SEND_AFTER_SECONDS` | `value` (managed) | `60` |
| `WF1_GOOGLE_CREDENTIALS_FILE` | `value` (managed) | `""` |
| `WF1_GOOGLE_CREDENTIALS_JSON` | `sync: false` (unmanaged/secret) | - |
| `WF1_GROUP_DEFER_SECONDS` | `value` (managed) | `20` |
| `WF1_HEALTH_PORT` | `value` (managed) | `""` |
| `WF1_HTTP_TIMEOUT_SECONDS` | `value` (managed) | `10` |
| `WF1_MAX_READY_AGE_SECONDS` | `value` (managed) | `300` |
| `WF1_POLL_INTERVAL_SECONDS` | `value` (managed) | `1` |
| `WF1_PROVIDE_TIME_MIN_AGE_SECONDS` | `value` (managed) | `0` |
| `WF1_SEATALK_APP_ID` | `value` (managed) | `MzQ0MDc5Nzc5MjQ2` |
| `WF1_SEATALK_APP_SECRET` | `sync: false` (unmanaged/secret) | - |
| `WF1_SEATALK_BASE_URL` | `value` (managed) | `https://openapi.seatalk.io` |
| `WF1_SEATALK_GROUP_ID` | `value` (managed) | `MTQ4Mzk5MDI0NjYw` |
| `WF1_SEATALK_MODE` | `value` (managed) | `bot` |
| `WF1_SEATALK_WEBHOOK_URL` | `sync: false` (unmanaged/secret) | - |
| `WF1_SELF_PING_INTERVAL_SECONDS` | `value` (managed) | `300` |
| `WF1_SELF_PING_URL` | `sync: false` (unmanaged/secret) | - |
| `WF1_SEND_MIN_INTERVAL_MS` | `value` (managed) | `1200` |
| `WF1_SEND_RETRY_BASE_MS` | `value` (managed) | `1000` |
| `WF1_SEND_RETRY_MAX_ATTEMPTS` | `value` (managed) | `5` |
| `WF1_SEND_RETRY_MAX_MS` | `value` (managed) | `30000` |
| `WF1_SHEET_ID` | `value` (managed) | `1mhzIfYfF1VSA9sPiqnLw7OgY1S_gI0wEzkXBQ1CCuDg` |
| `WF1_SHEET_RANGE` | `value` (managed) | `A1658:M` |
| `WF1_SHEET_TAB` | `value` (managed) | `MM LH Provided` |
| `WF1_STATE_FILE` | `value` (managed) | `data/workflow1-mm-lh-provided-state.json` |
| `WF1_STATUS_FILE` | `value` (managed) | `data/workflow1-mm-lh-provided-status.json` |

### Code Scan (Env Keys)

- Detected keys (prefix-filtered for this service): `GOOGLE_APPLICATION_CREDENTIALS`, `PORT`, `SEATALK_APP_ID`, `SEATALK_APP_SECRET`, `SEATALK_BASE_URL`, `SEATALK_SYSTEM_WEBHOOK_URL`, `WF1_AT_ALL`, `WF1_BOOTSTRAP_SEND_EXISTING`, `WF1_CONTINUOUS`, `WF1_DEBUG_LOG_SKIPS`, `WF1_DRY_RUN`, `WF1_ENABLE_HEALTH_SERVER`, `WF1_FORCE_SEND_AFTER_SECONDS`, `WF1_GOOGLE_CREDENTIALS_FILE`, `WF1_GOOGLE_CREDENTIALS_JSON`, `WF1_GROUP_DEFER_SECONDS`, `WF1_HEALTH_PORT`, `WF1_HTTP_TIMEOUT_SECONDS`, `WF1_MAX_READY_AGE_SECONDS`, `WF1_POLL_INTERVAL_SECONDS`, `WF1_PROVIDE_TIME_MIN_AGE_SECONDS`, `WF1_SEATALK_APP_ID`, `WF1_SEATALK_APP_SECRET`, `WF1_SEATALK_BASE_URL`, `WF1_SEATALK_GROUP_ID`, `WF1_SEATALK_MODE`, `WF1_SEATALK_WEBHOOK_URL`, `WF1_SELF_PING_INTERVAL_SECONDS`, `WF1_SELF_PING_URL`, `WF1_SEND_MIN_INTERVAL_MS`, `WF1_SEND_RETRY_BASE_MS`, `WF1_SEND_RETRY_MAX_ATTEMPTS`, `WF1_SEND_RETRY_MAX_MS`, `WF1_SHEET_ID`, `WF1_SHEET_RANGE`, `WF1_SHEET_TAB`, `WF1_STATE_FILE`, `WF1_STATUS_FILE`
- Missing from `render.yaml`:
  - `SEATALK_APP_ID`
  - `SEATALK_APP_SECRET`
  - `SEATALK_BASE_URL`

## go-bot-sync-bot-config-groups

- Build command: `go build -o bin/bot-config-group-sync ./cmd/bot-config-group-sync`
- Workflow source: `cmd/bot-config-group-sync`

### Render Vars (`render.yaml`)

| Key | Management | Value |
| --- | --- | --- |
| `BOT_CONFIG_SHEET_ID` | `value` (managed) | `1pLN46ZKWJIsidswMeoxhZwoacuFMR08sCaTFG6mLytc` |
| `BOT_CONFIG_SYNC_BASE_URL` | `value` (managed) | `https://openapi.seatalk.io` |
| `BOT_CONFIG_SYNC_HTTP_TIMEOUT_SECONDS` | `value` (managed) | `10` |
| `BOT_CONFIG_TAB` | `value` (managed) | `bot_config` |
| `WF21_GOOGLE_CREDENTIALS_FILE` | `value` (managed) | `""` |
| `WF21_GOOGLE_CREDENTIALS_JSON` | `sync: false` (unmanaged/secret) | - |

### Code Scan (Env Keys)

- Detected keys (prefix-filtered for this service): `GOOGLE_APPLICATION_CREDENTIALS`, `WF21_GOOGLE_CREDENTIALS_FILE`, `WF21_GOOGLE_CREDENTIALS_JSON`
- Missing from `render.yaml`: none

