# sshu — terminu fix

sshu 尚未符合 [terminu design principle](https://github.com/vulcanshen/terminu/tree/v0.1.20/principle)（tdp v0.1.20）的地方，逐條待修。
修好一條就刪掉一條，並同步 README（兩份）與 `docs/dev-remarks.md` 裡描述該行為的段落。有意不修的，改寫成
`dev-remarks.md`「偏離 tdp」的一條並附理由。

> **v0.1.20（2026-09-29）**：K11 / D3 —— 模式名夾在兩個框線接頭之間（雙線 `╡Drag╞`、單線 `┤Visual├`），模式色加粗、盡量一個詞，
> 放不下先截標題，panel 膠囊跟著外框換色；D6 —— icon 寬度量的是游標實際前進幾格，參考實作的完整清單、`<APP>_ICON_WIDTH`
> 覆寫、只在 unix 探測、做完的驗收。filu 的參考實作已完成，icon 寬度那一條現在可以做。本清單的每一條已照 v0.1.20 重新核對過。

盤點日期：2026-09-29（對照 tdp v0.1.18，v0.1.19 重核）。同一份清單的第 1–3b 條（模式名、hint 整組丟、連線中的 `Ctrl-C`、
focus 雙線）已於 2026-09-29 修完並刪除（設計文件 §11.67）；只剩下面這一條，**等 filu** 把 `width.go` 的做法定下來再照搬。

## 先看

- **等 filu。** filu 先補完自己的內容列、定下 `width.go`，sshu 再照搬。
- **每修一處補 model test，逐處 mutation**（編譯失敗不算抓到；否定的斷言要確認不是空轉的綠）。
- **同一個 commit 同步 README 兩份與 `dev-remarks.md`**，設計文件記一節（§11.68 起）；CHANGELOG 記在 `[Unreleased]`。
  `dev-remarks.md`「已知的牆與未做」的「尚未符合 tdp 的地方」一起改。
- **不 push、不發版。** 修完把這一輪寫進 terminu repo 的 `.local/family-fix/sshu/README.md`，然後刪掉這份清單。


## 4. icon 的實際寬度 —— D6（filu 已完成，照搬）

**filu 的參考實作已完成**（2026-09-29，`e1de220`，filu 第六輪）。照搬的東西（v0.1.20 的 D6 有同一份清單，細節在 terminu
`.local/family-fix/filu/README.md`「第六輪」最後的「D6 照搬清單」）：

- filu `internal/ui/width.go` 整個檔：`iconCells` / `IconCells()`、`isWideIcon()`、`iconCount()`、`dispWidth()`、`dispClip()`、
  `padDisp()`、`padDispRight()`、`truncate()`、`dispCutLeft()`、`compositeDisp()`（跟 `overlay.Composite` 同介面，直接換掉呼叫）、
  `centerDisp()`（取代 `lipgloss.Place`）、`blockWidth()`、`joinH()` / `joinV()`（取代 lipgloss 的 Join）。
- `iconwidth_unix.go` 的 `DetectIconWidth()`，在 `tea.NewProgram` 之前呼叫；手動覆寫用 `<APP>_ICON_WIDTH`（filu 是
  `FILU_ICON_WIDTH`）。探測只在 unix 做，Windows 預設一格、靠環境變數覆寫。
- 測試照 `d6_test.go`：icon 1 / 2 格下每一種 popup 各開一次，量**單獨的框**（並排的框量單一個）與**疊上去的整個畫面**每一列；
  `compositeDisp()` 的四種邊界（popup 列有 icon、被蓋的列有 icon、icon 被左 / 右框邊切半）。
- 驗收：`grep -n 'lipgloss.Width\|lipgloss.Size\|lipgloss.Place\|ansi.StringWidth\|ansi.Truncate' internal/ui/*.go` 只剩寬度函式本身。
- filu 的提醒：寬度改走 `dispWidth()` 後，在 `iconCells = 1` 的終端機上畫面完全不變（既有測試原封不動通過），只有探測到 2 才作用。


filu 要先補完自己的內容列，sshu 等 filu 做完、照它定下來的 `width.go` 搬。這一條先把 sshu 量寬度的地方盤點好。

**規則**：D6 ——「有些 CJK 用的 Nerd Font（例：Maple Mono NF CN）把 icon 畫成兩格，lipgloss 卻量成一格，框線就歪。app 啟動時
探測 icon 佔幾格，所有量寬度的地方（補空白、截斷、框線、疊 popup）都走同一個顯示寬度函式；L4 的畫面測試也跑一次『icon 佔
兩格』」。參考實作 filu `internal/ui/width.go`（`DetectIconWidth()`、`isWideIcon()`、`dispWidth()`、`dispClip()`），探測在
filu 的 `iconwidth_unix.go`（CPR 探測，另有 `FILU_ICON_WIDTH` 手動覆寫）。

**現況 —— sshu 自己畫的東西**：

- **漏斗已經有**：`internal/ui/width.go` 的 `dispW()`（= `lipgloss.Width`）、`truncate()`、`truncateHead()`、`clipANSI()`
  （= `ansi.Truncate`）、`padRight()`、`padLeft()`。37 個檔、兩百多處呼叫都經過它們（最多的是 `journals.go`、`sshtab.go`、
  `sftpview.go`、`form.go`、`table.go`、`hosts.go`），再加上建在它們上面的 `centerLine()`、`fitLines()`（`hosts.go`）、
  `fitPath()`（`path.go`）、`wrapAt()` / `wrapPlain()`（`sshtab.go`）、`wrapHint()`（`empty.go`）、`legendPairW()`、
  `keyLegend()`、`overlayRight()`、`panelChromeTone()`、`drawPopupBoxPad()`。把 `dispW()` 換成認得寬 icon 的版本，這些大多
  自動跟上。
- **漏斗裡要改的**：`clipANSI()` 用 `ansi.Truncate`，不認得寬 icon（照 filu 的 `dispClip()`）；`truncate()` /
  `truncateHead()` 逐字量 `dispW(string(r))`，`dispW` 換掉就對了。
- **繞過漏斗、要一起換的**：

| 位置 | 現況 | 怎麼改 |
|---|---|---|
| `chrome.go` `joinHorizontal()` / `joinVertical()` | `lipgloss.JoinHorizontal` / `JoinVertical`，量寬不認得寬 icon；用在網格與側欄（`sshtab.go`）、file transfer 左右兩側（`sftpview.go`）、Manage 的 nav 與內容（`preftab.go`） | 照 filu 的 `joinH` 自己拼 |
| `view.go` `View()` / `composeFloats()` 的 `overlay.Composite` | bubbletea-overlay 疊 popup 與 toast，量寬不認得寬 icon；popup 的標題都帶 glyph（`glyphWarn`、`glyphHelp`、`glyphEye`…） | 照 filu 當時的做法（自己疊，或疊之前先補齊） |
| `splash.go` `splashModel.render()` | 一個像素是 `pixelGlyph + " "`（`U+F0C8` + 空白，假設兩格）、沒亮的像素是兩個空白、`logoW := cols * 2`；icon 佔兩格時亮的像素變三格，logo 邊畫邊歪 | 像素寬度照探測結果（icon 兩格時不加空白），`logoW` 跟著算 |
| `sshtab.go` `sshModel.listItem()` | `const glyphCell = 2 // the glyph and its trailing space` | 用量的（`dispW(glyph) + 1`） |
| `table.go` | `truncate(h.Credential, colAuthW-2)` 與 `credAuthW = 12`（註解：glyph + 空白 + `privatekey`）假設 glyph 一格；外層的 `padRight` 會自己補正，但名字的預算會少算 | 預算扣 `dispW(glyph) + 1` |

- 不用改的：powerline 的圓角（`capLeft` / `capRight`，`U+E0B4` / `U+E0B6`）在 CJK icon 字型也是一格，filu 的 `isWideIcon()`
  就排除了；Braille 的 spinner 不是 PUA；loading icon（`U+F0A9E`–`U+F0AA5`）在 PUA-A，照規則算寬 icon。

**現況 —— 遠端畫的東西（PTY 格子）**：

- 哪個字在哪一格，照 vt 模擬器：`pty_unix.go` `ptyTerm.gridLines()` 逐格取 `p.term.Cell(x, y)`；遠端的尺寸（`resize(cols, rows)`）、
  游標、scrollback 都是 vt 的格數。這些**不走**顯示寬度函式，也不拿它去重排遠端的內容。
- 但 `gridLines()` 最後一步「切到格子寬、補空白」用的是 `clipANSI()` / `dispW()` —— 註解寫明是故意的：遠端印的 emoji、CJK
  終端機畫兩格、模擬器算一格，不照終端機實際畫的寬度切，格子的右框就被推出去。遠端的 Nerd Font icon（例：starship 的
  prompt）在 CJK icon 字型上是同一件事。建議這最後一道**照樣走**顯示寬度函式（認得寬 icon），代價跟 emoji 一樣是那一列最後
  一兩格；不走的話，遠端一印 icon 格子框就歪。
- 選取模式（`copymode.go`）在凍結的頁面（`copySnapshot()`，就是 `gridLines()` 的輸出）上畫游標與反白：欄位由 `lineChars()`
  逐字量 `dispW`，切段卻用 `ansi.Cut`（複製用的 `copyState.text()`、畫反白的 `markRow()`）。`dispW` 一換，兩邊就不一致，有 icon 的那一列反白
  會錯位。兩邊要用同一把尺 —— 照終端機畫的（認得寬 icon），把 `ansi.Cut` 換成認得寬 icon 的切法。複製出去的是字，不受影響。
- 遠端的字、sshu 自己重畫的（`[v]iew`、Errors 的全文、askpass 的問句、journal 各列）算「sshu 畫的」，走漏斗。

**巢狀要先想好**：內層 sshu 啟動時的探測，問的是外層的 vt 模擬器（sshu 的 pty emulator 會回答 `CSI 6n`，dev-remarks
「畫面」），而 vt10x 把 PUA 算一格 —— 在 CJK icon 字型上，內層會量到 1、照一格畫，外層再照上面那一道切掉尾巴。可能的做法：
手動覆寫的環境變數（照 filu 的 `FILU_ICON_WIDTH`，例：`SSHU_ICON_WIDTH`），或外層透過巢狀的通報通道（§11.44）告訴內層。等 filu
的探測定下來再一起決定。

**怎麼改**（filu 做完之後）：

- `width.go` 照 filu 搬：`iconCells`、`isWideIcon()`、認得寬 icon 的 `dispW()` 與 `clipANSI()`；啟動探測放在 `cmd/sshu/main.go`
  開 TUI 之前（`version`、askpass 之類的子命令不探測）。
- 上面「繞過漏斗」的五處一起換；`gridLines()` 的最後一道與 `copymode.go` 的切段照上面的建議。
- 測試：L4 的畫面測試加跑一次 `iconCells = 2` —— `TestSSHTabPreservesFrame`、`TestGridPreservesFrameAcrossLayouts`、
  `TestSFTPTabPreservesFrame`、`TestPrefTabPreservesFrame`、`TestPopupPreservesFrame`、`TestEveryPopupIsTheSameWidth`、
  `TestEmptyStatesPreserveFrame`、`TestSelectionModePreservesTheFrame`、`TestEditorPopupPreservesFrame`、`TestPickerFrameHolds`、
  `TestDialingPreservesFrame`、`TestSearchPreservesFrame`、`TestFullScreenPreservesTheFrame`，加上 splash 的每一列等寬；遠端印
  PUA icon 的格子框不歪；選取模式在有 icon 的列上反白對得上。`iconCells = 1` 時輸出跟現在一模一樣（量寬函式的 fast path）。
  mutation：任一處繞過漏斗改回 lipgloss 的量法。
- 文件：README 兩份的需求段（Nerd Font 那一條）補一句 CJK icon 字型也支援；dev-remarks「畫面」的 frame 不變量一條補上探測與
  手動覆寫。


## 5. 模式名沒有夾在框線接頭之間 —— K11、D3（v0.1.20）

**現況**：`chrome.go` `panelChromeMode()` 在上框右側寫 ` selection mode `（`sshtab.go` 的 `copyModeName`，前後各一個空白，沒有接頭），
兩個詞；滿版沒有框，名字疊在第 0 列靠右。

**規則**：K11（v0.1.20）—— 模式名夾在兩個框線接頭之間，像框上嵌了一個標籤。D3 —— 接頭跟框同色、線型跟著框（雙線 `╡` `╞`、
單線 `┤` `├`）；模式名用模式色加粗；盡量一個詞；放不下先截標題、模式名留著；panel 膠囊跟著外框換成模式色（sshu 已經是）。

**怎麼改**：`panelChromeMode()` 把名字畫成 `╡Select╞`（選取中的格子是 focus 的雙線；若是單線框就用 `┤` `├`），接頭用框色、名字用
Yellow 加粗；名字改成一個詞（例：`Select`，用字由 sshu 定）。`copyModeName` 也是 `?` 標題的來源，要不要拆成兩個常數由 sshu 定。
滿版沒有框、沒有接頭可夾：照舊疊在第 0 列靠右，只寫名字（模式色加粗），寫法寫進 dev-remarks。測試：選取模式的上框含 `╡Select╞`、
接頭是框色、名字是 Yellow；窄格子時標題先截、名字留著。參考 kbu `app.go` 的上框標籤（`248f883`）。

