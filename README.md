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
  - `new_bot_subscriber` (optional)

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

Default workflow list includes:

- `workflow_1_mm_lh_provided`
- `workflow_2_ob_pending_dispatch`

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

- `SEATALK_SYSTEM_WEBHOOK_URL`
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
- `WF1_POLL_INTERVAL_SECONDS` (default `10`)
- `WF1_FORCE_SEND_AFTER_SECONDS` (default `300`)
- `WF1_GROUP_DEFER_SECONDS` (default `20`)
- `WF1_SEND_MIN_INTERVAL_MS` (default `1200`)
- `WF1_SEND_RETRY_MAX_ATTEMPTS` (default `5`)
- `WF1_SEND_RETRY_BASE_MS` (default `1000`)
- `WF1_SEND_RETRY_MAX_MS` (default `30000`)
- `WF1_ENABLE_HEALTH_SERVER` (default `false`)
- `WF1_HEALTH_PORT` (default uses `PORT`, fallback `8080`)
- `WF1_SELF_PING_URL` (optional; set to your public `/healthz` URL)
- `WF1_SELF_PING_INTERVAL_SECONDS` (default `300`)

For fast trigger behavior when column `F` is formula-driven, run in watch mode:

```powershell
$env:WF1_CONTINUOUS = "true"
$env:WF1_POLL_INTERVAL_SECONDS = "5"
go run ./cmd/workflow-mm-lh-provided
```

Send rule:

- Normal send: when `F`, `H`, and `I` are available (with required `B`, `C`, `G`).
- If 2+ ready rows have the same `Provide Time` minute (`I`, seconds ignored), they are sent as one merged message.
- A ready row is held briefly (`WF1_GROUP_DEFER_SECONDS`) so rows arriving a few seconds apart can still merge.
- Force send: if `F` is already filled but `H/I` stay missing for 5 minutes, send once anyway.
- Special case: if `F` contains `DOUBLE` or `DOUBLE REQUEST`, send:
  `Double Request!` + `{C} - {M}` and force `@All` mention via `at_all=true`.
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
- Renders and sends two styled images by default:
  - `Backlogs Summary!C2:S64`
  - `SOLIIS & MINDANAO!B1:K41`
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
- `WF2_SECOND_RENDER_SCALE` (default `2`, range `1-4`)
- `WF2_SECOND_CAPTURE_TAB` (default `SOLIIS & MINDANAO`)
- `WF2_SECOND_CAPTURE_RANGE` (default `B1:K41`)
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

## Render deployment (24/7)

Use the included `render.yaml` blueprint to deploy as a web service.

1. Push this repo to GitHub.
2. In Render, create a new Blueprint and select the repo.
3. Set secret env vars in Render:
   - `SEATALK_SYSTEM_WEBHOOK_URL`
   - `WF1_GOOGLE_CREDENTIALS_JSON` (entire service-account JSON string)
4. After first deploy, set:
   - `WF1_SELF_PING_URL=https://<your-render-service>.onrender.com/healthz`

The service exposes:

- `GET /healthz`
- `GET /status` (returns `WF1_STATUS_FILE` JSON)

## Security notes

- Keep all secrets/webhook URLs private.
- Do not expose arbitrary shell execution; only run named workflows from `workflows.yaml`.
- Review every script called by workflow definitions.

