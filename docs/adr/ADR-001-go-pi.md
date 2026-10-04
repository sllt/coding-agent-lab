# ADR-001 Go/Pi 控制面与 Go Runner

状态：已接受（2026-10-04）

控制面使用 Go 与 sllt/pi，Runner 是同一模块的子命令。不并行维护 Rust 控制面，不把 CLI 放进 HTTP 处理协程。重新讨论的条件：出现独立的性能热点，或产品目标改成验证 Pipi。
