# 10. 擴展:Memory <-> File 同步

規範用語見 [00-overview](../00-overview.md#1-規範用語)。本擴展依賴 [01-domain](../core/01-domain.md)、[03-architecture](../core/03-architecture.md) 和 [04-log](../core/04-log.md)。

## 1. 概念

記憶體仍是唯一資料來源(`ARC-6`),快照檔只是備份。同步只發生在以下時間點:

| 時間點 | 方向 |
|---|---|
| 啟動 | file → memory |
| 定期(有變更時) | memory → file |
| 優雅關閉 | memory → file |

## 2. 設定

| 環境變數 | 預設值 | 說明 |
|---|---|---|
| `DATA_FILE` | 未設定 | 快照檔路徑 |
| `DATA_FLUSH_INTERVAL` | `5s` | 定期寫入間隔,使用 Go duration 格式;`0` 表示停用 |

- **PER-1**:未設定 `DATA_FILE` 時,系統 MUST 以純記憶體模式執行,且 MUST NOT 讀寫任何資料檔案。
- **PER-2**:`DATA_FLUSH_INTERVAL` 不合法或為負值時,行程 MUST 拒絕啟動,並以非 0 狀態碼結束。

## 3. 快照格式

```json
{
  "version": 1,
  "next_id": 4,
  "tasks": [
    {"id": 1, "name": "buy milk", "status": 0}
  ]
}
```

- **PER-3**:快照 MUST 包含 `version`、`next_id` 和 `tasks`。
- **PER-4**:快照 MUST 是某一時間點的一致狀態,`next_id` 和 `tasks` MUST 來自同一個臨界區。

## 4. 啟動載入

- **PER-5**:系統 MUST 在開始接受 HTTP 請求之前完成載入。
- **PER-6**:快照檔不存在時,系統 MUST 視為首次啟動,以空狀態開始。
- **PER-34**:除了檔案不存在以外,其他讀取失敗(例如權限不足、路徑是目錄)MUST 拒絕啟動、以非 0 狀態碼結束並記錄原因。
- **PER-7**:檔案存在但不符合下列任一條件時,系統 MUST 視為損毀:
  - 是合法 JSON,且符合 `PER-3`(空檔案視為損毀)
  - `version` 是此版本支援的值
  - 每個 Task 符合 `DOM-2`、`DOM-4`、`DOM-5`、`DOM-8`
  - `id` 沒有重複
  - `next_id` 大於所有 `id`,且為正整數
- **PER-8**:快照檔損毀時,行程 MUST 拒絕啟動、以非 0 狀態碼結束並記錄原因,且 MUST NOT 修改該檔案。
- **PER-9**:重啟後 MUST 繼續符合 `DOM-3`,包含任務被刪除後重啟的情況。
- **PER-10**:行程 SHOULD 在啟動時確認快照檔目錄可寫入,不可寫入時 SHOULD 拒絕啟動。

## 5. 寫入

- **PER-11**:寫入 MUST 是原子操作:在同一目錄建立暫存檔、寫入、fsync,再 rename 到目標路徑。任何失敗情況下,舊快照檔 MUST 保持完整。
- **PER-12**:rename 後 SHOULD 對所在目錄執行 fsync。
- **PER-13**:寫入失敗時 SHOULD 刪除暫存檔。
- **PER-14**:同一時間 MUST 只有一個寫入在進行。
- **PER-15**:寫入檔案時 MUST NOT 持有 Repository 的鎖。API 請求 MUST NOT 因檔案 I/O 而被阻塞。
- **PER-16**:快照檔權限 SHOULD 為 `0600`。

### 定期寫入

- **PER-17**:間隔大於 0 時,系統 MUST 依間隔檢查,且 MUST 只在上次成功寫入後有變更時才寫入。
- **PER-18**:系統 MUST 只在寫入成功後,才將該快照包含的變更視為已保存。寫入期間發生的新變更 MUST 留到下一次寫入。
- **PER-19**:定期寫入失敗時,系統 MUST 記錄錯誤、繼續提供服務,並 MUST 在下一個間隔重試。

## 6. 優雅關閉

- **PER-20**:關閉順序 MUST 為:
  1. 停止接受新連線,等待進行中的請求(`ARC-18`)
  2. 停止定期寫入,等待進行中的寫入結束
  3. 若有未保存的變更,執行最後一次寫入
  4. 結束行程
- **PER-21**:即使步驟 1 逾時,系統 MUST 仍執行步驟 2 和 3。
- **PER-22**:最後一次寫入失敗時,系統 MUST 記錄錯誤並以非 0 狀態碼結束。

## 7. 持久性保證

- **PER-23**:API 的成功回應 MUST NOT 被解讀為資料已寫入檔案。
- **PER-24**:優雅關閉且等待請求未逾時的情況下,所有已回應成功的變更 MUST 存在於快照檔。逾時時不提供此保證。
- **PER-25**:非優雅結束(SIGKILL、OOM、當機)後重啟,系統 MUST 還原到最後一次成功寫入的狀態。遺失範圍僅限該次寫入之後的變更。

## 8. 外部修改與多實例

- **PER-26**:系統 MUST NOT 在執行期間監看或重新載入快照檔。外部修改只有在重啟前未被覆寫時才會生效。
- **PER-27**:多個實例 MUST NOT 使用同一個快照檔。

## 9. Docker

- **PER-28**:映像檔 MUST 預設不設定 `DATA_FILE`,以符合 `PER-1`。
- **PER-29**:映像檔 MUST 提供 `/data` 目錄,且非 root 執行使用者 MUST 可寫入。
- **PER-30**:預設的 `docker stop`(10 秒寬限期)SHOULD 足以完成 `PER-20` 全部步驟。

```bash
docker run --rm -p 8080:8080 -v task-data:/data -e DATA_FILE=/data/tasks.json task-api
```

## 10. 測試

- **PER-31**:測試 MUST 涵蓋:存取往返、刪除最大 `id` 後重啟不重複使用、檔案不存在、每種損毀情況不覆寫原檔、寫入失敗時舊檔完整、未變更時不寫入。
- **PER-32**:測試 MUST 使用各自的暫存目錄。

## 11. 日誌事件

- **PER-33**:下列事件 MUST 符合 `04-log`,且 `log_type` MUST 為 `debug`:

| 事件 | 層級 | 額外欄位 | 對應需求 |
|---|---|---|---|
| `snapshot.load_failed` | `ERROR` | `path`、`error` | `PER-8`、`PER-34` |
| `snapshot.flush_failed` | `ERROR` | `path`、`error` | `PER-19`、`PER-22` |

## 決策與理由

| 決策 | 理由 |
|---|---|
| 除關閉外加上定期寫入 | 只在關閉時寫入的話,當機會遺失本次啟動後的所有修改 |
| 損毀時拒絕啟動 | 當成空資料會在下次寫入時覆蓋舊檔,資料無法救回 |
| 保存 `next_id` | 只靠現有任務推算的話,刪除最大 `id` 後重啟會重複使用 |
| 不監看外部修改 | 複雜度高,且與記憶體是唯一資料來源的前提衝突 |
| 未設定 `DATA_FILE` 就是純記憶體 | 符合 PDF,測試也不碰檔案系統 |
