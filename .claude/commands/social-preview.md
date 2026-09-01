---
description: 從一張 pixel-art SVG（icon）+ 一段字樣文字生成 GitHub social preview 圖（輸出到 /tmp）
argument-hint: "[字樣文字] (icon 預設 docs/icon.svg;沒給文字會問你)"
---

委派 `social-preview` agent 從 SVG（icon）+ 字樣文字生成 social preview 圖。

## 輸入

- **字樣文字**（icon 下方的 wordmark）：取自 `$ARGUMENTS`。
- **SVG 路徑**（icon 基礎）：預設 `docs/icon.svg`。若 `$ARGUMENTS` 裡明確帶了一個 `*.svg` 路徑，就用那張、其餘 token 當字樣文字。

## 步驟

1. **決定字樣文字**：
   - 若 `$ARGUMENTS` 有提供文字 → 用它。
   - 若沒有 → **先問使用者**要放什麼字樣（例如 `KubeUI` / `kbu` / `KBU`），拿到再往下。**不要自己編、不要沿用舊字。**
2. 用 **Agent 工具** 啟動 `social-preview` agent（`subagent_type: "social-preview"`），prompt 帶上 **SVG 路徑 + 字樣文字**兩者。
3. Agent 會把生成圖片寫到 `/tmp` 並回傳該圖片的絕對路徑。
4. 拿到路徑後，用 **Read** 開該 PNG 預覽給我看，並把絕對路徑明確列出來。
5. 不要自行把圖片複製進 `docs/`；要不要覆蓋 `docs/social-preview.png` 等我指示。
