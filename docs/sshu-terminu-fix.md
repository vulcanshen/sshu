# sshu — terminu fix

sshu 尚未符合 [terminu design principle](https://github.com/vulcanshen/terminu/tree/v0.1.19/principle)（tdp v0.1.19）的地方，逐條待修。
修好一條就刪掉一條，並同步 README（兩份）與 `docs/dev-remarks.md` 裡描述該行為的段落。有意不修的，改寫成
`dev-remarks.md`「偏離 tdp」的一條並附理由。

> **v0.1.19（2026-09-29，這份清單寫完後才出）**：L5 —— focus 不能只靠顏色分辨（模式會換框色），家族預設雙線；K10 ——
> 子程序還沒準備好時可以不轉送一般的鍵，但 `Ctrl-C` 照樣轉送。本清單的每一條已照 v0.1.19 重新核對過。

盤點日期：2026-09-29（對照 tdp v0.1.18）。以 `main` 的 `4258277` 為準（sshu 已對齊 v0.1.17，設計文件 §11.66）；位置寫檔案與
函式，不寫行號。這一輪**只對 v0.1.17 → v0.1.18 的改動**（`git diff v0.1.17 v0.1.18 -- principle/` 與 terminu CHANGELOG 的
v0.1.18 段）逐條拿程式碼核對：

| tdp 的改動 | 在這份清單 |
|---|---|
| K11、D2：模式名寫在模式所在框的上框右側，外框換 Yellow，離開就恢復 | 第 1 條（外框已經是 Yellow，缺模式名） |
| F1、D3：finder 的 focus 在哪一邊要看得出來 | 不適用（sshu 沒有 finder，見「已經符合」） |
| D2：失焦 panel 邊框上的 hint 用 Overlay0 / Surface2 | 不適用（sshu 的 panel 邊框沒有 hint，見「已經符合」） |
| D3：下框 hint 放不下時從尾端整組捨棄 | 第 2 條 |
| K10、K9：子程序還沒準備好時可以不轉送；PTY 裡 `q`、`Ctrl-C` 屬於子程序 | 第 3 條（連線中的格子的 `Ctrl-C`）；其餘已經符合 |
| D6：探測 icon 的實際寬度，所有量寬度的地方走同一個函式 | 第 4 條（**等 filu**） |

tdp 連結（兩份 README、`dev-remarks.md`、設計文件開頭）已由 terminu session 改釘 `v0.1.19`，跟這份清單一起留在工作樹，
還沒 commit；`.claude/rules/` 裡沒有 tdp 連結。


## 先看

- **清單先 commit，再動程式。** 先讀完這份清單與工作樹裡改好的四處連結，一起 commit；程式的 commit 跟清單分開。「待確認」
  兩題先問 user。commit 只加自己改的路徑。
- **「現況」照內容核對，不照行號。** 動手前 `grep` 一次現況。
- **每修一處補 model test，逐處 mutation**：把修正單獨改回舊行為，確認對應的測試會紅（編譯失敗不算抓到；否定的斷言要確認
  不是空轉的綠）。
- **同一個 commit 同步 README 兩份與 `dev-remarks.md`**，設計文件記一節（§11.67 起）；CHANGELOG 記在 `[Unreleased]`。
  `dev-remarks.md`「偏離 tdp」的「對照 tdp v0.1.17」與「已知的牆與未做」的「尚未符合 tdp 的地方」一起改。
- **第 4 條（icon 寬度）等 filu。** filu 先補完自己的內容列、把 `width.go` 的做法定下來，sshu 再照搬；這一輪先修第 1–3 條。
  其他條修完、第 4 條還在等時，清單留著、只剩第 4 條。
- **修完拿 v0.1.19 全文再逐條對一次**（不只 CHANGELOG）。
- **不 push、不發版。**
- 修完把這一輪寫進 terminu repo 的 `.local/family-fix/sshu/README.md`（加一節「對照 v0.1.18」，開頭的時間線與「發布」的
  commit 表一起更新）。
- patch 腳本與 commit message 先寫進檔案再執行：英文撇號會截斷 `-c '…'`。


## 1. 選取模式沒有在框上寫出模式名 —— K11、D2

**現況**：

- 外框已經照做：`internal/ui/sshtab.go` `sshModel.cellTone()` 在選取模式的那一格回 `toneSelect`，`chrome.go` `toneColor()` 給
  `selectColor`（Yellow `#f9e2af`）；左上的標題膠囊（`panelChip()`）跟著換 Yellow；離開模式（`copy.on` 變假）就恢復。sshu 的
  panel 不分 focus 線型（一律圓角，focus 只看顏色，L5 允許），所以「線型不變、只換顏色」本來就成立。
- 缺的是模式名：`chrome.go` `panelChromeTone()` 的上框只有左邊的標題膠囊，右邊是一路的 `─` 到 `╮`，沒有地方寫模式名。現在說
  「這一格在選取模式」的只有黃框與 footer 的 legend（`view.go` `copyLegendPairs()`）。
- 滿版（zoom max）：`sshModel.cellView()` 在 `zoomAt() == zoomMax` 時不畫框（`full`），直接回傳內容；`markMaxed()` 在選取模式
  時只把 legend 疊在最後一列，然後 `return` —— 這一層的 badge（`zoomBadge()`，第 0 列靠右）與出口的 hint（第 1 列靠右）都不畫。
  所以滿版的選取模式沒有框可以換色、也沒有模式名。

**規則**：K11 ——「模式要標示自己：模式名一律顯示在模式所在的框（panel 或 popup）的上框右側，外框換成模式色；離開模式就恢復。
focus 的 panel 在模式裡照樣是 focus 的線型（L5），只有顏色換掉」。D2 —— 模式色是 Yellow `#f9e2af`（外框與右上角的模式名）。

**怎麼改**：

- **有框的時候**（網格、zoom panel）：`panelChromeTone()` 多收一個右側的字（或另開一個變體），上框畫成
  `╭` + 標題膠囊 + `─…─` + 模式名 + `─╮`，模式名用 Yellow（照框色）、不做成膠囊（兩個膠囊會讀成兩個標題）。`cellView()` 在
  選取模式時傳入模式名，建議跟 `?` 的標題同一個字：`selection mode`。
- **寬度**：模式名「一律顯示」，所以窄格子先讓標題讓位 —— `cellTitle()` 的寬度預算扣掉模式名與兩側各一格 `─`；連模式名都放
  不下時才依序截模式名（例：`selection`）。上框每一列的寬度不變（frame 不變量，L4）。
- **滿版**：沒有框，右上角就是 badge 的位置。這一層在選取模式時本來就不畫 badge（`markMaxed()` 提早 `return`），所以單層時
  模式名疊在第 0 列靠右（Yellow），不會跟這一層的 badge 打架；最後一列的 legend 照舊。**巢狀時會被蓋住**：`Zoom max + lock`
  之後，外面幾層都是滿版、鎖住，最外層最後畫、把自己的 badge（`sshu`、層數 glyph 與數字、鎖頭 glyph，約 12 格）疊在同一個角，內層的
  `selection mode`（14 格）幾乎整個被蓋掉。怎麼處理見「待確認」第 1 題。
- 滿版時 `Alt-Esc:unzoom` 那一列：選取模式時 `markMaxed()` 不畫它（出口是 legend 裡的 `Alt-v:leave`），維持。
- F8：`?` 的 key reference 疊在選取模式上時，黃框與模式名跟著整片淡化，不用特別處理。
- 測試：選取模式的那一格上框右側有 `selection mode`、是 Yellow、框是 Yellow；離開後模式名消失、框回 focus 的 Blue；zoom panel
  同一套；窄格子時標題先截、模式名還在；`TestSelectionModePreservesTheFrame` 加一個窄寬與一個滿版的情境，每一列仍是終端機
  寬度；滿版時第 0 列靠右是模式名、最後一列是 legend。mutation：不傳模式名；模式名用框以外的顏色；離開模式沒清掉；標題
  不讓位（窄格子時模式名被截掉）。
- 文件：README 兩份的選取模式那一句（「freeze this cell to copy from it / 凍結這一格，從裡面複製」）或表下說明補「框變黃、
  右上角寫 `selection mode`」；dev-remarks「選取模式」一段的「外框轉黃就是在說這件事」補上模式名；設計文件記一節。


## 2. 下框 hint 放不下時截在項目中間 —— D3

**現況**：

- `internal/ui/popup.go` `drawPopupBoxPad()`：`hint = clipANSI(hint, innerW-1)` —— `hintLegend()` 已經把整串畫好，這裡照欄數
  硬切，切點落在哪個項目的哪個字都有可能。每一個用 `drawPopupBox()` / `drawPopupBoxPad()` 的 popup 都走這裡：confirm、input、
  askpass、Space menu（含 global operation、lock menu、兩個 picker）、Jobs、viewer、明細、key reference、toast、編輯器、
  identity file picker、四個表單、known_hosts 的 fetch。
- 實際會切到的寬度：popup 內寬是 `popupInnerW()` = `min(W − 2, 120) − 2`，hint 的預算再少一格。`minAppW` 32 欄時預算 27：
  Space menu 的 ` j/k:move Enter:run Esc:close `（30）、`Enter:overwrite Esc:cancel` 的 confirm（28）、askpass 的
  `Enter:yes Esc:cancel connection`（33）都被切；40 欄時預算 35：表單的 `Tab:next ←/→:switch Enter:save Esc:cancel`（43）、
  Jobs 的 `j/k:move Enter:open c:cancel Esc:close`（40）被切。
- `bundlepage.go`（Operation 頁，遮罩中）：`clipANSI("  "+hintLegend(pairs), innerW)`，同一個問題。
- 已經整組捨棄的：footer 的 `keyLegend()`（從尾端整組丟，D1）；滿版選取模式的 legend（`markMaxed()` 用 `keyLegend()`）；滿版
  的 `Alt-Esc:unzoom`（`overlayRight()` 放不下就整個不畫）。

**規則**：D3 ——「下框 hint 放不下時，從尾端整組捨棄（跟 D1 的 footer 一樣），不截在項目中間」。

**怎麼改**：

- 讓 `hintLegend()` 知道寬度：收 pairs 與預算，從尾端整組丟到放得下為止（跟 `keyLegend()` 同一段算法，量寬用 `legendPairW()`，
  兩個 renderer 共用）；`drawPopupBoxPad()` 改收 pairs、自己用 `innerW-1` 當預算，或各 popup 先算好再傳。`clipANSI` 只留給
  「連第一組都放不下」的最後防線（或乾脆不畫）。
- 位置指示（鍵為空字串的項目，例：`3 of 12`）也是一組，照樣整組丟。
- 順序：sshu 的 hint 一律把 `Esc` 放最後，窄的時候它最先掉。D3 只說從尾端丟；`Esc` 仍在 `?` 的 key reference 裡（K6）。要不要
  調整順序由 app 決定，這一輪可以不動。
- `bundlepage.go` 同一個改法（遮罩中，改了也碰不到，但別留一個舊的算法）。
- 測試：32 與 40 欄時每一個 popup 的下框 —— 去掉 ANSI 後，hint 是若干個完整的 `鍵:說明`（沒有被切斷的項目），而且仍然從
  第一組開始；寬的時候全部都在。mutation：改回 `clipANSI` 硬切；從頭丟而不是從尾丟。


## 3. 連線中的格子，`Ctrl-C` 走 sshu 的離開流程 —— K9、K10

**現況**（`internal/ui/app.go` `handleKey()`）：

- 格子有鍵盤、遠端還沒開口（`ptyFocused()` 真、`inPty()` 假）時，其他鍵都被吞掉（`if m.ptyFocused() { return m, nil }`），
  `Alt-Esc` 照樣可以離開（先問），footer 照樣揭露它 —— 這是 K10 新寫的那一條，已經符合。
- 但 `Ctrl-C` 的判斷排在吞鍵之前、只問 `!m.inPty()`：連線中按 `Ctrl-C`，走的是 sshu 的 `startQuit()` —— 連線中的那一條
  也算 session（`liveCount()` 數的是全部），所以跳「Quit sshu?」；再按一次 `Ctrl-C` 就立刻離開、帶走所有 session。遠端開口之後，同一個鍵是遠端的中斷；連線中的那幾秒，它是 sshu 的離開。

**規則**：K9（v0.1.18）——「focus 在 PTY 裡時兩個都屬於子程序（K10）」。K10（v0.1.18）——「子程序還沒準備好收鍵時（例：遠端
還在連線），app 可以不轉送按鍵；出口鍵照樣有效、照樣揭露」。連線中的格子就是 K10 那個例子：focus 在 PTY 裡，`Ctrl-C` 不該是
app 的離開流程。

**已定案**（user 2026-09-29，v0.1.19 寫進 K10）：**連線中的 `Ctrl-C` 轉送給 ssh**，不吞掉。user 的理由：focus 在 PTY 裡時，
使用者認為每個鍵都是在 PTY 裡按的，例外只有明示揭露的出口鍵；`Ctrl-C` 這種鍵也要進 PTY。v0.1.19 的 K10：還沒準備好時可以不轉送
**一般的鍵**，但 `Ctrl-C` 照樣轉送。

**怎麼改**：`Ctrl-C` 的判斷從 `!m.inPty()` 改成 `!m.ptyFocused()`（不再進 sshu 的離開流程），連線中的 `Ctrl-C` 寫進那一格的
PTY：ssh 還沒把 tty 切成 raw，line discipline 把它變成 SIGINT，當場中止這條連線；這條 session 以失敗結束，照一般失敗跳 toast、
記進 Errors。其他一般的鍵照舊吞掉。`q` 在連線中本來就被吞掉，不用動。PTY 上開著 popup 時（`ptyFocused()` 為假）`Ctrl-C`
照舊是離開流程。測試：連線中的格子按 `Ctrl-C`，不出離開的問題、不離開 sshu，`\x03` 寫進那一格的 PTY；遠端開口後照舊送到遠端；
清單上照舊是離開流程。mutation：改回 `!m.inPty()`；拿掉轉送。文件：dev-remarks「運作方式」與 README 兩份「Everywhere / 到處都通」那一行（session 裡 `Ctrl-C` 屬於它）不用改字，
行為跟著說法走了；設計文件附錄「全域」表的 `Ctrl-C` 列補「連線中也是」。


## 3b. focus 只靠顏色分辨 —— L5（v0.1.19）

**現況**：sshu 的 panel 不分 focus 線型（`panelChromeTone()` 一律圓角 `╭─╮`，focus 只換顏色）。第 1 條的「選取模式外框換
Yellow、線型不變」在 sshu 因此看不出 focus。

**規則**：L5（v0.1.19）—— focus 不能只靠顏色分辨，要有顏色以外的差別；家族預設 focus 雙線 `╔═╗`、失焦圓角 `╭─╮`，兩者同寬（D2）。
user 2026-09-29 裁定家族統一成雙線（kbu、filu、locku 已經是）。

**怎麼改**：focus 的 panel 與有鍵盤的格子畫雙線 `╔═╗`，失焦照舊圓角；兩套框線同寬、切換不位移（L5）。跟第 1 條一起做：選取模式
換成 Yellow 時保留雙線。滿版沒有框，不受影響。測試：focus 的框是雙線、失焦是圓角，切換前後每一列寬度不變；選取模式裡框是 Yellow
雙線。文件：dev-remarks、設計文件講 focus 的地方補線型。


## 4. icon 的實際寬度 —— D6（等 filu 做完再照搬）

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


## 已經符合、不用修的（對照 v0.1.18 的改動）

- **K11 的外框**：選取模式的那一格外框與標題膠囊都換 Yellow（`toneSelect` → `selectColor`），離開就恢復；sshu 的 panel 不分
  focus 線型，換的只有顏色。缺的模式名見第 1 條。sshu 只有這一個模式（K11）；Hosts 與 file transfer 的 `/` 是打字（K8），
  不是模式。
- **F1、D3 的 finder：不適用。** sshu 沒有「打字與清單之間用 `Tab` 換 focus」的 popup：identity file picker（`filepicker.go`）
  是附候選清單的 input（可列印的鍵都是字、方向鍵在候選之間移動、沒有 `Tab` 的階段）；host 與 credential 的 picker 是 menu
  （`spaceMenu` 的實例，不打字）；Hosts 與 file transfer 的 `/` 搜尋在 panel 裡，不是 popup，也沒有兩個階段。
- **D2 失焦 panel 邊框上的 hint：不適用。** sshu 的 panel 邊框（`panelChromeTone()`）只有左上的標題膠囊，沒有 hint；格子的
  標題與捲歷史的 `󰋚 N` 不是 hint；滿版的 `Alt-Esc:unzoom` 只在格子有鍵盤時存在。遮罩中的 Operation 頁在 panel 內容裡畫了一行
  hint（`bundlepage.go`），重新上架時照這一條的顏色核對。
- **D3 的 footer 與滿版的疊字**：footer（`keyLegend()`）、滿版選取模式的 legend、滿版的 `Alt-Esc:unzoom` 都是整組丟（見第 2 條）。
- **K10（v0.1.18 新增的一條）**：連線中的格子（`ptyFocused()` 真、`inPty()` 假）不轉送按鍵（`if m.ptyFocused() { return m, nil }`，
  註解寫明理由：ssh 還沒讀 stdin，鍵會在幾分鐘後落到遠端），`Alt-Esc` 照樣有效（先問，D5）、footer 照樣揭露（判斷用
  `ptyFocused()`）。例外是 `Ctrl-C`，見第 3 條。
- **K9：PTY 裡的 `q` 與 `Ctrl-C` 屬於子程序。** 遠端開口之後（`inPty()`）兩個都送到遠端（`q` 在 `handleKey()` 的 PTY 分支之後
  才被當成離開；`Ctrl-C` 的離開流程只在 `!m.inPty()` 時跑）；執行中的編輯器分支把所有鍵（含 `q`、`Ctrl-C`）交給編輯器，只留
  `Alt-Esc`。連線中的 `Ctrl-C` 見第 3 條。


## 待確認

沒有。上一版兩題 user 2026-09-29 裁定：

1. **滿版、巢狀時模式名被外層的 badge 蓋住（第 1 條）**：選 (a) —— 照做在右上角，巢狀時接受被蓋，靠最後一列的 legend 兜底。
   不做 (b)（通報格式多一欄）。
2. **連線中的 `Ctrl-C`（第 3 條）**：轉送給 ssh（見第 3 條「已定案」），v0.1.19 寫進 K10。
