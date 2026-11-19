# PsychGrader

AP Psychology **EBQ / AAQ** teacher-first grading studio built with **Go (Gin + pure-Go SQLite)** and **React 18 + Tailwind CSS v4 + Framer Motion**.

## Highlights

- **2026 College Board Official EBQ Rubric (7 pts = 1 + 1 + 2 + 1 + 2)**
  - Positive scoring with decoupled sub-parts (`B(ii)` / `C(ii)` can still score even if `B(i)` / `C(i)` misses a citation).
  - Deterministic post-LLM rule audit: catches cross-part **duplicate source** (`C(i)` = `B(i)`), **synonym concept collision** (e.g., *spacing effect* vs. *distributed practice* via built-in AP CED 2024–2026 canonical glossary), **ineligible prompt/source terms** (`retrieval`, `mean`, `random assignment`, `rehearsal`, `statistical significance`, etc.), and self-contradiction flags.
- **Persistent batch-grading tasks**
  - `grade-all` creates a real `grading_tasks` / `grading_task_items` job instead of a fire-and-forget goroutine.
  - The UI shows `N/M`, current students, success/failed/skipped counts, pause/resume/cancel, and survives page refresh or a server restart (interrupted tasks resume as paused).
  - Duplicate clicks reuse the same active task; each submission is claimed atomically so it is never graded twice concurrently.
- **Teacher review is protected**
  - A reviewed submission is never silently overwritten. Re-running requires confirmation, preserves teacher per-part scores/comments, and marks the paper `needs_rereview`.
  - Draft state is bound to a submission id; switching students/topics with unsaved overrides asks for confirmation.
- **Asymmetric Editorial 3-Column Layout**
  - `216px` collapsible student queue rail (`全部 / 待核 / AI 提示`)
  - Fluid dominant bilingual paper canvas: sentence-aligned **original English** + **literal unpolished Chinese translation**
  - `280px` sticky rubric deck with tactile segmented score pills (`0 | 1 | 2`) and keyboard-first workflow (`J/K`, `1–6`, `Alt+0/1/2`, `Enter` confirm & jump to next unreviewed, `G` grade, `?` shortcuts)
- **Batch PDF Split & Merge Workbench**
  - Upload merged AP Classroom / College Board PDF exports, auto-detect student headers (`Student:`, `AP ID`, page boundaries), preview & merge multi-page continuations before importing. PDF parsing has a 25MB/80-page/60s guard.
- **DeepSeek V4.1 Native Support**
  - Defaults to `https://api.deepseek.com` with `deepseek-flash` (`DeepSeek-V4.1-Flash-1017`) or `deepseek-v4-pro`, `thinking: { type: "enabled" }`, configurable `reasoning_effort`, JSON validation/retry, and offline EBQ heuristic fallback when no API key is configured (AAQ requires a key).

## Production Deployment (Docker + Domain)

```bash
cp .env.example .env
# Edit .env: set ADMIN_PASSCODE (>=16 chars) and APP_SECRET (>=32 chars).
# Optional: DEEPSEEK_API_KEY, ALLOWED_ORIGINS, ALLOWED_API_HOSTS.
docker compose up -d --build
```

The container listens on `0.0.0.0:8090` internally, but the compose port is published on **127.0.0.1 only** by default. Point Caddy / Nginx / Cloudflare Tunnel at `http://127.0.0.1:8090`; the proxy is the only public entry point. SQLite data persists in `./data/psychgrader.db`.

Security defaults:
- `HOST` defaults to `127.0.0.1` outside Docker; never expose an unauthenticated instance to a LAN.
- `ADMIN_PASSCODE` empty disables login (local only). Set it for any shared deployment.
- Auth tokens expire after 7 days; CSV export uses the `Authorization` header (no `?token=` in URLs/logs).
- `/api/settings/test` requires an explicitly entered API key and only accepts allow-listed API hosts (`api.deepseek.com`, localhost, or `ALLOWED_API_HOSTS`).
- Uploads are capped at 25MB; HTTP server timeouts and panic-safe batch workers are enabled.

## Local Development

```powershell
# First build the frontend (dist is intentionally not committed)
cd frontend
npm ci
npm run build
cd ..

# Start compiled single-binary server (serves frontend/dist on :8090)
.\\start-local.ps1

# Optional: explicit local passcode
.\\start-local.ps1 -Passcode "your-local-passphrase"
```

`start-local.ps1` loads simple `KEY=VALUE` pairs from `.env` when present, binds to `127.0.0.1`, and warns if `ADMIN_PASSCODE` is empty. Rebuild `frontend/dist` after frontend changes.

## Tests

```bash
cd frontend && npx tsc --noEmit
cd ../backend && go vet ./... && go test ./... && go build ./...
```

`backend/e2e_test.go` covers login, PDF split, single grading, reviewed-regrade protection, persistent batch progress, CED search and CSV export against a mock DeepSeek server.
