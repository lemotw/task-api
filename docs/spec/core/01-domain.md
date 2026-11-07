# 01. 領域模型

規範用語見 [00-overview](../00-overview.md#1-規範用語)。本文件不涉及 HTTP 與儲存方式。

## 1. Task

| 欄位 | 型別 | 來源 | 說明 |
|---|---|---|---|
| `id` | 正整數 | 系統指派 | 任務識別碼 |
| `name` | 字串 | 使用者提供 | 任務名稱 |
| `status` | 整數 | 使用者提供 | `0` 未完成,`1` 已完成 |

- **DOM-1**:每個 Task MUST 具備 `id`、`name`、`status` 三個欄位。

### `id`

- **DOM-2**:`id` MUST 由系統指派,且 MUST 為正整數。
- **DOM-3**:新任務的 `id` MUST 大於先前指派過的所有 `id`。已刪除任務的 `id` MUST NOT 被重複使用。

### `name`

- **DOM-4**:`name` MUST 是字串,且 MUST 包含至少一個非空白字元。空白字元指 Unicode `White_Space` 屬性的字元。
- **DOM-5**:`name` 的長度 MUST NOT 超過 255 個 Unicode code point。
- **DOM-6**:系統 MUST NOT 修改使用者提供的 `name`,包含去除前後空白。
- **DOM-7**:不同任務 MAY 具有相同的 `name`。

### `status`

- **DOM-8**:`status` MUST 是整數 `0` 或 `1`。

## 2. 操作規則

- **DOM-9**:建立任務時,使用者 MUST 提供 `name` 和 `status`。系統 MUST NOT 為缺少的欄位指定預設值。
- **DOM-10**:更新任務是完整取代。使用者 MUST 提供 `name` 和 `status`,且更新 MUST NOT 改變 `id`。
- **DOM-11**:失敗的操作 MUST NOT 改變任何任務或影響之後指派的 `id`。

## 3. 領域錯誤

- **DOM-12**:輸入違反 `DOM-4`、`DOM-5` 或 `DOM-8` 時,操作 MUST 回報「驗證錯誤」。
- **DOM-13**:操作的目標 `id` 不存在時,操作 MUST 回報「找不到」。

## 決策與理由

| 決策 | 理由 |
|---|---|
| `id` 使用遞增整數 | 簡單易讀;記憶體單實例不需要 UUID |
| `status` 建立時必填 | PDF 寫建立時至少包含此欄位;也讓建立與更新的規則一致 |
| 不修改 `name` | 去除空白等於默默改動使用者資料 |
| 長度以 code point 計算 | 以位元組計算時,中文會提早達到上限 |
| 不含 `created_at` / `updated_at` | 目前沒有功能會用到 |
