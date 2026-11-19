# PsychGrader 运维手册（2026-11-19）

## 1. 运行形态

- 单进程 Go 二进制 + 同目录 `static` 前端构建产物（Docker 从 `frontend/dist` 复制，或 `STATIC_DIR` 指定）。
- SQLite（WAL）文件默认 `./data/psychgrader.db`；同时产生 `-wal` / `-shm`。
- 默认监听 `127.0.0.1:8090`；Docker 内 `HOST=0.0.0.0`，compose 只把端口发布到宿主机 loopback。
- 生产必须放在 TLS 反代（Caddy / Nginx / Cloudflare Tunnel）之后；应用本身不做 TLS。

## 2. 环境变量

| 变量 | 默认 | 说明 |
|---|---|---|
| `HOST` | `127.0.0.1` | 监听地址；Dockerfile 设为 `0.0.0.0` |
| `PORT` | `8090` | 监听端口 |
| `DB_PATH` | `./data/psychgrader.db` | SQLite 路径；目录自动创建 |
| `STATIC_DIR` | `./static` 或 `../frontend/dist` | 前端构建产物目录 |
| `ADMIN_PASSCODE` | 空 | 空 = 关闭鉴权（仅本机）；共享部署必须设置 ≥16 字符随机值 |
| `APP_SECRET` | 空 | 空 = 每进程随机生成，重启后旧 token 失效；生产建议固定 ≥32 字节随机值 |
| `ALLOWED_ORIGINS` | `http://localhost:5173,http://127.0.0.1:5173` | 额外允许的 CORS origin，逗号分隔；生产填反代域名 |
| `TRUSTED_PROXIES` | 空 | 可信反代 CIDR 列表（逗号分隔）；为空时 `ClientIP` 用 `RemoteAddr`，反代后登录限流会按反代 IP 计数 |
| `ALLOWED_API_HOSTS` | `api.deepseek.com` | 额外允许的 DeepSeek API 域名 |
| `ALLOWED_API_BASE_URLS` | 空 | 额外允许的完整 base URL 前缀（适合代理） |
| `DEEPSEEK_API_KEY` | 空 | 首次启动/未在设置页填写时使用 |
| `DEEPSEEK_BASE_URL` | `https://api.deepseek.com` | 默认 API 地址 |
| `DEEPSEEK_MODEL` | `deepseek-flash` | 默认模型 |
| `GIN_MODE` | `release` | `debug` 可看到路由日志（会记录 URL，生产不建议） |

> 口令/密钥不要写进 git；`.env` 已在 `.gitignore` 与 `.dockerignore` 中。
>
> **重要**：`.env.example` 里的 `ADMIN_PASSCODE` / `APP_SECRET` 是占位值，部署前必须替换为随机值；如果 `.env` 曾经包含真实 DeepSeek Key，且该文件被共享、打包或上传过，请立即在 DeepSeek 控制台吊销并轮换。`backend/data/psychgrader.db*` 含学生 PII 与可能明文 API Key，不要随代码/压缩包分发。

## 3. 本地开发

```powershell
cd frontend
npm ci
npm run build
cd ..

# 读取 .env（若存在），绑定 127.0.0.1
.\\start-local.ps1

# 显式口令
.\\start-local.ps1 -Passcode "local-strong-passphrase"
```

- `frontend/dist` 被 `.gitignore` 忽略；新克隆必须构建，否则 `start-local.ps1` 会服务旧/缺失 UI。
- 修改前端后重新 `npm run build`；开发时可用 `npm run dev`（Vite `5173`，代理 `/api` 到 `8090`）。
- `start-local.ps1` 会停止已有 `psychgrader` 进程再启动；PID 会打印。
- 本机已有 Go 1.23.6：`D:/Desktop/Code/.toolchains/go/bin/go.exe`；可将其加入 PATH 后运行 `go test ./...`。没有 Go 的环境可用 Docker 构建。

## 4. Docker 部署

```bash
cp .env.example .env
# 必填：ADMIN_PASSCODE、APP_SECRET
# 可选：DEEPSEEK_API_KEY、ALLOWED_ORIGINS
docker compose up -d --build
docker compose logs -f psychgrader
```

compose 关键行为：

- `ports: 127.0.0.1:${HOST_PORT:-8090}:8090` → 公网不能直接访问 8090，必须经反代。
- `HOST=0.0.0.0` 只用于容器内；`cap_drop: ALL`、`no-new-privileges`。
- 容器用户 UID/GID **1000**；Linux 上如果 `./data` 由 root 创建，需要：

```bash
sudo mkdir -p data
sudo chown -R 1000:1000 data
```

否则 SQLite 写入会失败（`attempt to write a readonly database`）。

- Dockerfile 内置 `HEALTHCHECK` 请求 `http://127.0.0.1:8090/api/health`。
- 基础镜像/poppler 使用浮动 tag；生产建议固定 digest 并定期 `govulncheck` / `npm audit`。

### 反向代理

Caddy 示例：

```caddy
grader.example.com {
    encode zstd gzip
    reverse_proxy 127.0.0.1:8090
    header {
        Strict-Transport-Security "max-age=31536000; includeSubDomains"
        X-Content-Type-Options "nosniff"
        Referrer-Policy "strict-origin-when-cross-origin"
    }
}
```

- 不要打开 `HOST=0.0.0.0` + 公网端口；不要把 `9090`/`8090` 直接映射到 `0.0.0.0`。
- 如用 Cloudflare Tunnel，Access 可作为额外一层；应用内仍有口令/token。
- CORS 只影响浏览器跨域；反代同源部署时应用按 `scheme://Host` 判断同源。**反代必须保留原始 `Host` 并设置 `X-Forwarded-Proto`**（Caddy 默认满足；Nginx 默认会把 Host 改成上游地址，需要 `proxy_set_header Host $host; proxy_set_header X-Forwarded-Proto $scheme;`），否则同源写请求可能 403；也可以把生产域名加入 `ALLOWED_ORIGINS`。
- `APP_SECRET` 若仍是 `replace-with-a-random-secret` 或短于 32 字符，进程会拒绝启动；`start-local.ps1` 在本地会临时生成随机 secret（重启后 token 失效）。这是有意的 fail-closed 变更，升级前请替换旧 secret。

## 5. 数据与备份

SQLite 使用 WAL；**不要只复制 `.db`**，运行中可能缺 WAL 内容。

推荐备份：

```bash
# 容器内/宿主均可，需安装 sqlite3
sqlite3 data/psychgrader.db ".backup 'backup-$(date +%F).db'"
```

或停服后复制 `data/psychgrader.db`、`-wal`、`-shm` 三件套。

恢复：

1. 停服；
2. 恢复 `.db`（同时删除旧 `-wal/-shm` 或一并恢复）；
3. 启动；`db.Open` 会自动建缺失的任务表并执行启动对账。

迁移策略：当前只有 `CREATE TABLE IF NOT EXISTS`，没有版本化 `ALTER`。新增列前需补 `PRAGMA user_version` 迁移；不要直接手改生产库。

## 6. 安全清单（上线前）

- [ ] `ADMIN_PASSCODE` ≥16 字符随机，`APP_SECRET` ≥32 字节随机。
- [ ] `curl http://127.0.0.1:8090/api/auth/status` → `auth_required:true`；无 token 访问 `/api/problem-sets` 返回 401。
- [ ] 从外网/另一台机器确认 8090 不可直连；只开 443。
- [ ] `ALLOWED_API_HOSTS` 仅包含实际使用的 DeepSeek 域名；不要加内网地址。
- [ ] `./data` 权限 700、DB 600；容器 UID 1000 可写。
- [ ] 日志不包含 token；CSV 下载已改 Header，不再有 `?token=`。
- [ ] 定期备份并做一次恢复演练。
- [ ] 若 `.env`/DB 曾泄露，立即在 DeepSeek 控制台吊销 Key，并轮换口令/`APP_SECRET`。
- [ ] 生产禁用 `GIN_MODE=debug`（避免 URL/Query 日志）。

## 7. 故障排查

| 现象 | 检查 |
|---|---|
| 打开页面立即回登录页 | token 过期或 `APP_SECRET` 变化（重启后随机 secret）；重新登录 |
| 批改任务一直“已暂停” | 服务重启对账；在任务条点“继续批改”；检查 `grading_tasks.status` |
| 答卷一直“排队中” | 对应任务是否 paused/canceled；`grading_task_items` 是否 queued；恢复任务 |
| `attempt to write a readonly database` | `./data` 属主/权限；Docker UID 1000 |
| PDF 解析失败 | 文件 >25MB / >80 页 / 非 PDF / `pdftotext` 60s 超时；看后端日志 |
| 一键批改返回“没有待批改” | 全部已 `ai_graded/reviewed/needs_rereview`；`only_unscored=false` 才会重批 AI 已评 |
| 重跑返回 409 | 答卷 `reviewed`/`needs_rereview` 需 `force=true`（前端确认后自动带） |
| 设置测试连接 400 | 必须显式填 API Key；Base URL 不在白名单 |
| CSV 中文文件名乱码 | 已用 RFC 2231；若反代改写 Content-Disposition，检查代理配置 |
| 端口被占用 | `start-local.ps1` 会停止 `psychgrader`；其他进程用 `netstat -ano | findstr :8090` |

## 8. 日志与可观测性

- Gin `Logger` 输出请求行；生产日志应接入 journald/Docker logging driver。
- 任务失败原因写入 `grading_task_items.error_msg` 与 `submissions.error_msg`，UI 可见。
- 没有指标端点；后续可加 `/metrics`（任务数、失败率、LLM 延迟、token 用量）。
- 当前没有 token 用量记录；成本估算只能从上游账单看。

## 9. 升级流程

1. 备份 DB；
2. `git pull` / 构建新镜像；
3. 停旧进程/容器；
4. 启动新版本（自动建表 + 启动对账）；
5. 打开题库，确认 `active-task` 为 paused 的任务可继续；
6. 抽查一份答卷的评审、重跑、CSV；
7. 若回滚：恢复旧二进制 + 备份 DB（新表对旧版本无害，但不要依赖旧版本清理任务表）。
