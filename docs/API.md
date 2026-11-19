# PsychGrader API 参考（2026-11-19）

Base URL：`http://<host>:8090/api`
除 `GET /health`、`GET /auth/status`、`POST /auth/login` 外，所有接口都需要：

```http
Authorization: Bearer <token>
```

> 所有写接口（POST/PUT/DELETE）必须发送 `Content-Type: application/json`；`POST /problem-sets/:id/parse-pdf` 例外，必须使用 `multipart/form-data`。跨站 `Origin` 会被拒绝（CSRF 防护）。

Token 由登录返回，HMAC 签名，默认 **7 天过期**；`APP_SECRET` 为空时每次进程重启都会重新生成 secret，旧 token 失效。所有接口均可能返回 `401 {"error":"需要口令登录","auth_required":true}`。

---

## 1. 健康检查与鉴权

### `GET /api/health`（公开）

```json
{ "ok": true }
```

用于 Docker healthcheck；实际路由在 `/api` group 下。

### `GET /auth/status`

```json
{ "auth_required": true, "authenticated": false }
```

### `POST /auth/login`

```json
{ "passcode": "teacher-passphrase" }
```

成功：`200 {"token":"<base64exp>.<hexmac>"}`
失败：`401 {"error":"访问口令不正确"}`；同一 IP 1 分钟失败 5 次后 `429`。

> 空 `ADMIN_PASSCODE` 表示关闭鉴权，仅建议本机使用；服务默认绑定 `127.0.0.1`。

---

## 2. 设置

### `GET /settings`

```json
{
  "api_base_url": "https://api.deepseek.com",
  "api_key_masked": "sk-a••••••••z9",
  "has_api_key": true,
  "model": "deepseek-flash",
  "thinking_enabled": true,
  "reasoning_effort": "high",
  "concurrency": 6,
  "auth_enabled": true
}
```

### `PUT /settings`

部分更新；字段均可选：

```json
{
  "api_base_url": "https://api.deepseek.com",
  "api_key": "sk-...",
  "clear_api_key": false,
  "model": "deepseek-flash",
  "thinking_enabled": true,
  "reasoning_effort": "high",
  "concurrency": 6
}
```

- `api_base_url` 必须通过白名单：`https` + `api.deepseek.com`，或 localhost，或 `ALLOWED_API_HOSTS` / `ALLOWED_API_BASE_URLS`。
- `concurrency` 1–16。
- 返回最新 `AppSettings`。

### `POST /settings/test`

**必须显式传 `api_key`**，不会回退已保存 Key，避免把 Key 发到任意 URL：

```json
{ "api_base_url": "https://api.deepseek.com", "api_key": "sk-...", "model": "deepseek-flash" }
```

成功：`200 {"ok":true,"reply":"OK","model":"deepseek-flash"}`
失败：`400 {"ok":false,"error":"..."}`

---

## 3. 题集

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/problem-sets` | 列表，含 `total_count/graded_count/reviewed_count/avg_score` |
| POST | `/problem-sets` | 创建；`title`、`prompt_text` 必填 |
| GET | `/problem-sets/:id` | 详情 |
| PUT | `/problem-sets/:id` | 更新；`title/prompt_text` 必填 |
| DELETE | `/problem-sets/:id` | 级联删除答卷与任务 |

`ProblemSet` 主要字段：

```json
{
  "id": 1,
  "title": "2026 CB Calibration · Spaced vs Massed (EBQ)",
  "question_type": "EBQ",
  "prompt_text": "...",
  "sources": [{"id":"Source 1","label":"Source 1","title":"...","summary":"..."}],
  "ineligible_concepts": ["mean","retrieval"],
  "scoring_notes": "..."
}
```

---

## 4. PDF 解析与导入

### `POST /problem-sets/:id/parse-pdf`（multipart）

字段：

| 字段 | 类型 | 说明 |
|---|---|---|
| `file` | file | PDF，≤ 25MB，≤ 80 页 |
| `mode` | string | `auto`（默认）或 `pages_per_student` |
| `pages_per_student` | int | `mode=pages_per_student` 时使用 |

响应：

```json
{
  "filename": "cb_batch.pdf",
  "page_count": 3,
  "pages": ["..."],
  "segments": [
    {
      "index": 1,
      "student_name": "Lucas Wang",
      "student_code": "99881122",
      "page_range": "P.1-2",
      "raw_text": "...",
      "word_count": 123
    }
  ]
}
```

`pdftotext` 60s 超时；纯 Go 回退；`SplitIntoSentences` 在评分时切句。

### `POST /problem-sets/:id/import`

```json
{
  "segments": [{ "index":1, "student_name":"...", "student_code":"...", "page_range":"P.1", "raw_text":"...", "word_count":0 }],
  "auto_grade": true
}
```

响应 `201`：

```json
{
  "imported_count": 2,
  "submissions": [ ... ],
  "task": { "id": 7, "status": "queued", "total": 2, "...": "..." },
  "task_error": "仅当答卷已导入但任务创建失败时出现"
}
```

- 单次最多 1000 段；JSON body ≤ 12MB。
- `auto_grade=true` 且创建任务成功时返回 `task`，前端应立即显示进度。

---

## 5. 答卷

### `GET /problem-sets/:id/submissions`

返回完整答卷列表（含 `raw_text` 与 `result`）。用于打开题集/任务结束后刷新。

### `GET /problem-sets/:id/submission-statuses`

轻量轮询用：返回 `id/problem_set_id/student_name/student_code/page_range/status/ai_score/final_score/teacher_notes/error_msg/created_at/updated_at`，无 `raw_text`、无 `result`。

### `GET /submissions/:id`

完整答卷。

### `POST /submissions/:id/grade`

单份批改，**同步**（最长约 150s）。

```json
{ "force": false }
```

- 答卷 `grading` 或 `queued`：`409 {"error":"该答卷正在批量批改中...","busy":true}`
- `reviewed`/`needs_rereview` 且 `force=false`：`409 {"reviewed":true}`
- `force=true`：新 AI 结果合并教师 `teacher_score/teacher_comment`，状态置 `needs_rereview`，`final_score` 保留。
- 成功：`200` 返回更新后的 `Submission`；失败 `502`。
- 若答卷已被删除：`404`（不会返回 `null`）。

### `PUT /submissions/:id/review`

```json
{
  "final_score": 6,
  "teacher_notes": "C(ii) 重新核定",
  "rubric_parts": [
    { "part_id":"part_a","max_score":1,"ai_score":1,"teacher_score":1,"reason_zh":"..." }
  ]
}
```

- `final_score` 必须在 `0..max_total_score`（默认 7）。
- 每个 part 的 `ai_score`/`teacher_score` 必须在 `0..max_score`。
- 保存成功：`200`，状态 `reviewed`；`result_json.rubric_parts` 更新。
- 保存是单事务；不会与批量写交叉覆盖（批量写 reviewed 会被拒绝）。

### `DELETE /submissions/:id`

删除答卷；任务项级联删除。

---

## 6. 批量任务

### `POST /problem-sets/:id/grade-all`

```json
{ "only_unscored": true }
```

- `only_unscored=true`（缺省值）：跳过 `ai_graded`、`grading`、`queued`、`reviewed`、`needs_rereview`。
- `only_unscored=false`：重新批改 `ai_graded`，仍跳过 `reviewed`、`needs_rereview`、`grading`、`queued`。
- 同一题集已有 `queued/running/paused` 任务：`200 {"task":{...},"reused":true,"queued":0,"skipped":0}`。
- 新建成功：`202 {"task":{...},"reused":false,"queued":N,"skipped":M}`。
- 没有可批改项：`200 {"task":null,"reused":false,"queued":0,"skipped":M}`。

`GradingTask`：

```json
{
  "id": 7,
  "problem_set_id": 1,
  "status": "running",
  "total": 18,
  "succeeded": 11,
  "failed": 1,
  "canceled": 0,
  "source": "grade-all",
  "error_summary": "",
  "current": [
    {"task_id":7,"submission_id":88,"student_name":"Alice","status":"running","started_at":"..."}
  ],
  "created_at": "2026-11-19 21:30:00",
  "started_at": "2026-11-19 21:30:01",
  "finished_at": ""
}
```

### `GET /tasks`

所有活跃任务（`queued/running/paused`），按 id 升序。

### `GET /tasks/:id`

单个任务；`current` 只包含 `running` 项。

### `GET /problem-sets/:id/active-task`

```json
{ "task": { ... } }
```

没有活跃任务时 `{"task":null}`。打开题集时用它恢复进度。

### `POST /tasks/:id/cancel`

- 只允许非终态任务；终态返回 `409`。
- 设置任务 `canceled`、未开始/运行中的 item `canceled`、对应 `queued/grading` 答卷 `canceled`。
- 已在飞行中的 LLM 请求可能已完成；已产生费用不退。

### `POST /tasks/:id/resume`

- 只允许 `paused` / `failed`。
- `failed` item 重新排队，`error/canceled` 答卷回到 `queued`；返回任务当前状态（通常 `queued`，worker 很快置 `running`）。

### 推荐轮询

- 任务：每 1.5s `GET /tasks/:id`；
- 队列：每 2 次任务轮询拉一次 `GET /problem-sets/:id/submissions`（完整结果）；
- 终态后停止轮询并刷一次题集统计。
- 终态后停止轮询并刷一次题集统计。
- 页面隐藏时建议暂停轮询（前端后续可加 `visibilitychange`），恢复时立即拉一次。

---

## 7. CED 搜索

### `GET /ced/search?q=interference`

```json
[
  { "term":"Retroactive Interference","unit":"Unit 2: Cognition","aliases":["retroactive"],"definition":"..." }
]
```

---

## 8. CSV 导出

### `GET /problem-sets/:id/export.csv`

必须带 `Authorization` header。**不再支持 `?token=`**。

响应：`text/csv; charset=utf-8`，UTF-8 BOM，`Content-Disposition` 使用 RFC 2231 中文文件名。

- 所有用户可控文本经过公式转义：首字符为 `= + - @ \t` 时前置 `'`；CR/LF 替换为空格。
- 列包含状态、分项、AI 分、最终分、引用来源、概念、AI 总结、教师备注。
- 教师改分优先写入分项列；未核定的 AI 分也会导出，需结合 `Status` 列判断。
- 前端用 `fetch + Blob` 下载，避免 token 进入 URL/日志。

---

## 9. 错误码约定

| 状态 | 场景 |
|---|---|
| 400 | JSON/表单参数错误、分数越界、Base URL 不在白名单、PDF 解析失败 |
| 401 | 缺失/过期 token，或口令错误 |
| 404 | 题集/答卷/任务不存在，或答卷在批改期间被删除 |
| 409 | 答卷正在批改、已核定但未 force、任务已结束、任务不满足 resume 条件 |
| 413 | PDF/JSON 超过大小上限 |
| 429 | 登录失败次数过多 |
| 500 | 数据库/SQLite 错误 |
| 502 | DeepSeek 请求失败（含上游 4xx/5xx、超时、JSON 校验失败） |

错误体统一为 `{"error":"中文可读信息"}`；部分错误附加 `auth_required` / `reviewed` / `busy`。
