# Render Env Sync Worker

Cloudflare Worker that listens to GitHub `push` webhooks and updates `docs/render-env.md`.

## What it does

1. Verifies GitHub webhook signature.
2. Filters to pushes on one branch (`GITHUB_BRANCH`).
3. Reads `render.yaml` + `cmd/**/*.go` from GitHub API.
4. Rebuilds `docs/render-env.md`.
5. Commits only when content changed.

## Setup

1. Install deps:

```bash
npm install
```

2. Set Worker secrets:

```bash
npx wrangler secret put GITHUB_TOKEN
npx wrangler secret put GITHUB_WEBHOOK_SECRET
```

3. Configure repo vars in `wrangler.toml`:
- `GITHUB_OWNER`
- `GITHUB_REPO`
- `GITHUB_BRANCH`

4. Deploy:

```bash
npx wrangler deploy
```

No separate build command is required. `wrangler deploy` compiles/bundles `src/index.ts` automatically using the `main` entry in `wrangler.toml`.

5. Add GitHub webhook:
- URL: `https://<worker-subdomain>/github-webhook` (or root URL if you bind at `/`)
- Content type: `application/json`
- Secret: same value as `GITHUB_WEBHOOK_SECRET`
- Event: `push`

## Token scopes

Use a fine-grained GitHub token with:
- Repository metadata: Read
- Repository contents: Read and write

## Loop protection

The Worker adds commit message token: `[skip-render-env-sync]`.
If a push includes this token, it is ignored.
