# WF3 Render Deployment Guide

This guide deploys `wf3` (`workflows/wf3-mdt-updates/cmd`) on Render as its own Docker web service.

WF3 does all of the following:
- watches a Google Sheet digest range
- waits for the sheet to stabilize
- exports configured ranges as PDF
- converts PDF pages to PNG
- sends the resulting images to a SeaTalk group
- stores local state/status files so restarts do not blindly replay work

## Recommended Render model

Use one dedicated Render web service:
- Service name: `go-bot-workflow3-mdt-updates`
- Runtime: Docker
- Dockerfile: `workflows/wf3-mdt-updates/Dockerfile.render`
- Healthcheck path: `/healthz`
- Disk mount path: `/var/data`
- Replica count: `1`

Why this is the right shape:
- WF3 depends on Poppler/ImageMagick in the container image.
- WF3 writes state/status files that should survive deploys.
- WF3 temporarily changes Google Sheet row visibility during image export, so it should not run multiple replicas.

## Render blueprint

WF3 is not included in the current checked-in `render.yaml`.

Recommended runtime defaults for the service:
- `WF3_PDF_CONVERTER=pdftoppm`
- `WF3_ENABLE_HEALTH_SERVER=true`
- `WF3_STATE_FILE=/var/data/workflow3-mdt-updates-state.json`
- `WF3_STATUS_FILE=/var/data/workflow3-mdt-updates-status.json`

Unmanaged values you must set in Render:
- `WF3_GOOGLE_CREDENTIALS_JSON`
- `WF3_SEATALK_APP_ID`
- `WF3_SEATALK_APP_SECRET`
- `WF3_SEATALK_GROUP_ID`

## Render UI setup

1. Create a dedicated Render web service for WF3 using `workflows/wf3-mdt-updates/Dockerfile.render`.
2. Open the WF3 service in Render Dashboard.
3. In `Environment`, set the unmanaged `WF3_*` secret values.
4. In `Disks`, confirm the persistent disk is mounted at `/var/data`.
5. In `Settings`, confirm:
   - Auto-Deploy: off
   - Health Check Path: `/healthz`
6. Deploy the service manually.

## First safe deploy

Before the first live send, temporarily set:

```env
WF3_DRY_RUN=true
WF3_TEST_SEND_ONCE=true
```

Deploy once and confirm:
- build succeeds from `workflows/wf3-mdt-updates/Dockerfile.render`
- `/healthz` is healthy
- Google Sheet export works
- PDF-to-PNG conversion works
- state/status files appear under `/var/data`

Then switch back to:

```env
WF3_DRY_RUN=false
WF3_TEST_SEND_ONCE=false
```

and deploy again.

## Operational notes

- Keep `WF3_PDF_CONVERTER=pdftoppm` unless you specifically need ImageMagick.
- Keep one instance only.
- If you change the disk mount path, update `WF3_STATE_FILE` and `WF3_STATUS_FILE` to match.
- WF3 remains separate from `workflows.yaml`, so enabling Render deployment does not wire it into the existing bot-runner workflow list.

## Railway

`workflows/wf3-mdt-updates/railway.toml` is still kept as a fallback path only.
Point the Railway service config path to `/workflows/wf3-mdt-updates/railway.toml`.
