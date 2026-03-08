# WF3 Complete Railway Deployment Guide (Production + UI Runbook)

This guide deploys **WF3 only** (`workflows/wf3-mdt-updates/cmd`) on Railway as its own continuously running service.

WF3 does all of the following:
- watches a Google Sheet range for digest changes
- waits for the dashboard to stabilize
- exports Google Sheets ranges as PDF
- converts the PDF pages to PNG
- sends multiple images to a SeaTalk group
- stores local state/status files so it does not resend blindly after every restart

This guide is written for the current repo layout and the current Railway UI model.

## 1. Recommended Deployment Model

Use **one dedicated Railway service** for WF3.

Recommended name:
- `go-bot-workflow3-mdt-updates`

Recommended build/runtime:
- Builder: Dockerfile
- Config-as-code file: `/workflows/wf3-mdt-updates/railway.toml`
- Root directory: **repo root**
- Replica count: `1`
- Volume: **yes**

Why this is the correct setup:
- WF3 uses shared root-level files from this monorepo (`go.mod`, `go.sum`, `internal/`).
- WF3 writes local state/status files.
- WF3 temporarily changes row visibility on the target sheet while exporting images.
- WF3 should run as a **single worker**, not multiple concurrent replicas.

## 2. Why Repo Root Matters

Do **not** set the service root directory to `workflows/wf3-mdt-updates`.

Why:
- the Docker build uses root-level `go.mod`
- the Docker build uses shared code from `internal/`
- the Dockerfile path itself is outside a fully isolated package model

Use:
- Root directory: repository root
- Config-as-code path: `/workflows/wf3-mdt-updates/railway.toml`

That matches Railway's shared-monorepo model.

## 3. What You Need Before Opening Railway

Prepare these first:

### 3.1 Google access

You need a Google service account with access to the WF3 spreadsheet.

The service account must be able to:
- read the monitored range
- read/export the image ranges from Google Sheets
- read row metadata and write temporary hidden-row changes during export

If the service account cannot edit row visibility, WF3 image export can fail.

### 3.2 SeaTalk access

You need:
- `WF3_SEATALK_APP_ID`
- `WF3_SEATALK_APP_SECRET`
- `WF3_SEATALK_GROUP_ID`

### 3.3 Repo readiness

The branch you deploy must contain:
- `workflows/wf3-mdt-updates/cmd/main.go`
- `workflows/wf3-mdt-updates/Dockerfile.render`
- `workflows/wf3-mdt-updates/railway.toml`

## 4. Recommended Runtime Values

These are the repo-aligned starting values for Railway.

### 4.1 Non-secret values

```env
PORT=8080
WF3_ENABLE_HEALTH_SERVER=true
WF3_HEALTH_PORT=8080

WF3_SHEET_ID=1pLN46ZKWJIsidswMeoxhZwoacuFMR08sCaTFG6mLytc
WF3_SHEET_TAB=mdt
WF3_MONITOR_RANGE=G1:O227
WF3_IMAGE_RANGES=mdt!B1:P42,mdt!B44:P108,mdt!B109:P166,mdt!B167:P196,mdt!B198:P231
WF3_IMAGE1_FIXED_HIDE_ROWS=16-26,28,30,32-37,39-40

WF3_CONTINUOUS=true
WF3_BOOTSTRAP_SEND_EXISTING=false
WF3_DRY_RUN=true
WF3_TEST_SEND_ONCE=true
WF3_POLL_INTERVAL_SECONDS=3
WF3_SEND_DEBOUNCE_SECONDS=180
WF3_SEND_MIN_INTERVAL_SECONDS=1
WF3_STABILITY_RUNS=3
WF3_STABILITY_WAIT_SECONDS=2
WF3_HTTP_TIMEOUT_SECONDS=90
WF3_TIMEZONE=Asia/Manila

WF3_PDF_DPI=180
WF3_PDF_CONVERTER=pdftoppm
WF3_IMAGE_MAX_BASE64_BYTES=5242880
WF3_SEATALK_BASE_URL=https://openapi.seatalk.io

WF3_STATE_FILE=data/workflow3-mdt-updates-state.json
WF3_STATUS_FILE=data/workflow3-mdt-updates-status.json
WF3_TEMP_DIR=
```

### 4.2 Secret values

Set these as Railway service variables:
- `WF3_GOOGLE_CREDENTIALS_JSON`
- `WF3_SEATALK_APP_ID`
- `WF3_SEATALK_APP_SECRET`
- `WF3_SEATALK_GROUP_ID`

Notes:
- Railway can also use `WF3_GOOGLE_CREDENTIALS_FILE`, but for WF3 the JSON variable is simpler.
- Keep `WF3_SEATALK_BASE_URL` as a normal variable unless you intentionally use a different SeaTalk host.

## 5. Volume Recommendation

WF3 writes:
- `WF3_STATE_FILE`
- `WF3_STATUS_FILE`

By default these are **relative paths** under `data/...`.

Because the Docker image runs from `/app`, the simplest Railway setup is:
- attach a volume
- mount it to `/app/data`

That lets the existing relative defaults persist automatically.

Recommended mount:
- Volume mount path: `/app/data`

Then you can keep:
- `WF3_STATE_FILE=data/workflow3-mdt-updates-state.json`
- `WF3_STATUS_FILE=data/workflow3-mdt-updates-status.json`

Alternative:
- Mount to `/data`
- Set:
  - `WF3_STATE_FILE=/data/workflow3-mdt-updates-state.json`
  - `WF3_STATUS_FILE=/data/workflow3-mdt-updates-status.json`

The `/app/data` approach is simpler for this repo.

## 6. Railway UI Setup: Full Walkthrough

### Step 1: Create or open the Railway project

In Railway:
1. Open your workspace.
2. Create a new project or open the existing project that will hold WF3.

Recommended:
- keep WF3 as a separate service in the same project as your other automation services
- do not bundle WF3 into the WF2.1 service

### Step 2: Add a new service from GitHub

In the project canvas:
1. Click `New`.
2. Choose `GitHub Repo`.
3. Select this repository.
4. Select the branch you want to deploy.
5. Create the service.

### Step 3: Open the WF3 service settings

After the service exists:
1. Click the new service.
2. Open `Settings`.

### Step 4: Set the root directory correctly

In `Settings`:
- leave the root directory as repo root
- do **not** set it to `workflows/wf3-mdt-updates`

Reason:
- WF3 is a shared monorepo build, not an isolated package

### Step 5: Point Railway to the WF3 config-as-code file

In the service settings:
1. Find the config-as-code setting.
2. Set the file path to:

```text
/workflows/wf3-mdt-updates/railway.toml
```

This is important because the `railway.toml` file is not at repo root.

### Step 6: Confirm build mode

WF3 should build from Dockerfile.

The config file already specifies:
- builder = `DOCKERFILE`
- dockerfile path = `workflows/wf3-mdt-updates/Dockerfile.render`

So in the UI, you should not need to override build/start commands manually unless you are debugging.

### Step 7: Add variables in the Railway UI

Open the `Variables` tab for the WF3 service.

Add the recommended values from section 4.

Fastest options:
- use Railway's variable form
- or use the `RAW Editor` and paste the non-secret block

Then add secrets:
- `WF3_GOOGLE_CREDENTIALS_JSON`
- `WF3_SEATALK_APP_ID`
- `WF3_SEATALK_APP_SECRET`
- `WF3_SEATALK_GROUP_ID`

Important Railway behavior:
- variable changes become staged changes
- staged changes must be reviewed/deployed before they affect the running service

### Step 8: Create and attach the volume

In Railway:
1. Open the project canvas.
2. Create a volume.
3. Attach it to the WF3 service.
4. Set mount path to:

```text
/app/data
```

Recommended volume name:
- `wf3-state`

Keep replica count at `1`.

### Step 9: Configure the healthcheck

WF3 exposes `/healthz` when:
- `WF3_ENABLE_HEALTH_SERVER=true`

Recommended values:
- `PORT=8080`
- `WF3_HEALTH_PORT=8080`

Set Railway healthcheck path to:

```text
/healthz
```

The current `railway.toml` already sets:
- `healthcheckPath = "/healthz"`
- `healthcheckTimeout = 120`

### Step 10: Public networking

WF3 does not need a public user-facing site.

Recommended:
- keep it as a service with the normal Railway domain
- do not add custom domains unless you need them

Having the default Railway domain is still useful if you want to manually inspect:
- `/healthz`
- `/status`

### Step 11: First deployment

For the first deploy, keep these values:

```env
WF3_DRY_RUN=true
WF3_TEST_SEND_ONCE=true
```

Why:
- `WF3_DRY_RUN=true` prevents live SeaTalk sends
- `WF3_TEST_SEND_ONCE=true` lets the workflow exercise the send path logic once without waiting for a real dashboard change cycle

Then trigger the deploy.

## 7. What Success Looks Like

A healthy first deployment should:
- build successfully from the WF3 Dockerfile
- start the container
- pass `/healthz`
- read the Google Sheet
- export PDF snapshots
- convert PDF to PNG
- write state/status files under the attached volume
- log dry-run behavior without sending to SeaTalk

## 8. After the First Safe Deploy

Once the dry-run deployment looks good:

1. Set:

```env
WF3_DRY_RUN=false
WF3_TEST_SEND_ONCE=false
```

2. Deploy the staged changes.
3. Wait for the real monitored range to change.
4. Confirm WF3 sends the expected images to the target SeaTalk group.

## 9. Recommended Operational Settings

Keep these settings unless you have a reason to change them:
- `WF3_PDF_CONVERTER=pdftoppm`
- `WF3_PDF_DPI=180`
- `WF3_CONTINUOUS=true`
- `WF3_POLL_INTERVAL_SECONDS=3`
- `WF3_SEND_DEBOUNCE_SECONDS=180`
- `WF3_STABILITY_RUNS=3`
- `WF3_STABILITY_WAIT_SECONDS=2`

Why `pdftoppm`:
- the Docker image already installs it
- it is the safest choice for PDF-to-PNG conversion in this workflow

## 10. Important Behavior Notes

### 10.1 Single replica only

WF3 should stay at one running instance.

Reasons:
- it writes state locally
- it mounts one volume
- it temporarily changes Google Sheet row visibility during export

Running multiple replicas would create race conditions.

### 10.2 Volume-backed services can have brief deploy downtime

Railway healthchecks help deployment safety, but attached volumes still mean there can be a brief switchover downtime during redeploy.

That is expected for this class of service.

### 10.3 Google permissions matter more than the code here

WF3 needs the service account to:
- read the sheet
- export the ranges
- update row hidden state during image preparation

If the service account is read-only, image export can fail even though plain sheet reads succeed.

## 11. Troubleshooting

### Build succeeds but deploy fails healthcheck

Check:
- `PORT=8080`
- `WF3_ENABLE_HEALTH_SERVER=true`
- `WF3_HEALTH_PORT=8080`
- Railway healthcheck path is `/healthz`

### WF3 starts but state does not persist

Check:
- volume is attached to the WF3 service
- mount path is `/app/data`
- `WF3_STATE_FILE` and `WF3_STATUS_FILE` still point to `data/...`

### PDF export or conversion fails

Check:
- `WF3_PDF_CONVERTER=pdftoppm`
- service is building from `workflows/wf3-mdt-updates/Dockerfile.render`
- Google service account has access to the sheet

### SeaTalk send fails

Check:
- `WF3_SEATALK_APP_ID`
- `WF3_SEATALK_APP_SECRET`
- `WF3_SEATALK_GROUP_ID`
- target bot is a member of the target group

### Nothing sends in production

Check:
- `WF3_DRY_RUN=false`
- `WF3_TEST_SEND_ONCE=false`
- monitored range is actually changing
- debounce window (`WF3_SEND_DEBOUNCE_SECONDS`) is not longer than expected for your test

## 12. Optional CLI Helpers

If you want to sync only WF3 variables from the repo template:

```powershell
powershell -ExecutionPolicy Bypass -File ./scripts/sync_railway_env.ps1 -EnvFile .env.example -Service go-bot-workflow3-mdt-updates -Prefix WF3_
```

Then add secrets manually in the Railway UI or with the Railway CLI.

## 13. Short Version

If you want the short checklist:

1. Create a new Railway service from this repo.
2. Keep root directory at repo root.
3. Set config-as-code path to `/workflows/wf3-mdt-updates/railway.toml`.
4. Add a volume mounted at `/app/data`.
5. Set `PORT=8080`, `WF3_ENABLE_HEALTH_SERVER=true`, `WF3_HEALTH_PORT=8080`.
6. Add all `WF3_*` variables.
7. Start with `WF3_DRY_RUN=true` and `WF3_TEST_SEND_ONCE=true`.
8. Deploy and verify.
9. Turn dry-run/test-send off and deploy again.

## Sources

Official Railway docs used for this guide:
- Config as code: https://docs.railway.com/config-as-code
- Config as code reference: https://docs.railway.com/reference/config-as-code
- Monorepo deployments: https://docs.railway.com/deployments/monorepo
- Dockerfiles: https://docs.railway.com/builds/dockerfiles
- Variables: https://docs.railway.com/variables
- Volumes guide: https://docs.railway.com/volumes
- Volumes reference: https://docs.railway.com/volumes/reference
- Healthchecks: https://docs.railway.com/deployments/healthchecks
