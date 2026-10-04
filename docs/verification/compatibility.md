# 兼容矩阵

空白和「未测」不是通过。本机这次只在 linux/amd64 上跑过下面写明的检查。

| 平台 | 执行器 | CLI | 认证 | 结果 |
|---|---|---|---|---|
| linux/amd64 | native-trusted | fixture 假 CLI | 无供应商登录 | 已测：正确/错误/空补丁、伪造 PASS、取消孙进程、重启不重跑 |
| linux/amd64 | Docker | 未运行 | 未测 | 未测。参数会拒绝 privileged、宿主网络和未钉死的镜像；本机没有 Docker daemon |
| linux/amd64 | native-trusted | Cursor `agent` | 未授权 | 未测。doctor 只做静态探测，不会发起模型调用 |
| linux/amd64 | native-trusted | Grok `grok` | 未授权 | 未测 |
| linux/amd64 | native-trusted | OpenCode `opencode` | 未授权 | 未测 |
| Intel Mac | native-trusted / Docker | 任一 | 未测 | 未测 |
| Windows | native-trusted | 任一 | 未测 | 未测 |

网络 `restricted` 没有强制执行，不能和 unrestricted 的结果放在同一张默认对比表里。

用量：假 CLI 不报告 token 或费用，界面保持未知，不会写成 0。真实供应商的 usage 未测。
