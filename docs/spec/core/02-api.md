# 02. HTTP API

規範用語見 [00-overview](../00-overview.md#1-規範用語)。欄位與業務規則定義於 [01-domain](01-domain.md)。

## 1. 總則

- **API-1**:帶有內容的回應 MUST 為 UTF-8 編碼的 JSON,且 `Content-Type` MUST 為 `application/json`。
- **API-2**:用戶端 SHOULD 在帶 body 的請求加上 `Content-Type: application/json`。伺服器 MUST NOT 因 `Content-Type` 而拒絕請求。
- **API-3**:路徑中的 `{id}` MUST 符合 `^[1-9][0-9]*$`,且 MUST 在實作的整數範圍內;否則伺服器 MUST 回傳 `400`。只有 `/tasks/` 之後恰好一個路徑區段才算 `{id}` 路徑,其他路徑 MUST 依 `API-15` 處理。
- **API-4**:伺服器 MAY 將結尾多一個 `/` 的路徑視為相同路徑。用戶端 SHOULD NOT 依賴此行為。
- **API-19**:本文件定義 `/tasks`、`/tasks/{id}` 和未被擴展佔用的路徑。擴展 MAY 定義其他路徑的行為,但 MUST NOT 改變 `/tasks` 和 `/tasks/{id}` 的行為。

### 請求 body

適用於 `POST /tasks` 和 `PUT /tasks/{id}`。

- **API-5**:body MUST 是單一 JSON 物件,且 MUST 同時包含 `name` 和 `status`。欄位名稱比對 MUST 不分大小寫。以下情況伺服器 MUST 回傳 `400`:
  - body 為空或不是合法 JSON
  - JSON 值之後還有其他資料
  - 缺少必填欄位,或欄位值為 `null`
  - 出現未定義的欄位,包含 `id`
  - 欄位型別錯誤,例如 `"status": "1"`、`"status": 0.5`、`"status": 1e0`
  - 違反 `DOM-4`、`DOM-5` 或 `DOM-8`
- **API-6**:body 超過 1 MiB(1,048,576 位元組)時,伺服器 MUST 回傳 `413`。

### 處理順序

- **API-7**:請求 MUST 依下列順序檢查,並回傳第一個失敗項目對應的錯誤:

  1. 路徑是否存在(`404`)
  2. 方法是否允許(`405`)
  3. `{id}` 格式(`400`)
  4. body 大小(`413`)
  5. body 格式與領域驗證(`400`)
  6. 目標任務是否存在(`404`)

## 2. 端點

Task 的 JSON 表示:

```json
{"id": 1, "name": "buy milk", "status": 0}
```

### `GET /tasks`

- **API-10**:伺服器 MUST 回傳 `200` 和所有任務組成的陣列,依 `id` 遞增排序。沒有任務時 MUST 回傳 `[]`,MUST NOT 回傳 `null`。

### `POST /tasks`

```json
{"name": "buy milk", "status": 0}
```

- **API-11**:成功時伺服器 MUST 回傳 `201`、新建的 Task,以及 `Location: /tasks/{id}` 標頭。

### `PUT /tasks/{id}`

```json
{"name": "buy milk", "status": 1}
```

- **API-12**:成功時伺服器 MUST 回傳 `200` 和更新後的 Task。目標不存在時 MUST 回傳 `404`,且 MUST NOT 建立任務。

### `DELETE /tasks/{id}`

- **API-13**:成功時伺服器 MUST 回傳 `204` 且不帶 body。目標不存在時 MUST 回傳 `404`。

## 3. 其他路由

- **API-14**:方法不被允許時,伺服器 MUST 回傳 `405` 和 `Allow` 標頭。`/tasks` 的值 MUST 為 `GET, POST`;`/tasks/{id}` 的值 MUST 為 `PUT, DELETE`。
- **API-15**:未定義的路徑 MUST 回傳 `404`。

## 4. 錯誤格式

```json
{"error": "status must be 0 or 1"}
```

- **API-16**:所有 4xx 和 5xx 回應 MUST 使用上述格式,`error` MUST 為非空字串。
- **API-17**:`error` 是給人閱讀的訊息。用戶端 MUST NOT 依賴其內容判斷錯誤類型,SHOULD 依狀態碼判斷。
- **API-18**:`500` 回應 MUST NOT 洩漏內部細節。

## 5. 狀態碼總表

| 狀態碼 | 意義 | 需求 |
|---|---|---|
| `200` | 查詢或更新成功 | `API-10`、`API-12` |
| `201` | 建立成功 | `API-11` |
| `204` | 刪除成功 | `API-13` |
| `400` | id 或 body 不合法 | `API-3`、`API-5` |
| `404` | 路徑或任務不存在 | `API-12`、`API-13`、`API-15` |
| `405` | 方法不被允許 | `API-14` |
| `413` | body 過大 | `API-6` |
| `500` | 伺服器內部錯誤 | `API-18` |

## 決策與理由

| 決策 | 理由 |
|---|---|
| `PUT` 是完整取代,不是部分更新 | 符合 PUT 語意;部分更新屬於 PATCH |
| `PUT` 不存在的 id 回 `404`,不建立 | `id` 由系統指派(`DOM-2`) |
| 拒絕未定義欄位 | 拼錯欄位名稱時能立即發現 |
| `id` 只接受標準十進位格式 | 避免 `01`、`+1` 指向同一筆任務 |
| 方法先於 `{id}` 格式檢查 | 與路由套件一致:路由依路徑和方法比對後,才進入 handler 驗證 `{id}` |
| 不提供 `GET /tasks/{id}` | PDF 未要求,因此回 `405` |
| 不檢查 `Content-Type` | `curl -d` 不加標頭也能使用;代價是其他網站可對本機服務發出跨站 POST,因此服務不應對外暴露 |
| 欄位名稱不分大小寫 | 沿用 `encoding/json` 的行為,不多寫解析程式 |
| 重複的 key、不合法的 UTF-8 不定義結果 | 不為這些輸入多寫解析程式 |
