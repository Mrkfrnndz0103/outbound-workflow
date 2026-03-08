# WF2.1 Complete Railway Setup (Workflow Drive CSV Consolidation)

This guide deploys `cmd/workflow-drive-csv-consolidation` on Railway with:
- `WF21` logic only (no WF3)
- SeaTalk bot summary send enabled
- Persistent state/status files via Railway Volume

## 1. Preconditions

- Repo is connected to Railway (GitHub deploy) or you can deploy via Railway CLI.
- You have values for required secrets:
  - `WF21_GOOGLE_CREDENTIALS_JSON` (or use `WF21_GOOGLE_CREDENTIALS_FILE`)
  - `WF21_R2_ACCOUNT_ID`
  - `WF21_R2_BUCKET`
  - `WF21_R2_ACCESS_KEY_ID`
  - `WF21_R2_SECRET_ACCESS_KEY`
  - `WF21_SEATALK_APP_ID`
  - `WF21_SEATALK_APP_SECRET`
  - `WF21_SEATALK_GROUP_ID`

## 2. Create Railway Service

1. Create a new Railway project/service from this repository.
2. Service type: standard service (not function).
3. Keep auto-deploy as you prefer.

## 3. Force Railway to Use the WF21 Dockerfile

In the service `Variables` tab, add:

```dotenv
RAILWAY_DOCKERFILE_PATH=cmd/workflow-drive-csv-consolidation/Dockerfile.render
```

This ensures Railway builds the WF21 container defined in this repo.

## 4. Configure Healthcheck

In service settings:
- Healthcheck path: `/healthz`

Set:

```dotenv
WF21_ENABLE_HEALTH_SERVER=true
```

`WF21_HEALTH_PORT` can be left empty so app uses Railway `PORT`.

## 5. Add Persistent Volume

1. Add a Volume to this service.
2. Mount path: `/data`
3. Set:

```dotenv
WF21_STATE_FILE=/data/workflow2-1-drive-csv-consolidation-state.json
WF21_STATUS_FILE=/data/workflow2-1-drive-csv-consolidation-status.json
```

Without this, state/status are ephemeral and can reset on redeploy.

## 6. Set WF21 Variables (Bot Send Mode)

Use this as baseline (replace placeholders):

```dotenv
# Required runtime
WF21_GOOGLE_CREDENTIALS_JSON=<service-account-json>
WF21_R2_ACCOUNT_ID=<cloudflare-account-id>
WF21_R2_BUCKET=<r2-bucket-name>
WF21_R2_ACCESS_KEY_ID=<r2-access-key-id>
WF21_R2_SECRET_ACCESS_KEY=<r2-secret-access-key>

# Core workflow
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

# Summary send
WF21_SUMMARY_SEND_ENABLED=true
WF21_USE_BOT_CONFIG=false
WF21_SUMMARY_SEATALK_MODE=bot
WF21_SEATALK_APP_ID=<seatalk-app-id>
WF21_SEATALK_APP_SECRET=<seatalk-app-secret>
WF21_SEATALK_GROUP_ID=<seatalk-group-id>
WF21_SEATALK_BASE_URL=https://openapi.seatalk.io

# Summary snapshot render
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

Notes:
- `WF21_USE_BOT_CONFIG=false` keeps SeaTalk target from env variables.
- `WF21_SUMMARY_SEATALK_MODE=bot` sends via bot API (`group_id + app credentials`).

## 7. Deploy

Trigger deploy from Railway UI (or push commit if auto-deploy is on).

Expected startup:
- build uses `cmd/workflow-drive-csv-consolidation/Dockerfile.render`
- service becomes healthy on `/healthz`
- logs show watch mode and poll interval

## 8. Validate End-to-End

1. Check `/healthz` returns HTTP 200.
2. Watch logs for:
   - ZIP detection from Drive
   - import to destination tabs
   - summary sync cell update (`config!B1`)
   - SeaTalk bot caption/image sends
3. Confirm `WF21_STATUS_FILE` is updating in mounted volume path.

## 9. Optional: Run as Cron Instead of Always-On

If you want scheduled runs:

1. Set:
```dotenv
WF21_CONTINUOUS=false
```
2. Configure Railway Cron Schedule in service settings.
3. Railway cron minimum interval is 5 minutes; schedules are UTC.

Only use cron if your cycle can finish before the next schedule.

## 10. Troubleshooting

- `403 Access Denied` from Google Sheets export:
  - Share required sheets with the service account email in `WF21_GOOGLE_CREDENTIALS_JSON`.
- SeaTalk send errors:
  - Re-check `WF21_SEATALK_APP_ID`, `WF21_SEATALK_APP_SECRET`, `WF21_SEATALK_GROUP_ID`.
  - Confirm bot is in the target group.
- State not persisting:
  - Confirm volume is mounted and `WF21_STATE_FILE`/`WF21_STATUS_FILE` are under `/data`.
- Deploy never healthy:
  - Ensure `WF21_ENABLE_HEALTH_SERVER=true` and health path is `/healthz`.

