# `internal/memory` 測試提案

臨時 review 文件,不會留存,也不具規範效力。需求定義以 `docs/spec` 為準。

- **測試替身:** 不需要
- **執行:** `go test -race ./internal/memory`
- **RED 階段:** 空殼會 `panic` 並中止整個測試程式,所以要用 `-run` 逐一執行,確認每個測試都失敗。

**測試方式:** 直接測真實實作,併發測試搭配 `-race`。

## 成功

| 情境 | 預期 | 需求 |
|---|---|---|
| 空的 Repository 呼叫 `List` | 回傳空切片,不是 `nil` | `API-10` |
| 連續 `Create` | `id` 為 1、2、3 | `DOM-2`、`DOM-3` |
| 建立多筆後 `List` | 依 `id` 遞增排序 | `API-10` |
| `Update` | 內容被取代,`id` 不變 | `DOM-10` |
| `Delete` 最大的 `id` 後再 `Create` | `id` 不重複 | `DOM-3` |
| 刪光所有任務後再 `Create` | `id` 不重複 | `DOM-3` |
| 建立兩筆同名任務 | 兩筆都存在 | `DOM-7` |
| 修改 `List` 回傳的切片 | 內部資料不受影響 | `ARC-7` |
| 修改 `Export` 回傳的 `Tasks` | 內部資料不受影響 | `PER-4` |
| `Export` 後再更新 Repository | 已取得的快照不變 | `PER-4` |
| 對已有資料的實例呼叫 `Import` | 舊資料完全被取代 | `PER-5` |
| `Import` 後修改傳入的切片 | 內部資料不受影響 | `PER-5` |
| `Export` 後 `Import` 到新實例 | `List` 和下一個 `id` 都一致 | `PER-9` |
| 每次成功 `Create`、`Update`、`Delete` | `revision` 增加 | `PER-17` |
| 併發 `Create` | `id` 全部不重複,最後筆數正確 | `ARC-7`、`DOM-3` |
| `List` 和 CRUD 混合併發,搭配 `-race` | 沒有 data race | `ARC-7` |

## 失敗

| 情境 | 預期 | 需求 |
|---|---|---|
| `Update`、`Delete` 不存在的 `id` | `ErrNotFound` | `DOM-13` |
| 上述失敗操作 | `revision` 和下一個 `id` 都不變 | `DOM-11` |
| 同一 `id` 併發 `Update` 和 `Delete` | 無 data race;刪除後不會被寫回 | `ARC-7` |
| 併發寫入時 `Export` | `NextID` 永遠大於所有 `id` | `PER-4` |
| 只併發 `Create` 時 `Export` | 筆數等於最大 `id`,`NextID` 等於最大 `id` 加 1 | `PER-4` |
