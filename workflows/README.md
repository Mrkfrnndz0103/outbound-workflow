# Workflow Isolation Layout

Each workflow now lives in its own folder under `workflows/` so code, tests, and workflow-specific docs stay isolated.

- `wf1-mm-lh-provided`
  - Entrypoint: `./workflows/wf1-mm-lh-provided/cmd`
- `wf2-ob-pending-dispatch`
  - Entrypoint: `./workflows/wf2-ob-pending-dispatch/cmd`
- `wf21-drive-csv-consolidation`
  - Entrypoint: `./workflows/wf21-drive-csv-consolidation/cmd`
  - Dockerfile: `workflows/wf21-drive-csv-consolidation/Dockerfile.render`
  - Docs: `workflows/wf21-drive-csv-consolidation/docs/`
  - Render guide: `docs/wf21-render-setup.md`
- `wf3-mdt-updates`
  - Entrypoint: `./workflows/wf3-mdt-updates/cmd`
  - Dockerfile: `workflows/wf3-mdt-updates/Dockerfile.render`
  - Docs: `workflows/wf3-mdt-updates/README.md`
