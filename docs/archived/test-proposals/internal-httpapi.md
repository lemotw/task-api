# `internal/httpapi` 測試提案

臨時 review 文件,不會留存,也不具規範效力。需求定義以 `docs/spec` 為準。

- **測試替身:** 真實 Service 和 memory;只有 `500` 用假 Service;記錄用的假 Logger
- **執行:** `go test -race ./internal/httpapi`
- **RED 階段:** 空殼會 `panic` 並中止整個測試程式,所以要用 `-run` 逐一執行,確認每個測試都失敗。

**測試方式:** 用 `httptest` 接上真實 Service 和 memory,驗證完整的 HTTP 契約。每個錯誤案例都檢查狀態碼、`Content-Type` 和錯誤格式。表格測試參數化,在 `Deps.UI` 有設定和為 nil 時各跑一次,涵蓋 `UI-7`。

## 成功

| 請求 | 預期 | 需求 |
|---|---|---|
| `GET /tasks`(無資料) | `200`,body 為 `[]` | `API-10` |
| `GET /tasks`(多筆) | `200`,依 `id` 排序 | `API-10` |
| `POST /tasks` 合法內容 | `201`、Task、`Location: /tasks/1` | `API-11` |
| `POST` 帶 `Content-Type: text/plain` | 仍然成功 | `API-2` |
| `POST`、`PUT` 的 body 剛好 1 MiB,用空白補足 | 分別回 `201`、`200` | `API-6` |
| 合法 JSON 後接空白或換行 | 成功 | `API-5` |
| `{"NAME":"a","STATUS":0}` | `201`,欄位名稱不分大小寫 | `API-5` |
| `PUT /tasks/1` 合法內容 | `200`、更新後的 Task | `API-12` |
| `DELETE /tasks/1` | `204`、沒有 body | `API-13` |
| 建立、更新、列表、刪除的完整流程 | 每步狀態正確 | `API-10` 到 `API-13` |
| 任一請求 | 剛好一筆 access 事件,`status`、`route` 正確,`bytes` 等於實際 body 大小 | `LOG-11` |
| `DELETE` 成功 | access 事件的 `bytes` 為 0 | `LOG-11` |
| 未知路徑 | access 事件的 `route` 為 `unmatched` | `LOG-11` |
| 請求 body、回應 body、標頭各放不同識別字串 | 都不出現在日誌 | `LOG-16` |
| 真實 Logger 搭配會失敗的 Sink | API 仍正常回應 | `LOG-18` |
| `PUT /tasks/1?x=secret` | `route=/tasks/{id}`、`path=/tasks/1`,日誌沒有 `secret` | `LOG-11`、`LOG-16` |
| `Deps.UI` 有設定時 `GET /` | 由 UI handler 處理 | `UI-1` |
| `Deps.UI` 有設定時 `GET /nope` | 仍回 `404`,不被首頁接走 | `API-15`、`UI-7` |

## 失敗

**Body(`POST` 和 `PUT` 都要測)**

| Body | 預期 | 需求 |
|---|---|---|
| 空的、`{`、`[]`、`"x"` | `400` | `API-5` |
| `{"name":"a","status":0}{}`、`{"name":"a","status":0} x` | `400` | `API-5` |
| 缺少 `name` 或 `status` | `400` | `API-5`、`DOM-9` |
| `{"name":null,"status":0}`、`{"name":"a","status":null}` | `400` | `API-5` |
| `{"name":"a","status":0,"id":9}`、多餘欄位 `foo` | `400` | `API-5` |
| `"status":"1"`、`0.5`、`1e0`、`1.0` | `400` | `API-5` |
| `"name":1` | `400` | `API-5` |
| `"status":2`、`"name":"  "`、256 字 `name` | `400` | `DOM-4`、`DOM-5`、`DOM-8` |
| 1 MiB + 1 位元組 | `413` | `API-6` |
| 未知長度(chunked)且超過 1 MiB | `413` | `API-6` |
| 第一個字元就不合法且超過 1 MiB(`POST`、`PUT`,已知與未知長度) | `413`,大小先於格式檢查 | `API-6`、`API-7` |

**路由與 id**

| 請求 | 預期 | 需求 |
|---|---|---|
| `PUT /tasks/abc`、`0`、`01`、`+1`、`-1`、超大數字 | `400` | `API-3` |
| `PUT`、`DELETE` 不存在的 `id` | `404`,不會建立任務 | `API-12`、`API-13` |
| `PATCH /tasks` | `405`、`Allow: GET, POST` | `API-14` |
| `GET /tasks/1` | `405`、`Allow: PUT, DELETE` | `API-14` |
| `GET /nope` | `404` | `API-15` |
| `PUT /tasks/abc/def`、`DELETE /tasks/1/2` | `404`,不是 `{id}` 路徑 | `API-3`、`API-15` |
| `Deps.UI` 為 nil 時 `GET /` | `404` | `API-15` |

**檢查順序**

| 請求 | 預期 | 需求 |
|---|---|---|
| `GET /tasks/abc` | `400`,id 先於方法檢查 | `API-7` |
| `PUT /tasks/abc` 帶超大 body | `400`,id 先於大小檢查 | `API-7` |
| `POST /tasks` 帶不合法且超大的 body | `413`,大小先於格式檢查 | `API-7` |
| `DELETE /nope` | `404`,路徑先於方法檢查 | `API-7` |
| `PATCH /tasks` 帶超大 body | `405`,方法先於大小檢查 | `API-7` |
| `PUT /tasks/99` 帶不合法 body | `400`,驗證先於存在檢查 | `API-7` |

**內部錯誤(使用假 Service 回傳未知錯誤)**

| 情境 | 預期 | 需求 |
|---|---|---|
| Service 回傳含 `secret` 的錯誤 | `500`,回應不含 `secret` | `API-16`、`API-18` |
| 同上,檢查日誌 | 有 `http.internal_error`,含錯誤內容;access 事件的 `status` 為 `500` | `LOG-11`、`LOG-14` |

**不變性:** 每個失敗案例之後呼叫 `GET /tasks`,確認資料沒有改變,再新增一筆,確認 `id` 沒有跳號(`DOM-11`)。

## 已定案

| 問題 | 決定 |
|---|---|
| `{"NAME":"a","status":0}` 算不算未定義欄位? | 不算。欄位名稱不分大小寫,沿用 `encoding/json`(`API-5`) |
| `/tasks/abc/def` 的狀態碼 | `404`(`API-3`、`API-15`) |
| `text/plain` 要接受還是回 `415`? | 接受,維持 `API-2`;風險記錄在 `02-api` 的決策 |
| 前端佔用 `/` 與 API 規則的衝突 | `API-19` 限定範圍,前端只佔用 `/`(`UI-7`) |
