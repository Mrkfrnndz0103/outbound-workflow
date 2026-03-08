# Workflow 2.1 Implementation Notes

Workflow name: `workflow_2_1_drive_csv_consolidation`

Command:

```powershell
go run ./workflows/wf21-drive-csv-consolidation/cmd
```

## What it does

1. Watches a Google Drive parent folder for new `.zip` uploads and processes pending files oldest -> newest.
2. Reads all `.csv` files from the zip.
3. Consolidates CSV files into one canonical CSV:
   - Uses the first CSV header as canonical.
   - Aligns subsequent CSV rows by header name.
   - Optionally drops hidden/unnamed leading column (default enabled).
4. Uploads the consolidated CSV to Cloudflare R2.
5. Writes matched rows to destination columns `A:K` only (does not clear `L+`) across three tabs.
6. After import, waits for update propagation, captures `[SOC] Backlogs Summary!B2:Q59` as a styled image, then sends it to SeaTalk group via system account webhook.

## Default source/destination

- Drive parent folder ID: `1oU9kj5VIJIoNrR388wYCHSdtHGanRrgZ`
- Destination sheet ID: `1mdi-8ACluDHGZ7yAyNLwXLwpmQ4f6VAx3kpbaJORViA`
- Destination tabs:
  - `pending_rcv` (`Receive Status` contains `Pending Receive`)
  - `packed_in_another_to` (`Remark` contains both `Pack in another TO` and `Pack in another HandoverTask`)
  - `no_lhpacking` (`Remark` contains `Receive in`)

## Filter and output rules

Source file columns: `A:AH` (header row in row 1).

Import conditions:

- `pending_rcv`: `Receive Status` contains `Pending Receive`
- `packed_in_another_to`: `Remark` contains both `Pack in another TO` and `Pack in another HandoverTask`
- `no_lhpacking`: `Remark` contains `Receive in`

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
- Google Sheet writes are chunked in batches (`WF21_SHEETS_BATCH_SIZE`, default `7000`).
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
- `WF21_DESTINATION_TAB_PENDING_RCV` (default `pending_rcv`)
- `WF21_DESTINATION_TAB_PACKED_IN_ANOTHER_TO` (default `packed_in_another_to`)
- `WF21_DESTINATION_TAB_NO_LHPACKING` (default `no_lhpacking`)
- `WF21_R2_OBJECT_PREFIX` (default `wf2-1`)
- `WF21_STATE_FILE` (default `data/workflow2-1-drive-csv-consolidation-state.json`; also supports `r2://...` object keys in the configured R2 bucket)
- `WF21_STATUS_FILE` (default `data/workflow2-1-drive-csv-consolidation-status.json`, set `none` to disable, also supports `r2://...`)
- `WF21_LOCK_FILE` (optional advisory lock path; supports local path or `r2://...`)
- `WF21_LOCK_STALE_AFTER_SECONDS` (default `1200`)
- `WF21_BOOTSTRAP_PROCESS_EXISTING` (default `true`)
- `WF21_DROP_LEADING_UNNAMED_COLUMN` (default `true`)
- `WF21_DRY_RUN` (default `false`)
- `WF21_CONTINUOUS` (default `true`)
- `WF21_ENABLE_HEALTH_SERVER` (default `true`)
- `WF21_HEALTH_PORT` (default uses `PORT`, fallback `8080`)
- `WF21_POLL_INTERVAL_SECONDS` (default `3`)
- `WF21_SHEETS_BATCH_SIZE` (default `7000`)
- `WF21_SHEETS_WRITE_RETRY_MAX_ATTEMPTS` (default `6`)
- `WF21_SHEETS_WRITE_RETRY_BASE_MS` (default `1000`)
- `WF21_SHEETS_WRITE_RETRY_MAX_MS` (default `15000`)
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
- `WF21_SUMMARY_WAIT_SECONDS` (default `5`)
- `WF21_SUMMARY_STABILITY_RUNS` (default `3`)
- `WF21_SUMMARY_STABILITY_WAIT_SECONDS` (default `2`)
- `WF21_SUMMARY_RENDER_MODE` (default `styled`; `pdf_png` uses Google Sheets PDF export -> PNG conversion for closer visual fidelity)
- `WF21_SUMMARY_RENDER_SCALE` (default `2`)
- `WF21_SUMMARY_AUTO_FIT_COLUMNS` (default `false`; set `false` to preserve sheet layout, `true` to auto-resize columns for long text)
- `WF21_SUMMARY_PDF_DPI` (default `180`; used when `WF21_SUMMARY_RENDER_MODE=pdf_png`)
- `WF21_SUMMARY_PDF_CONVERTER` (default `auto`; `auto|pdftoppm|magick`, used when `WF21_SUMMARY_RENDER_MODE=pdf_png`)
- `WF21_SUMMARY_IMAGE_MAX_WIDTH_PX` (default `3000`)
- `WF21_SUMMARY_IMAGE_MAX_BASE64_BYTES` (default `5242880`)
- `WF21_SUMMARY_HTTP_TIMEOUT_SECONDS` (default `10`)
- `WF21_TIMEZONE` (default `Asia/Manila`; used for summary caption timestamp)

PDF mode dependency note:

- `WF21_SUMMARY_RENDER_MODE=pdf_png` requires either:
  - Poppler (`pdftoppm` in PATH), or
  - ImageMagick (`magick` in PATH)

## Railway note

Railway is the recommended hosted path for WF2.1 when you need `WF21_SUMMARY_RENDER_MODE=pdf_png`.

- `workflows/wf21-drive-csv-consolidation/Dockerfile.render` installs Poppler (`pdftoppm`) and ImageMagick (`magick`).
- `workflows/wf21-drive-csv-consolidation/railway.toml` pins Railway to that Dockerfile and `/healthz`.
- Use a Railway volume mounted at `/data` for `WF21_STATE_FILE` and `WF21_STATUS_FILE`.
- This workflow exposes `GET /healthz` and `GET /status` (when `WF21_STATUS_FILE` is enabled).

## Quick run examples

One-shot:

```powershell
$env:WF21_CONTINUOUS = "false"
go run ./workflows/wf21-drive-csv-consolidation/cmd
```

Watch mode:

```powershell
$env:WF21_CONTINUOUS = "true"
$env:WF21_POLL_INTERVAL_SECONDS = "3"
go run ./workflows/wf21-drive-csv-consolidation/cmd
```

