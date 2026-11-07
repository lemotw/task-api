# `internal/task` 測試提案

臨時 review 文件,不會留存,也不具規範效力。需求定義以 `docs/spec` 為準。

- **測試替身:** `Repository`:假 Repository,記錄呼叫次數
- **執行:** `go test -race ./internal/task`
- **RED 階段:** 空殼會 `panic` 並中止整個測試程式,所以要用 `-run` 逐一執行,確認每個測試都失敗。

**測試方式:** 使用假 Repository,確認驗證規則,以及驗證失敗時不會碰到儲存層。所有欄位驗證都要同時測 `Create` 和 `Update`。

## 成功

| 情境 | 預期 | 需求 |
|---|---|---|
| `Create("buy milk", 0)`、`("buy milk", 1)` | 回傳 Repository 建立的 Task | `DOM-8` |
| `name` 剛好 255 個中文字或 emoji | 成功,以 code point 計算而非位元組 | `DOM-5` |
| `name` 為 `"  buy milk  "` | 原樣傳給 Repository | `DOM-6` |
| 兩個任務同名 | 都成功 | `DOM-7` |
| `Update` 傳入合法值 | 回傳更新後的 Task | `DOM-10` |
| 合法的 `Create`、`Update` | 假 Repository 只被呼叫一次,收到的 `id`、`name`、`status` 都正確 | `DOM-9`、`DOM-10` |
| `Validate` 傳入合法值 | 回傳 `nil` | `DOM-4`、`DOM-5`、`DOM-8` |
| `List` | 回傳 Repository 的結果 | `API-10` |
| `Delete` 目標存在 | 回傳 `nil` | `API-13` |
| `ValidationError.Error()` | 回傳 `Message` | `DOM-12` |

## 失敗

| 情境 | 預期 | 需求 |
|---|---|---|
| `name` 為 `""`、`"   "`、全形空白 `"　"`、`"\t\n"` | `*ValidationError` | `DOM-4`、`DOM-12` |
| `name` 為 256 個 code point | `*ValidationError` | `DOM-5`、`DOM-12` |
| `name` 為 255 個字加一個空白 | `*ValidationError`,長度以原字串計算 | `DOM-5`、`DOM-6` |
| `status` 為 `2`、`-1` | `*ValidationError` | `DOM-8`、`DOM-12` |
| 驗證失敗 | 假 Repository 的呼叫次數為 0 | `DOM-11` |
| `Update`、`Delete` 目標不存在 | `errors.Is(err, ErrNotFound)` | `DOM-13` |
| `Update`、`Delete` 時 Repository 回傳未知錯誤 | 原錯誤向上傳遞,讓 Handler 能回 `500` | `API-18`、`ARC-4` |
| `Validate` 傳入不合法的值 | `*ValidationError` | `DOM-12` |

## 介面調整(已完成)

已新增 `Validate(name string, status int) error`,讓 `Service` 和 `snapshot.File` 共用同一份規則(`PER-7`)。
