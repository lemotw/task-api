# `internal/logging` 測試提案

臨時 review 文件,不會留存,也不具規範效力。需求定義以 `docs/spec` 為準。

- **測試替身:** `Sink`:記錄內容、可設定失敗的假 Sink
- **執行:** `go test -race ./internal/logging`
- **RED 階段:** 空殼會 `panic` 並中止整個測試程式,所以要用 `-run` 逐一執行,確認每個測試都失敗。

**測試方式:** 注入固定的 `Now` 和 `Instance`,把假 Sink 收到的內容解析成 JSON 後比對。

## 成功

| 情境 | 預期 | 需求 |
|---|---|---|
| 輸出任一事件 | 只有一行合法 JSON,包含全部共用欄位,`v` 為 `1`、`service` 為 `task-api` | `LOG-1`、`LOG-5` |
| `time` 欄位 | UTC、RFC 3339、精確到毫秒 | `LOG-5` |
| 連續輸出 | `seq` 從 1 開始,每次加 1 | `LOG-5` |
| 混合被過濾和實際輸出的事件 | 被過濾的事件不消耗 `seq` | `LOG-5`、`LOG-9` |
| 混合 access 和 debug 事件 | `instance` 固定,`seq` 共用同一序列 | `LOG-5` |
| 事件帶自訂欄位 | 欄位位於 JSON 第一層 | `LOG-6` |
| `Access` | `log_type=access`、`event=http.request`,且包含 6 個專屬欄位 | `LOG-11` |
| 狀態碼 `200`、`404`、`500` | 層級分別為 `INFO`、`INFO`、`ERROR` | `LOG-12` |
| 四種層級方法 | `log_type=debug`,`level` 為全大寫 | `LOG-7`、`LOG-13` |
| `LOG_LEVEL` 過濾(4 × 4 的矩陣) | 只輸出大於或等於設定值的事件 | `LOG-9` |
| `LOG_LEVEL=ERROR` 時呼叫 `Access` | 仍會輸出 | `LOG-10` |
| `ParseLevel("debug")`、`("WARN")` | 不分大小寫都能解析 | `LOG-8` |
| 欄位含換行或引號 | 正確跳脫,仍然只有一行 | `LOG-1` |
| 併發輸出,搭配 `-race` | 每行都是合法 JSON,事件數完整,`seq` 不重複且依輸出順序遞增 | `LOG-4`、`LOG-5` |
| `WriterSink.Write` | 內容原樣寫入 writer | `LOG-2` |
| `WriterSink.Close` | 不關閉底層 writer | `LOG-2` |
| `JSONLogger.Close` | 關閉 Sink | `LOG-19` |

## 失敗

| 情境 | 預期 | 需求 |
|---|---|---|
| `ParseLevel("TRACE")`、`("")` | 回傳錯誤 | `LOG-8` |
| 自訂欄位撞到共用欄位名稱 | 保留共用欄位的值,丟棄撞名欄位 | `LOG-24` |
| Sink 寫入失敗 | 呼叫端不 panic,後續事件仍可輸出 | `LOG-18` |
| `path` 或 `error` 過長 | 不超過 1400 位元組,帶 `truncated: true`,共用欄位完整 | `LOG-17` |
| `path` 和 `error` 同時過長,內容有大量引號、反斜線、換行 | 以 JSON 編碼後的大小計算,不超過 1400 位元組 | `LOG-17` |
| 截斷點落在多位元組字元中間 | 結果仍是合法 UTF-8 和 JSON | `LOG-1`、`LOG-17` |
| `WriterSink` 的 writer 回傳錯誤 | `Write` 回傳該錯誤 | `LOG-3` |
| Sink 關閉失敗時呼叫 `JSONLogger.Close` | 回傳該錯誤 | `LOG-19` |

## 已定案

| 問題 | 決定 |
|---|---|
| 自訂欄位撞到共用欄位名稱 | 共用欄位優先,丟棄自訂欄位(`LOG-24`);這是非法呼叫的防禦行為 |
