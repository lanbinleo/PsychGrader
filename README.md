# PsychGrader

AP Psychology **EBQ / AAQ** teacher-first grading studio built with **Go (Gin + pure-Go SQLite)** and **React 18 + Tailwind CSS v4 + Framer Motion**.

## Highlights

- **2026 College Board Official EBQ Rubric (7 pts = 1 + 1 + 2 + 1 + 2)**
  - Positive scoring with decoupled sub-parts (`B(ii)` / `C(ii)` can still score even if `B(i)` / `C(i)` misses a citation).
  - Deterministic post-LLM rule audit: catches cross-part **duplicate source** (`C(i)` = `B(i)`), **synonym concept collision** (e.g., *spacing effect* vs. *distributed practice* via built-in AP CED 2024–2026 canonical glossary), **ineligible prompt/source terms** (`retrieval`, `mean`, `random assignment`, `rehearsal`, `statistical significance`, etc.), and self-contradiction flags.
- **Asymmetric Editorial 3-Column Layout**
  - `252px` collapsible student queue rail (`全部 / 待核 / 警示`)
  - Fluid dominant bilingual paper canvas: sentence-aligned **original English** (with 5-category inline highlights: Claim, Evidence, Reasoning, AP Concept, Rule Issue) + **literal unpolished Chinese translation** (faithful to student ambiguities and rough phrasing)
  - `376px` sticky rubric deck with tactile segmented score pills (`0 | 1 | 2`) and keyboard-first workflow (`J/K`, `1–5`, `Alt+0/1/2`, `Enter` confirm & jump to next unreviewed, `G` grade, `I` import, `D` CED glossary, `?` shortcuts)
- **Batch PDF Split & Merge Workbench**
  - Upload merged AP Classroom / College Board PDF exports, auto-detect student headers (`Student:`, `AP ID`, page boundaries), preview & merge multi-page continuations before importing.
- **DeepSeek V4.1 Native Support**
  - Defaults to `https://api.deepseek.com` with `deepseek-flash` (`DeepSeek-V4.1-Flash-1017`) or `deepseek-v4-pro`, `thinking: { type: "enabled" }`, configurable `reasoning_effort`, and offline heuristic fallback when no API key is configured.

## Production Deployment (Docker + Domain)

```bash
cp .env.example .env
# Edit .env: set ADMIN_PASSCODE, APP_SECRET, DEEPSEEK_API_KEY
docker compose up -d --build
```

Point Caddy / Nginx / Cloudflare Tunnel at `http://127.0.0.1:8090`. SQLite database and settings persist in `./data/psychgrader.db`.

## Local Development

```powershell
# Start compiled single-binary server (serves frontend/dist on :8090)
.\start-local.ps1
```
