# `cmd/server` 測試提案

臨時 review 文件,不會留存,也不具規範效力。需求定義以 `docs/spec` 為準。

- **測試替身:** 全部用真實元件,黑箱測試
- **執行:** `go test -race ./cmd/server`
- **RED 階段:** 空殼會 `panic` 並中止整個測試程式,所以要用 `-run` 逐一執行,確認每個測試都失敗。

**測試方式:** `parseConfig` 用表格測試。`run` 以 `PORT=0` 啟動,從 stdout 的 `server.started` 取得位址,取消 `ctx` 來關閉。

## 成功

| 情境 | 預期 | 需求 |
|---|---|---|
| 未設定任何環境變數 | 連接埠 `8080`、`INFO`、`stdout`、無 `DATA_FILE`、`5s` | `ARC-10`、`LOG-2`、`LOG-8`、`PER-1` |
| 設定 `PORT`、`LOG_LEVEL=debug`、`DATA_FLUSH_INTERVAL=0` | 正確解析 | `ARC-10`、`LOG-8` |
| 啟動後呼叫 API,再關閉 | 結束碼 0,stdout 有 `server.started`、`http.request`、`server.stopped` | `ARC-18`、`LOG-14` |
| 未設定 `DATA_FILE` | 不產生任何資料檔 | `PER-1` |
| 設定 `DATA_FILE`,建立任務後重啟 | 任務還在,新 `id` 不重複 | `PER-9`、`PER-24` |
| `DATA_FLUSH_INTERVAL=0`,建立任務、正常關閉、重啟 | 任務還在 | `PER-20`、`PER-24` |
| 刪除最大 `id` 後關閉、重啟,再新增 | 新 `id` 不重複 | `PER-9` |
| 用只送一半 body 的請求觸發關閉,期限內補完 | 請求成功,變更存在於最後的快照 | `PER-20`、`PER-24` |
| 用子行程分別送 SIGINT、SIGTERM | 優雅關閉,結束碼 0 | `ARC-18` |
| 執行中修改快照檔 | API 回傳的資料不變 | `PER-26` |

## 失敗

| 情境 | 預期 | 需求 |
|---|---|---|
| `LOG_LEVEL=TRACE`、`LOG_OUTPUT=udp://x` | `parseConfig` 回傳錯誤,`run` 回傳非 0 | `LOG-2`、`LOG-8` |
| `DATA_FLUSH_INTERVAL=abc`、`-1s` | 回傳錯誤,`run` 回傳非 0 | `PER-2` |
| 連接埠已被佔用 | `run` 回傳非 0,有 `server.start_failed` | `ARC-19`、`LOG-14`、`LOG-19` |
| 只送一半 body 且不送完,導致等待逾時 | 有 `server.shutdown_timeout`,仍執行最後一次寫入 | `ARC-18`、`PER-21` |
| 同上,檢查結束碼 | `run` 回傳非 0 | `ARC-20` |
| 定期寫入失敗 | API 仍可繼續服務 | `PER-19` |
| `DATA_FILE` 損毀 | `run` 回傳非 0,檔案不變,不開始服務 | `PER-5`、`PER-8` |
| `DATA_FLUSH_INTERVAL=0`,建立任務確保有未保存變更,刪除快照目錄後關閉 | `run` 回傳非 0,有 `snapshot.flush_failed` | `PER-22` |

## Docker 驗收

| 操作 | 預期 | 需求 |
|---|---|---|
| `docker build` 和 `docker run` | API 可用,以非 root 執行 | `ARC-14`、`ARC-15` |
| 掛載 volume、建立任務、`docker stop`、重新啟動 | 任務還在 | `PER-24`、`PER-29` |
| `docker stop` | 10 秒內正常結束 | `PER-30` |
| 未設定 `DATA_FILE` 啟動映像 | 以純記憶體模式執行 | `PER-28` |
| 定期寫入完成後 `docker kill`,再重新啟動 | 還原到最後一次寫入的狀態 | `PER-25` |

## 已定案

| 問題 | 決定 |
|---|---|
| 關閉時等待逾時的結束碼與 `PER-24` | 結束碼非 0(`ARC-20`);`PER-24` 只保證未逾時的情況 |
## 測試上的取捨

- **請求卡住:** 不加測試專用 handler,用只送一半 body 的真實請求控制。5 秒的等待時間要做成內部可調,否則測試會很慢。
- **關閉順序:** 不為此拆介面。進行中請求的變更有進入快照,證明等待請求先於寫檔;`Save` 不併發由 `Syncer` 測試涵蓋。
