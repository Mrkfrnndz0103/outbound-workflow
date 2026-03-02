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

## go-bot-workflow-mm-lh-provided

- Build command: `go build -o bin/workflow-mm-lh-provided ./cmd/workflow-mm-lh-provided`
- Workflow source: `cmd/workflow-mm-lh-provided`

### Render Vars (`render.yaml`)

| Key | Management | Value |
| --- | --- | --- |
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

## go-bot-workflow-drive-csv-consolidation

- Build command: `go build -o bin/workflow-drive-csv-consolidation ./cmd/workflow-drive-csv-consolidation`
- Workflow source: `cmd/workflow-drive-csv-consolidation`

### Render Vars (`render.yaml`)

| Key | Management | Value |
| --- | --- | --- |
| `SEATALK_SYSTEM_WEBHOOK_URL` | `sync: false` (unmanaged/secret) | - |
| `WF21_BOOTSTRAP_PROCESS_EXISTING` | `value` (managed) | `true` |
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
| `WF21_POLL_INTERVAL_SECONDS` | `value` (managed) | `1` |
| `WF21_R2_ACCESS_KEY_ID` | `sync: false` (unmanaged/secret) | - |
| `WF21_R2_ACCOUNT_ID` | `sync: false` (unmanaged/secret) | - |
| `WF21_R2_BUCKET` | `sync: false` (unmanaged/secret) | - |
| `WF21_R2_OBJECT_PREFIX` | `value` (managed) | `wf2-1` |
| `WF21_R2_SECRET_ACCESS_KEY` | `sync: false` (unmanaged/secret) | - |
| `WF21_SEATALK_APP_ID` | `value` (managed) | `Mjk2ODA1MDg0MTMw` |
| `WF21_SEATALK_APP_SECRET` | `sync: false` (unmanaged/secret) | - |
| `WF21_SEATALK_BASE_URL` | `value` (managed) | `https://openapi.seatalk.io` |
| `WF21_SEATALK_GROUP_ID` | `value` (managed) | `NTMwNTQyNTc2OTg4` |
| `WF21_SEATALK_WEBHOOK_URL` | `sync: false` (unmanaged/secret) | - |
| `WF21_SHEETS_BATCH_SIZE` | `value` (managed) | `7000` |
| `WF21_SHEETS_WRITE_RETRY_BASE_MS` | `value` (managed) | `1000` |
| `WF21_SHEETS_WRITE_RETRY_MAX_ATTEMPTS` | `value` (managed) | `6` |
| `WF21_SHEETS_WRITE_RETRY_MAX_MS` | `value` (managed) | `15000` |
| `WF21_STATE_FILE` | `value` (managed) | `data/workflow2-1-drive-csv-consolidation-state.json` |
| `WF21_STATUS_FILE` | `value` (managed) | `data/workflow2-1-drive-csv-consolidation-status.json` |
| `WF21_SUMMARY_AUTO_FIT_COLUMNS` | `value` (managed) | `false` |
| `WF21_SUMMARY_HTTP_TIMEOUT_SECONDS` | `value` (managed) | `90` |
| `WF21_SUMMARY_IMAGE_MAX_BASE64_BYTES` | `value` (managed) | `5242880` |
| `WF21_SUMMARY_IMAGE_MAX_WIDTH_PX` | `value` (managed) | `1800` |
| `WF21_SUMMARY_PDF_CONVERTER` | `value` (managed) | `auto` |
| `WF21_SUMMARY_PDF_DPI` | `value` (managed) | `180` |
| `WF21_SUMMARY_RANGE` | `value` (managed) | `B2:Q59` |
| `WF21_SUMMARY_RENDER_MODE` | `value` (managed) | `styled` |
| `WF21_SUMMARY_RENDER_SCALE` | `value` (managed) | `1` |
| `WF21_SUMMARY_SEATALK_MODE` | `value` (managed) | `bot` |
| `WF21_SUMMARY_SECOND_IMAGE_ENABLED` | `value` (managed) | `false` |
| `WF21_SUMMARY_SEND_ENABLED` | `value` (managed) | `true` |
| `WF21_SUMMARY_SHEET_ID` | `value` (managed) | `1mdi-8ACluDHGZ7yAyNLwXLwpmQ4f6VAx3kpbaJORViA` |
| `WF21_SUMMARY_STABILITY_RUNS` | `value` (managed) | `3` |
| `WF21_SUMMARY_STABILITY_WAIT_SECONDS` | `value` (managed) | `2` |
| `WF21_SUMMARY_SYNC_CELL` | `value` (managed) | `config!B1` |
| `WF21_SUMMARY_TAB` | `value` (managed) | `[SOC] Backlogs Summary` |
| `WF21_SUMMARY_WAIT_SECONDS` | `value` (managed) | `8` |
| `WF21_TEMP_DIR` | `value` (managed) | `""` |
| `WF21_TIMEZONE` | `value` (managed) | `Asia/Manila` |

### Code Scan (Env Keys)

- Detected keys (prefix-filtered for this service): `GOOGLE_APPLICATION_CREDENTIALS`, `PORT`, `SEATALK_APP_ID`, `SEATALK_APP_SECRET`, `SEATALK_BASE_URL`, `SEATALK_SYSTEM_WEBHOOK_URL`, `WF21_BOOTSTRAP_PROCESS_EXISTING`, `WF21_CONTINUOUS`, `WF21_DESTINATION_SHEET_ID`, `WF21_DESTINATION_TAB_NO_LHPACKING`, `WF21_DESTINATION_TAB_PACKED_IN_ANOTHER_TO`, `WF21_DESTINATION_TAB_PENDING_RCV`, `WF21_DRIVE_PARENT_FOLDER_ID`, `WF21_DROP_LEADING_UNNAMED_COLUMN`, `WF21_DRY_RUN`, `WF21_ENABLE_HEALTH_SERVER`, `WF21_GOOGLE_CREDENTIALS_FILE`, `WF21_GOOGLE_CREDENTIALS_JSON`, `WF21_HEALTH_PORT`, `WF21_POLL_INTERVAL_SECONDS`, `WF21_R2_ACCESS_KEY_ID`, `WF21_R2_ACCOUNT_ID`, `WF21_R2_BUCKET`, `WF21_R2_OBJECT_PREFIX`, `WF21_R2_SECRET_ACCESS_KEY`, `WF21_SEATALK_APP_ID`, `WF21_SEATALK_APP_SECRET`, `WF21_SEATALK_BASE_URL`, `WF21_SEATALK_GROUP_ID`, `WF21_SEATALK_WEBHOOK_URL`, `WF21_SHEETS_BATCH_SIZE`, `WF21_SHEETS_WRITE_RETRY_BASE_MS`, `WF21_SHEETS_WRITE_RETRY_MAX_ATTEMPTS`, `WF21_SHEETS_WRITE_RETRY_MAX_MS`, `WF21_STATE_FILE`, `WF21_STATUS_FILE`, `WF21_SUMMARY_AUTO_FIT_COLUMNS`, `WF21_SUMMARY_HTTP_TIMEOUT_SECONDS`, `WF21_SUMMARY_IMAGE_MAX_BASE64_BYTES`, `WF21_SUMMARY_IMAGE_MAX_WIDTH_PX`, `WF21_SUMMARY_PDF_CONVERTER`, `WF21_SUMMARY_PDF_DPI`, `WF21_SUMMARY_RANGE`, `WF21_SUMMARY_RENDER_MODE`, `WF21_SUMMARY_RENDER_SCALE`, `WF21_SUMMARY_SEATALK_MODE`, `WF21_SUMMARY_SECOND_IMAGE_ENABLED`, `WF21_SUMMARY_SECOND_RANGES`, `WF21_SUMMARY_SECOND_TAB`, `WF21_SUMMARY_SEND_ENABLED`, `WF21_SUMMARY_SHEET_ID`, `WF21_SUMMARY_STABILITY_RUNS`, `WF21_SUMMARY_STABILITY_WAIT_SECONDS`, `WF21_SUMMARY_SYNC_CELL`, `WF21_SUMMARY_TAB`, `WF21_SUMMARY_WAIT_SECONDS`, `WF21_TEMP_DIR`, `WF21_TIMEZONE`
- Missing from `render.yaml`:
  - `SEATALK_APP_ID`
  - `SEATALK_APP_SECRET`
  - `SEATALK_BASE_URL`
  - `WF21_SUMMARY_SECOND_RANGES`
  - `WF21_SUMMARY_SECOND_TAB`

