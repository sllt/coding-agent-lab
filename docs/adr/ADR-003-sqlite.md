# ADR-003 SQLite 与本地不可变制品

状态：已接受（2026-10-04）

元数据使用 modernc.org/sqlite。制品先写临时文件再 rename，然后提交引用。不在首版兼容 PostgreSQL。重新讨论的条件：本地持久化成为实测瓶颈，或需要共享控制面。
