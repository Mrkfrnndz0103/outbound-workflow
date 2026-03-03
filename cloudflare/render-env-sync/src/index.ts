import YAML from "yaml";

interface Env {
  GITHUB_TOKEN: string;
  GITHUB_WEBHOOK_SECRET: string;
  GITHUB_OWNER: string;
  GITHUB_REPO: string;
  GITHUB_BRANCH: string;
}

type PushPayload = {
  ref?: string;
  after?: string;
  repository?: {
    full_name?: string;
    name?: string;
    owner?: { login?: string };
  };
  sender?: { login?: string };
  head_commit?: { message?: string };
  commits?: Array<{
    added?: string[];
    modified?: string[];
    removed?: string[];
  }>;
};

type Blueprint = {
  services?: RenderService[];
};

type RenderService = {
  name?: string;
  buildCommand?: string;
  startCommand?: string;
  dockerfilePath?: string;
  dockerContext?: string;
  envVars?: RenderEnvVar[];
};

type RenderEnvVar = {
  key?: string;
  value?: unknown;
  sync?: boolean;
};

type TreeEntry = {
  path: string;
  mode: string;
  type: "blob" | "tree";
  sha: string;
  size?: number;
  url: string;
};

const ENV_KEY_RE =
  /\b(?:WF\d+_[A-Z0-9_]+|SEATALK_[A-Z0-9_]+|GOOGLE_APPLICATION_CREDENTIALS|PORT)\b/g;
const CMD_DIR_RE = /\.\/cmd\/([a-zA-Z0-9\-_]+)/;
const DOCKER_DIR_RE = /(?:^|[/\\])cmd[/\\]([a-zA-Z0-9\-_]+)(?:[/\\]|$)/;
const PREFIX_KEY_RE = /^(WF\d+)_/;

const SHARED_KEY_SET = new Set([
  "SEATALK_SYSTEM_WEBHOOK_URL",
  "SEATALK_BASE_URL",
  "SEATALK_APP_ID",
  "SEATALK_APP_SECRET",
  "GOOGLE_APPLICATION_CREDENTIALS",
  "PORT",
]);

const MISSING_IGNORE = new Set(["GOOGLE_APPLICATION_CREDENTIALS", "PORT"]);

const RELEVANT_PATH_PREFIXES = [
  "render.yaml",
  "cmd/",
  "cloudflare/render-env-sync/",
];

const SKIP_TOKEN = "[skip-render-env-sync]";

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    const url = new URL(request.url);
    const path = normalizePath(url.pathname);
    if (path !== "/" && path !== "/github-webhook") {
      return new Response("Not Found", { status: 404 });
    }

    if (request.method === "GET") {
      return new Response("ok", { status: 200 });
    }
    if (request.method !== "POST") {
      return new Response("Method Not Allowed", { status: 405 });
    }

    const event = request.headers.get("x-github-event") ?? "";
    if (event !== "push") {
      return new Response("ignored: non-push event", { status: 200 });
    }

    const signature = request.headers.get("x-hub-signature-256") ?? "";
    const bodyRaw = await request.text();
    const valid = await verifyGitHubSignature(bodyRaw, signature, env.GITHUB_WEBHOOK_SECRET);
    if (!valid) {
      return new Response("invalid signature", { status: 401 });
    }

    let payload: PushPayload;
    try {
      payload = JSON.parse(bodyRaw) as PushPayload;
    } catch {
      return new Response("invalid JSON payload", { status: 400 });
    }

    const expectedRef = `refs/heads/${env.GITHUB_BRANCH}`;
    if (payload.ref !== expectedRef) {
      return new Response(`ignored: ref ${payload.ref ?? "(none)"}`, { status: 200 });
    }

    const repoOwner = payload.repository?.owner?.login ?? "";
    const repoName = payload.repository?.name ?? "";
    if (repoOwner !== env.GITHUB_OWNER || repoName !== env.GITHUB_REPO) {
      return new Response("ignored: repository mismatch", { status: 200 });
    }

    const headMsg = payload.head_commit?.message ?? "";
    if (headMsg.includes(SKIP_TOKEN)) {
      return new Response("ignored: skip token", { status: 200 });
    }

    if (!hasRelevantChange(payload)) {
      return new Response("ignored: no relevant file changes", { status: 200 });
    }

    if (!payload.after) {
      return new Response("ignored: missing commit sha", { status: 200 });
    }

    try {
      const result = await syncRenderEnvDoc(env, payload.after);
      return new Response(result, { status: 200 });
    } catch (err) {
      const message = err instanceof Error ? err.message : "unknown error";
      return new Response(`sync failed: ${message}`, { status: 500 });
    }
  },
};

function normalizePath(pathname: string): string {
  if (!pathname) {
    return "/";
  }
  if (pathname.length > 1 && pathname.endsWith("/")) {
    return pathname.slice(0, -1);
  }
  return pathname;
}

async function syncRenderEnvDoc(env: Env, commitSha: string): Promise<string> {
  const tree = await getRecursiveTree(env, commitSha);
  const treeMap = new Map<string, TreeEntry>();
  for (const entry of tree) {
    treeMap.set(entry.path, entry);
  }

  const renderYaml = await readBlobText(env, treeMap, "render.yaml");
  const blueprint = YAML.parse(renderYaml) as Blueprint;
  const services = blueprint.services ?? [];

  const markdown = await buildRenderEnvDoc(env, services, treeMap);

  const existing = await getRepoFile(env, "docs/render-env.md", env.GITHUB_BRANCH);
  if (existing?.content === markdown) {
    return "no update needed";
  }

  const message = `docs: update render env reference ${SKIP_TOKEN}`;
  await putRepoFile(
    env,
    "docs/render-env.md",
    markdown,
    message,
    env.GITHUB_BRANCH,
    existing?.sha,
  );

  return "updated docs/render-env.md";
}

async function buildRenderEnvDoc(
  env: Env,
  services: RenderService[],
  treeMap: Map<string, TreeEntry>,
): Promise<string> {
  const lines: string[] = [];
  lines.push("# Render Environment Variables");
  lines.push("");
  lines.push("Generated from `render.yaml` and workflow code env lookups.");
  lines.push("");
  lines.push("Regenerate with:");
  lines.push("");
  lines.push("```powershell");
  lines.push("go run ./scripts/generate_render_env_doc.go");
  lines.push("```");
  lines.push("");

  for (const svc of services) {
    const name = svc.name?.trim() || "(unnamed service)";
    const buildCommand = svc.buildCommand?.trim() || "";
    const dockerfilePath = svc.dockerfilePath?.trim() || "";
    const dockerContext = svc.dockerContext?.trim() || "";
    const cmdDir = inferCmdDir(buildCommand, dockerfilePath);

    const renderVars = (svc.envVars ?? [])
      .map((v) => ({
        key: (v.key ?? "").trim(),
        value: v.value,
        sync: v.sync,
      }))
      .filter((v) => v.key !== "");
    renderVars.sort((a, b) => a.key.localeCompare(b.key));

    lines.push(`## ${name}`);
    lines.push("");
    lines.push(`- Build command: \`${buildCommand}\``);
    if (dockerfilePath) {
      lines.push(`- Dockerfile: \`${dockerfilePath}\``);
    }
    if (dockerContext) {
      lines.push(`- Docker context: \`${dockerContext}\``);
    }
    if (cmdDir) {
      lines.push(`- Workflow source: \`${cmdDir}\``);
    }
    lines.push("");

    lines.push("### Render Vars (`render.yaml`)");
    lines.push("");
    lines.push("| Key | Management | Value |");
    lines.push("| --- | --- | --- |");
    for (const rv of renderVars) {
      lines.push(`| \`${rv.key}\` | ${renderMode(rv)} | ${renderValue(rv.value)} |`);
    }
    lines.push("");

    if (!cmdDir) {
      lines.push("### Code Scan");
      lines.push("");
      lines.push("Could not infer command directory from build command.");
      lines.push("");
      continue;
    }

    const codeKeys = await collectCodeEnvKeys(env, treeMap, cmdDir);
    const prefixes = detectServicePrefixes(renderVars.map((v) => v.key));
    const filtered = filterKeysForService(codeKeys, prefixes);

    const renderKeySet = new Set(renderVars.map((v) => v.key));
    const missing = filtered
      .filter((k) => !renderKeySet.has(k))
      .filter((k) => !MISSING_IGNORE.has(k))
      .sort((a, b) => a.localeCompare(b));

    lines.push("### Code Scan (Env Keys)");
    lines.push("");
    lines.push(`- Detected keys (prefix-filtered for this service): \`${filtered.join("`, `")}\``);
    if (missing.length === 0) {
      lines.push("- Missing from `render.yaml`: none");
    } else {
      lines.push("- Missing from `render.yaml`:");
      for (const key of missing) {
        lines.push(`  - \`${key}\``);
      }
    }
    lines.push("");
  }

  return `${lines.join("\n")}\n`;
}

function inferCmdDir(buildCommand: string, dockerfilePath: string): string {
  const match = buildCommand.match(CMD_DIR_RE);
  if (match && match[1]) {
    return `cmd/${match[1]}`;
  }
  const dockerMatch = dockerfilePath.match(DOCKER_DIR_RE);
  if (dockerMatch && dockerMatch[1]) {
    return `cmd/${dockerMatch[1]}`;
  }
  return "";
}

function renderMode(v: { value?: unknown; sync?: boolean }): string {
  if (v.sync === false) {
    return "`sync: false` (unmanaged/secret)";
  }
  if (v.value !== undefined) {
    return "`value` (managed)";
  }
  return "unset";
}

function renderValue(value: unknown): string {
  if (value === undefined) {
    return "-";
  }
  const s = String(value).trim();
  if (!s) {
    return "`\"\"`";
  }
  return `\`${s.replaceAll("`", "'")}\``;
}

async function collectCodeEnvKeys(
  env: Env,
  treeMap: Map<string, TreeEntry>,
  cmdDir: string,
): Promise<string[]> {
  const keys = new Set<string>();
  const paths = [...treeMap.keys()]
    .filter((p) => p.startsWith(`${cmdDir}/`))
    .filter((p) => p.endsWith(".go"))
    .filter((p) => !p.endsWith("_test.go"))
    .sort((a, b) => a.localeCompare(b));

  for (const path of paths) {
    const content = await readBlobText(env, treeMap, path);
    const matches = content.match(ENV_KEY_RE) ?? [];
    for (const key of matches) {
      keys.add(key);
    }
  }
  return [...keys].sort((a, b) => a.localeCompare(b));
}

function detectServicePrefixes(renderKeys: string[]): Set<string> {
  const set = new Set<string>();
  for (const key of renderKeys) {
    const m = key.match(PREFIX_KEY_RE);
    if (m && m[1]) {
      set.add(m[1]);
    }
  }
  return set;
}

function filterKeysForService(keys: string[], prefixes: Set<string>): string[] {
  const out = new Set<string>();
  for (const key of keys) {
    if (SHARED_KEY_SET.has(key)) {
      out.add(key);
      continue;
    }
    const m = key.match(PREFIX_KEY_RE);
    if (m && m[1] && prefixes.has(m[1])) {
      out.add(key);
    }
  }
  return [...out].sort((a, b) => a.localeCompare(b));
}

function hasRelevantChange(payload: PushPayload): boolean {
  const commits = payload.commits ?? [];
  for (const c of commits) {
    const files = [...(c.added ?? []), ...(c.modified ?? []), ...(c.removed ?? [])];
    for (const file of files) {
      if (RELEVANT_PATH_PREFIXES.some((p) => file === p || file.startsWith(p))) {
        return true;
      }
    }
  }
  return false;
}

async function verifyGitHubSignature(
  body: string,
  signatureHeader: string,
  secret: string,
): Promise<boolean> {
  if (!signatureHeader.startsWith("sha256=") || !secret) {
    return false;
  }
  const expectedHex = signatureHeader.slice("sha256=".length);
  const key = await crypto.subtle.importKey(
    "raw",
    new TextEncoder().encode(secret),
    { name: "HMAC", hash: "SHA-256" },
    false,
    ["sign"],
  );
  const digest = await crypto.subtle.sign("HMAC", key, new TextEncoder().encode(body));
  const actualHex = [...new Uint8Array(digest)].map((b) => b.toString(16).padStart(2, "0")).join("");
  return timingSafeEqual(actualHex, expectedHex);
}

function timingSafeEqual(a: string, b: string): boolean {
  if (a.length !== b.length) {
    return false;
  }
  let diff = 0;
  for (let i = 0; i < a.length; i++) {
    diff |= a.charCodeAt(i) ^ b.charCodeAt(i);
  }
  return diff === 0;
}

async function getRecursiveTree(env: Env, sha: string): Promise<TreeEntry[]> {
  const data = await ghJSON<{ tree: TreeEntry[] }>(
    env,
    `/repos/${env.GITHUB_OWNER}/${env.GITHUB_REPO}/git/trees/${sha}?recursive=1`,
  );
  return data.tree ?? [];
}

async function readBlobText(env: Env, treeMap: Map<string, TreeEntry>, path: string): Promise<string> {
  const entry = treeMap.get(path);
  if (!entry || entry.type !== "blob") {
    throw new Error(`missing blob: ${path}`);
  }
  const blob = await ghJSON<{ content: string; encoding: string }>(
    env,
    `/repos/${env.GITHUB_OWNER}/${env.GITHUB_REPO}/git/blobs/${entry.sha}`,
  );
  if (blob.encoding !== "base64") {
    throw new Error(`unexpected blob encoding for ${path}`);
  }
  const normalized = blob.content.replace(/\n/g, "");
  return new TextDecoder().decode(base64ToBytes(normalized));
}

async function getRepoFile(
  env: Env,
  path: string,
  ref: string,
): Promise<{ sha: string; content: string } | null> {
  const res = await ghFetch(
    env,
    `/repos/${env.GITHUB_OWNER}/${env.GITHUB_REPO}/contents/${encodePath(path)}?ref=${encodeURIComponent(ref)}`,
  );
  if (res.status === 404) {
    return null;
  }
  if (!res.ok) {
    throw new Error(`get contents ${path} failed: ${res.status} ${await res.text()}`);
  }
  const data = (await res.json()) as { sha: string; content: string; encoding: string };
  if (data.encoding !== "base64") {
    throw new Error(`unexpected contents encoding for ${path}`);
  }
  const normalized = (data.content ?? "").replace(/\n/g, "");
  return {
    sha: data.sha,
    content: new TextDecoder().decode(base64ToBytes(normalized)),
  };
}

async function putRepoFile(
  env: Env,
  path: string,
  content: string,
  message: string,
  branch: string,
  sha?: string,
): Promise<void> {
  const body: Record<string, unknown> = {
    message,
    content: bytesToBase64(new TextEncoder().encode(content)),
    branch,
    committer: {
      name: "render-env-sync-bot",
      email: "render-env-sync-bot@users.noreply.github.com",
    },
    author: {
      name: "render-env-sync-bot",
      email: "render-env-sync-bot@users.noreply.github.com",
    },
  };
  if (sha) {
    body.sha = sha;
  }

  const res = await ghFetch(
    env,
    `/repos/${env.GITHUB_OWNER}/${env.GITHUB_REPO}/contents/${encodePath(path)}`,
    {
      method: "PUT",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(body),
    },
  );
  if (!res.ok) {
    throw new Error(`put contents ${path} failed: ${res.status} ${await res.text()}`);
  }
}

function encodePath(path: string): string {
  return path
    .split("/")
    .map((seg) => encodeURIComponent(seg))
    .join("/");
}

async function ghJSON<T>(env: Env, path: string): Promise<T> {
  const res = await ghFetch(env, path);
  if (!res.ok) {
    throw new Error(`GitHub API ${path} failed: ${res.status} ${await res.text()}`);
  }
  return (await res.json()) as T;
}

function ghFetch(env: Env, path: string, init?: RequestInit): Promise<Response> {
  const headers = new Headers(init?.headers);
  headers.set("authorization", `Bearer ${env.GITHUB_TOKEN}`);
  headers.set("accept", "application/vnd.github+json");
  headers.set("user-agent", "cloudflare-render-env-sync");
  return fetch(`https://api.github.com${path}`, {
    ...init,
    headers,
  });
}

function base64ToBytes(base64: string): Uint8Array {
  const bin = atob(base64);
  const bytes = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) {
    bytes[i] = bin.charCodeAt(i);
  }
  return bytes;
}

function bytesToBase64(bytes: Uint8Array): string {
  let bin = "";
  for (let i = 0; i < bytes.length; i++) {
    bin += String.fromCharCode(bytes[i]);
  }
  return btoa(bin);
}
