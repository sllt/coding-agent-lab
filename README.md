# Coding Agent Lab

面向个人开发者的自托管 Coding Agent 实验与验收工作台。控制面使用 Go 与 [sllt/pi](https://github.com/sllt/pi)（锁定查阅提交 `4c292d04b898d95862ea74e17be7adb115a4ad54`），执行器是同一个可执行文件里的 `runner` 子命令。元数据在本地 SQLite，制品在本地目录。

产品说明见 `docs/01-产品需求文档.md`，技术边界见 `docs/02-技术设计文档.md`，工单见 `docs/03-实施任务包.md`。

## 运行

需要 Go 1.24 或更新版本（本仓库用 Go 1.27 验证）。前端构建另需 Node.js 22。

```bash
go test ./...
go run ./cmd/agentlab serve 127.0.0.1:43117
```

浏览器打开 <http://127.0.0.1:43117>。服务只绑定 loopback，第一次启动需要创建管理员。

`examples/` 里的 YAML 是草稿合同，含 `REPLACE_` 的字段不能发布。doctor 在未获授权时只做静态检查，不会发起模型调用。

## 验证记录

各工单的实际测试记录在 `docs/verification/`。兼容矩阵里的空白表示未测，不会填成已支持。
