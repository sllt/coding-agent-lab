# ADR-005 Trial 与 Attempt 分离

状态：已接受（2026-10-04）

默认 `max_auto_infra_retries=0`。人工重试创建新 Attempt，不覆盖旧结果。重新讨论的条件：能证明尚未调用 Provider 的准备失败，才允许自动重试。
