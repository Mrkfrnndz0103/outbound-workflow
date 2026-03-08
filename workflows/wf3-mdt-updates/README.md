# Workflow 3: MDT Updates

`wf3-mdt-updates` is isolated from the existing workflows.

What it does:
- Watches `WF3_MONITOR_RANGE` for digest changes.
- Waits for the change to stay stable for `WF3_SEND_DEBOUNCE_SECONDS`.
- Exports each configured sheet slice through Google Sheets PDF export.
- Converts the PDF page to PNG and sends the images to a SeaTalk group.
- Temporarily hides fixed rows for image 1 before export, then restores the original row visibility.

Entrypoint:
- `go run ./workflows/wf3-mdt-updates/cmd`

Docker:
- Use `workflows/wf3-mdt-updates/Dockerfile.render` when running `WF3_PDF_CONVERTER=auto|pdftoppm|magick`.
- The image installs both Poppler (`pdftoppm`) and ImageMagick so PDF to PNG conversion is predictable in Linux deployments.

Railway:
- Config-as-code file: `workflows/wf3-mdt-updates/railway.toml`
- Keep the Railway service rooted at the repo root, not `workflows/wf3-mdt-updates/`, because the Docker build needs `go.mod`, `go.sum`, and `internal/`.
- In Railway service settings, point the config file path to `/workflows/wf3-mdt-updates/railway.toml`.
- Full UI runbook: `docs/wf3-fly-cloudflare-railway-setup.md`

Suggested runtime notes:
- Prefer Docker for hosted deployment. PDF export plus conversion is the unstable part across environments.
- Keep `WF3_PDF_CONVERTER=pdftoppm` in Docker unless you specifically need `magick`.
- Start with `WF3_TEST_SEND_ONCE=true` and `WF3_DRY_RUN=true` when validating a new sheet layout.
- Leave this workflow unregistered in `workflows.yaml` until you want the bot runner to invoke it explicitly.
