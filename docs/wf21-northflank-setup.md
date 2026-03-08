# WF2.1 Complete Setup on Northflank (Production Runbook)

This guide deploys **WF2.1** (`cmd/workflow-drive-csv-consolidation`) on Northflank as a continuously-running service, with:
- Google Drive ZIP polling
- Google Sheets import
- R2 upload
- SeaTalk bot summary send
- persistent WF2.1 state/status files

It assumes WF3 is removed from this repo.

## 1. What To Deploy

Deploy this container build:
- Dockerfile: `/cmd/workflow-drive-csv-consolidation/Dockerfile.render`
- Build context: `/`

Why this Dockerfile:
- It already installs `poppler-utils` and `imagemagick` required by `WF21_SUMMARY_RENDER_MODE=pdf_png`.

## 2. Prerequisites

Prepare these values first:
- `WF21_GOOGLE_CREDENTIALS_JSON` (service-account JSON string)
- `WF21_R2_ACCOUNT_ID`
- `WF21_R2_BUCKET`
- `WF21_R2_ACCESS_KEY_ID`
- `WF21_R2_SECRET_ACCESS_KEY`
- `WF21_SEATALK_APP_ID`
- `WF21_SEATALK_APP_SECRET`
- `WF21_SEATALK_GROUP_ID`

Google requirements:
- Share the source Drive folder and destination Sheets to the service account email in `WF21_GOOGLE_CREDENTIALS_JSON`.

## 3. UI Setup (Click-by-Click)

Use this exact order in the Northflank UI.

### 3.1 Create Project + Service

UI path:
- `Workspace` -> `Create project` (if you do not have one yet)
- Open project -> `Create` -> `Service`

In the wizard:
1. Select `Combined service`.
2. Connect/select your Git repository.
3. Select branch.
4. Build method: `Dockerfile`.
5. Dockerfile location: `/cmd/workflow-drive-csv-consolidation/Dockerfile.render`.
6. Build context: `/`.
7. Create service.

### 3.2 Configure Ports + Health Check

UI path:
- `Project` -> `Service` -> `Network`

Set:
1. Add HTTP port `8080`.
2. Keep it private unless you need public access.

UI path:
- `Project` -> `Service` -> `Health checks`

Set one HTTP health check:
1. Check type: `Liveness` (or `Readiness`).
2. Protocol: `HTTP`.
3. Port: `8080`.
4. Path: `/healthz`.

WF2.1 env values for health server:
```dotenv
WF21_ENABLE_HEALTH_SERVER=true
WF21_HEALTH_PORT=8080
```

### 3.3 Add Persistent Volume

UI path:
- `Project` -> `Service` -> `Volumes` -> `Add volume`

Set:
1. Volume name: `wf21-state` (or your preferred name).
2. Mount path: `/data`.
3. Save/apply.

Then set:
```dotenv
WF21_STATE_FILE=/data/workflow2-1-drive-csv-consolidation-state.json
WF21_STATUS_FILE=/data/workflow2-1-drive-csv-consolidation-status.json
```

Important behavior with attached volume:
- Deployment is limited to one instance.
- Old instance is terminated before new instance starts.

### 3.4 Add Environment Variables + Secrets

UI path:
- `Project` -> `Service` -> `Environment`

Add non-secret values as normal runtime variables.
Add secret values using Northflank secrets/secret groups, then inject into this service.

## 4. Configure Environment Variables

Add these in `Service -> Environment`.

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

Use Northflank secret/runtime variable injection for sensitive values rather than plain text variables.

### 4.2 Core WF2.1 runtime

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

WF21_SEATALK_BASE_URL=https://openapi.seatalk.io
```

### 4.3 Summary send via SeaTalk bot

```dotenv
WF21_SUMMARY_SEND_ENABLED=true
WF21_USE_BOT_CONFIG=false
WF21_SUMMARY_SEATALK_MODE=bot
```

If you prefer routing from shared `bot_config` sheet:
```dotenv
WF21_USE_BOT_CONFIG=true
BOT_CONFIG_SHEET_ID=<sheet-id>
BOT_CONFIG_TAB=bot_config
```
When enabled, `wf21` row in `bot_config` can override mode/group/app/webhook settings.

### 4.4 Summary image render settings

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

## 5. Deploy from UI

UI path:
- `Project` -> `Service` -> `Deployments`

Steps:
1. Click `Deploy` (or `Redeploy` if already created).
2. Open build logs and confirm Dockerfile path is `/cmd/workflow-drive-csv-consolidation/Dockerfile.render`.
3. Wait for deployment status `Running` and health check `Healthy`.

## 6. Verify End-To-End from UI

UI path:
- `Project` -> `Service` -> `Logs`
- `Project` -> `Service` -> `Network` (to test health endpoint if exposed)
- `Project` -> `Service` -> `Volumes` (confirm mount exists)

Validate:
1. Logs show watch mode startup (`watch mode enabled ...`).
2. Logs show cycle activity (ZIP detected / imported).
3. Logs show summary send flow.
4. Destination sheet tabs update.
5. SeaTalk target group receives caption + images.
6. `WF21_STATUS_FILE` under `/data` is updated every cycle.

## 7. Optional: One-shot / Scheduled Job Mode

Northflank supports cron jobs, but for WF2.1 this is usually not the best fit because WF2.1 relies on persistent local state files.

If you still run a one-shot pattern:
- Set `WF21_CONTINUOUS=false`
- Trigger manually or on schedule as a job
- Ensure you have a persistence strategy for state/status; otherwise replay/baseline behavior may be inconsistent across runs.

## 8. Troubleshooting

### Google export/import errors (403, access denied)
- Confirm the service account has access to:
  - source Drive folder
  - destination spreadsheet
  - summary sheet/tab/range

### SeaTalk bot send fails
- Re-check:
  - `WF21_SUMMARY_SEATALK_MODE=bot`
  - `WF21_SEATALK_APP_ID`
  - `WF21_SEATALK_APP_SECRET`
  - `WF21_SEATALK_GROUP_ID`
- Confirm bot is a member of target group.

### Service unhealthy
- Confirm app is listening on `8080` (`WF21_HEALTH_PORT=8080`).
- Confirm health check path is `/healthz`.

### State resets after redeploy
- Confirm volume mount exists at `/data`.
- Confirm state/status env paths point to `/data/...`.

## 9. Northflank Docs Referenced

- Build with Dockerfile: https://northflank.com/docs/v1/application/build/build-with-a-dockerfile
- Configure ports: https://northflank.com/docs/v1/application/network/configure-ports
- Configure health checks: https://northflank.com/docs/v1/application/observe/configure-health-checks
- Add a persistent volume: https://northflank.com/docs/v1/application/databases-and-persistence/add-a-volume
- Inject runtime variables/secrets: https://northflank.com/docs/v1/application/secure/inject-secrets
- Run continuously: https://northflank.com/docs/v1/application/run/run-an-image-continuously
- Run once/on schedule: https://northflank.com/docs/v1/application/run/run-an-image-once-or-on-a-schedule
