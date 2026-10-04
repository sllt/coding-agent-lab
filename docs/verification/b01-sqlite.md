# B01 · 存储验证

日期：2026-10-04

- 驱动：`modernc.org/sqlite`（与 Pi 锁定依赖一致），不启用 `WithManagedSQL`。
- 每个新连接通过 DSN `_pragma` 设置 `foreign_keys=ON`、`journal_mode=WAL`、`synchronous=FULL`、`busy_timeout=5000`。`Open` 会取出两条连接核对，不符合则拒绝启动。
- 写事务由单一互斥锁串行化。CAS 更新要求恰好影响一行；同一事务里第二次 CAS 失败时，第一次修改一并回滚。
- 关闭数据库后重新打开，已提交的任务行仍在。已发布 `task_versions` 的 UPDATE 被触发器拒绝。
- 命令：`go test ./internal/store/sqlite`，通过。
