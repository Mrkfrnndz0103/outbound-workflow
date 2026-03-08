# WF3 Render Status

WF3 is not a Render-managed workflow in this repo.

Use Railway instead:
- Railway guide: `./docs/wf3-fly-cloudflare-railway-setup.md`
- Config-as-code: `./workflows/wf3-mdt-updates/railway.toml`
- Workflow notes: `./workflows/wf3-mdt-updates/README.md`

Reason:
- WF3 depends on Dockerized PDF-to-PNG tooling and is standardized on Railway alongside `wf2.1`.
