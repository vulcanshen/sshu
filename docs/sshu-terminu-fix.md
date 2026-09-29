# sshu — terminu fix

sshu 尚未符合 [terminu design principle](https://github.com/vulcanshen/terminu/tree/v0.1.17/principle)（tdp v0.1.17）的地方，逐條待修。
修好一條就刪掉一條，並同步 README（兩份）與 `docs/dev-remarks.md` 裡描述該行為的段落。有意不修的，改寫成
`dev-remarks.md`「偏離 tdp」的一條並附理由。

> **v0.1.17（2026-09-29，這份清單寫完後才出）**：只補了 M5 三點，已照它調整本清單 —— menu 與 key reference **說明欄**裡提到的鍵
> 算句子，加方括號；**README** 內文的鍵用 Markdown code 標（`` `Enter` ``），不加方括號，鍵名與寫法照 M5；**別的工具自己的按鍵**
> （tmux 的 `prefix l`、`C-a x`）照那個工具的寫法。其餘條目照 v0.1.14–v0.1.16。

盤點日期：2026-09-29（對照 tdp v0.1.16；同日稍早對照 v0.1.14 寫過一版，已併進這一版）。以 `main` 的 `ba97f1e` 為準（sshu
已對齊 v0.1.13，設計文件 §11.65）；位置寫檔案與函式，不寫行號。這一輪**只對 v0.1.13 → v0.1.16 的改動**
（`git diff v0.1.13 v0.1.16 -- principle/` 與 terminu CHANGELOG 的 v0.1.14–v0.1.16 段）逐條拿程式碼核對：

| tdp 的改動 | 版本 | 在這份清單 |
|---|---|---|
| F1、F8、K11：toast 除了 `Esc` 不收鍵；模式裡回應 `Tab` 的 toast 還在時，第一個 `Esc` 先收掉它 | v0.1.14 | 第 1 條 |
| K10、D5：家族的 PTY 出口鍵是 `Alt-Esc`；`Alt-Esc` 讓 focus 離開 PTY 或結束子程序時一律先 confirm | v0.1.14、v0.1.16 | 第 2、3 條 |
| 術語「模式」：版面的切換（zoom）不是模式，`Esc` 不必退出它 | v0.1.14 | 已經符合 |
| M5：鍵名、依位置的寫法（label / 句子 / hint 與 footer / key reference）；D1–D4 的例子；D2 的鍵色 | v0.1.14、v0.1.15 | 第 4 條 |
| M6：`?` 的 key reference 跟 menu 同一套；說明別的 surface 的一段照亮顯示 | v0.1.14、v0.1.16 | 第 5、6 條；`ssh grid` 那段已經符合 |

核對時順帶看到、不屬於這幾版改動的三個舊問題，放在第 7–9 條（第 9 條已由 user 定案）。tdp 連結（兩份 README、
`dev-remarks.md`、設計文件開頭）已由 terminu session 改釘 `v0.1.17`，跟這份清單一起留在工作樹，還沒 commit。


## 先看

- **清單先 commit，再動程式。** 先讀完這份清單與工作樹裡改好的四處連結，一起 commit；程式的 commit 跟清單分開。commit 只加
  自己改的路徑。
- **「現況」照內容核對，不照行號。** 動手前 `grep` 一次現況；前面的條目一改，後面的位置就漂。
- **每修一處補 model test，逐處 mutation**：把修正單獨改回舊行為，確認對應的測試會紅（編譯失敗不算抓到；否定的斷言要確認
  不是空轉的綠）。
- **同一個 commit 同步 README 兩份與 `dev-remarks.md`**，設計文件記一節（§11.66 起）；CHANGELOG 記在 `[Unreleased]`。
  `dev-remarks.md`「偏離 tdp」的「對照 tdp v0.1.13」與「已知的牆與未做」的「尚未符合 tdp 的地方：目前沒有」一起改。
- **修完拿 v0.1.17 全文再逐條對一次**（不只 CHANGELOG）。
- **不 push、不發版。**
- 修完把這一輪寫進 terminu repo 的 `.local/family-fix/sshu/README.md`（加一節「對照 v0.1.14–v0.1.16」，開頭的時間線與「發布」的
  commit 表一起更新），然後刪掉這份清單。
- 建議順序：第 1 條（小、獨立）→ 第 2、3 條（`Alt-Esc` 的 confirm，同一個形狀）→ 第 5、6 條（key reference 能變暗、menu 標出
  條件）→ 第 4 條（寫法；新增的 confirm 與 hint 一次寫對，測試改一輪）→ 第 7、8 條 → 第 9 條。
- patch 腳本與 commit message 先寫進檔案再執行：英文撇號會截斷 `-c '…'`。


## 1. 選取模式裡 `Tab` 跳出的 toast，第一個 `Esc` 沒有先收掉它 —— K11、F1（F3、K4）

**現況**（`internal/ui/app.go`）：

- `handleKey()` 在 `m.ssh.copy.on && !m.popupOpen()` 時把鍵整個交給 `copyModeKey()`。toast 不算 popup（`floatsOpen()` 不數它
  —— 這是對的，它不收鍵），所以 `Tab` 跳出 `Esc leaves selection mode first` 之後按 `Esc`，鍵直接進 `copyKey("esc")` →
  `copyState.key()`（`copymode.go`）：有選取就丟掉選取，沒有就離開模式。toast 留在畫面上，等 `toastLife`（2.2 秒）自己消失。
- 同一個形狀在 panel 上也有：`handleKey()` 的 `Esc` 區塊在 `closeTop()`（只有它會問 toast）之前有三個分支 —— Hosts 搜尋中的
  `m.hosts.clearFilter()`、Operation 頁的 `textPage()` 回 nav（目前遮罩中，碰不到）、file transfer 的 `clearFilter()` / `up()`。
  toast 在畫面上時按 `Esc`，這些分支先作用，toast 不收。例：file transfer 按 `R`，`Refreshed — N items` 還在時按 `Esc`，
  是回上一層目錄。

**規則**：F1 的 toast 列 ——「`Esc` 或時間到就收掉；除了 `Esc`，按鍵都穿過它」。K11 的 `Tab` 列 ——「toast 還在時，第一個 `Esc`
先收掉它（K4）」。F3、K4 本來就要求 `Esc` 先關看得到的 popup（包括 toast）；v0.1.14 把它寫進 toast 列與 K11，行為要求沒變
—— sshu 在模式裡與上面三個 panel 分支一直沒做到。

**怎麼改**：

- 不含 Alt 的 `Esc`，在交給選取模式、以及上面三個 panel 分支之前，先問 `m.toast.anim.owns()`，是就 `m.toast.close()`，這一鍵
  到此為止。選取模式那一段放在 `copyModeKey()` 被叫之前（或它的 `switch` 裡）；panel 那一段放在 `Esc` 區塊的第一個判斷。
- PTY 不動：`inPty()` 的 `Esc` 分支（裸 `Esc` 給遠端）與執行中的編輯器照舊 —— 那裡 `Esc` 屬於子程序（K10），toast 等時間到。
  注意選取模式開著時 `inPty()` 也是真的，選取模式的檢查要放在它自己的分派前面，不能靠 PTY 分支之後那一段。
- 其他鍵維持穿過 toast（已符合，見最後）。toast 的字照第 4 條改成 `[Esc] leaves selection mode first`。
- 測試（接在 `tdpmode_test.go` 的 `TestTabInSelectionModeSaysHowToLeave` 後面）：`Tab` → `Esc`：toast 開始關、模式還在、選取
  還在；再 `Esc` 才丟選取 / 離開。file transfer：toast 在時 `Esc` 不改 `cwd`；Hosts 搜尋中：先關 toast、再按才清搜尋。PTY 裡
  toast 在時 `Esc` 仍送到遠端（守 K10）。mutation：分別拿掉模式與 panel 那兩處的檢查。
- 文件：dev-remarks「選取模式」一段的「`Tab` 跳 toast 說先 `Esc`」後面補「toast 還在時，第一個 `Esc` 先收掉它」；設計文件附錄
  選取模式那一列同樣補上。README 兩份沒寫到這個細節，不用改。


## 2. 編輯器裡的 `Alt-Esc` 不問就結束 `$EDITOR`，本機副本一起刪掉 —— K10、D5

**現況**：

- `internal/ui/app.go` `handleKey()` 最前面的編輯器分支（`m.editorUI.running()`）：`Alt+Esc` 直接 `abandonEdit()`，其餘鍵全部
  `pty.write()` 給編輯器。
- `abandonEdit()`（`internal/ui/edit.go`）→ `closeEdit(false)`：停掉編輯器的 PTY、`os.RemoveAll(job.dir)`，不寫回，跳
  `Abandoned <name>`。遠端檔案時，編輯器裡已經 `:w` 存進本機副本的內容跟著副本一起刪掉；本機檔案（就地編輯，`inPlace()`）
  時，編輯器 buffer 裡還沒存的內容沒了。
- `dev-remarks.md`「運作方式 → `Alt+Esc` 與 `Alt+Enter`」自己寫了已知誤觸：vim 裡快速連按兩次 `Esc` 會被讀成 `Alt+Esc`。在
  `[e]dit` 的編輯器裡誤觸，整份編輯就沒了。

**規則**：K10 —— 家族的 PTY 出口鍵是 `Alt-Esc`（見 D5）。D5（v0.1.16）——「`Alt-Esc` 一律先 confirm：按了會讓 focus 離開 PTY
或結束子程序時，不論子程序留不留著，都先跳 confirm（`Enter` 離開、`Esc` 回到 PTY）」；理由是終端機把 Alt 組合送成「`Esc` 加
那個鍵」，app 忙的時候兩次 `Esc` 會黏成 `Alt-Esc`（tdp 用 bubbletea v1.3.10 量過：前面有鍵在排隊時，間隔 150ms 也會黏）。

**怎麼改**：

- 執行中的編輯器裡，`Alt-Esc` 改成開一個 confirm，疊在編輯器上（F4 保留 source）：問句寫出動作與對象（例：`Abandon <name>?`），
  明細說代價（遠端：不會寫回，本機副本會刪掉；本機：編輯器裡沒存的內容會丟掉）。`Enter` 才 `abandonEdit()`；`Esc` 取消、
  回到編輯器，鍵又送進 PTY。confirm 開著時子程序照跑，只是鍵不給它。參考 kbu：`internal/ui/app.go` 的 `ptyLeaveRequestMsg` →
  `ConfirmLeaveEdit`，接受才 `Kill()`。
- **「放在最上層」三處**：
  - 路由有兩個地方要動：`handleKey()` 最前面的編輯器分支吃掉每一個鍵，confirm 開著時要讓路；後面 popup 的 `switch` 裡
    `case m.editorUI.anim.owns(): return m, nil` 排在 `confirm` 之前，也會把 confirm 的鍵吃掉，要把 confirm 排到它前面。
  - `closeTop()`：confirm 已經排在 `editorUI` 之前，但它會呼叫 `declineEdit()` —— 新的 action 取消時什麼都不做（不能刪副本）。
  - 繪製：`floats()` 裡 confirm 已在 `editorUI` 之後；層數用 `above()`。
  - 編輯器跑著時不會有另一個 confirm（binary / overwrite 兩個都在編輯器沒跑的時候），可以借 `m.confirm`，加一個
    `confirmAction`（例：`confirmEditAbandon`）。
- **誤觸之後接著打的鍵**：使用者以為還在 vim 裡，可能接著打 `:wq⏎`。confirm 上的 `q` 照 K9 進離開流程，而 `quitCost()` 現在
  不算進行中的編輯（編輯器吃掉所有鍵時碰不到它）：沒有 session 與傳輸時，`q` 會直接離開。建議 `quitCost()` 加一行「進行中的
  編輯會丟掉」（D5 的離開流程：有未存的草稿時先 confirm）。實作時把「連按兩次 `Esc`，再打 `:wq⏎`」當一個情境跑一遍，看最後
  停在哪裡。第 3 條的格子也一樣（有 session 時 `q` 先問）。
- 下框 hint 照樣常駐揭露出口（K10），寫法照第 4 條：`Alt-Esc:abandon`。
- 測試：改寫 `edit_test.go` 的 `TestAltEscStillAbandonsTheEdit`（守舊規則的測試改名、反轉斷言）：`Alt-Esc` → confirm 開著、
  編輯器還在跑；`Esc` → 回到編輯器，再送一個字元會進 PTY；`Enter` → 編輯器結束、`job.dir` 不在了。另外：confirm 開著時字元
  不進編輯器；`Esc` 取消不刪副本。mutation：`Alt-Esc` 直接 abandon；編輯器分支不讓路；`switch` 裡 `editorUI` 仍排在 confirm
  之前；`declineEdit()` 對新 action 刪副本。
- 文件：README 兩份「Everywhere / 到處都通」那一行（編輯器裡的 `Ctrl-C` 屬於它、先 `Alt-Esc`）與 `[F]ile transfer` 表的 `e`
  那一列補「編輯器裡 `Alt-Esc` 放棄這次編輯，會先問」；dev-remarks「運作方式」的誤觸那段與「已知的牆與未做」的誤觸一條，
  跟第 3 條一起改寫。


## 3. 格子的 `Alt-Esc` 回清單不問 —— D5（v0.1.16）

**現況**（`internal/ui/app.go` `handleKey()` 的 `Alt+Esc` 分支）：`m.ptyFocused()` 時先 `m.ssh.unzoomOne()`，有階可退就退一階；
沒有就 `m.ssh.setFocus(panelSessions)`，不問。會走到這一下的路徑：

- 格子有鍵盤、遠端在讀（`inPty()`）。
- 連線中的格子：`ptyFocused()` 在遠端還沒開口時就為真，所以連線中按 `Alt-Esc` 一樣直接回清單。
- 窄寬（`sshNarrowW` 以下）網格佔滿畫面時，清單也是這樣叫回來的，同一條路。

`dev-remarks.md` 的誤觸說明（「意外跳出 pty；按 `Enter` 回得去」）與上一版清單都把這一下當成「只讓 focus 離開、子程序留著，
可以不問」—— v0.1.14 的 D5 是這樣寫的，v0.1.16 改了。

**規則**：D5（v0.1.16）——「`Alt-Esc` 一律先 confirm：按了會讓 focus 離開 PTY 或結束子程序時，不論子程序留不留著，都先跳
confirm（`Enter` 離開、`Esc` 回到 PTY）；在 PTY 裡面的動作（例：退一階 zoom）不用問」。user 2026-09-29 裁定：家族都用
`Alt-Esc`，不換鍵，由 sshu 補 confirm。

**怎麼改**：

- `unzoomOne()` 沒有階可退、要 `setFocus(panelSessions)` 的那一下，改成開 confirm，疊在網格上：問句寫出動作（例：
  `Back to the list?`），明細說 session 留著、之後 `Enter` 回得來。`Enter` 才 `setFocus(panelSessions)`；`Esc` 關掉 confirm、
  回到格子（`popupOpen()` 變假，`ptyFocused()` 重新成立，鍵又給遠端）。confirm 開著時遠端照跑，畫面照 F8 變暗，鍵不給遠端
  （`ptyFocused()` 在有 popup 時為假，這一段不用改）。
- 跟第 2 條同一個形狀（`Enter` 離開、`Esc` 回 PTY），可以共用一個 `confirmAction` 家族。confirm 上再按一次 `Alt-Esc`：現在的
  路由會走 `closeTop()`，等於取消、回到格子 —— 雙重誤觸落回原處，建議維持，並寫一條測試釘住。
- **不用問的**（都在 PTY 裡面，程式碼確認過）：
  - `unzoomOne()` 退一階 zoom（滿版 → 網格 zoom → 正常）。
  - 選取模式裡的 `Alt-Esc`：`copyModeKey()` 只做 `m.ssh.copy.stop()`，`focus` 仍是 `panelPty`，留在格子裡；再按一次才走上面的
    confirm。
  - 鎖住的格子：`Alt-Esc` 穿透給內層，這一層不出 confirm（內層的 sshu 照這一條自己問）。
- 格子的 footer 維持 `Alt-Esc:leave pty` / `Alt-Esc:unzoom`（寫法見第 4 條）。
- 測試：
  - 新增：格子有鍵盤時 `Alt-Esc` → confirm 開、`focus` 仍是 `panelPty`；`Esc` → confirm 關、再送一個字元會到遠端；`Enter` →
    `focus == panelSessions`、session 還活著。連線中的格子同一套。zoom 中 `Alt-Esc` 退一階、沒有 confirm；選取模式裡 `Alt-Esc`
    離開模式、沒有 confirm；鎖住的格子 `Alt-Esc` 送到內層、沒有 confirm；confirm 上再按 `Alt-Esc` 回到格子。
  - 既有的：拿 `alt+esc` 離開格子的測試要多按一個 `Enter`（建議加一個 helper）。按 `alt+esc` 的測試在 `copymode_test.go`、
    `lockmenu_test.go`、`ptyscroll_test.go`、`sftpmenu_test.go`、`sshscroll_test.go`、`sshtab_test.go`、`sshzoom_test.go`、
    `tabkeys_test.go`、`tdpmenu_test.go`、`tdprouting_test.go`：逐一看它按的是「退一階 zoom / 離開選取模式」（不變）還是
    「回清單」（要多按 `Enter`）。`sshtab_test.go` 的「連線中的格子 `Alt-Esc` 一定出得去」改成「出 confirm、`Enter` 出得去」。
  - mutation：拿掉 confirm（直接回清單）；退一階 zoom 也出 confirm；`copy.stop()` 也出 confirm；`Esc` 取消後 focus 沒回格子。
- 文件：README 兩份的 `Alt-Esc`（「Five keys / 五個鍵」表下那段、`[S]SH` 的說明與表）補「會先問」；dev-remarks「運作方式 →
  `Alt+Esc` 與 `Alt+Enter`」的誤觸段改成「跳出 confirm，`Esc` 回到 pty」、「已知的牆與未做」的誤觸一條跟著改；設計文件 §4.6
  與附錄 `[S]SH` 表的 `Alt-Esc` 列補上 confirm。


## 4. 按鍵的寫法 —— M5（v0.1.15 定案）

**規則**（M5，v0.1.15；D1–D4 的例子與 D2 的色碼跟著改）：

- **鍵名**（畫面上所有地方與 README 都一樣）：用鍵帽上的名字、大駝峰、不自創縮寫 —— `Esc`、`Tab`、`Enter`、`Space`、
  `Backspace`、`Delete`、`Home`、`End`、`PgUp`、`PgDn`；方向鍵 `↑` `↓` `←` `→`。字母照實際要按的大小寫（`q`、`A`、`Alt-z`）；
  `Ctrl` 後面的字母一律大寫（`Ctrl-C`、`Ctrl-U`）。modifier 用 `-`；幾個鍵做同一件事用 `/`（`j/k`）；範圍用 `–`（`1–9`）。
- **依位置**：label（menu 的列、statusbar chip、panel 標題）用括號標記；句子（空狀態、toast、錯誤訊息）鍵一律加方括號
  （`Press [A] or [Space]`）；hint 與 footer 寫 `鍵:說明`，冒號前後不空格、項目之間一個空格（`j/k:move Enter:run Esc:close`）；
  key reference 兩欄，鍵不加括號、不加冒號。
- **顏色**（D2）：hint、footer、key reference 的鍵 Blue `#89b4fa`；hint 與 footer 的冒號與說明 Overlay0 `#6c7086`；key reference
  的說明 Text `#cdd6f4`。

以下分塊列「現在 → 改成」。bubbletea 的鍵名（`case "alt+esc"`、`msg.String() == …`、`nav.go` 的 `"ctrl+u"`、測試送進 `Update()`
的 `"alt+v"`）不是畫面上的字，不動。

### 4.1 hint 與 footer 的畫法（兩個 renderer）

| 函式 | 現在 | 改成 |
|---|---|---|
| `popup.go` `hintLegend()`（每個 popup 的下框） | ` 鍵 說明  鍵 說明 `：pair 內一個空格、pair 之間兩個空格；鍵 `focusColor` `#89b4fa`、說明 `dimColor` `#6c7086` | ` 鍵:說明 鍵:說明 `：冒號 Overlay0（`dimColor`），pair 之間一個空格；鍵與說明的顏色已經對 |
| `chrome.go` `keyLegend()`（footer） | `鍵 說明`，pair 之間三個空格（`sep`）；`plainW()` 照這個量寬；顏色同上 | `鍵:說明`，pair 之間一個空格；`plainW()` 跟著改；顏色已經對 |

- **位置指示不是鍵**：`spaceMenu.view()`、`transfersPopup.view()`、`viewer.view()` 把 `N of M` 塞成一個 pair
  （`{itoa(top + 1), "of " + …}`），新格式下會畫成藍色的 `3:of 12`。改成不經過 pair（例：暗色的 `3 of 12` 放在鍵之前），樣子由
  app 決定。
- **寬度會變**：新格式每個 pair 比現在短（footer 每個 pair 少兩欄、hint 少一欄），窄寬時丟掉的順序跟著變。選取模式 80 欄現在
  只放到 `w/e/b`，新格式放得下到 `0/$`。§11.33、§11.47 那幾個「窄 footer 留住誰」的測試要重新量過前提（見 4.8）。

### 4.2 每一個 popup 的下框 hint

| 檔案 · 函式 | 現在 | 改成 |
|---|---|---|
| `confirm.go` `confirmPopup` 的 view | `Enter <動詞>  Esc cancel` | `Enter:<動詞> Esc:cancel`（D3 的例子） |
| `inputpopup.go` | `Enter <動詞>  Esc cancel` | `Enter:<動詞> Esc:cancel` |
| `askpass.go` | `Enter <accept>  Esc cancel connection` | `Enter:<accept> Esc:cancel connection` |
| `spacemenu.go` `spaceMenu.view()`（Space menu、global operation、lock menu、兩個 picker） | `j/k move  Enter run  Esc close`；沒有可執行的列時 `Esc close` | `j/k:move Enter:run Esc:close`（D4 的例子）；`Esc:close` |
| `transfer.go` `transfersPopup.view()`（Jobs） | `j/k move  Enter open  c cancel  Esc close` | `j/k:move Enter:open c:cancel Esc:close`（`c` 的變暗見第 5 條） |
| `viewer.go` | `j/k scroll  Esc close` | `j/k:scroll Esc:close` |
| `detail.go` | `j/k scroll  Enter <accept>  Esc close` | `j/k:scroll Enter:<accept> Esc:close` |
| `helppopup.go` `helpPopup.view()` | `j/k scroll  ? close` | `j/k:scroll ?:close` |
| `toast.go` | `Esc close` | `Esc:close` |
| `edit.go` `editorPopup.view()` | 編輯器跑著：`alt+esc abandon`；取檔 / 寫回：`Esc cancel` | `Alt-Esc:abandon`；`Esc:cancel` |
| `filepicker.go` | `↑↓ select  Enter pick  Esc cancel`（`arrowUpDown`） | `↑/↓:select Enter:pick Esc:cancel` |
| `form.go`（host 表單）、`credform.go`、`sshcfgform.go` | `Tab next  ←→ switch  Enter save  Esc cancel`（`arrowGlyphs`）；`Enter browse / choose  Tab next  Esc cancel`；`Enter save  Backspace clear  Esc cancel`；`Enter add option  Esc cancel` | `Tab:next ←/→:switch Enter:save Esc:cancel`；`Enter:browse Tab:next Esc:cancel`；`Enter:save Backspace:clear Esc:cancel`；`Enter:add option Esc:cancel` |
| `knownform.go` | `Tab next  Enter fetch  Esc cancel` | `Tab:next Enter:fetch Esc:cancel` |
| `bundlepage.go`（Operation 頁，遮罩中） | `Tab next  Enter <submit>  Esc back to the nav` | `Tab:next Enter:<submit> Esc:back to the nav` |

`arrowGlyphs`（`←→`）與 `arrowUpDown`（`↑↓`）在 `theme.go`：hint 裡要的是 `←/→`、`↑/↓`，改常數或在用的地方加 `/`。

### 4.3 footer 與 legend（`view.go`）

| 函式 · 狀態 | 現在 | 改成 |
|---|---|---|
| `footer()` panel | `space menu   ? help   1-2 M/F/S panel tab   M 1 unread error   q quit`（file transfer 是 `1-4`） | `Space:menu ?:help 1–2:panels M/F/S:tabs M:1 unread error q:quit`（file transfer `1–4`） |
| `footer()` Operation 頁（遮罩中） | `tab field   enter run   esc back` | `Tab:field Enter:run Esc:back` |
| `footer()` 格子有鍵盤 | `alt+esc leave pty`（zoom 中 `leave zoom`）、`pgup/pgdn history`、`alt+v select`、`alt+z <下一階>`、`alt+enter lock`、`alt+←→↑↓ cell` | `Alt-Esc:leave pty`（zoom 中 `Alt-Esc:unzoom`）、`PgUp/PgDn:history`、`Alt-v:select`、`Alt-z:<下一階>`、`Alt-Enter:lock`、`Alt-←/→/↑/↓:cell` |
| `footer()` 鎖住的格子 | `alt+enter release` | `Alt-Enter:release` |
| `copyLegendPairs()`（選取模式的 footer，與滿版時疊在最後一列的 legend 共用） | `? help   y copy   v/V select   alt+v leave   hjkl move   w/e/b word   0/$ line start/end   u/d half page` | `?:help y:copy v/V:select Alt-v:leave h/j/k/l:move w/e/b:word 0/$:line start/end u/d:half page` |

- panel footer 的 `1-2 M/F/S` 是兩組鍵擠在一個 pair；用 `/` 連起來會讀成「做同一件事」，建議拆成 `1–2:panels`、`M/F/S:tabs`
  兩個 pair。D1 的例子是 `Tab/1–N:panels`：Manage 與 file transfer 的 `Tab` 也換 panel，可以寫 `Tab/1–2:panels`、
  `Tab/1–4:panels`；SSH tab 上 `Tab` 不作用（偏離 K2），不寫。要不要把 `Tab` 放進 footer 由 app 決定。
- `Alt-z`：程式只認小寫（`handleKey()` 的 `string(msg.Runes) == "z"`），footer 本來就寫小寫的 `z`，保留。

### 4.4 key reference（`helppopup.go` 的 `helpPopup.view()` 與各份清單）

- **顏色**：鍵現在是 `handColor` `#bac2de`（Subtext1，sshu 自己的游標色；`theme_test.go` 已經寫明「handColor is the cursor, not a
  key」）→ Blue `focusColor` `#89b4fa`。說明 `textColor` `#cdd6f4`（Text）已經對。兩欄、鍵不加括號不加冒號，已經對。
- **鍵欄**：

| 清單 | 現在 → 改成 |
|---|---|
| `coreKeyReference` | `M · F · S` → `M/F/S`；`1-9` → `1–9`；`Ctrl+C` → `Ctrl-C` |
| `gridKeyReference` | `Alt+arrows` → `Alt-←/→/↑/↓`；`Alt+Z` → `Alt-z`；`Alt+Enter` → `Alt-Enter`；`Alt+Esc` → `Alt-Esc`；`PgUp · PgDn` → `PgUp/PgDn`；`Alt+v` → `Alt-v`；`hjkl · u · d` → `h/j/k/l/u/d`；`w · e · b` → `w/e/b`；`0 · $` → `0/$` |
| `navKeyReference` | `j · k` → `j/k`；`u · d` → `u/d`；`gg · G` → `gg/G` |
| `copymode.go` `copyModeHelp()` | `h j k l` → `h/j/k/l`；`w · e · b` → `w/e/b`；`0 · $` → `0/$`；`u · d` → `u/d`；`v · V` → `v/V`；`Alt+v` → `Alt-v`；`q · Ctrl+C` → `q/Ctrl-C` |
| `app.go` `popupHelp()` | `j · k` → `j/k`（scroll、menu、Jobs、picker 四處）；`u · d` → `u/d`；`Ctrl+C` → `Ctrl-C`（quit）；`Alt+Enter` → `Alt-Enter`（lock menu）；menu 的 `letter`（run its row at once）不是鍵名 → 例：`a–z/A–Z` |
| `app.go` `panelKeyReference()` | 從 Space menu 讀的鍵（單一字母、`Enter`）已經對 |

### 4.5 label（menu 的列）

括號標記照舊（`bracketHotkey()`），已經對 —— 例外是 `Enter` 的列。`bracketHotkey()` 對多字元的 key 原樣回傳 label，所以這幾列
的 label 沒有標記，鍵寫在說明欄開頭（`Enter . …`），或乾脆沒寫：

| 檔案 · 表 | 現在（label / 說明） | 改成 |
|---|---|---|
| `app.go` `hostActions` | `Connect` / `Enter . what it is, then in` | `[Enter] Connect` / `what it is, then in` |
| `credkeys.go` | `View` / `Enter . how this one authenticates` | `[Enter] View` / `how this one authenticates` |
| `knownkeys.go` | `View` / `Enter . the whole key, and where it sits` | `[Enter] View` / … |
| `sshcfgkeys.go` | `View` / `Enter . every option this block sets` | `[Enter] View` / … |
| `sftpkeys.go` `sftpActions` | `Enter` / `Enter . open it, or go to a result` | 例：`[Enter] Open` / `open it, or go to a result`（label 要是動作名） |
| `sshkeys.go` `sshActions` | `Open` / `Enter . show and take the keyboard` | `[Enter] Open` / `show and take the keyboard` |
| `app.go` `menuItems()` Connections、Changes | `Open in full` / `every entry, whole`（鍵沒有標出來） | `[Enter] Open in full` / `every entry, whole` |

M5 的 label 表（多字元、不在字裡就放前面）與 D4（`[Enter] Edit`）v0.1.15 沒改，sshu 一直沒照做，這一輪一起改：`bracketHotkey()`
認得 `enter`、印成 `[Enter] `。`panelKeyReference()` 拿 `strings.ToLower(it.label)` 當說明，改完照樣可用（不要把 `[Enter]`
一起放進說明）。

### 4.6 句子裡的鍵（空狀態、toast、錯誤訊息、menu 的說明列）

| 檔案 · 函式 | 現在 | 改成 |
|---|---|---|
| `hosts.go` `hostsModel.emptyState()` | `Press [A] to add a host, or Space to see what you can do here` | `…, or [Space] to see what you can do here` |
| `knownlist.go` `knownModel.view()` | `Press [A] to fetch a host's key, or Space to see …` | `…, or [Space] to see …` |
| `sshcfglist.go` `sshcfgModel.emptyState()` | `Press [A] to add one, or Space to see …` | `…, or [Space] to see …` |
| `credlist.go`、`sftpview.go`（`fileRows()`、`marksPanel()`、`noHostBody()`） | `[A]`、`[H]`、`[a]` 已經加括號 | 已經對 |
| `sshtab.go` `sshModel.gridEmpty()` | `Tab in [1] toggles a session's cell — Enter shows one and takes the keyboard` | `[H] in [1] shows or hides a session's cell — [Enter] shows one and takes the keyboard`（`Tab` 也過時，見第 7 條） |
| `app.go` `copyModeKey()` 的 toast | `Esc leaves selection mode first` | `[Esc] leaves selection mode first` |
| `app.go` host 表單的錯誤列 | `Choose a credential (Enter opens the list)` | `Choose a credential ([Enter] opens the list)` |
| `splash.go` | `Press Esc to close` | `Press [Esc] to close` |
| `app.go` `menuItems()` nav 的說明列 | `j/k choose a section — Enter opens it` | `[j/k] choose a section — [Enter] opens it` |
| `app.go` `menuItems()` Operation 頁（遮罩中） | `a form — letters type, Enter runs it` | `a form — letters type, [Enter] runs it` |
| `sshkeys.go` `sshMenuItems()` 格子 | `alt+esc comes back · hold alt, arrows switch cells` | `[Alt-Esc] comes back · [Alt-←/→/↑/↓] switch cells` |
| `sshkeys.go` `sshMenuItems()` layout | `j/k choose an arrangement — it applies as you move`；`Enter on custom asks for rows × columns` | `[j/k] choose an arrangement — it applies as you move`；`[Enter] on custom asks for the number of columns`（欄數見第 7 條） |

`emptyHint()` 的 keys 參數跟著換（`"Space"` → `"[Space]"`）。

### 4.7 README（兩份同一批位置，鍵的寫法相同）

| 段落（EN / 繁中） | 現在 | 改成 |
|---|---|---|
| Highlights / 特色 | `PgUp` / `PgDown`；`Alt+v`；`Alt+Z`、`Alt+Enter` | `PgUp/PgDn`；`Alt-v`；`Alt-z`、`Alt-Enter` |
| Requirements / 需求 | `Alt+…`、`Alt+Esc`、`Alt+v`、`Alt+Z` | `Alt-…`、`Alt-Esc`、`Alt-v`、`Alt-z` |
| Five keys / 五個鍵 表的 `Tab` 列 | `on the ssh tab use `1` `2`` / `ssh tab 用 `1` `2`` | `1–2` |
| 同一節表下那段 | **`M` / `F` / `S`**；`Alt+Esc` | **`M/F/S`**；`Alt-Esc`（會先問，第 3 條） |
| The three tabs / 三個 tab 的 `[S]SH` | `` `Alt`+arrows `` / `` `Alt`+方向鍵 ``；`Alt+Z`；`Alt+Esc` | `Alt-←/→/↑/↓`；`Alt-z`；`Alt-Esc` |
| Key bindings / 按鍵 開頭 | `[A]dd` is shift+A / 是 shift+A | `Shift-A` |
| Everywhere / 到處都通（code block） | `M / F / S`；`j k    u d (half page)     gg G`；`Ctrl+C` 兩處；`Alt+Esc` | `M/F/S`；`j/k    u/d (half page)    gg/G`；`Ctrl-C`；`Alt-Esc` |
| `[M]anage` 的 host 表單 | `Tab` / `Shift+Tab` / `↑` `↓`；`←` `→` | `Tab/Shift-Tab/↑/↓`；`←/→` |
| `[F]ile transfer` 表 | `h` `l`；`c` / `C` | `h/l`；`c/C` |
| `[S]SH` 表 | `PgUp` / `PgDown`；`Alt+Z`；`Alt+Enter`；`Alt+arrows` / `Alt+方向鍵`；`Alt+Esc`；`Alt+v`；表下（`Alt+v`） | `PgUp/PgDn`；`Alt-z`；`Alt-Enter`；`Alt-←/→/↑/↓`；`Alt-Esc`；`Alt-v`；（`Alt-v`） |
| 選取模式表 | `h` `j` `k` `l`；`w` / `e` / `b`；`0` / `$`；`u` / `d`；`v` / `V` | `h/j/k/l`；`w/e/b`；`0/$`；`u/d`；`v/V` |
| layout 那句 | `j` / `k` | `j/k` |

開頭那句「`Tab` / `Enter` / `Esc` / `Space` / `?` drive everything / 驅動一切」是五個不同的鍵的列舉，不是「做同一件事」，可以留。

### 4.8 測試

- 斷言畫面字串的，跟著改：`askpass_test.go`（`Enter yes`）、`copymode_test.go`（footer 的 `alt+v`、`hjkl`）、`formenter_test.go`
  （`Enter save`）、`lockmenu_test.go`（`alt+enter`）、`nav_test.go`（`u · d`、`Alt+arrows`、`Alt+Enter`、`Alt+Esc`、
  `PgUp · PgDn`）、`popup_test.go`（`Esc close`）、`ptyscroll_test.go`（`alt+esc`）、`sshtab_test.go`（footer 與 Space menu 說明列
  的 `alt+esc`）、`sshzoom_test.go`（`alt+esc`、`? help`、`hjkl`、`alt+v`、`alt+z`）、`tabkeys_test.go`（`alt+esc`、`M/F/S`）、
  `tdpmenu_test.go`（`gg · G`、`Alt+Esc`）、`tdpmode_test.go`（`h j k l`、`w · e · b`、`0 · $`、`u · d`、`v · V`、`Alt+v`）、
  `tdprouting_test.go`（`alt+esc`）；fixture：`theme_test.go` 的 `keyLegend` / `hintLegend`、`empty_test.go` 的 `emptyHint`（`Space`）。
- **否定的斷言一定要一起換**，否則字串一改就永遠是綠的：`formenter_test.go` 的「不含 `Enter next`」、`tdpmenu_test.go` 的
  「hosts panel 的 reference 不該有 `Alt+Esc`」與「popup 的 help 不該有 `Alt+Esc`」、`sshzoom_test.go` 的「滿版不畫 footer
  （不含 `alt+esc`）」、`copymode_test.go` 的「legend 不含 `alt+esc`」與「pty 列不含 `v/V`」、`tabkeys_test.go` 的「pty footer
  不含 `M/F/S`」。
- **寬度**：`copymode_test.go` 的 `TestThePtyFooterOffersTheWayIn`（40 欄）與 `TestTheWayOutSurvivesAnEightyColumnSelectionRow`
  （80 欄）、`sshzoom_test.go` 的 zoom 標籤測試都假設「幾欄放得下幾個 pair」；新格式比較短，重新確認它們仍在量想量的東西
  （例：80 欄那條要的是「放不下全部」，確認 `u/d` 仍然被擠掉）。
- **顏色**：`theme_test.go` 的 `TestALegendKeyIsBlueEverywhere` 加兩件事：冒號是 Overlay0；key reference 的鍵是 Blue、不是
  `handColor`。
- mutation：`hintLegend()` / `keyLegend()` 改回空格分隔；冒號改成鍵色；key reference 的鍵改回 `handColor`；任一處改回 `+` 或 `·`。

### 4.9 建議一起對齊（規則沒要求，以下是建議）

- `docs/dev-remarks.md`：鍵的寫法照上面同一套（`+` → `-`、`Alt+Z` → `Alt-z`、`PgDown` → `PgDn`、`shift+A` → `Shift-A`、`·` 分隔的
  鍵 → `/`）。出現的地方：「運作方式」的 `### Alt+Esc 與 Alt+Enter` 標題與底下兩段、「zoom 與巢狀」、「選取模式（`Alt+v`）」標題、
  「歷史與 `PgUp` / `PgDown`」（含 `Ctrl+L`）、「連線與失敗」的 `Ctrl+C`、「設計決定 → 按鍵與 menu」的 `shift+A`（跟 README 那句是
  同一件事，建議一定跟著改）與 `Alt+Esc`、「PTY 裡的鍵」整段（含 footer 引文 `alt+enter release`）、「已否決，不要重提」的
  `Alt+p/f/s` 與 `Alt+1..9`、「已知的牆與未做」的 `Alt+Esc` 誤觸。
- `docs/sshu-ui-design.md`：描述現況的章節 —— §0、§A、§1、§4（含 §4.4 hint 的寫法、§4.6 的標題）、§5、§6、§7 與附錄「按鍵全表」。
  §10 開發順序與 §11 各節是帶版號與日期的歷史，不改。
- `CHANGELOG.md`：已發版的各節不改。`[Unreleased]` 還沒發，裡面 4 處（`Ctrl+C` 三處、footer 那條的 `alt+esc`）建議一起換，
  發版時 release notes 才跟畫面一致；這一輪的新條目直接照 M5 寫。
- 程式註解裡的鍵名不在 M5 範圍，不必改。


## 5. `?` 的 key reference 沒辦法把按不到的鍵變暗 —— M6（M4）

**現況**：

- `helpEntry`（`internal/ui/helppopup.go`）只有 `key`、`desc`；`helpPopup.view()` 一律用亮的鍵色與字色畫。
- file transfer：傳輸進行中，`[H]ost`、`[D]isconnect` 在 Space menu 裡變暗、按了不作用（`sftpMenuItems()` 的
  `disabled: busy && a.needsIdle`）。`panelKeyReference()`（`app.go`）從同一份 menu 讀列，`selectable()` 不排除 disabled，所以
  這兩列照樣列出、但是亮的 —— 看起來按得了。
- Jobs 的 key reference（`popupHelp()` 的 `transfersUI` 那一支）固定列 `j · k`、`Enter open this job`、`c cancel this job`。
  游標那一筆已經結束時 `c` 不作用（`transferModel.cancelJob()` 只取消 `xferRunning` 的），照樣亮著；一筆 job 都沒有時，
  `j · k`、`Enter`、`c` 都沒有對象，照樣列著（下框 hint 在沒有 job 時已經只剩 `Esc close`）。
- SSH tab 的 key reference 接的是全 app 共用的 `coreKeyReference`，裡面有 `Tab next panel in this tab`；SSH tab 上 `Tab`
  不作用（`panelKey()` 的 `tabSSH` 分支，dev-remarks「偏離 tdp」的 K2）。
- `[2]` layout：Space menu 只有說明列（`sshMenuItems()`），所以 key reference 沒有這個 panel 的鍵。`Enter` 只在游標停在 custom
  時問欄數（`sshModel.layoutKey()`），key reference 裡既沒有列、也談不上變暗（這一半是 M4 的缺）。

**規則**：M6 ——「`?` 的 key reference 照同一套：對象存在、現在不能按的鍵照樣列出、變暗；對象不存在就不列。下框 hint 與
footer 空間有限、常駐畫面，只列現在按得了的鍵也可以，由 app 決定」。M4 —— panel 上至少列出 core key 與這個 panel 能按的鍵。

**怎麼改**：

- `helpEntry` 加一個變暗的旗標；`helpPopup.view()` 對它用 dim 的鍵色與字色（跟 `spaceMenu.view()` 的 disabled 列同一個
  register）。亮的列照第 4 條用 Blue 的鍵、Text 的說明。
- `panelKeyReference()`：從 menu 讀列時帶上 `it.disabled`。
- Jobs：`c` 在游標那一筆不是 `xferRunning` 時變暗；沒有 job 時只列 `Esc`、`?`。下框 hint（app 決定）：游標那一筆已結束時
  `c:cancel` 現在亮著 —— 變暗或拿掉，二選一。
- SSH tab 不列 `Tab`：它在這個 tab 不是「現在不能」，是沒有這件事。`coreKeyReference` 把 `Tab` 那一列拆出來，依 tab 決定。
- layout：key reference 加 layout 的鍵（`j/k` 換排列、`Enter` 在 custom 上問欄數），游標不在 custom 時 `Enter` 變暗。
- footer 現在只列按得了的（`PgUp/PgDn` 要 `canScroll()`、`Alt-v` 要 `inPty()`、未讀錯誤要有），M6 允許，維持。
- `ssh grid` 那一段不動，見「已經符合」。
- 測試：傳輸進行中 file transfer 的 `?`：`H`、`D` 在、而且是 dim 的樣式（量顏色要開 truecolor，預期值寫死）；Jobs 游標在
  已結束的 job 上時 `c` 是 dim；空的 Jobs 沒有 `c`；SSH tab 的 key reference 沒有 `Tab`；layout 的 `Enter` 隨游標變暗。mutation：
  拿掉 `disabled` 的傳遞；`c` 永遠亮；SSH tab 照樣接 `Tab`。


## 6. file transfer 有條件的動作：menu 不變暗、按了跳原因 —— M6

M6 的 menu 那一半在 v0.1.14 以前就這樣要求，前幾輪只修了 `[H]ost`、`[D]isconnect`。v0.1.14 讓 `?` 照同一套之後一起浮出來：
key reference 從 menu 讀列，menu 標不出條件，第 5 條就沒有東西可以變暗。

**現況**（`internal/ui/sftpkeys.go`；`sftpApplicable()` / `appliesTo()` 只看「有沒有 host、有沒有游標那一列」）：

| 鍵 | 條件 | 現在 |
|---|---|---|
| `a` | 游標那一列還在被寫入（`transfers.arrivals().receiving()`） | 列是亮的；按了關整疊、跳 `Still arriving — not all of it is here yet`（`sftpToggleMark()`） |
| `e` | 游標那一列是目錄，或不是一般檔案 | 列是亮的；按了跳 `Cannot edit a directory` / `Not a regular file`（`sftpEdit()`） |
| `t`、`T` | 另一側還沒有 host | 列是亮的；按了關整疊、跳 `The other side has no host yet`（`sftpQueue()`） |
| `T`、`X`、`C` | 這一側沒有 mark | 列照樣出現；`T`、`X` 跳 `Nothing marked on this side`（`sftpSendMarks()`、`sftpDeleteMarks()`），`C` 什麼都不做 |

**規則**：M6 ——「對象不存在：那一列不出現」；「對象存在、但現在不能執行：列照樣出現、變暗，說明欄維持原本那句，不另外寫原因；
按 `Enter` 或熱鍵都不作用」；key reference 照同一套（第 5 條）。

**怎麼改**：

- `a`（還在寫入）、`e`（目錄 / 非一般檔案）、`t` / `T`（另一側沒有 host）：`sftpMenuItems()` 標 `disabled`；`sftpKey()` 對
  disabled 的列不作用、不跳 toast —— 跟 `needsIdle` 同一個守門，熱鍵與 menu 走同一個判斷（M3）。判斷用畫面上已有的資料
  （清單的 `IsDir`、arrivals），不要為了畫 menu 去 `Stat`。
- `T`、`X`、`C`（沒有 mark）：對象不存在，列不出現（`appliesTo()` 多問一個「有沒有 mark」），熱鍵也不作用。
- `Nothing under the cursor` 那幾個 toast 碰不到（沒有游標那一列時 item 動作本來就不在清單裡），不用動；`Cannot read <name>`
  是 I/O 失敗（F5），留著。
- 文件：dev-remarks「`[F]ile transfer`」的「正在被寫入的檔案不能 mark」一條、設計文件附錄 file transfer 表的「拒絕並說明」與
  「跳出『先去 `[J]obs` 取消』」（後者 §11.55 起就不對了）一起改；README 兩份的「傳輸進行中 `H` 與 `D` 變暗」那句可以擴成
  「現在按不了的列會變暗」。
- 測試：每一個條件各一條（menu 有這一列而且 disabled、熱鍵不作用也沒有 toast、`?` 裡是 dim）；沒有 mark 時 `T` / `X` / `C`
  不在 menu、也不在 `?`。mutation：拿掉任一個條件。


## 7. 空網格與 layout 的提示說的是舊的鍵 —— 文件正確性（順帶發現，非這幾版的改動）

**現況**：

- `sshtab.go` `sshModel.gridEmpty()`：`Tab in [1] toggles a session's cell — Enter shows one and takes the keyboard`。§11.56 起
  `Tab` 在 SSH tab 不作用，顯示 / 隱藏格子是 `H`（`[H]ide`）。
- `sshkeys.go` `sshMenuItems()` 的 layout 說明列：`Enter on custom asks for rows × columns`。§11.31 起只問欄數
  （`applyGridDims()`、`Columns for the grid, 1-9`）。

**怎麼改**：兩句的新寫法在第 4 條 4.6（鍵加方括號、`Tab` 換 `[H]`、只問欄數）。目前沒有測試斷言這兩句；補一條量新句子的。


## 8. Errors 的 `Enter` 不在 Space menu，`?` 也沒列 —— M3、M4（順帶發現，非這幾版的改動）

**現況**：`preftab.go` 的 journal 分支：Errors 上 `Enter` 開那一筆的全文（`openErrorDetail()`），README 兩份也這樣寫。可是
`menuItems()`（`app.go`）給 Errors 的 menu 只有 `Clear errors`（`Open in full` 只給 Connections、Changes）；`panelKeyReference()`
從 menu 讀，所以 `?` 也沒有 `Enter`。

**規則**：M3 —— 每個 item operation 都在該 panel 的 Space menu 裡；M4 —— panel 的 key reference 列出這個 panel 能按的鍵。

**怎麼改**：Errors 有一筆以上時，item operation 區加一列，照第 4 條 4.5 寫成 `[Enter] Open`（說明例：everything the far end
said），key 是 `enter`；key reference 自然跟上。測試：Errors 的 Space menu 有這一列，從 menu 執行等於按 `Enter`；`?` 列出 `Enter`。


## 9. zoom max 時看不到出口鍵 —— K10、M1（順帶發現，已定案）

**現況**：`view.go` `View()`：`m.ssh.maxed()` 時整個畫面只畫 `m.panel()`，footer 不畫；滿版時唯一的 overlay 是右上角的 badge
（`sshModel.zoomBadge()`：`sshu`、層數、鎖頭），選取模式另有疊在最後一列的 legend。格子有鍵盤、又在 zoom max 時，`Alt-Esc`
在畫面上哪裡都看不到。設計文件 §11.47 當時刻意不留（「滿版沒有 footer，所以最後一跳無標示，跟 `Alt+Esc` 一樣」），而
dev-remarks「PTY 裡的鍵」寫的是「格子有鍵盤時 footer 常駐列出這些和絃」。

**規則**：K10 —— 出口鍵在 focus 位於 PTY 時常駐揭露。

**已定案**（user 2026-09-29）：在滿版的 badge 旁疊一個 `Alt-Esc` 的 hint，跟 badge 同一套 overlay —— 蓋掉輸出的幾個 cell、
不佔任何一列；每層畫在同一個座標、互相蓋住，不隨深度累積。

**怎麼改**：

- 寫法照第 4 條的 hint（`鍵:說明`，鍵 Blue、冒號與說明 Overlay0）：**`Alt-Esc:unzoom`**（user 2026-09-29 裁定）。zoom max 裡按
  `Alt-Esc` 是 `unzoomOne()` 退一階，不是離開格子；`leave` 留給真的離開 PTY 的那一下（`Alt-Esc:leave pty`），zoom 裡的說明不用
  `leave` 這個字。footer 在 zoom 中原本的 `leave zoom` 也改成 `unzoom`（第 4 條）。
- 鎖住的格子：`Alt-Esc` 穿透給內層，這一層的出口是 `Alt-Enter`，照 footer 寫 `Alt-Enter:release`。
- 位置：badge 在右上角（§11.47：最下面一列是正在輸入的那一行，不能蓋）；hint 貼在 badge 左邊或下面，選一個不會隨層數改變寬度
  的位置（badge 的層數位數會變）。選取模式時最後一列已經有 legend，不重複。
- 測試：zoom max 的畫面含 `Alt-Esc:unzoom`、行數仍是終端機高度、每一列寬度不變（frame 不變量）；鎖住時是
  `Alt-Enter:release`；`sshzoom_test.go` 的「滿版不畫 footer（不含 `alt+esc`）」要改寫 —— 現在畫面上會有 `Alt-Esc`，改成量
  「沒有 tab 列、沒有 footer 的其他 pair」。mutation：拿掉 overlay；鎖住時仍寫 `Alt-Esc`。
- 文件：dev-remarks「PTY 裡的鍵」的揭露那句改成「格子有鍵盤時 footer 常駐列出這些和絃；滿版沒有 footer，badge 旁疊
  `Alt-Esc:unzoom`（鎖住時 `Alt-Enter:release`）」；設計文件 §11.47 那句「最後一跳無標示」後面記一句新的決定（新的一節
  寫理由，§11.47 本身是歷史，不改寫）。


## 已經符合、不用修的（對照 v0.1.14–v0.1.16 的改動）

- **F1、F8：toast 除了 `Esc` 不收鍵。** toast 不在 `floatsOpen()` 裡，`popupOpen()` 不會因為它變真：toast 在畫面上時，`Space`
  照樣開 Space menu、`?` 開 key reference、字母與數字照常作用。`composeFloats()` 用的 `floats()` 不含 toast，它不讓任何東西
  變暗；`closeTop()` 在 askpass 之後就問它。`Esc` 沒先收它的地方見第 1 條。
- **PTY 裡的 toast**：格子（`inPty()`）與執行中的編輯器裡，裸 `Esc` 給子程序、toast 等時間到 —— K10 的按鍵屬於子程序，
  不算違反 F1。
- **K10 的出口鍵是家族的 `Alt-Esc`**：格子與編輯器都用 `Alt-Esc`，常駐揭露在 footer 與編輯器的下框（寫法見第 4 條，zoom max
  見第 9 條）。要不要先問見第 2、3 條。
- **鎖住的格子**：除了 `Alt-Enter`，所有鍵（含 `Alt-Esc`）穿透到內層，這一層不出 confirm；`Alt-Enter` 要不要問由 app 決定
  （D5），不列。
- **zoom 不是模式**：zoom 只在格子有鍵盤時存在（`zoomAt()` 先問 `inCell()`）；放大之後沒有任何鍵換意思 —— 裸鍵照樣給遠端，
  裸 `Esc` 也給遠端、不退 zoom；由它自己的 `Alt-z` 循環回到不放大（`nextZoom()` 最後一站是 `zoomOff`）。`Alt-Esc` 一次退一階
  （`unzoomOne()`）是出口鍵在 PTY 裡的做法（D5：PTY 裡的動作不用問），不是把 zoom 當模式。程式、README、dev-remarks 都沒把
  zoom 叫「模式」；選取模式才是（K11）。
- **K11 的其他列**：選取模式裡 `Space` 不作用、`?` 是模式的 key reference（`copyModeHelp()`）、`q` / `Ctrl-C` 走 `startQuit()`、
  `Tab` / `Shift-Tab` 跳 toast、footer 由 `?` 開頭（`copyLegendPairs()`）—— v0.1.10 修過，這幾版沒改這幾列。
- **M5 的 label 與空狀態的方括號**：menu 的列（`bracketHotkey()`，`Enter` 的列見 4.5）、tab chip（`[M]anage`）、panel 標題
  （`[1]`）用括號；空狀態的 `Press [A] …`、`[H]`、`[a]` 已經照 v0.1.15 的「句子裡的鍵加方括號」（`Space` 見 4.6）。
  hint、footer 的鍵色 Blue 與說明色 Overlay0、key reference 的說明色 Text、key reference 兩欄不加括號，已經對。
- **M6 的 `ssh grid` 那一段**（v0.1.16 新增的一句）：SSH tab 的 key reference 另外加標題、說明格子（別的 surface，格子裡 `?`
  屬於遠端、看不到 key reference）的鍵，照亮顯示 —— sshu 現在就是這樣（`gridKeyReference`，標題 `ssh grid`）。user 2026-09-29
  裁定維持亮的，v0.1.16 寫進 M6。
- **M6 的「對象不存在就不列」**：Hosts、Credentials、Config、KnownHosts 沒有游標那一列時 item 動作不出現（`hostsApplicable()`
  的 `needsHost` 等）；file transfer 沒有 host 時只有 `[H]ost`（`appliesTo()`）；journal 空的時候沒有 `Clear`；SSH tab 沒有
  session 時 `H` / `C` / `D` / Close all sessions 不出現；明細沒有 offer 時 key reference 不列 `Enter`（`popupHelp()` 的 detail
  分支）。例外見第 6 條（沒有 mark 的 `T` / `X` / `C`）。
- **global operation popup 與 lock menu 的 key reference**：列的是這個 menu 怎麼操作（`j/k`、`Enter`、字母），不逐列；變暗的列
  在 menu 裡已經畫暗（目前所在的 tab、lock menu 的另一個狀態）。


## 待確認

沒有。上一版的三題 user 2026-09-29 都裁定了：鍵名的大小寫與拼法由 v0.1.15 的 M5 回答（第 4 條）；`ssh grid` 那一段維持亮的，
v0.1.16 寫進 M6（已經符合）；zoom max 在 badge 旁疊 `Alt-Esc` 的 hint（第 9 條）。
