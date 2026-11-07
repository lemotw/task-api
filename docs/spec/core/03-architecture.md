# 03. 架構

規範用語見 [00-overview](../00-overview.md#1-規範用語)。

## 1. 分層

```text
HTTP Handler  →  Service  →  Repository(介面)
                                   ↑
                             memory 實作
```

| 層 | 負責 | 不負責 |
|---|---|---|
| Handler | 路由、解析路徑 id、解析 JSON、檢查必填欄位、對應狀態碼 | 業務規則 |
| Service | 驗證欄位值、協調 Repository | HTTP、儲存細節 |
| Repository | 存取資料、指派 `id`、處理併發 | 驗證 |

- **ARC-1**:依賴方向 MUST 為 Handler → Service → Repository 介面。下層 MUST NOT 依賴上層。
- **ARC-2**:Service 和 Repository MUST NOT 依賴 `net/http`。
- **ARC-3**:檢查欄位是否存在與 JSON 型別 MUST 在 Handler 完成;`DOM-4`、`DOM-5`、`DOM-8` 的值驗證 MUST 在 Service 完成。
- **ARC-4**:領域錯誤(`DOM-12`、`DOM-13`)對應 HTTP 狀態碼的邏輯 MUST 只存在於 Handler。

## 2. 目錄結構

```text
cmd/server/        組裝各層、啟動與關閉
internal/task/     Task、領域錯誤、Service、Repository 介面
internal/memory/   Repository 的 in-memory 實作
internal/httpapi/  Handler 和請求/回應的 DTO
```

- **ARC-5**:Repository 介面 MUST 定義在 `internal/task`,由使用方擁有。

## 3. 儲存與併發

- **ARC-6**:任務資料在執行期間 MUST 存放於記憶體,且所有 API 讀寫 MUST 以記憶體為唯一資料來源。
- **ARC-7**:Repository 的每個操作 MUST 可安全地併發呼叫。更新與刪除的「檢查存在並修改」MUST 在同一個臨界區內完成。

介面示意(僅供參考):

```go
type Repository interface {
    List() []Task
    Create(name string, status int) Task
    Update(id int, name string, status int) (Task, error)
    Delete(id int) error
}
```

## 4. 執行環境與設定

- **ARC-8**:程式碼 MUST 可在 Go 1.18 編譯與執行,`go.mod` MUST 宣告 `go 1.18`,且 MUST NOT 使用更新版本才有的語言或標準函式庫功能。
- **ARC-9**:正式程式碼 SHOULD NOT 使用第三方相依套件。
- **ARC-10**:伺服器 MUST 預設監聽 `8080`,並 MUST 支援以 `PORT` 環境變數覆寫。

實作注意(僅供參考):以下功能都晚於 Go 1.18,因此受 `ARC-8` 限制:帶方法的 `ServeMux` 路由模式(1.22)、`http.MaxBytesError`(1.19)、`errors.Join`(1.20)、`slices` 套件(1.21)。

## 5. 測試

- **ARC-11**:專案 MUST 提供單元測試,涵蓋 Service 的驗證規則、Repository 的操作與併發,以及 Handler 的狀態碼與回應格式。
- **ARC-12**:`go test -race ./...` MUST 通過。
- **ARC-13**:測試 MUST NOT 依賴外部服務或固定的網路連接埠。

## 6. 交付

- **ARC-14**:專案 MUST 提供 Dockerfile,且 MUST 可用 `docker build` 和 `docker run` 啟動 API。
- **ARC-15**:容器 SHOULD 以非 root 使用者執行。
- **ARC-16**:程式碼 MUST 以 GitHub repository 管理。
- **ARC-17**:README MUST 說明如何執行、測試、以 Docker 啟動,並列出 API。

## 7. 生命週期

- **ARC-18**:收到 SIGINT 或 SIGTERM 時,伺服器 MUST 停止接受新連線,並等待進行中的請求完成,最多 5 秒。
- **ARC-19**:啟動失敗時(例如無法監聽連接埠),行程 MUST 以非 0 狀態碼結束。

## 決策與理由

| 決策 | 理由 |
|---|---|
| 三層,不加 use case 層或 DI 框架 | 足以隔離 HTTP、業務規則與儲存;更多層在此規模沒有效益 |
| 拆成多個 package | 由編譯器約束依賴方向 |
| 必填檢查放 Handler | 「沒送」和「送了 0」只有在解析 JSON 時才分得出來 |
| Repository 提供 `Update`,不用 `Get` + `Save` | 避免併發時把剛刪除的任務寫回去 |
