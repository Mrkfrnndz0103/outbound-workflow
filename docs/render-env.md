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

