# WF2.1 Complete Railway Deployment Guide (Production + UI Runbook)

This guide deploys **WF2.1 only** (`cmd/workflow-drive-csv-consolidation`) on Railway as a continuously running service, with:

- Google Drive ZIP polling
- CSV consolidation + Google Sheets import
- Cloudflare R2 upload
- SeaTalk summary send
- persistent WF2.1 state/status files via Railway Volume
- external uptime checks via **UptimeRobot every 5 minutes**

This guide assumes WF3 is removed and not deployed.

## 1. Deployment Model (Railway)

WF2.1 is stateful and must persist local cursor/status files across redeploys.

- Build from Dockerfile: `cmd/workflow-drive-csv-consolidation/Dockerfile.render`
- Keep one running replica (single-writer state file model)
- Attach volume at `/data`
- Use Railway Healthcheck for deploy safety (`/healthz`)
- Use UptimeRobot for continuous external monitoring every 5 minutes

Important:

- Railway healthchecks are used during deployment cutover and are **not** continuous runtime monitoring.
- For stateful services with attached volumes, Railway can have a brief redeploy downtime (expected behavior).

## 2. Preflight Checklist

Prepare these before opening Railway UI.

### 2.1 Required secrets

- `WF21_GOOGLE_CREDENTIALS_JSON` (service-account JSON string)
- `WF21_R2_ACCOUNT_ID`
- `WF21_R2_BUCKET`
- `WF21_R2_ACCESS_KEY_ID`
- `WF21_R2_SECRET_ACCESS_KEY`
- `WF21_SEATALK_APP_ID` (bot mode)
- `WF21_SEATALK_APP_SECRET` (bot mode)
- `WF21_SEATALK_GROUP_ID` (bot mode)
- `WF21_SEATALK_GROUP_IDS` (bot mode, optional multi-group list)
- `WF21_SEATALK_WEBHOOK_URL` (webhook mode only)

### 2.2 Required non-secret values

- `WF21_DRIVE_PARENT_FOLDER_ID`
- `WF21_DESTINATION_SHEET_ID`
- `WF21_DESTINATION_TAB_PENDING_RCV`
- `WF21_DESTINATION_TAB_PACKED_IN_ANOTHER_TO`
- `WF21_DESTINATION_TAB_NO_LHPACKING`

### 2.3 Access checks

Google:

- Share source Drive folder with service account email
- Share destination spreadsheet with service account email
- Share summary spreadsheet/tab (if different)
- Share `bot_config` sheet too if using `WF21_USE_BOT_CONFIG=true`

SeaTalk:

- Bot mode: bot is in target group
- Webhook mode: webhook URL is valid and active

## 3. Railway UI Setup (Click-by-Click)

Follow this order.

### 3.1 Create project and service from GitHub

UI path:

1. Railway Dashboard -> `New Project`
2. `Deploy from GitHub repo`
3. Select repository
4. Railway creates service in your project

If Railway creates multiple services, keep only the WF2.1 service for this runbook.

### 3.2 Force Railway to use WF2.1 Dockerfile

UI path:

1. Open service -> `Variables`
2. Add variable:

```dotenv
RAILWAY_DOCKERFILE_PATH=cmd/workflow-drive-csv-consolidation/Dockerfile.render
```

This is required because the Dockerfile is not at repo root.

### 3.3 Configure deploy healthcheck

UI path:

1. Open service -> `Settings`
2. `Deploy` section -> `Healthcheck Path`
3. Set:
   - Healthcheck Path: `/healthz`

App settings:

```dotenv
WF21_ENABLE_HEALTH_SERVER=true
WF21_HEALTH_PORT=
```

Leave `WF21_HEALTH_PORT` empty so WF2.1 listens on Railway-injected `PORT`.
Do not manually set `PORT` unless you have a specific advanced routing reason.

### 3.4 Attach persistent volume

UI path:

1. Project canvas -> create/connect `Volume` (or Command Palette)
2. Attach volume to WF2.1 service
3. Set mount path: `/data`

Then set:

```dotenv
WF21_STATE_FILE=/data/workflow2-1-drive-csv-consolidation-state.json
WF21_STATUS_FILE=/data/workflow2-1-drive-csv-consolidation-status.json
```

### 3.5 Configure public domain (needed for UptimeRobot)

UI path:

1. Open service -> `Settings`
2. `Networking` -> `Public Networking`
3. Click `Generate Domain`
4. Save the generated `https://<service>.up.railway.app` URL

You will use this URL for `/healthz` monitoring.

### 3.6 Keep it always-on (recommended for WF2.1)

UI path:

1. Open service -> `Settings`
2. `Deploy` -> `Serverless`
3. Keep Serverless disabled for always-on behavior

WF2.1 also emits outbound traffic in normal operation, but always-on mode is the most predictable for this workflow.

## 4. Environment Variables (Paste-Ready)

Set these under `Service -> Variables`.

### 4.1 Required secrets

```dotenv
WF21_GOOGLE_CREDENTIALS_JSON=<service-account-json>
WF21_R2_ACCOUNT_ID=<r2-account-id>
WF21_R2_BUCKET=<r2-bucket-name>
WF21_R2_ACCESS_KEY_ID=<r2-access-key-id>
WF21_R2_SECRET_ACCESS_KEY=<r2-secret-access-key>
WF21_SEATALK_APP_ID=<seatalk-app-id>
WF21_SEATALK_APP_SECRET=<seatalk-app-secret>
WF21_SEATALK_GROUP_ID=<seatalk-group-id>
# Optional multi-group send in bot mode (comma/newline/semicolon separated)
WF21_SEATALK_GROUP_IDS=
```

### 4.2 Core runtime baseline

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
WF21_HEALTH_PORT=
WF21_STATE_FILE=/data/workflow2-1-drive-csv-consolidation-state.json
WF21_STATUS_FILE=/data/workflow2-1-drive-csv-consolidation-status.json
```

### 4.3 Summary send (bot mode baseline)

```dotenv
WF21_SUMMARY_SEND_ENABLED=true
WF21_USE_BOT_CONFIG=false
WF21_SUMMARY_SEATALK_MODE=bot
WF21_SEATALK_BASE_URL=https://openapi.seatalk.io
```

To send to multiple groups in bot mode, set:

```dotenv
WF21_SEATALK_GROUP_IDS=<group-id-1>,<group-id-2>
```

`WF21_SEATALK_GROUP_ID` remains supported for single-group mode/backward compatibility.
Set `WF21_SUMMARY_SEND_MIN_INTERVAL_SECONDS=5` for 5-second delay between groups/messages.

### 4.4 Summary render profile (`pdf_png`)

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
WF21_SUMMARY_SEND_MIN_INTERVAL_SECONDS=1
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

### 4.5 Optional routing alternatives

Webhook mode:

```dotenv
WF21_SUMMARY_SEATALK_MODE=webhook
WF21_SEATALK_WEBHOOK_URL=<webhook-url>
```

Use shared `bot_config` sheet:

```dotenv
WF21_USE_BOT_CONFIG=true
BOT_CONFIG_SHEET_ID=<sheet-id>
BOT_CONFIG_TAB=bot_config
```

## 5. Deploy on Railway

UI path:

1. Open service -> `Deployments`
2. Click `Deploy` (or redeploy latest commit)
3. Watch build logs

Confirm:

- build uses `cmd/workflow-drive-csv-consolidation/Dockerfile.render`
- deploy becomes healthy with `/healthz`
- service enters `Active` state

## 6. Validate End-to-End

### 6.1 Railway-level checks

- Logs show WF2.1 startup and watch mode cycle
- `/healthz` returns HTTP 200 from public domain
- `/status` returns JSON (if `WF21_STATUS_FILE` enabled)
- Volume is attached and mounted at `/data`

### 6.2 Workflow checks

- ZIP changes are detected from Drive
- Destination tabs are updated
- Summary sync cell updates (`config!B1`)
- SeaTalk receives caption + images
- Status file under `/data` updates every cycle

## 7. UptimeRobot Setup (Ping Every 5 Minutes)

Use UptimeRobot for external uptime checks after deployment.

### 7.1 Create monitor

1. Login to UptimeRobot dashboard
2. Click `+ Add New Monitor`
3. Monitor Type: `HTTP(s)`
4. URL: `https://<your-railway-domain>/healthz`
5. Friendly Name: `WF21 Railway Health`
6. Monitoring Interval: `5 minutes`
7. Choose alert contacts
8. Click `Create Monitor`

### 7.2 Optional second monitor (`/status`)

Create another monitor for:

- URL: `https://<your-railway-domain>/status`
- Type: `Keyword` (optional)
- Keyword: `workflow_2_1_drive_csv_consolidation`

This validates app liveness plus WF2.1 status JSON availability.

### 7.3 Important notes

- UptimeRobot free plan monitors every 5 minutes.
- Railway deployment healthcheck and UptimeRobot serve different purposes:
  - Railway healthcheck: deploy cutover safety
  - UptimeRobot: continuous external uptime monitoring/alerting

## 8. Operations

### 8.1 Safe config changes

1. Update variables
2. Trigger redeploy
3. Verify `/healthz`, `/status`, logs, and sheet/SeaTalk outputs

### 8.2 Rollback

1. Open `Deployments`
2. Select last known good deployment
3. Redeploy it
4. Confirm volume still mounted at `/data`

### 8.3 State + downtime behavior

With attached volume, brief redeploy downtime is expected to avoid data corruption.

## 9. Troubleshooting

### 9.1 Deploy fails healthcheck

Check:

- `WF21_ENABLE_HEALTH_SERVER=true`
- `WF21_HEALTH_PORT=` (empty, or matches Railway `PORT`)
- Healthcheck path exactly `/healthz`

### 9.2 Deploy is healthy but later app goes down

Expected possibility: Railway healthcheck is deploy-time only.  
Fix: use UptimeRobot monitor + alerting.

### 9.3 State resets after redeploy

Check:

- volume is attached
- mount path is `/data`
- `WF21_STATE_FILE`/`WF21_STATUS_FILE` point to `/data/...`

### 9.4 Google access errors (`403`, `access denied`)

Share required Drive folder and spreadsheets with service account email from `WF21_GOOGLE_CREDENTIALS_JSON`.

### 9.5 SeaTalk send failures

Bot mode:

- `WF21_SUMMARY_SEATALK_MODE=bot`
- app ID / app secret / group ID are valid
- bot is in target group

Webhook mode:

- `WF21_SUMMARY_SEATALK_MODE=webhook`
- webhook URL is valid and reachable

### 9.6 `pdf_png` converter errors

If you see converter availability errors:

- confirm Dockerfile path variable points to `cmd/workflow-drive-csv-consolidation/Dockerfile.render`
- keep `WF21_SUMMARY_PDF_CONVERTER=pdftoppm` or `auto`

## 10. References

- Railway Dockerfiles: https://docs.railway.com/builds/dockerfiles
- Railway Healthchecks: https://docs.railway.com/deployments/healthchecks
- Railway Volumes: https://docs.railway.com/volumes
- Railway Variables: https://docs.railway.com/variables
- Railway Public Networking: https://docs.railway.com/networking/public-networking
- Railway Domains: https://docs.railway.com/networking/domains/working-with-domains
- Railway Serverless (App Sleeping): https://docs.railway.com/deployments/serverless
- UptimeRobot monitor creation: https://help.uptimerobot.com/en/articles/11358364-how-to-create-your-first-monitor-on-uptimerobot-quick-setup-guide
- UptimeRobot monitoring interval: https://help.uptimerobot.com/en/articles/11360876-what-is-a-monitoring-interval-in-uptimerobot
