# Coding Agent Lab

面向个人开发者的自托管 Coding Agent 实验与验收工作台。控制面使用 Go 与 [sllt/pi](https://github.com/sllt/pi)（锁定查阅提交 `4c292d04b898d95862ea74e17be7adb115a4ad54`），执行器是同一个可执行文件里的 `runner` 子命令。控制面每次运行都会拉起独立的 `agentlab runner` 进程，这个进程不打开控制数据库。元数据在本地 SQLite，制品在本地目录。

产品说明见 `docs/01-产品需求文档.md`，技术边界见 `docs/02-技术设计文档.md`，工单见 `docs/03-实施任务包.md`。

## 运行

需要 Go 1.24 或更新版本（本仓库用 Go 1.27 验证）。前端构建另需 Node.js 22。

```bash
go test ./...
go run ./cmd/agentlab serve 127.0.0.1:43117
```

开发界面另开一个终端：

```bash
cd web
npm install
npm run dev
```

开发服务器是 <http://127.0.0.1:43118>，把 `/api` 转到控制面。也可以先 `npm run build`，再打开 <http://127.0.0.1:43117>，由控制面提供 `web/dist`。两个地址都只在本机 loopback。第一次启动需要创建管理员，密码至少 8 位。

前端检查：`npm test`（vitest）、`npm run lint`、`npm run build`。CI 见 `.github/workflows/ci.yml`（gofmt / vet / `go test -race` / 生成客户端一致性 / 前端 lint+test+build / 内嵌构建）。

### 界面

左侧导航：**工作台**（运行概况）· **实验**（列表 / 新建 / 任务 × 配置矩阵）· **任务库**（项目、允许根、任务草稿与发布）· **Agents**（账号引用、配置卡片、doctor 就绪度、发布）· **对比**（按实验多选，可比表 + 跨任务汇总）· **系统**（策略、维护、安全边界、审计、Harbor、版本）。Trial 详情是三栏：左侧 Attempt 列表与冻结版本，中间事件流（实时追加）/ 改动 / 独立验收 / 制品，右侧结论、耗时、用量、运行环境（含 Landlock 沙箱）、人工意见与重试。

- 技术栈：React 19 + Vite + Tailwind 4，TanStack Query 管数据与轮询（有运行中的 Trial 时 2 秒，否则 15 秒），Radix 原语 + 自带的 shadcn 风格组件（`web/src/components/ui/`），lucide 图标，sonner 提示。
- 浅色 / 深色 / 跟随系统，设计令牌在 `web/src/index.css`（zinc + indigo，语义色 success/warning/danger/info）。
- 破坏性操作（取消、发布、保留清理、隔离区清理、采纳补丁、真实冒烟测试）都要二次确认；不可恢复的删除需要输入确认词。
- 生产构建不含内联脚本，符合控制面的 `script-src 'self'` CSP；较重的页面按路由懒加载。

### 单文件发布（内嵌前端）

```bash
scripts/build-web.sh ./agentlab   # npm build → 复制到 internal/webui/dist → go build -tags webembed
```

控制面按以下顺序找前端，先找到的生效：`$AGENTLAB_WEB_DIR` → `./web/dist` → `<可执行文件目录>/web/dist` → `<可执行文件目录>/dist` → 编译时内嵌的副本（仅 `-tags webembed`）。都没有时访问页面会返回 404 和构建提示，API 不受影响。

`examples/` 里的 YAML 是草稿合同，含 `REPLACE_` 的字段不能发布。doctor 在未获授权时只做静态检查，不会发起模型调用。

## 验证记录

各工单的实际测试记录在 `docs/verification/`。兼容矩阵里的空白表示未测，不会填成已支持。

## 真实 Agent（Cursor / Grok / OpenCode）

发布配置时会做一次静态探测，不调用模型：

| 级别 | 含义 | 能否发布/运行 |
| --- | --- | --- |
| `unavailable` | 找不到 CLI，或 `--version` 失败 | 否 |
| `cli_detected` | CLI 能跑，但帮助里没有无头参数（如 `-p`、`--output-format`） | 否 |
| `needs_credentials` | 无头参数齐全，但没有凭据来源 | 否 |
| `ready_unverified` | CLI、无头参数、凭据来源都在 | 是 |
| `verified` | 另外做过一次经授权的冒烟调用（`POST /profiles/{id}/doctor` 带 `allow_model_call:true`，会产生费用） | 是 |

凭据只按**环境变量名**配置：配置里的 `credential_env: ["CURSOR_API_KEY"]`，或账号的 `credential_ref: "env:XAI_API_KEY"`。值在启动时从控制面进程环境读取，交给 runner，绝不写进数据库或事件。`LD_*`、`PATH`、`HOME`、`AGENTLAB_*` 等变量拒绝转发。CLI 登录文件（如 `~/.cursor/cli-config.json`）只有在配置打开 `inherit_home` 时才能用，这会让 Agent 看到真实 HOME，每个 Attempt 都会记录 `home_inherited:true`。

Agent 非零退出、输出里有认证错误、且工作区没有任何改动时，结论记为 `inconclusive`（原因 `authentication_failed`），不会记成 Agent 未通过；这条信号来自 Agent 输出，所以不会自动封账号。

## 调度与时限

- 排队按 `enqueue_seq` 先进先出；重试回到队尾。
- 调度器不再等一整批结束：任何一个 Attempt 结束就立刻补位，某个账号满了也不会挡住其他账号的任务。
- Agent 墙钟按顺序取：任务 `limits.agent_wall_seconds` → 配置 `wall_seconds` → 设置 `agent_wall_seconds` → 环境变量 `AGENTLAB_AGENT_WALL_SECONDS` → 默认 1800 秒。
- 每个 Attempt 记录 `budget`（墙钟、日志、工作区上限）和 `budget_digest`；对比只把预算一致的结果放进同一张榜。

## 安全边界

- **会话与登录**：HttpOnly + SameSite=Strict cookie；CSRF 令牌用常量时间比较。同一用户名 15 分钟内失败 5 次锁定（1 分钟起，每次翻倍，最多 30 分钟），返回 429 和 `Retry-After`；未知用户名也走一次 bcrypt，不泄露用户是否存在。会话滑动续期最多每分钟写一次库。
- **CSP**：`script-src 'self'`，没有 `unsafe-inline` / `eval`；`connect-src 'self'`。只有样式允许内联（UI 库会写 style 属性，样式注入不能执行代码）。开发服务器（vite）为热更新放宽，生产由 Go 服务端下发严格策略。
- **Webhook**：签名是 `hex(HMAC-SHA256(secret, timestamp + "\n" + dedupe + "\n" + body))`，请求头 `X-Agentlab-Timestamp`（Unix 秒）、`X-Agentlab-Dedupe`、`X-Agentlab-Signature`。时间戳与服务器相差超过 5 分钟拒绝；去重键参与签名，不能挪到别的请求上重放。`notify_url` 只接受带端口的 `http://` 本机地址，投递时拨号器只连 loopback、不走代理、不跟随重定向。
- **错误码**：未知错误返回 500（详情只进服务日志，带 request_id）；`restricted` / `offline` 网络尚未强制执行，提交即 422，不会悄悄存成 unrestricted；并发备份 409，备份结束不会清掉运维手动打开的维护模式。
- **脱敏**：响应全部是 snake_case，JSON 文本列以嵌套对象返回。不返回凭据值、attempt token、`runtime.identity_token`、制品存储路径、服务器绝对路径（备份路径显示为 `$DATA/...`）。任务预检的原始验证器输出（可能引用隐藏测试）不入库、不返回。runner 会把转发的凭据变量值（≥8 字符）从 Agent 输出里替换成 `[REDACTED]`，事件接口再按常见密钥形状（`sk-…`、`ghp_…`、`Bearer …`、`*_API_KEY=…`）兜底一遍。
- **GET 不写库**：列表与详情只读；实验状态在每次 Trial 状态变化的同一事务里汇总，启动时修复。实验列表、详情、对比、导出都改为批量查询，不再按 Trial 逐条查库。
- **事件流（SSE）**：按字节偏移增量读取，只发送完整行，未写完的尾行留到下次；前端按序号追加，断线后浏览器带 `Last-Event-ID` 续传。

### 文件系统沙箱（Landlock，部分隔离）

Linux 5.13+ 且内核启用 Landlock 时，真实 Agent（cursor / grok / opencode）通过 `agentlab sandbox-exec` 启动：设置 `no_new_privs`，应用 Landlock 规则后再 `exec` CLI。

- **可读+执行**：`/usr` `/bin` `/sbin` `/lib*` `/etc` `/opt` `/proc` `/sys` `/run` `/nix` `/snap` `/var/lib`，`PATH` 里的目录，CLI 可执行文件所在目录及其安装前缀（例如 `…/lib/node_modules` 的上级）。包含数据目录或真实 HOME 的非系统目录会被剔除。
- **可写**：本次运行的工作区、私有 HOME、私有 TMPDIR、`/dev`；数据目录不在 `/tmp` 下时还有 `/tmp`；开启 `inherit_home` 时还有真实 HOME。
- **不可达**：控制面数据目录（数据库、其他 Attempt、备份、webhook 密钥）、未继承时的真实 HOME。
- **不做的事**：不限制网络、`/proc` 可见性、对同用户其他进程发信号、IPC、资源用量和一般系统调用。数据目录若放在上面的系统只读根之下会保持可读，运行记录的 `runtime.sandbox.exposed` 会列出来。这是防误操作的部分隔离，不是对抗恶意 Agent 的安全边界；需要强隔离请用虚拟机或容器。
- 内核不支持时照常运行，并在 `runtime.sandbox` 记 `mode=none, reason=landlock_unavailable`；`AGENTLAB_SANDBOX=off` 可手动关闭。请求了沙箱但 helper 应用失败时，Agent 不会启动（fail closed）。
