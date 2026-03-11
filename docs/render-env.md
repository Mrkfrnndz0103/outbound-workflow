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

## go-bot-workflow2-1-drive-csv-consolidation

- Build command: ``
- Dockerfile: `workflows/wf21-drive-csv-consolidation/Dockerfile.render`
- Docker context: `.`
- Workflow source: `workflows/wf21-drive-csv-consolidation/cmd`

### Render Vars (`render.yaml`)

| Key | Management | Value |
| --- | --- | --- |
| `BOT_CONFIG_SHEET_ID` | `value` (managed) | `1pLN46ZKWJIsidswMeoxhZwoacuFMR08sCaTFG6mLytc` |
| `BOT_CONFIG_TAB` | `value` (managed) | `bot_config` |
| `WF21_BOOTSTRAP_PROCESS_EXISTING` | `value` (managed) | `false` |
| `WF21_CONTINUOUS` | `value` (managed) | `true` |
| `WF21_DESTINATION_SHEET_ID` | `value` (managed) | `1mdi-8ACluDHGZ7yAyNLwXLwpmQ4f6VAx3kpbaJORViA` |
| `WF21_DESTINATION_TAB_NO_LHPACKING` | `value` (managed) | `no_lhpacking` |
| `WF21_DESTINATION_TAB_PACKED_IN_ANOTHER_TO` | `value` (managed) | `packed_in_another_to` |
| `WF21_DESTINATION_TAB_PENDING_RCV` | `value` (managed) | `pending_rcv` |
| `WF21_DRIVE_PARENT_FOLDER_ID` | `value` (managed) | `1oU9kj5VIJIoNrR388wYCHSdtHGanRrgZ` |
| `WF21_DROP_LEADING_UNNAMED_COLUMN` | `value` (managed) | `true` |
| `WF21_DRY_RUN` | `value` (managed) | `false` |
| `WF21_ENABLE_HEALTH_SERVER` | `value` (managed) | `true` |
| `WF21_GOOGLE_CREDENTIALS_FILE` | `value` (managed) | `""` |
| `WF21_GOOGLE_CREDENTIALS_JSON` | `sync: false` (unmanaged/secret) | - |
| `WF21_HEALTH_PORT` | `value` (managed) | `""` |
| `WF21_NEWRELIC_ENVIRONMENT` | `value` (managed) | `render` |
| `WF21_NEWRELIC_LICENSE_KEY` | `sync: false` (unmanaged/secret) | - |
| `WF21_NEWRELIC_LOGS_BATCH_SIZE` | `value` (managed) | `50` |
| `WF21_NEWRELIC_LOGS_BATCH_WAIT_SECONDS` | `value` (managed) | `2` |
| `WF21_NEWRELIC_LOGS_ENABLED` | `value` (managed) | `false` |
| `WF21_NEWRELIC_LOGS_QUEUE_SIZE` | `value` (managed) | `1000` |
| `WF21_NEWRELIC_LOGS_TIMEOUT_SECONDS` | `value` (managed) | `8` |
| `WF21_NEWRELIC_LOG_API_URL` | `value` (managed) | `https://log-api.newrelic.com/log/v1` |
| `WF21_NEWRELIC_SERVICE` | `value` (managed) | `wf21-drive-csv-consolidation` |
| `WF21_NEWRELIC_SOURCE` | `value` (managed) | `workflow_2_1_drive_csv_consolidation` |
| `WF21_POLL_INTERVAL_SECONDS` | `value` (managed) | `3` |
| `WF21_R2_ACCESS_KEY_ID` | `sync: false` (unmanaged/secret) | - |
| `WF21_R2_ACCOUNT_ID` | `sync: false` (unmanaged/secret) | - |
| `WF21_R2_BUCKET` | `sync: false` (unmanaged/secret) | - |
| `WF21_R2_OBJECT_PREFIX` | `value` (managed) | `wf2-1` |
| `WF21_R2_SECRET_ACCESS_KEY` | `sync: false` (unmanaged/secret) | - |
| `WF21_SEATALK_APP_ID` | `sync: false` (unmanaged/secret) | - |
| `WF21_SEATALK_APP_SECRET` | `sync: false` (unmanaged/secret) | - |
| `WF21_SEATALK_BASE_URL` | `value` (managed) | `https://openapi.seatalk.io` |
| `WF21_SEATALK_GROUP_ID` | `sync: false` (unmanaged/secret) | - |
| `WF21_SEATALK_GROUP_IDS` | `sync: false` (unmanaged/secret) | - |
| `WF21_SEATALK_WEBHOOK_URL` | `sync: false` (unmanaged/secret) | - |
| `WF21_SHEETS_BATCH_SIZE` | `value` (managed) | `7000` |
| `WF21_SHEETS_WRITE_RETRY_BASE_MS` | `value` (managed) | `1000` |
| `WF21_SHEETS_WRITE_RETRY_MAX_ATTEMPTS` | `value` (managed) | `6` |
| `WF21_SHEETS_WRITE_RETRY_MAX_MS` | `value` (managed) | `15000` |
| `WF21_STATE_FILE` | `value` (managed) | `/var/data/workflow2-1-drive-csv-consolidation-state.json` |
| `WF21_STATUS_FILE` | `value` (managed) | `/var/data/workflow2-1-drive-csv-consolidation-status.json` |
| `WF21_SUMMARY_AUTO_FIT_COLUMNS` | `value` (managed) | `false` |
| `WF21_SUMMARY_EXTRA_IMAGES` | `value` (managed) | `""` |
| `WF21_SUMMARY_EXTRA_IMAGES_ENABLED` | `value` (managed) | `true` |
| `WF21_SUMMARY_HTTP_TIMEOUT_SECONDS` | `value` (managed) | `90` |
| `WF21_SUMMARY_IMAGE_MAX_BASE64_BYTES` | `value` (managed) | `2097152` |
| `WF21_SUMMARY_IMAGE_MAX_WIDTH_PX` | `value` (managed) | `1800` |
| `WF21_SUMMARY_PDF_CONVERTER` | `value` (managed) | `pdftoppm` |
| `WF21_SUMMARY_PDF_DPI` | `value` (managed) | `180` |
| `WF21_SUMMARY_PDF_STRICT` | `value` (managed) | `false` |
| `WF21_SUMMARY_RANGE` | `value` (managed) | `B2:Q59` |
| `WF21_SUMMARY_RENDER_MODE` | `value` (managed) | `pdf_png` |
| `WF21_SUMMARY_RENDER_SCALE` | `value` (managed) | `1` |
| `WF21_SUMMARY_SEATALK_MODE` | `value` (managed) | `bot` |
| `WF21_SUMMARY_SECOND_IMAGE_ENABLED` | `value` (managed) | `true` |
| `WF21_SUMMARY_SECOND_RANGES` | `value` (managed) | `E154:Y184` |
| `WF21_SUMMARY_SECOND_TAB` | `value` (managed) | `config` |
| `WF21_SUMMARY_SEND_ENABLED` | `value` (managed) | `true` |
| `WF21_SUMMARY_SEND_MIN_INTERVAL_SECONDS` | `value` (managed) | `1` |
| `WF21_SUMMARY_STABILITY_RUNS` | `value` (managed) | `3` |
| `WF21_SUMMARY_STABILITY_WAIT_SECONDS` | `value` (managed) | `1` |
| `WF21_SUMMARY_SYNC_CELL` | `value` (managed) | `config!B1` |
| `WF21_SUMMARY_TAB` | `value` (managed) | `[SOC] Backlogs Summary` |
| `WF21_SUMMARY_WAIT_SECONDS` | `value` (managed) | `1` |
| `WF21_TEMP_DIR` | `value` (managed) | `""` |
| `WF21_TIMEZONE` | `value` (managed) | `Asia/Manila` |
| `WF21_USE_BOT_CONFIG` | `value` (managed) | `false` |

### Code Scan (Env Keys)

- Detected keys (prefix-filtered for this service): ``
- Missing from `render.yaml`: none

## go-bot-workflow3-mdt-updates

- Build command: ``
- Dockerfile: `workflows/wf3-mdt-updates/Dockerfile.render`
- Docker context: `.`
- Workflow source: `workflows/wf3-mdt-updates/cmd`

### Render Vars (`render.yaml`)

| Key | Management | Value |
| --- | --- | --- |
| `WF3_BOOTSTRAP_SEND_EXISTING` | `value` (managed) | `false` |
| `WF3_CONTINUOUS` | `value` (managed) | `true` |
| `WF3_DRY_RUN` | `value` (managed) | `false` |
| `WF3_ENABLE_HEALTH_SERVER` | `value` (managed) | `true` |
| `WF3_GOOGLE_CREDENTIALS_FILE` | `value` (managed) | `""` |
| `WF3_GOOGLE_CREDENTIALS_JSON` | `sync: false` (unmanaged/secret) | - |
| `WF3_HEALTH_PORT` | `value` (managed) | `""` |
| `WF3_HTTP_TIMEOUT_SECONDS` | `value` (managed) | `90` |
| `WF3_IMAGE1_FIXED_HIDE_ROWS` | `value` (managed) | `16-26,28,30,32-37,39-40` |
| `WF3_IMAGE_MAX_BASE64_BYTES` | `value` (managed) | `5242880` |
| `WF3_IMAGE_RANGES` | `value` (managed) | `mdt!B1:P42,mdt!B44:P108,mdt!B109:P166,mdt!B167:P196,mdt!B198:P231` |
| `WF3_MONITOR_RANGE` | `value` (managed) | `G1:O227` |
| `WF3_PDF_CONVERTER` | `value` (managed) | `pdftoppm` |
| `WF3_PDF_DPI` | `value` (managed) | `180` |
| `WF3_POLL_INTERVAL_SECONDS` | `value` (managed) | `3` |
| `WF3_SEATALK_APP_ID` | `sync: false` (unmanaged/secret) | - |
| `WF3_SEATALK_APP_SECRET` | `sync: false` (unmanaged/secret) | - |
| `WF3_SEATALK_BASE_URL` | `value` (managed) | `https://openapi.seatalk.io` |
| `WF3_SEATALK_GROUP_ID` | `sync: false` (unmanaged/secret) | - |
| `WF3_SEND_DEBOUNCE_SECONDS` | `value` (managed) | `180` |
| `WF3_SEND_MIN_INTERVAL_SECONDS` | `value` (managed) | `1` |
| `WF3_SHEET_ID` | `value` (managed) | `1pLN46ZKWJIsidswMeoxhZwoacuFMR08sCaTFG6mLytc` |
| `WF3_SHEET_TAB` | `value` (managed) | `mdt` |
| `WF3_STABILITY_RUNS` | `value` (managed) | `3` |
| `WF3_STABILITY_WAIT_SECONDS` | `value` (managed) | `2` |
| `WF3_STATE_FILE` | `value` (managed) | `/var/data/workflow3-mdt-updates-state.json` |
| `WF3_STATUS_FILE` | `value` (managed) | `/var/data/workflow3-mdt-updates-status.json` |
| `WF3_TEMP_DIR` | `value` (managed) | `""` |
| `WF3_TEST_SEND_ONCE` | `value` (managed) | `false` |
| `WF3_TIMEZONE` | `value` (managed) | `Asia/Manila` |

### Code Scan (Env Keys)

- Detected keys (prefix-filtered for this service): `GOOGLE_APPLICATION_CREDENTIALS`, `PORT`, `SEATALK_APP_ID`, `SEATALK_APP_SECRET`, `SEATALK_BASE_URL`, `WF3_BOOTSTRAP_SEND_EXISTING`, `WF3_CONTINUOUS`, `WF3_DRY_RUN`, `WF3_ENABLE_HEALTH_SERVER`, `WF3_GOOGLE_CREDENTIALS_FILE`, `WF3_GOOGLE_CREDENTIALS_JSON`, `WF3_HEALTH_PORT`, `WF3_HTTP_TIMEOUT_SECONDS`, `WF3_IMAGE1_FIXED_HIDE_ROWS`, `WF3_IMAGE_MAX_BASE64_BYTES`, `WF3_IMAGE_RANGES`, `WF3_MONITOR_RANGE`, `WF3_PDF_CONVERTER`, `WF3_PDF_DPI`, `WF3_POLL_INTERVAL_SECONDS`, `WF3_SEATALK_APP_ID`, `WF3_SEATALK_APP_SECRET`, `WF3_SEATALK_BASE_URL`, `WF3_SEATALK_GROUP_ID`, `WF3_SEND_DEBOUNCE_SECONDS`, `WF3_SEND_MIN_INTERVAL_SECONDS`, `WF3_SHEET_ID`, `WF3_SHEET_TAB`, `WF3_STABILITY_RUNS`, `WF3_STABILITY_WAIT_SECONDS`, `WF3_STATE_FILE`, `WF3_STATUS_FILE`, `WF3_TEMP_DIR`, `WF3_TEST_SEND_ONCE`, `WF3_TIMEZONE`
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

