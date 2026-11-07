# 04. 日誌

規範用語見 [00-overview](../00-overview.md#1-規範用語)。本文件依賴 [02-api](02-api.md) 和 [03-architecture](03-architecture.md)。

## 1. 輸出

- **LOG-1**:每個日誌事件 MUST 是一個 UTF-8 JSON 物件,獨立佔一行。
- **LOG-2**:本版日誌 MUST 輸出到 stdout。`LOG_OUTPUT` 未設定或值為 `stdout` 時 MUST 正常啟動,其他值 MUST 拒絕啟動並以非 0 狀態碼結束。
- **LOG-3**:日誌事件的產生與編碼 MUST 和輸出目的地分離,呼叫端 MUST NOT 依賴特定目的地。
- **LOG-4**:日誌功能 MUST 可安全地併發使用,不同事件的內容 MUST NOT 交錯。

## 2. 共用欄位

- **LOG-5**:每個事件 MUST 包含下列欄位:

| 欄位 | 型別 | 說明 |
|---|---|---|
| `v` | 整數 | 格式版本,本版為 `1` |
| `log_type` | 字串 | `access` 或 `debug` |
| `time` | 字串 | UTC 時間,RFC 3339 格式,精確到毫秒 |
| `level` | 字串 | `DEBUG`、`INFO`、`WARN` 或 `ERROR` |
| `service` | 字串 | 固定為 `task-api` |
| `instance` | 字串 | 行程啟動時產生的識別碼,在該行程內固定不變 |
| `seq` | 整數 | 行程內從 `1` 開始,每次輸出遞增 `1` |
| `event` | 字串 | 穩定的事件名稱 |

- **LOG-6**:事件專屬欄位 MUST 放在 JSON 第一層,且 MUST NOT 使用共用欄位的名稱。
- **LOG-24**:呼叫端違反 `LOG-6` 時,日誌功能 MUST 保留共用欄位的值,並 MUST 丟棄撞名的事件專屬欄位。

## 3. 日誌類型與層級

`log_type` 表示日誌類別,`level` 表示嚴重程度,兩者彼此獨立。

- **LOG-7**:`level` MUST 依下表使用,且 MUST 以全大寫輸出:

| 層級 | 使用時機 |
|---|---|
| `DEBUG` | 開發與排查用的細節,正常運作時不需要 |
| `INFO` | 正常運作的事件 |
| `WARN` | 發生異常,但服務仍可繼續運作,且不需要立即處理 |
| `ERROR` | 操作失敗,或可能影響資料與服務,需要處理 |

- **LOG-8**:`LOG_LEVEL` MUST 接受 `DEBUG`、`INFO`、`WARN`、`ERROR`,不分大小寫,預設值 MUST 為 `INFO`。其他值 MUST 拒絕啟動並以非 0 狀態碼結束。
- **LOG-9**:`LOG_LEVEL` MUST 只過濾 `log_type` 為 `debug` 的事件,且 MUST 只輸出層級大於或等於設定值的事件。層級由低到高為 `DEBUG`、`INFO`、`WARN`、`ERROR`。
- **LOG-10**:`access` 事件 MUST 不受 `LOG_LEVEL` 影響。

## 4. Access log

- **LOG-11**:每個進入應用程式的 HTTP 請求,在回應完成後 MUST 輸出恰好一個 `http.request` 事件,其 `log_type` MUST 為 `access`,並 MUST 包含下列欄位:

| 欄位 | 型別 | 說明 |
|---|---|---|
| `method` | 字串 | HTTP 方法 |
| `route` | 字串 | 符合的路由樣板,例如 `/tasks/{id}`;沒有符合時為 `unmatched` |
| `path` | 字串 | 請求路徑,不含 query string |
| `status` | 整數 | 回應狀態碼 |
| `duration_ms` | 整數 | 處理時間,單位毫秒 |
| `bytes` | 整數 | 回應 body 的位元組數 |

- **LOG-12**:`status` 為 5xx 時,`level` MUST 為 `ERROR`,其餘 MUST 為 `INFO`。

```json
{"v":1,"log_type":"access","time":"2026-11-07T06:54:07.123Z","level":"INFO","service":"task-api","instance":"a1b2c3","seq":42,"event":"http.request","method":"PUT","route":"/tasks/{id}","path":"/tasks/1","status":200,"duration_ms":1,"bytes":38}
```

## 5. Debug log

- **LOG-13**:除 `http.request` 外,所有事件的 `log_type` MUST 為 `debug`。
- **LOG-14**:系統 MUST 在下列情況輸出對應事件:

| 事件 | 層級 | 額外欄位 | 時機 |
|---|---|---|---|
| `server.started` | `INFO` | `addr` | 開始監聽 |
| `server.stopped` | `INFO` | 無 | 關閉流程結束 |
| `server.start_failed` | `ERROR` | `error` | 啟動失敗(`ARC-19`) |
| `server.shutdown_timeout` | `WARN` | 無 | 等待進行中的請求逾時(`ARC-18`) |
| `http.internal_error` | `ERROR` | `route`、`path`、`error` | 回應 `500` 時 |

- **LOG-15**:開發者 MAY 新增其他 `debug` 事件。

```json
{"v":1,"log_type":"debug","time":"2026-11-07T06:54:07.456Z","level":"ERROR","service":"task-api","instance":"a1b2c3","seq":43,"event":"http.internal_error","route":"/tasks","path":"/tasks","error":"unexpected failure"}
```

## 6. 內容限制

- **LOG-16**:日誌 MUST NOT 包含請求或回應的 body、任何標頭,以及 query string。
- **LOG-17**:單一事件編碼後 MUST NOT 超過 1400 位元組。超過時 MUST 截斷 `path` 或 `error` 等可變長度的欄位,並 MUST 加上 `"truncated": true`。共用欄位 MUST NOT 被截斷。

## 7. 可靠性與生命週期

- **LOG-18**:日誌寫入失敗 MUST NOT 改變 API 回應或導致請求失敗。
- **LOG-19**:日誌功能 MUST 在其他啟動步驟之前完成初始化,並 MUST 在關閉流程的最後才停止,以便記錄啟動與關閉時的失敗。

## 8. 分層

- **LOG-20**:Service 和 Repository MUST NOT 輸出日誌,MUST 以回傳錯誤的方式向上傳遞失敗。

## 9. 相容性

- **LOG-21**:新增欄位或事件 MUST NOT 遞增 `v`。移除欄位、改變欄位型別或語意 MUST 遞增 `v`。
- **LOG-22**:日誌的讀取端 MUST 忽略不認識的欄位。

## 10. 測試

- **LOG-23**:單元測試 MUST 驗證共用欄位、`access` 事件、`LOG_LEVEL` 過濾規則、大小上限與截斷,以及 `LOG-16` 的內容限制。

## 未來擴充(僅供參考)

| 接法 | 做法 |
|---|---|
| Sidecar 讀取 stdout | 應用程式不用修改,依 `log_type` 分流 |
| 檔案 | 新增 `LOG_OUTPUT=file://...`,內容與 stdout 相同;輪替交給外部工具 |
| UDP | 新增 `LOG_OUTPUT=udp://...`,一個 datagram 放一個事件 |

網路輸出需要非同步寫入和有上限的佇列,才能符合 `LOG-18`。UDP 可能掉包,收集端可用 `instance` 和 `seq` 發現遺失,但無法補回。

分流範例:

```bash
docker logs task-api 2>&1 | jq -c 'select(.log_type == "access")'
```

實作注意:實作可以使用 `log/slog`,但輸出仍要符合本文件的欄位定義,例如以 `event` 取代預設的 `msg`。

## 決策與理由

| 決策 | 理由 |
|---|---|
| 日誌屬於核心 | 服務運作和排查都需要,不是可拔除的功能 |
| 兩種日誌共用 stdout,以 `log_type` 區分 | Docker 的 `tag` 無法逐行區分;欄位區分也適用於未來的輸出方式 |
| 本版只輸出 stdout | 交給執行環境落地,符合容器部署慣例 |
| Access log 不受 `LOG_LEVEL` 影響 | 調高層級時,請求紀錄不應該消失 |
| `4xx` 的 `level` 為 `INFO` | 這是用戶端的錯誤,不該觸發服務端告警 |
| 大小上限 1400 位元組 | 未來改用 UDP 時,能避免 IP 分片 |
| 事件專屬欄位放第一層 | 查詢與閱讀都比較簡單 |
