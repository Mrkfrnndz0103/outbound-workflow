# SeaTalk Bot + System Account Sender (Go)

This project supports two modes:

- `bot` mode: callback-driven SeaTalk bot events/commands (`/callback`).
- `system_account` mode: outbound-only group messaging via system account webhook.

## Project layout

```text
cmd/seatalk-bot/main.go        # app entrypoint
internal/bot/                  # callback handling and command routing (bot mode)
internal/systemaccount/        # outbound send APIs (system_account mode)
internal/seatalk/              # SeaTalk clients/models
internal/workflow/             # workflow config + process runner (bot mode)
workflows.yaml                 # allowlisted workflows (bot mode)
docs/system_account-messaging.txt
```

## Prerequisites

- Go 1.22+

## Configure

Copy `.env.example` and choose one mode.
The app auto-loads `.env` from the working directory at startup.

### Bot mode

Set:

- `SEATALK_MODE=bot`
- `SEATALK_SIGNING_SECRET` (required)
- Optional: `SEATALK_APP_ID` + `SEATALK_APP_SECRET` for outbound single-chat replies

SeaTalk setup:

- Set Event Callback URL to `https://<your-public-host>/callback`
- Subscribe to:
  - `event_verification`
  - `message_from_bot_subscriber`
  - `new_bot_subscriber` (deprecated but supported)
  - `user_enter_chatroom_with_bot`
  - `new_mentioned_message_from_group_chat` / `new_mentioned_message_received_from_group_chat`
  - `new_message_received_from_thread`
  - `interactive_message_click`
  - `bot_added_to_group_chat`
  - `bot_removed_from_group_chat`

### System account mode

Set:

- `SEATALK_MODE=system_account`
- `SEATALK_SYSTEM_WEBHOOK_URL` (required, format like `https://openapi.seatalk.io/webhook/group/...`)

SeaTalk setup:

- Create/manage a system account from group chat settings in SeaTalk desktop.
- Copy the system account webhook URL shown in the group settings.
- No callback URL and no app credentials are required for this mode.

## Run

```powershell
go mod tidy
go run ./cmd/seatalk-bot
```

Default bind: `:8080`

## APIs

### Bot mode

- `POST /callback`
- `GET /healthz`

Commands:

- `/help`
- `/list`
- `/run <workflow> [args...]`

Workflow toggle:

- In `workflows.yaml`, set `enabled: false` under a workflow to disable it without deleting its block.

Default workflow list includes:

- `workflow_1_mm_lh_provided`
- `workflow_2_1_drive_csv_consolidation`

### System account mode

- `POST /send/text`
- `POST /send/image`
- `GET /healthz`

Text example:

```powershell
curl -X POST http://localhost:8081/send/text `
  -H "Content-Type: application/json" `
  -d "{\"content\":\"Hello World\",\"format\":1}"
```

Image example (`content` is Base64):

```powershell
curl -X POST http://localhost:8081/send/image `
  -H "Content-Type: application/json" `
  -d "{\"base64_content\":\"iVBORw0KGgoAAAANSUhEUg...\"}"
```

## Notes

- System account messaging follows the behavior documented in `docs/system_account-messaging.txt`.
- System accounts are outbound-only by design.
- System accounts are rate-limited (see SeaTalk doc; e.g., per-minute sending limit and silence period on abuse).

## Workflow 1: MM LH Provided Trigger

`workflow_1_mm_lh_provided` reads Google Sheet:

- Spreadsheet: `1mhzIfYfF1VSA9sPiqnLw7OgY1S_gI0wEzkXBQ1CCuDg`
- Tab: `MM LH Provided`
- Range: `A2:M`

Trigger behavior:

- Tracks each row's `Plate #` (`F` column) in a local state file.
- Sends to SeaTalk only when `F` transitions from blank to non-blank.
- Message format (single row):

```text
<mention-tag target="seatalk://user?id=0"/> For Docking

      **{C} - {M}**
      **Plate #: {F}**
      {G} - {H}
      pvd_tme: {I}
```

- Message format (merged when multiple rows share the same `I` minute; seconds ignored):

```text
<mention-tag target="seatalk://user?id=0"/> For Docking

      {C1} - {M1}
      Plate_#: {F1}
      {G1}-{H1}

      {C2} - {M2}
      Plate_#: {F2}
      {G2}-{H2}

Provided Time: {I minute}  # e.g. 2/25/2026 12:25 PM
```

Required env for this workflow:

- `WF1_SEATALK_MODE` (`webhook` or `bot`; default `webhook`)
- If `WF1_SEATALK_MODE=webhook`:
  - `WF1_SEATALK_WEBHOOK_URL` (fallback `SEATALK_SYSTEM_WEBHOOK_URL`)
- If `WF1_SEATALK_MODE=bot`:
  - `WF1_SEATALK_GROUP_ID` (fallback `WF2_SEATALK_GROUP_ID`)
  - `WF1_SEATALK_APP_ID` / `WF1_SEATALK_APP_SECRET` (fallbacks: `WF2_*`, then global `SEATALK_*`)
- Google credentials via one of:
  - `WF1_GOOGLE_CREDENTIALS_FILE` (or `GOOGLE_APPLICATION_CREDENTIALS`)
  - `WF1_GOOGLE_CREDENTIALS_JSON`

Optional env:

- `WF1_STATE_FILE` (default `data/workflow1-mm-lh-provided-state.json`)
- `WF1_STATUS_FILE` (default `data/workflow1-mm-lh-provided-status.json`, set `none` to disable)
- `WF1_BOOTSTRAP_SEND_EXISTING` (default `false`)
- `WF1_AT_ALL` (default `false`)
- `WF1_DRY_RUN` (default `false`)
- `WF1_DEBUG_LOG_SKIPS` (default `false`)
- `WF1_CONTINUOUS` (default `false`)
- `WF1_POLL_INTERVAL_SECONDS` (default `10`; project setting uses `1` for near-real-time polling)
- `WF1_FORCE_SEND_AFTER_SECONDS` (default `300`; project setting uses `60`)
- `WF1_MAX_READY_AGE_SECONDS` (default `300`; skip stale ready rows older than this window)
- `WF1_PROVIDE_TIME_MIN_AGE_SECONDS` (legacy compatibility flag; project setting keeps this at `0`)
- `WF1_GROUP_DEFER_SECONDS` (default `20`)
- `WF1_SEND_MIN_INTERVAL_MS` (default `1200`)
- `WF1_SEND_RETRY_MAX_ATTEMPTS` (default `5`)
- `WF1_SEND_RETRY_BASE_MS` (default `1000`)
- `WF1_SEND_RETRY_MAX_MS` (default `30000`)
- `WF1_SEATALK_BASE_URL` (default `https://openapi.seatalk.io`)
- `WF1_ENABLE_HEALTH_SERVER` (default `false`)
- `WF1_HEALTH_PORT` (default uses `PORT`, fallback `8080`)
- `WF1_SELF_PING_URL` (optional; set to your public `/healthz` URL)
- `WF1_SELF_PING_INTERVAL_SECONDS` (default `300`)

For fast trigger behavior when column `F` is formula-driven, run in watch mode:

```powershell
$env:WF1_CONTINUOUS = "true"
$env:WF1_POLL_INTERVAL_SECONDS = "1"
go run ./cmd/workflow-mm-lh-provided
```

Send rule:

- Normal send: when `F`, `H`, and `I` are available.
- If 2+ ready rows have the same `Provide Time` minute (`I`, seconds ignored), they are sent as one merged message.
- A ready row is held briefly (`WF1_GROUP_DEFER_SECONDS`) so rows arriving a few seconds apart can still merge.
- Stale-ready safeguard: rows that stay unsent longer than `WF1_MAX_READY_AGE_SECONDS` are skipped to avoid replaying old backlog when webhook recovers.
- Force send: if `F` is already filled but `H` or `I` is still missing for `WF1_FORCE_SEND_AFTER_SECONDS` (project setting: 60s), send once anyway.
- Forced-send message keeps missing `H/I` values blank.
- Special case: if `F` contains `DOUBLE` or `DOUBLE REQUEST`, send:
  `Double Request!` + `{C} - {M}` and force `@All` mention.
- Outbound sends are throttled/retried to handle SeaTalk rate limits (`429`, code `8`).

Status output:

- The workflow writes per-cycle status to `WF1_STATUS_FILE`.
- Includes `last_cycle_at`, `rows_read`, send/skip counts, `pending_force_send_count`, and `last_sent_row`.
- Check with:

```powershell
Get-Content .\data\workflow1-mm-lh-provided-status.json
```

## Workflow 2: OB Pending Dispatch Snapshot (Bot Group Chat)

`workflow_2_ob_pending_dispatch` reads Google Sheet:

- Spreadsheet: `17cvCc6ffMXNs6JYnpMYvDO_V8nBCRKRm3G78oINj_yo`
- Tab: `Backlogs Summary`
- Trigger cell: `G4`
- Snapshot range: `C2:S64`

Trigger behavior:

- Stores latest `G4` value in a local state file.
- Sends only when `G4` changes from the stored value.
- First run baselines `G4` (no send) unless `WF2_BOOTSTRAP_SEND_EXISTING=true`.

Send behavior:

- Uses SeaTalk bot API endpoint `POST /messaging/v2/group_chat` (not system account webhook).
- Sends text first:
  - `<mention-tag target="seatalk://user?id=0"/> OB Pending for Dispatch as of {local_time}`
  - Local time format: `3:04 PM Jan-02` in `WF2_TIMEZONE` (default `Asia/Manila`)
- Renders and sends one styled image by default: `Backlogs Summary!C2:S64`
- Waits for trigger + captured values to stabilize before sending (`WF2_STABILITY_RUNS` checks, `WF2_STABILITY_WAIT_SECONDS` between checks).
- Enforces SeaTalk image limit (`<= 5MB` Base64): PNG first, JPEG fallback if needed.

Required env for this workflow:

- `WF2_SEATALK_GROUP_ID`
- SeaTalk app credentials via either:
  - `WF2_SEATALK_APP_ID` + `WF2_SEATALK_APP_SECRET`
  - or fallback to `SEATALK_APP_ID` + `SEATALK_APP_SECRET`
- Google credentials via one of:
  - `WF2_GOOGLE_CREDENTIALS_FILE` (or `GOOGLE_APPLICATION_CREDENTIALS`)
  - `WF2_GOOGLE_CREDENTIALS_JSON`

Optional env:

- `WF2_STATE_FILE` (default `data/workflow2-ob-pending-dispatch-state.json`)
- `WF2_STATUS_FILE` (default `data/workflow2-ob-pending-dispatch-status.json`, set `none` to disable)
- `WF2_CONTINUOUS` (default `true`)
- `WF2_POLL_INTERVAL_SECONDS` (default `10`)
- `WF2_DRY_RUN` (default `false`)
- `WF2_BOOTSTRAP_SEND_EXISTING` (default `false`)
- `WF2_TIMEZONE` (default `Asia/Manila`)
- `WF2_IMAGE_MAX_WIDTH_PX` (default `3000`)
- `WF2_IMAGE_MAX_BASE64_BYTES` (default `5242880`)
- `WF2_RENDER_SCALE` (default `2`, range `1-4`)
- `WF2_STABILITY_RUNS` (default `3`, min `2`)
- `WF2_STABILITY_WAIT_SECONDS` (default `2`, min `1`)
- `WF2_ENABLE_HEALTH_SERVER` (default `true`)
- `WF2_HEALTH_PORT` (default uses `PORT`, fallback `8080`)

Run one-shot locally:

```powershell
$env:WF2_CONTINUOUS = "false"
go run ./cmd/workflow-ob-pending-dispatch
```

Run in watch mode locally:

```powershell
$env:WF2_CONTINUOUS = "true"
$env:WF2_POLL_INTERVAL_SECONDS = "5"
go run ./cmd/workflow-ob-pending-dispatch
```

## Workflow 2.1: Drive ZIP CSV Consolidation -> R2 -> Filtered Sheet Import

`workflow_2_1_drive_csv_consolidation` does:

1. Polls a Google Drive parent folder for new `.zip` files and processes pending uploads oldest -> newest.
2. Reads all `.csv` files from that zip and consolidates into one CSV.
3. Keeps only the first CSV header as canonical header and aligns subsequent rows by header name.
4. Detects/drops hidden leading unnamed column (default enabled).
5. Uploads consolidated CSV to Cloudflare R2.
6. Overwrites only destination columns `A:K` (keeps `L+` formulas untouched) and imports matched rows into three tabs in batches (lightweight for large datasets).
   - Pre-filter before tab routing: `Receiver Type == Station` and `Current Station == SOC 5`.
7. Imports destination tabs in order: `pending_rcv`, then `packed_in_another_to`, then `no_lhpacking`.
8. As soon as `pending_rcv` import is completed, writes a local timestamp to helper cell `config!B1` (same destination sheet by default), then continues remaining tab imports.
9. After import flow, waits briefly for recalculation, captures `[SOC] Backlogs Summary!B2:Q59` as styled image, then sends to SeaTalk group.

Defaults:

- Drive parent folder: `1oU9kj5VIJIoNrR388wYCHSdtHGanRrgZ`
- Destination sheet: `1mdi-8ACluDHGZ7yAyNLwXLwpmQ4f6VAx3kpbaJORViA`
- Destination tabs:
  - `pending_rcv` (when `Receive Status` contains `Pending Receive`)
  - `packed_in_another_to` (when `Remark` contains `Pack in another TO`)
  - `no_lhpacking` (when `Remark` contains `Receive in`)
- Output columns:
  - `TO Number`, `SPX Tracking Number`, `Receiver Name`, `TO Order Quantity`, `TO Number`, `Operator`, `Create Time`, `Complete Time`, `Remark`, `Receive Status`, `Staging Area ID`

Required env:

- Google credentials via one of:
  - `WF21_GOOGLE_CREDENTIALS_FILE` (or `GOOGLE_APPLICATION_CREDENTIALS`)
  - `WF21_GOOGLE_CREDENTIALS_JSON`
- `WF21_R2_ACCOUNT_ID`
- `WF21_R2_BUCKET`
- `WF21_R2_ACCESS_KEY_ID`
- `WF21_R2_SECRET_ACCESS_KEY`
- `WF21_SUMMARY_SEATALK_MODE` (`bot` or `webhook`) when `WF21_SUMMARY_SEND_ENABLED=true` (default)
- `WF21_SEATALK_GROUP_ID` + `WF21_SEATALK_APP_ID` / `WF21_SEATALK_APP_SECRET` when mode is `bot` (supports `WF2_*` and global `SEATALK_*` fallbacks)
- `WF21_SEATALK_WEBHOOK_URL` (or `SEATALK_SYSTEM_WEBHOOK_URL`) when mode is `webhook`

Optional env:

- `WF21_DRIVE_PARENT_FOLDER_ID`
- `WF21_DESTINATION_SHEET_ID`
- `WF21_DESTINATION_TAB_PENDING_RCV` (default `pending_rcv`)
- `WF21_DESTINATION_TAB_PACKED_IN_ANOTHER_TO` (default `packed_in_another_to`)
- `WF21_DESTINATION_TAB_NO_LHPACKING` (default `no_lhpacking`)
- `WF21_R2_OBJECT_PREFIX` (default `wf2-1`)
- `WF21_STATE_FILE`
- `WF21_STATUS_FILE` (set `none` to disable)
- `WF21_BOOTSTRAP_PROCESS_EXISTING` (default `true`)
- `WF21_DROP_LEADING_UNNAMED_COLUMN` (default `true`)
- `WF21_DRY_RUN` (default `false`)
- `WF21_CONTINUOUS` (default `true`)
- `WF21_ENABLE_HEALTH_SERVER` (default `true`)
- `WF21_HEALTH_PORT` (default uses `PORT`, fallback `8080`)
- `WF21_POLL_INTERVAL_SECONDS` (default `30`)
- `WF21_SHEETS_BATCH_SIZE` (default `7000`)
- `WF21_SHEETS_WRITE_RETRY_MAX_ATTEMPTS` (default `6`)
- `WF21_SHEETS_WRITE_RETRY_BASE_MS` (default `1000`)
- `WF21_SHEETS_WRITE_RETRY_MAX_MS` (default `15000`)
- `WF21_TEMP_DIR` (optional)
- `WF21_SUMMARY_SEND_ENABLED` (default `true`)
- `WF21_SUMMARY_SEATALK_MODE` (default `bot`)
- `WF21_SEATALK_GROUP_ID` (fallback to `WF2_SEATALK_GROUP_ID`)
- `WF21_SEATALK_APP_ID` / `WF21_SEATALK_APP_SECRET` (fallback to `WF2_SEATALK_APP_ID` / `WF2_SEATALK_APP_SECRET`, then `SEATALK_APP_ID` / `SEATALK_APP_SECRET`)
- `WF21_SEATALK_BASE_URL` (fallback to `WF2_SEATALK_BASE_URL`, then `SEATALK_BASE_URL`, default `https://openapi.seatalk.io`)
- `WF21_SEATALK_WEBHOOK_URL` (fallback to `SEATALK_SYSTEM_WEBHOOK_URL`)
- `WF21_SUMMARY_SHEET_ID` (default `WF21_DESTINATION_SHEET_ID`)
- `WF21_SUMMARY_TAB` (default `[SOC] Backlogs Summary`)
- `WF21_SUMMARY_RANGE` (default `B2:Q59`)
- `WF21_SUMMARY_SECOND_IMAGE_ENABLED` (default `true`)
- `WF21_SUMMARY_SECOND_TAB` (default `[SOC5] SOCPacked_Dashboard`)
- `WF21_SUMMARY_SECOND_RANGES` (default `A1:U9,B142:T167`; supports first token with tab prefix like `[SOC5] SOCPacked_Dashboard!A1:U9, B142:T167`)
- `WF21_SUMMARY_SYNC_CELL` (default `config!B1`; helper cell updated with local timestamp after import sync, before the summary wait/send flow)
- `WF21_SUMMARY_WAIT_SECONDS` (default `8`)
- `WF21_SUMMARY_STABILITY_RUNS` (default `3`)
- `WF21_SUMMARY_STABILITY_WAIT_SECONDS` (default `2`)
- `WF21_SUMMARY_RENDER_MODE` (default `styled`; `pdf_png` uses Google Sheets PDF export -> PNG conversion for closer visual fidelity)
- `WF21_SUMMARY_RENDER_SCALE` (default `2`)
- `WF21_SUMMARY_AUTO_FIT_COLUMNS` (default `false`; set `false` to preserve sheet layout, `true` to auto-resize columns for long text)
- `WF21_SUMMARY_PDF_DPI` (default `180`; used when `WF21_SUMMARY_RENDER_MODE=pdf_png`)
- `WF21_SUMMARY_PDF_CONVERTER` (default `auto`; `auto|pdftoppm|magick`, used when `WF21_SUMMARY_RENDER_MODE=pdf_png`)
- `WF21_SUMMARY_IMAGE_MAX_WIDTH_PX` (default `3000`)
- `WF21_SUMMARY_IMAGE_MAX_BASE64_BYTES` (default `5242880`)
- `WF21_SUMMARY_HTTP_TIMEOUT_SECONDS` (default `45`)
- `WF21_TIMEZONE` (default `Asia/Manila`; used for caption timestamp like `Outbound Pending for Dispatch as of ...`)

PDF mode dependency note:

- `WF21_SUMMARY_RENDER_MODE=pdf_png` requires either:
  - Poppler (`pdftoppm` in PATH), or
  - ImageMagick (`magick` in PATH)

Run one-shot locally:

```powershell
$env:WF21_CONTINUOUS = "false"
go run ./cmd/workflow-drive-csv-consolidation
```

## Render deployment (24/7)

Use the included `render.yaml` blueprint to deploy as a web service.

1. Push this repo to GitHub.
2. In Render, create a new Blueprint and select the repo.
3. Set secret env vars in Render:
   - `SEATALK_SYSTEM_WEBHOOK_URL`
   - `WF1_GOOGLE_CREDENTIALS_JSON` (entire service-account JSON string)
4. After first deploy, set:
   - `WF1_SELF_PING_URL=https://<your-render-service>.onrender.com/healthz`

### Render env reference file

- Generated file: `docs/render-env.md`
- Regenerate locally:

```powershell
go run ./scripts/generate_render_env_doc.go
```

- Auto-update locally when `.env`, `.env.example`, or `render.yaml` changes:

```powershell
powershell -ExecutionPolicy Bypass -File ./scripts/watch_render_env_doc.ps1
```

- Auto-update via Cloudflare Worker webhook:
  - Worker project: `cloudflare/render-env-sync`
  - Endpoint: `POST /github-webhook`
  - It regenerates and commits `docs/render-env.md` when relevant files change.
  - Setup guide: `cloudflare/render-env-sync/README.md`

The service exposes:

- `GET /healthz`
- `GET /status` (returns `WF1_STATUS_FILE` JSON)

## Security notes

- Keep all secrets/webhook URLs private.
- Never paste `.env` values, access keys, or credential JSON into tickets, chat, or deployment logs.
- Do not expose arbitrary shell execution; only run named workflows from `workflows.yaml`.
- Review every script called by workflow definitions.

