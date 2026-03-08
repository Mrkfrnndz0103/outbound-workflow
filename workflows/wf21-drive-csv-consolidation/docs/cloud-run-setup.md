# WF21 Cloud Run Setup Guide (Plain English)

This guide moves `workflow-drive-csv-consolidation` (WF21) to Google Cloud Run so you can run `pdf_png` rendering with Poppler/ImageMagick, without needing a local domain.

This is written for beginners and uses simple steps.

## What You Will Build

You will deploy WF21 as a **Cloud Run Job** and trigger it using **Cloud Scheduler**.

Why this is recommended:
- A job starts, runs once, and exits.
- Scheduler triggers it on a schedule (for example every minute).
- Better fit for WF21 than keeping a web service always running.

## Before You Start

You need:
- A Google Cloud project
- Billing enabled (free tier may still apply, but billing must be enabled)
- Access to Cloud Shell or local `gcloud`
- Your current WF21 environment values (from `.env` / Render)

## Step 1: Add a Dockerfile to the Repo

Create this file in the repo root:

Path: `Dockerfile`

```dockerfile
# syntax=docker/dockerfile:1
FROM golang:1.22-bookworm AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/workflow ./workflows/wf21-drive-csv-consolidation/cmd

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates tzdata poppler-utils imagemagick \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=build /out/workflow /app/workflow
CMD ["/app/workflow"]
```

Commit and push this file.

## Step 2: Open Cloud Shell and Set Variables

Run:

```bash
export PROJECT_ID="your-project-id"
export REGION="asia-southeast1"
export REPO="wf21-repo"
export IMAGE="wf21"

gcloud config set project "$PROJECT_ID"
```

## Step 3: Enable Required APIs

```bash
gcloud services enable \
  run.googleapis.com \
  cloudbuild.googleapis.com \
  artifactregistry.googleapis.com \
  cloudscheduler.googleapis.com \
  secretmanager.googleapis.com \
  storage.googleapis.com
```

## Step 4: Build and Push Container Image (No Local Docker Needed)

In Cloud Shell:

```bash
cd ~
git clone https://github.com/<owner>/<repo>.git
cd <repo>

gcloud artifacts repositories create "$REPO" \
  --repository-format=docker \
  --location="$REGION" || true

export IMAGE_URI="$REGION-docker.pkg.dev/$PROJECT_ID/$REPO/$IMAGE:$(date +%Y%m%d-%H%M%S)"
gcloud builds submit --tag "$IMAGE_URI"

echo "$IMAGE_URI"
```

Save the printed `IMAGE_URI`. You will use it in Cloud Run.

## Step 5: Create a Bucket for WF21 State/Status Files

WF21 uses state files. Create a bucket so state survives between job runs.

```bash
export STATE_BUCKET="${PROJECT_ID}-wf21-state"
gcloud storage buckets create "gs://$STATE_BUCKET" --location="$REGION"
```

## Step 6: Create Runtime Service Account

```bash
export RUNTIME_SA="wf21-job-sa"
gcloud iam service-accounts create "$RUNTIME_SA" \
  --display-name="WF21 Job Runtime SA"
```

Grant permissions:

```bash
gcloud projects add-iam-policy-binding "$PROJECT_ID" \
  --member="serviceAccount:${RUNTIME_SA}@${PROJECT_ID}.iam.gserviceaccount.com" \
  --role="roles/secretmanager.secretAccessor"

gcloud storage buckets add-iam-policy-binding "gs://$STATE_BUCKET" \
  --member="serviceAccount:${RUNTIME_SA}@${PROJECT_ID}.iam.gserviceaccount.com" \
  --role="roles/storage.objectUser"
```

## Step 7: Add Secrets in Secret Manager

In Google Cloud Console:
- Go to **Security > Secret Manager**
- Create secrets for sensitive values, for example:
  - `wf21-google-creds-json`
  - `wf21-r2-access-key-id`
  - `wf21-r2-secret-access-key`
  - `wf21-seatalk-app-secret`

Add each secret value from your existing environment.

## Step 8: Deploy Cloud Run Job

In Cloud Console:
- Go to **Cloud Run > Jobs > Create Job**
- Job name: `wf21-job`
- Container image: use `IMAGE_URI` from Step 4
- Region: same as above
- Task count: `1`
- Service account: `wf21-job-sa@<project-id>.iam.gserviceaccount.com`
- Timeout: set enough time (for example 30 minutes)

Set environment variables:
- `WF21_CONTINUOUS=false`
- `WF21_SUMMARY_RENDER_MODE=pdf_png`
- `WF21_SUMMARY_PDF_CONVERTER=pdftoppm`
- `WF21_SUMMARY_PDF_DPI=180`
- `WF21_SUMMARY_IMAGE_MAX_BASE64_BYTES=5242880`
- `WF21_STATE_FILE=/mnt/state/workflow2-1-drive-csv-consolidation-state.json`
- `WF21_STATUS_FILE=/mnt/state/workflow2-1-drive-csv-consolidation-status.json`
- Plus your existing non-secret WF21 vars (`WF21_DESTINATION_*`, `WF21_DRIVE_PARENT_FOLDER_ID`, `WF21_R2_ACCOUNT_ID`, `WF21_R2_BUCKET`, etc.)

Attach secrets as env vars:
- `WF21_GOOGLE_CREDENTIALS_JSON` -> `wf21-google-creds-json`
- `WF21_R2_ACCESS_KEY_ID` -> `wf21-r2-access-key-id`
- `WF21_R2_SECRET_ACCESS_KEY` -> `wf21-r2-secret-access-key`
- `WF21_SEATALK_APP_SECRET` -> `wf21-seatalk-app-secret`

Add Cloud Storage volume mount:
- Volume type: Cloud Storage
- Bucket: `STATE_BUCKET`
- Mount path: `/mnt/state`

Deploy the job.

## Step 9: Test Job Manually

Run once:

```bash
gcloud run jobs execute wf21-job --region="$REGION" --wait
```

Check execution history:

```bash
gcloud run jobs executions list --job=wf21-job --region="$REGION"
```

Open logs in Cloud Console to verify:
- ZIP detection
- Import to destination tabs
- `config!B1` timestamp update after `pending_rcv` import
- Summary image send to SeaTalk

## Step 10: Schedule Automatic Runs

In Cloud Console:
- Go to **Cloud Run > Jobs > wf21-job > Triggers**
- Add a scheduler trigger
- Example cron: `* * * * *` (every minute)

Now the job runs automatically.

## Important Notes

- If there is no new ZIP, WF21 will not import.
- `pdf_png` works because Poppler/ImageMagick are inside the container image.
- You do not need a custom domain for this job+scheduler model.
- If you need a public HTTP endpoint, use Cloud Run Service instead (different setup).

## Quick Troubleshooting

If summary image fails:
- Check logs for missing converter errors.
- Confirm env:
  - `WF21_SUMMARY_RENDER_MODE=pdf_png`
  - `WF21_SUMMARY_PDF_CONVERTER=pdftoppm`

If state is not persisted:
- Confirm bucket mount exists.
- Confirm `WF21_STATE_FILE` and `WF21_STATUS_FILE` are under `/mnt/state`.

If secrets fail:
- Confirm secret names and versions.
- Confirm runtime SA has `Secret Manager Secret Accessor`.

