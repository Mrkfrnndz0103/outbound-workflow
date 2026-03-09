# WF2.1 Complete Setup on Northflank (Production Runbook)

This runbook deploys **WF2.1** (`workflows/wf21-drive-csv-consolidation/cmd`) on Northflank as an always-on service with:

- Google Drive ZIP polling
- CSV consolidation and filtered Google Sheets import
- R2 upload
- SeaTalk summary send (bot or webhook)
- persistent WF2.1 state/status files

This guide assumes **WF3 is removed** from this repository.

## 1. Deployment Model and Constraints

WF2.1 is stateful. It writes cursor/status JSON files locally and expects those files to survive restarts.

- Container build:
  - Dockerfile: `/workflows/wf21-drive-csv-consolidation/Dockerfile.render`
  - Build context: `/`
- Why this Dockerfile:
  - Includes `poppler-utils` and `imagemagick` for `WF21_SUMMARY_RENDER_MODE=pdf_png`
- Health endpoints exposed by app:
  - `GET /healthz`
  - `GET /status` (when `WF21_STATUS_FILE` is enabled)

Important with persistent volume attached:

- Run with **one replica** only.
- During deploy/redeploy, old instance is stopped before new instance starts when the same volume is attached.
- Do not configure horizontal scaling for this service.

## 2. Preflight Checklist

Collect these values before touching Northflank UI.

### 2.1 Required secret values

- `WF21_GOOGLE_CREDENTIALS_JSON` (service account JSON string)  
  (alternative: `WF21_GOOGLE_CREDENTIALS_FILE`, but JSON secret is easier on Northflank)
- `WF21_R2_ACCOUNT_ID`
- `WF21_R2_BUCKET`
- `WF21_R2_ACCESS_KEY_ID`
- `WF21_R2_SECRET_ACCESS_KEY`
- `WF21_SEATALK_APP_ID` (bot mode)
- `WF21_SEATALK_APP_SECRET` (bot mode)
- `WF21_SEATALK_GROUP_ID` (bot mode)
- `WF21_SEATALK_WEBHOOK_URL` (webhook mode only)

### 2.2 Required non-secret values

- `WF21_DRIVE_PARENT_FOLDER_ID`
- `WF21_DESTINATION_SHEET_ID`
- Destination tabs:
  - `WF21_DESTINATION_TAB_PENDING_RCV`
  - `WF21_DESTINATION_TAB_PACKED_IN_ANOTHER_TO`
  - `WF21_DESTINATION_TAB_NO_LHPACKING`

### 2.3 Google permission checks (mandatory)

Share all required assets with the service account email found in `WF21_GOOGLE_CREDENTIALS_JSON`:

- source Drive folder (`WF21_DRIVE_PARENT_FOLDER_ID`)
- destination spreadsheet (`WF21_DESTINATION_SHEET_ID`)
- summary sheet (if different from destination sheet)
- optional `bot_config` spreadsheet (if `WF21_USE_BOT_CONFIG=true`)

### 2.4 SeaTalk checks

- For `WF21_SUMMARY_SEATALK_MODE=bot`:
  - bot credentials are valid
  - bot is a member of target group
- For `WF21_SUMMARY_SEATALK_MODE=webhook`:
  - webhook URL is valid and active

## 3. Northflank UI Setup (Click-by-Click)

Use the order below.

### 3.1 Create Project and Service

UI path:

- `Workspace` -> `Create project` (if needed)
- Open project -> `Create` -> `Service`

In service creation wizard:

1. Choose `Combined service`.
2. Connect/select your repository.
3. Choose branch.
4. Build method: `Dockerfile`.
5. Dockerfile path: `/workflows/wf21-drive-csv-consolidation/Dockerfile.render`.
6. Build context: `/`.
7. Create service.

### 3.2 Configure Runtime (recommended baseline)

UI path:

- `Project` -> `Service` -> `Deployment` / `Runtime` (label varies by UI version)

Set:

- Instances/replicas: `1`
- Start command: leave default (Dockerfile `CMD`)
- Auto deploy: your preference (`on` for GitOps workflow)

### 3.3 Configure Network Port

UI path:

- `Project` -> `Service` -> `Network`

Set:

1. Add HTTP port `8080`.
2. Keep private unless external access is required.

### 3.4 Configure Health Checks

UI path:

- `Project` -> `Service` -> `Health checks`

Add one HTTP health check:

1. Type: `Liveness` (or `Readiness`)
2. Protocol: `HTTP`
3. Port: `8080`
4. Path: `/healthz`

Required env for this:

```dotenv
WF21_ENABLE_HEALTH_SERVER=true
WF21_HEALTH_PORT=8080
```

### 3.5 Add Persistent Volume

UI path:

- `Project` -> `Service` -> `Volumes` -> `Add volume`

Set:

1. Volume name: `wf21-state` (or preferred)
2. Mount path: `/data`
3. Save/apply

Then set:

```dotenv
WF21_STATE_FILE=/data/workflow2-1-drive-csv-consolidation-state.json
WF21_STATUS_FILE=/data/workflow2-1-drive-csv-consolidation-status.json
```

### 3.6 Add Secrets and Environment Variables

UI path:

- `Project` -> `Service` -> `Environment`

Recommended:

1. Create secrets (or a secret group) for sensitive values.
2. Inject them into this service.
3. Add non-secret values as runtime variables.
4. Keep variable names exactly as WF21 expects.

## 4. Environment Variables (Paste-Ready)

Add these under `Service -> Environment`.

### 4.1 Required secrets

```dotenv
WF21_GOOGLE_CREDENTIALS_JSON=<service-account-json>
WF21_R2_ACCOUNT_ID=<r2-account-id>
WF21_R2_BUCKET=<r2-bucket>
WF21_R2_ACCESS_KEY_ID=<r2-access-key-id>
WF21_R2_SECRET_ACCESS_KEY=<r2-secret-access-key>
WF21_SEATALK_APP_ID=<seatalk-app-id>
WF21_SEATALK_APP_SECRET=<seatalk-app-secret>
WF21_SEATALK_GROUP_ID=<seatalk-group-id>
```

### 4.2 Core runtime baseline (recommended for production)

```dotenv
WF21_CONTINUOUS=true
WF21_POLL_INTERVAL_SECONDS=3
WF21_DRY_RUN=false
WF21_BOOTSTRAP_PROCESS_EXISTING=false
WF21_DROP_LEADING_UNNAMED_COLUMN=true

WF21_DRIVE_PARENT_FOLDER_ID=1oU9kj5VIJIoNrR388wYCHSdtHGanRrgZ
WF21_DESTINATION_SHEET_ID=1mdi-8ACluDHGZ7yAyNLwXLwpmQ4f6VAx3kpbaJORViA
WF21_DESTINATION_TAB_PENDING_RCV=pending_rcv
WF21_DESTINATION_TAB_PACKED_IN_ANOTHER_TO=packed_in_another_to
WF21_DESTINATION_TAB_NO_LHPACKING=no_lhpacking

WF21_R2_OBJECT_PREFIX=wf2-1
WF21_SHEETS_BATCH_SIZE=7000
WF21_SHEETS_WRITE_RETRY_MAX_ATTEMPTS=6
WF21_SHEETS_WRITE_RETRY_BASE_MS=1000
WF21_SHEETS_WRITE_RETRY_MAX_MS=15000
WF21_TEMP_DIR=
WF21_TIMEZONE=Asia/Manila

WF21_ENABLE_HEALTH_SERVER=true
WF21_HEALTH_PORT=8080
WF21_STATE_FILE=/data/workflow2-1-drive-csv-consolidation-state.json
WF21_STATUS_FILE=/data/workflow2-1-drive-csv-consolidation-status.json
```

### 4.3 Summary send settings (bot mode)

```dotenv
WF21_SUMMARY_SEND_ENABLED=true
WF21_USE_BOT_CONFIG=false
WF21_SUMMARY_SEATALK_MODE=bot
WF21_SEATALK_BASE_URL=https://openapi.seatalk.io
```

### 4.4 Summary render settings (pdf_png profile)

```dotenv
WF21_SUMMARY_SHEET_ID=1mdi-8ACluDHGZ7yAyNLwXLwpmQ4f6VAx3kpbaJORViA
WF21_SUMMARY_TAB=[SOC] Backlogs Summary
WF21_SUMMARY_RANGE=B2:Q59

WF21_SUMMARY_SECOND_IMAGE_ENABLED=true
WF21_SUMMARY_SECOND_TAB=config
WF21_SUMMARY_SECOND_RANGES=E157:Y195

WF21_SUMMARY_EXTRA_IMAGES_ENABLED=false
WF21_SUMMARY_EXTRA_IMAGES=

WF21_SUMMARY_SYNC_CELL=config!B1
WF21_SUMMARY_WAIT_SECONDS=5
WF21_SUMMARY_STABILITY_RUNS=3
WF21_SUMMARY_STABILITY_WAIT_SECONDS=2

WF21_SUMMARY_RENDER_MODE=pdf_png
WF21_SUMMARY_RENDER_SCALE=1
WF21_SUMMARY_AUTO_FIT_COLUMNS=false
WF21_SUMMARY_PDF_DPI=180
WF21_SUMMARY_PDF_CONVERTER=pdftoppm
WF21_SUMMARY_PDF_STRICT=true
WF21_SUMMARY_IMAGE_MAX_WIDTH_PX=1800
WF21_SUMMARY_IMAGE_MAX_BASE64_BYTES=5242880
WF21_SUMMARY_HTTP_TIMEOUT_SECONDS=90
```

### 4.5 Alternative modes

Webhook mode:

```dotenv
WF21_SUMMARY_SEATALK_MODE=webhook
WF21_SEATALK_WEBHOOK_URL=<webhook-url>
```

Shared `bot_config` override mode:

```dotenv
WF21_USE_BOT_CONFIG=true
BOT_CONFIG_SHEET_ID=<sheet-id>
BOT_CONFIG_TAB=bot_config
```

When `WF21_USE_BOT_CONFIG=true` and `BOT_CONFIG_SHEET_ID` is set, the `wf21` row in `bot_config` overrides mode/target/app/webhook.

## 5. Deploy From UI

UI path:

- `Project` -> `Service` -> `Deployments`

Steps:

1. Click `Deploy` (or `Redeploy`).
2. In build logs verify Dockerfile path is `/workflows/wf21-drive-csv-consolidation/Dockerfile.render`.
3. Wait until deployment is `Running`.
4. Confirm health check is `Healthy`.

## 6. Post-Deploy Verification (UI + Functional)

UI path:

- `Project` -> `Service` -> `Logs`
- `Project` -> `Service` -> `Network`
- `Project` -> `Service` -> `Volumes`

Check:

1. Startup logs show watch mode and poll interval.
2. Health endpoint returns 200 on `/healthz`.
3. `/status` returns JSON (if `WF21_STATUS_FILE` enabled).
4. Logs show cycle processing for ZIP detection/import.
5. Destination tabs update as expected.
6. SeaTalk receives summary caption + images.
7. `/data/workflow2-1-drive-csv-consolidation-status.json` timestamp updates every cycle.

## 7. Operations Runbook

### 7.1 Safe config changes

1. Update env variables.
2. Redeploy service.
3. Confirm new instance healthy.
4. Validate logs and `/status`.

### 7.2 Rollback

Use Northflank deployment history:

1. Open previous successful deployment.
2. Redeploy that version.
3. Confirm volume is still mounted at `/data`.

### 7.3 Scaling guidance

- Keep replicas at `1` for stateful mode with local JSON state files.
- If you need multi-replica processing, redesign state storage to external shared store first.

### 7.4 First-run replay behavior

- `WF21_BOOTSTRAP_PROCESS_EXISTING=true` (code default): first run can process existing historical ZIPs.
- `WF21_BOOTSTRAP_PROCESS_EXISTING=false` (recommended here): first run sets baseline and processes only new uploads.

## 8. Optional One-shot / Scheduled Job Pattern

Northflank jobs/cron can run WF2.1, but this is usually not ideal due to local state semantics.

If you still choose one-shot:

- set `WF21_CONTINUOUS=false`
- trigger manually or schedule runs
- keep persistent storage for state/status files

Without persistent state, replay and dedup behavior becomes inconsistent across executions.

## 9. Full WF21 Environment Reference

Defaults below are from WF2.1 runtime code.

| Variable | Required | Default | Notes |
| --- | --- | --- | --- |
| `WF21_GOOGLE_CREDENTIALS_FILE` | Conditionally | empty | Alternative to JSON secret; falls back from `GOOGLE_APPLICATION_CREDENTIALS`. |
| `WF21_GOOGLE_CREDENTIALS_JSON` | Conditionally | empty | Required if file-based credentials not provided. |
| `WF21_DRIVE_PARENT_FOLDER_ID` | No | `1oU9kj5VIJIoNrR388wYCHSdtHGanRrgZ` | Source Drive folder ID. |
| `WF21_DESTINATION_SHEET_ID` | No | `1mdi-8ACluDHGZ7yAyNLwXLwpmQ4f6VAx3kpbaJORViA` | Destination spreadsheet. |
| `WF21_DESTINATION_TAB_PENDING_RCV` | Yes | `pending_rcv` | Must be unique across destination tabs. |
| `WF21_DESTINATION_TAB_PACKED_IN_ANOTHER_TO` | Yes | `packed_in_another_to` | Must be unique across destination tabs. |
| `WF21_DESTINATION_TAB_NO_LHPACKING` | Yes | `no_lhpacking` | Must be unique across destination tabs. |
| `WF21_R2_ACCOUNT_ID` | Yes | empty | R2 account ID. |
| `WF21_R2_BUCKET` | Yes | empty | R2 bucket. |
| `WF21_R2_ACCESS_KEY_ID` | Yes | empty | R2 access key. |
| `WF21_R2_SECRET_ACCESS_KEY` | Yes | empty | R2 secret key. |
| `WF21_R2_OBJECT_PREFIX` | No | `wf2-1` | Object key prefix. |
| `WF21_STATE_FILE` | No | `data/workflow2-1-drive-csv-consolidation-state.json` | Use `/data/...` on Northflank volume. |
| `WF21_STATUS_FILE` | No | `data/workflow2-1-drive-csv-consolidation-status.json` | Set `none` or `off` to disable status file and `/status`. |
| `WF21_CONTINUOUS` | No | `true` | `false` for one-shot mode. |
| `WF21_POLL_INTERVAL_SECONDS` | No | `3` | Runtime minimum is `3`. |
| `WF21_DRY_RUN` | No | `false` | Skips write/send side effects. |
| `WF21_BOOTSTRAP_PROCESS_EXISTING` | No | `true` | Recommended `false` in production cutover. |
| `WF21_DROP_LEADING_UNNAMED_COLUMN` | No | `true` | Drops hidden leading unnamed CSV column. |
| `WF21_SHEETS_BATCH_SIZE` | No | `7000` | Runtime minimum is `100`. |
| `WF21_SHEETS_WRITE_RETRY_MAX_ATTEMPTS` | No | `6` | Runtime clamped to `1..10`. |
| `WF21_SHEETS_WRITE_RETRY_BASE_MS` | No | `1000` | Runtime minimum `100ms`. |
| `WF21_SHEETS_WRITE_RETRY_MAX_MS` | No | `15000` | Runtime max `60000ms`, cannot be below base delay. |
| `WF21_TEMP_DIR` | No | empty | Optional temp path override. |
| `WF21_ENABLE_HEALTH_SERVER` | No | `true` | Enables `/healthz` and `/status`. |
| `WF21_HEALTH_PORT` | No | `PORT` fallback, then `8080` | Set `8080` explicitly for Northflank health check consistency. |
| `WF21_SUMMARY_SEND_ENABLED` | No | `true` | Disable to skip SeaTalk summary sending. |
| `WF21_SUMMARY_SEATALK_MODE` | No | `bot` | Allowed: `bot`, `webhook`. |
| `WF21_SEATALK_GROUP_ID` | Bot mode | empty | Fallback: `WF2_SEATALK_GROUP_ID`. |
| `WF21_SEATALK_APP_ID` | Bot mode | empty | Fallbacks: `WF2_SEATALK_APP_ID`, `SEATALK_APP_ID`. |
| `WF21_SEATALK_APP_SECRET` | Bot mode | empty | Fallbacks: `WF2_SEATALK_APP_SECRET`, `SEATALK_APP_SECRET`. |
| `WF21_SEATALK_BASE_URL` | No | `https://openapi.seatalk.io` | Fallbacks: `WF2_SEATALK_BASE_URL`, `SEATALK_BASE_URL`. |
| `WF21_SEATALK_WEBHOOK_URL` | Webhook mode | empty | Fallback: `SEATALK_SYSTEM_WEBHOOK_URL`. |
| `WF21_USE_BOT_CONFIG` | No | `false` | When true + `BOT_CONFIG_SHEET_ID`, `wf21` row can override send config. |
| `BOT_CONFIG_SHEET_ID` | Optional | empty | Required only if using bot_config override mode. |
| `BOT_CONFIG_TAB` | Optional | `bot_config` | Bot config tab name. |
| `WF21_SUMMARY_SHEET_ID` | No | `WF21_DESTINATION_SHEET_ID` | Summary source sheet ID. |
| `WF21_SUMMARY_TAB` | No | `[SOC] Backlogs Summary` | Summary tab name. |
| `WF21_SUMMARY_RANGE` | No | `B2:Q59` | Main summary capture range. |
| `WF21_SUMMARY_SECOND_IMAGE_ENABLED` | No | `true` | Enables second image capture. |
| `WF21_SUMMARY_SECOND_TAB` | Conditional | `config` | Required when second image is enabled. |
| `WF21_SUMMARY_SECOND_RANGES` | Conditional | `E154:Y184` | Comma-separated A1 ranges; required when second image is enabled. |
| `WF21_SUMMARY_EXTRA_IMAGES_ENABLED` | No | `true` | Enables optional extra image list. |
| `WF21_SUMMARY_EXTRA_IMAGES` | No | empty | Comma-separated image refs, supports `tab!range`. |
| `WF21_SUMMARY_SYNC_CELL` | Conditional | `config!B1` | Required when summary send is enabled and not dry-run. |
| `WF21_SUMMARY_WAIT_SECONDS` | No | `5` | Wait after import before summary capture/send. |
| `WF21_SUMMARY_STABILITY_RUNS` | No | `3` | Runtime minimum `1`. |
| `WF21_SUMMARY_STABILITY_WAIT_SECONDS` | No | `2` | Delay between stability checks. |
| `WF21_SUMMARY_RENDER_MODE` | No | `styled` | Allowed: `styled`, `pdf_png`. |
| `WF21_SUMMARY_RENDER_SCALE` | No | `2` | Runtime clamped to `1..4`. |
| `WF21_SUMMARY_AUTO_FIT_COLUMNS` | No | `false` | Auto-resize columns in styled mode. |
| `WF21_SUMMARY_PDF_DPI` | No | `180` | Runtime clamped to `72..600`. |
| `WF21_SUMMARY_PDF_CONVERTER` | No | `auto` | Allowed: `auto`, `pdftoppm`, `magick`. |
| `WF21_SUMMARY_PDF_STRICT` | No | `false` | In `pdf_png`, fail hard if converter/export fails. |
| `WF21_SUMMARY_IMAGE_MAX_WIDTH_PX` | No | `3000` | Runtime minimum `1200`. |
| `WF21_SUMMARY_IMAGE_MAX_BASE64_BYTES` | No | `5242880` | If below `524288`, runtime resets to default. |
| `WF21_SUMMARY_HTTP_TIMEOUT_SECONDS` | No | `45` | HTTP timeout for summary send. |
| `WF21_TIMEZONE` | No | `Asia/Manila` | Used in summary caption timestamps. |

## 10. Troubleshooting

### 10.1 Config validation fails at startup

Typical log examples:

- `set WF21_GOOGLE_CREDENTIALS_FILE/GOOGLE_APPLICATION_CREDENTIALS or WF21_GOOGLE_CREDENTIALS_JSON`
- `WF21_R2_ACCOUNT_ID, WF21_R2_BUCKET, WF21_R2_ACCESS_KEY_ID, WF21_R2_SECRET_ACCESS_KEY are required`
- `WF21_SUMMARY_SEATALK_MODE must be one of: bot, webhook`

Fix:

- verify env variable names and values exactly
- ensure required mode-specific values are provided

### 10.2 Google 403 / access denied

Fix:

- share Drive folder and Sheets with service account email
- check summary tab/range is in the shared spreadsheet

### 10.3 SeaTalk send failures

Fix for bot mode:

- verify `WF21_SUMMARY_SEATALK_MODE=bot`
- verify `WF21_SEATALK_APP_ID`, `WF21_SEATALK_APP_SECRET`, `WF21_SEATALK_GROUP_ID`
- verify `WF21_USE_BOT_CONFIG=false` unless you intentionally want the shared `bot_config` row to override WF21 env vars
- if `WF21_USE_BOT_CONFIG=true`, verify the `wf21` row app ID/app secret in `bot_config`
- verify bot is in target group

Fix for webhook mode:

- verify `WF21_SUMMARY_SEATALK_MODE=webhook`
- verify `WF21_SEATALK_WEBHOOK_URL` (or global fallback) is valid

### 10.4 Service unhealthy

Fix:

- ensure `WF21_ENABLE_HEALTH_SERVER=true`
- ensure `WF21_HEALTH_PORT=8080`
- ensure Northflank health check path is `/healthz` on port `8080`

### 10.5 State resets after redeploy

Fix:

- ensure volume is mounted at `/data`
- ensure `WF21_STATE_FILE` and `WF21_STATUS_FILE` point to `/data/...`
- ensure service is not scaled above one replica

### 10.6 pdf_png render errors

Typical errors:

- `WF21_SUMMARY_PDF_CONVERTER=pdftoppm but pdftoppm is not installed`
- `WF21_SUMMARY_RENDER_MODE=pdf_png requires converter availability`

Fix:

- use Dockerfile `/workflows/wf21-drive-csv-consolidation/Dockerfile.render`
- keep `WF21_SUMMARY_PDF_CONVERTER=pdftoppm` or `auto`

## 11. Northflank Docs Referenced

- Build with Dockerfile: https://northflank.com/docs/v1/application/build/build-with-a-dockerfile
- Configure ports: https://northflank.com/docs/v1/application/network/configure-ports
- Configure health checks: https://northflank.com/docs/v1/application/observe/configure-health-checks
- Add a persistent volume: https://northflank.com/docs/v1/application/databases-and-persistence/add-a-volume
- Inject runtime variables/secrets: https://northflank.com/docs/v1/application/secure/inject-secrets
- Run continuously: https://northflank.com/docs/v1/application/run/run-an-image-continuously
- Run once/on schedule: https://northflank.com/docs/v1/application/run/run-an-image-once-or-on-a-schedule

