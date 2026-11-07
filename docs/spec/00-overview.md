# 00. 總覽

## 1. 規範用語

本 spec 中的關鍵字 "MUST"、"MUST NOT"、"REQUIRED"、"SHALL"、"SHALL NOT"、"SHOULD"、"SHOULD NOT"、"RECOMMENDED"、"MAY" 和 "OPTIONAL",依 [RFC 2119](https://www.rfc-editor.org/rfc/rfc2119) 的定義解讀。

本專案另外約定:

- 只有**全大寫的英文關鍵字**具規範效力,小寫或中文敘述不具規範效力(與 [RFC 8174](https://www.rfc-editor.org/rfc/rfc8174) 一致)。
- 規範性需求以編號標示(例如 `DOM-1`)。未編號的內容、範例與「決策與理由」都僅供參考。
- 需求編號 MUST NOT 改作其他用途;刪除的編號 MUST 保留並標示為「已移除」。

## 2. 目的與範圍

本專案是一個以 Go 實作的 RESTful 任務 API,用於完成 Senior Backend Engineer 作業。spec 分為兩部分:

| 部分 | 內容 | 來源 |
|---|---|---|
| **核心** (`core/`) | 作業要求的功能與品質,以及服務運作必備的日誌 | 作業 PDF、專案自訂 |
| **擴展** (`extensions/`) | memory <-> file 同步、測試用前端 | 專案自行擴展 |

擴展功能全部停用時,系統 MUST 仍完全符合核心 spec。

## 3. 非目標

以下項目不在本版範圍內:

- 驗證與授權
- 分頁、篩選、排序參數
- 查詢單筆任務的端點(`GET /tasks/{id}`)
- 多實例部署與共用儲存
- 執行期間監看並載入快照檔的外部修改

## 4. 文件結構與依賴

```text
docs/spec/
├── 00-overview.md
├── core/
│   ├── 01-domain.md
│   ├── 02-api.md
│   ├── 03-architecture.md
│   └── 04-log.md
└── extensions/
    ├── 10-persistence.md
    └── 11-web-ui.md
```

```text
01-domain ← 02-api ← 11-web-ui
    ↑
10-persistence

03-architecture 描述核心各層如何組成;擴展可引用它。
04-log 依賴 02-api、03-architecture;10-persistence 依賴 04-log。
```

撰寫規則:

- 每個事實只定義在一處,其他文件以需求編號引用。
- 文件只引用其依賴的下層;核心文件不引用擴展文件。
- 決策記錄在該文件最後的「決策與理由」。

## 5. 作業需求對照

| PDF 需求 | 對應需求 |
|---|---|
| `GET /tasks` | `API-10` |
| `POST /tasks` | `API-11` |
| `PUT /tasks/{id}` | `API-12` |
| `DELETE /tasks/{id}` | `API-13` |
| `name` 欄位(string) | `DOM-4` |
| `status` 欄位(integer,0 或 1) | `DOM-8` |
| 執行環境 Go 1.18+ | `ARC-8` |
| 提供單元測試 | `ARC-11`、`ARC-12` |
| 提供 Dockerfile | `ARC-14` |
| 以 GitHub 管理程式碼並提供連結 | `ARC-16` |
| 使用記憶體儲存 | `ARC-6` |

## 6. 術語

| 術語 | 定義 |
|---|---|
| Task | 本系統管理的任務資源 |
| 快照(Snapshot) | 某一時間點的完整記憶體狀態 |
| 快照檔 | 寫入檔案系統的快照 |
| 優雅關閉 | 收到 SIGINT 或 SIGTERM 後,依定義順序結束服務 |
