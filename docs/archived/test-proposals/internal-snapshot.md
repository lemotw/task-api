# `internal/snapshot` 測試提案

臨時 review 文件,不會留存,也不具規範效力。需求定義以 `docs/spec` 為準。

- **測試替身:** `File` 用真實暫存目錄;`Syncer` 的 `Source`、`Storage`、`Logger` 全用假物件
- **執行:** `go test -race ./internal/snapshot`
- **RED 階段:** 空殼會 `panic` 並中止整個測試程式,所以要用 `-run` 逐一執行,確認每個測試都失敗。

## `File`

**測試方式:** 在 `t.TempDir()` 中操作真實檔案。

**成功**

| 情境 | 預期 | 需求 |
|---|---|---|
| `Save` 後 `Load` | 內容相同,`found=true` | `PER-3` |
| 讀取手寫的合法快照 | 內容正確 | `PER-3` |
| `Save` 的輸出 | 符合固定的 `version`、`next_id`、`tasks` 格式 | `PER-3` |
| 刪除最大 `id` 後存取 | `NextID` 被保留 | `PER-9` |
| 檔案不存在時 `Load` | `found=false`,沒有錯誤 | `PER-6` |
| 連續 `Save` 兩次 | 內容為最新,目錄沒有暫存檔 | `PER-11` |
| 第二次 `Save` 後比對檔案識別 | 是新檔案,不是原地寫入 | `PER-11` |
| 檔案權限 | `0600` | `PER-16` |
| 目錄可寫時 `CheckWritable` | 回傳 `nil`,不留下檔案 | `PER-10` |

**失敗**

| 情境 | 預期 | 需求 |
|---|---|---|
| 空檔案、不合法 JSON | `errors.Is(err, ErrCorrupt)` | `PER-7` |
| 根節點不是物件、欄位型別錯誤、合法 JSON 後接第二份 JSON | `ErrCorrupt` | `PER-7` |
| 任務清單為空,`next_id` 為 0 或負數 | `ErrCorrupt` | `PER-7` |
| 缺少 `version`、`next_id` 或 `tasks` | `ErrCorrupt` | `PER-3`、`PER-7` |
| `version` 為 `2` | `ErrCorrupt` | `PER-7` |
| Task 的 `id` 為 0、`name` 空白或過長、`status` 為 2 | `ErrCorrupt` | `PER-7` |
| Task 的 `id`、`name`、`status` 為 `null` | `ErrCorrupt` | `PER-7` |
| `tasks` 為 `null` | `ErrCorrupt` | `PER-3`、`PER-7` |
| 重複的 `id`、`next_id` 不大於最大 `id` | `ErrCorrupt` | `PER-7` |
| 上述所有損毀情況 | 檔案內容逐位元組不變 | `PER-8` |
| `Save` 到不存在的目錄 | 回傳錯誤 | `PER-11` |
| 快照路徑是目錄、或沒有讀取權限時 `Load` | 回傳錯誤,但不是 `ErrCorrupt` | `PER-34` |
| 目錄唯讀時 `Save` | 回傳錯誤,舊檔案完整 | `PER-11` |
| 目標路徑是目錄,讓 rename 失敗 | 回傳錯誤,沒有留下暫存檔 | `PER-11`、`PER-13` |
| 目錄唯讀時 `CheckWritable` | 回傳錯誤 | `PER-10` |

權限相關測試在 root 下不會失敗,所以 root 執行時要 `t.Skip`。

## `Syncer`

**測試方式:** 使用假 `Source`、`Storage` 和 `Logger`。定期寫入放在 `synctest.Test` 內,以虛擬時間推進。

**成功**

| 情境 | 預期 | 需求 |
|---|---|---|
| `Load` 且有快照 | 呼叫 `Import` | `PER-5` |
| `Load` 且沒有快照 | 不呼叫 `Import`,沒有錯誤 | `PER-6` |
| `Load` 後沒有變更 | 不寫入 | `PER-17` |
| `Run` 期間有變更 | 下一個間隔寫入一次 | `PER-17` |
| `Run` 期間沒有變更 | 不寫入 | `PER-17` |
| 間隔為 0 | 不做定期寫入 | `PER-17` |
| 取消 `ctx` | 等進行中的寫入結束才返回 | `PER-20` |
| `Flush` 且有變更 | 寫入一次 | `PER-20` |
| `Flush` 且沒有變更 | 不寫入 | `PER-20` |
| 間隔為 0 時有變更,呼叫 `Flush` | 寫入一次 | `PER-20` |
| 寫入失敗後重試成功,再過一個間隔 | 不重複寫入 | `PER-17` |

**失敗**

| 情境 | 預期 | 需求 |
|---|---|---|
| `Load` 回傳 `ErrCorrupt` | 回傳錯誤、記錄 `snapshot.load_failed`(`ERROR`,含 `path`、`error`)、不呼叫 `Import`、不寫入 | `PER-8`、`PER-33` |
| `CheckWritable` 失敗 | `Load` 回傳錯誤 | `PER-10` |
| `Load` 回傳其他讀取錯誤 | 回傳錯誤、記錄 `snapshot.load_failed`、不呼叫 `Import` | `PER-34`、`PER-33` |
| 定期 `Save` 失敗 | 記錄 `snapshot.flush_failed`,層級為 `ERROR`,含 `path`、`error` | `PER-19`、`PER-33` |
| 定期 `Save` 失敗後沒有新修改 | 下一個間隔仍重試 | `PER-19` |
| 寫入期間又有新變更 | 下一個間隔再寫一次 | `PER-18` |
| `Save` A 阻塞時產生變更 B,取消 `Run`,等 A 完成後 `Flush` | B 被保存 | `PER-18`、`PER-20` |
| 寫入很慢時又到間隔 | 同時只有一個 `Save` | `PER-14` |
| `Save` 阻塞時,對真實 `memory.Repository` 呼叫 `Create` | `Create` 仍能完成 | `PER-15` |
| `Flush` 時 `Save` 失敗 | 回傳錯誤,記錄 `snapshot.flush_failed`,含 `path`、`error` | `PER-22`、`PER-33` |

## 已定案

| 問題 | 決定 |
|---|---|
| 快照檔無法讀取 | 回傳非 `ErrCorrupt` 的錯誤,拒絕啟動(`PER-34`) |
| Syncer 啟動後的基準 `revision` | `Import` 後呼叫一次 `Export` 記錄,介面不用改 |
## 介面調整(已完成)

- `Storage` 已新增 `Path() string`,讓 `Syncer` 能在日誌記錄 `path`(`PER-33`)。
- `File` 使用 `task.Validate` 檢查 Task 內容(`PER-7`)。

## 測試上的取捨

不另外加故障注入 hook。檔案識別測試擋下原地寫入,唯讀目錄和 rename 失敗測試涵蓋舊檔完整與暫存檔清理。寫入或 fsync 中途失敗沒有自動測試,靠實作流程保護。
