# 測試提案(封存)

這些是開發期間為各 package 撰寫的測試提案,用來在寫測試前 review 測試範圍。它們記錄當時的規劃,不會再更新,也不具規範效力。

- 需求以 [docs/spec](../../spec/00-overview.md) 為準。
- 實際的測試以各 package 的 `*_test.go` 為準。

| 檔案 | Package |
|---|---|
| [internal-logging.md](internal-logging.md) | `internal/logging` |
| [internal-task.md](internal-task.md) | `internal/task` |
| [internal-memory.md](internal-memory.md) | `internal/memory` |
| [internal-httpapi.md](internal-httpapi.md) | `internal/httpapi` |
| [internal-snapshot.md](internal-snapshot.md) | `internal/snapshot` |
| [internal-webui.md](internal-webui.md) | `internal/webui` |
| [cmd-server.md](cmd-server.md) | `cmd/server` |

`internal-webui.md` 寫於前端拆成 `/` 和 `/test` 兩個頁面之前,內容只涵蓋原本位於 `/` 的測試主控台。
