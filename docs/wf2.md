# Workflow 2.1 Implementation Notes

Workflow name: `workflow_2_1_drive_csv_consolidation`

Command:

```powershell
go run ./cmd/workflow-drive-csv-consolidation
```

## What it does

1. Watches a Google Drive parent folder for new `.zip` uploads and processes pending files oldest -> newest.
2. Reads all `.csv` files from the zip.
3. Consolidates CSV files into one canonical CSV:
   - Uses the first CSV header as canonical.
   - Aligns subsequent CSV rows by header name.
   - Optionally drops hidden/unnamed leading column (default enabled).
4. Uploads the consolidated CSV to Cloudflare R2.
5. Writes filtered rows to destination columns `A:K` only (does not clear `L+`).
6. After import, waits for update propagation, captures `[SOC] Backlogs Summary!B2:Q59` as a styled image, then sends it to SeaTalk group via system account webhook.

## Default source/destination

- Drive parent folder ID: `1oU9kj5VIJIoNrR388wYCHSdtHGanRrgZ`
- Destination sheet ID: `1mdi-8ACluDHGZ7yAyNLwXLwpmQ4f6VAx3kpbaJORViA`
- Destination tab: `generated_file`

## Filter and output rules

Source file columns: `A:AH` (header row in row 1).

Filter conditions:

- `Current Station` = `SOC 5`
- `Receiver Type` = `Station`

Imported output columns (in order):

1. `TO Number`
2. `SPX Tracking Number`
3. `Receiver Name`
4. `TO Order Quantity`
5. `TO Number`
6. `Operator`
7. `Create Time`
8. `Complete Time`
9. `Remark`
10. `Receive Status`
11. `Staging Area ID`

## Lightweight strategy for large files (100k+ rows)

- Zip is downloaded to temp file (disk), not memory.
- CSV rows are streamed file-by-file.
- Consolidated CSV is streamed to temp output file.
- Google Sheet writes are chunked in batches (`WF21_SHEETS_BATCH_SIZE`, default `5000`).
- State file tracks the processed cursor and prevents reprocessing already handled zips.

## Required environment variables

- `WF21_GOOGLE_CREDENTIALS_FILE` or `WF21_GOOGLE_CREDENTIALS_JSON`
- `WF21_R2_ACCOUNT_ID`
- `WF21_R2_BUCKET`
- `WF21_R2_ACCESS_KEY_ID`
- `WF21_R2_SECRET_ACCESS_KEY`
- `WF21_SUMMARY_SEATALK_MODE` (`bot` or `webhook`) when `WF21_SUMMARY_SEND_ENABLED=true` (default)
- `WF21_SEATALK_GROUP_ID` + `WF21_SEATALK_APP_ID` / `WF21_SEATALK_APP_SECRET` when mode is `bot` (supports `WF2_*` and global `SEATALK_*` fallbacks)
- `WF21_SEATALK_WEBHOOK_URL` (or `SEATALK_SYSTEM_WEBHOOK_URL`) when mode is `webhook`

## Cloudflare R2 free-tier setup (quick)

1. Create/Sign in to Cloudflare account.
2. Open `R2 Object Storage` in dashboard.
3. Create an R2 bucket (use this as `WF21_R2_BUCKET`).
4. Create R2 API token / access key pair with write access to that bucket.
5. Copy:
   - Account ID -> `WF21_R2_ACCOUNT_ID`
   - Access key ID -> `WF21_R2_ACCESS_KEY_ID`
   - Secret access key -> `WF21_R2_SECRET_ACCESS_KEY`
6. Keep bucket private unless you intentionally need public access.

## Optional environment variables

- `WF21_DRIVE_PARENT_FOLDER_ID`
- `WF21_DESTINATION_SHEET_ID`
- `WF21_DESTINATION_TAB`
- `WF21_R2_OBJECT_PREFIX` (default `wf2-1`)
- `WF21_STATE_FILE` (default `data/workflow2-1-drive-csv-consolidation-state.json`)
- `WF21_STATUS_FILE` (default `data/workflow2-1-drive-csv-consolidation-status.json`, set `none` to disable)
- `WF21_BOOTSTRAP_PROCESS_EXISTING` (default `true`)
- `WF21_DROP_LEADING_UNNAMED_COLUMN` (default `true`)
- `WF21_DRY_RUN` (default `false`)
- `WF21_CONTINUOUS` (default `true`)
- `WF21_ENABLE_HEALTH_SERVER` (default `true`)
- `WF21_HEALTH_PORT` (default uses `PORT`, fallback `8080`)
- `WF21_POLL_INTERVAL_SECONDS` (default `30`)
- `WF21_SHEETS_BATCH_SIZE` (default `5000`)
- `WF21_TEMP_DIR` (optional temp directory override)
- `WF21_SUMMARY_SEND_ENABLED` (default `true`)
- `WF21_SUMMARY_SEATALK_MODE` (default `bot`)
- `WF21_SEATALK_GROUP_ID` (fallback to `WF2_SEATALK_GROUP_ID`)
- `WF21_SEATALK_APP_ID` / `WF21_SEATALK_APP_SECRET` (fallback to `WF2_SEATALK_APP_ID` / `WF2_SEATALK_APP_SECRET`, then `SEATALK_APP_ID` / `SEATALK_APP_SECRET`)
- `WF21_SEATALK_BASE_URL` (fallback to `WF2_SEATALK_BASE_URL`, then `SEATALK_BASE_URL`, default `https://openapi.seatalk.io`)
- `WF21_SEATALK_WEBHOOK_URL` (fallback to `SEATALK_SYSTEM_WEBHOOK_URL`)
- `WF21_SUMMARY_SHEET_ID` (default `WF21_DESTINATION_SHEET_ID`)
- `WF21_SUMMARY_TAB` (default `[SOC] Backlogs Summary`)
- `WF21_SUMMARY_RANGE` (default `B2:Q59`)
- `WF21_SUMMARY_WAIT_SECONDS` (default `8`)
- `WF21_SUMMARY_STABILITY_RUNS` (default `3`)
- `WF21_SUMMARY_STABILITY_WAIT_SECONDS` (default `2`)
- `WF21_SUMMARY_RENDER_SCALE` (default `2`)
- `WF21_SUMMARY_IMAGE_MAX_WIDTH_PX` (default `3000`)
- `WF21_SUMMARY_IMAGE_MAX_BASE64_BYTES` (default `5242880`)
- `WF21_SUMMARY_HTTP_TIMEOUT_SECONDS` (default `10`)
- `WF21_TIMEZONE` (default `Asia/Manila`; used for summary caption timestamp)

## Render note for plans without worker service type

Use a `web` service with `healthCheckPath: /healthz`.
This workflow now exposes:

- `GET /healthz`
- `GET /status` (returns `WF21_STATUS_FILE` JSON when enabled)

## Quick run examples

One-shot:

```powershell
$env:WF21_CONTINUOUS = "false"
go run ./cmd/workflow-drive-csv-consolidation
```

Watch mode:

```powershell
$env:WF21_CONTINUOUS = "true"
$env:WF21_POLL_INTERVAL_SECONDS = "30"
go run ./cmd/workflow-drive-csv-consolidation
```
