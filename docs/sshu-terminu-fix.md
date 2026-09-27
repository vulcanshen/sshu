# sshu — terminu fix

sshu 尚未符合 [terminu design principle](https://github.com/vulcanshen/terminu/tree/v0.1.1/principle)（tdp v0.1.1）的地方，逐條待修。
修好一條就刪掉一條，並同步 README（兩份）、`docs/sshu-ui-design.md` 與 `?` help（`helppopup.go` 的 `helpContent`）裡描述該行為的段落。有意不修的，改寫成
`dev-remarks.md`「偏離 tdp」的一條並附理由。

盤點日期：2026-09-26。行號以當天的 `main` 為準（檔案都在 `internal/ui/`）。

**2026-09-27 裁定**：原本的第 1 條（SSH tab 的 `Tab`）、第 2 條（表單的 `Enter`）與「待確認」（格子上只有熱鍵的動作）保留 sshu 現在的做法，改寫成 `dev-remarks.md`「偏離 tdp」；條號不重排。同日對照 tdp v0.1.1 補上第 15、16 條，並修正第 7、13 條。


## 先看：locku、webu 修完的經驗（2026-09-27 更新）

locku（v0.1.2、v0.1.3）與 webu（v0.4.0）已照 tdp 修完，修的時候發現這些，這裡的各條也適用：

- **每修一條補一個 model test**，測試名稱或註解寫明 tdp 條目（locku `internal/ui/app_test.go`）。
- **`?` menu 的 global 列也適用 F4**：選了會開下一個框（confirm、輸入、選項）的列，`?` menu 留在底下，
  取消回到它，完成才清掉整疊。浮層因此最多三層（menu、它開出的框、框上的 `?` help），D2 的層色要夠用。
- **K9 的離開流程**：離開的 confirm 開著時，`Ctrl-C` 直接離開；`q` 在離開 confirm 上不再疊一個。
- **S3**：splash 的判斷放在 `Ctrl-C` 之前，任何鍵都只關 splash。
- **F3**：判斷「popup 還在不在」用開啟中或已開（`owns()` 一類），不用含關閉中的 `isActive()`；toast 也一樣。
- **刻意保留的行為寫成偏離**：locku 的全螢幕預覽「任何鍵就回來」、欄位字典取代 `?` menu，都寫進
  `dev-remarks.md`「偏離 tdp」並附理由，而不是留在 fix 檔。
- **文件裡的 tdp 連結釘在 tdp 的版本 tag（目前 `v0.1.1`）**（本檔與 README、dev-remarks 已改好）。
- **修完一批就更新 CHANGELOG、發一版**，README 與 `ux.md` 等描述行為的段落同一個 commit 改。
- **（webu）動手前先把 fix 清單互相對一遍。** 清單裡的條目可能互相衝突（webu 同時要「global 列標 `[?]`」與
  「popup 上的 `?` 顯示該 popup 的 help」—— Space menu 也是 popup，只能成立一條）。
- **（webu）行號很快就過期。** 前幾條一改，後面引用的行號全部位移；換註解時用內容比對，不要照行號。
- **（webu）F4 可以統一在按鍵路由處理**：記下按鍵前最上層的等級（menu、options、動作開出的框），按鍵後若不是
  `Esc`、最上層掉了一級以上，就關掉底下的 menu。執行 menu 的一列時，在 **dispatch 回傳的 model** 上判斷有沒有
  開出新框，不要在舊 model 上關（會關在沒人回傳的副本上；locku 也踩過 value receiver 的同類陷阱）。
- **（webu）改完一條就回頭檢查其他畫面的提示。** `Space` 不再關 popup 之後，下框的 `Space close` 要跟著改。
- **（webu）`bracketHotkey` 會重複加括號**：label 已寫出鍵的列（`[/] Search`）變成 `[/] [/] Search`。
- **（webu）每個 panel 的 Space menu 都接上 global 區之後，原本只有一區的 menu 要補上區塊標題**（tdp v0.1.1 M2）。
- **（locku）打 tag 前先 `git branch --show-current`**：locku 0.1.2 的 tag 打在分支上，推上去的 `main` 沒有變動。
- **tdp v0.1.1 新增 K11「模式」**：模式裡 `Space` 列出模式的鍵（可執行）、`?` 是模式的 help、`Esc` 離開、`q` /
  `Ctrl-C` 照 K9、`Tab` 可暫停但要有回應。D3、D4 也補了路由（help 在最上層）與 `key reference` 不可選的做法。
- 完整紀錄：terminu repo 的 `.local/family-fix/locku/`、`.local/family-fix/webu/`（本機）。

---

## 3. `Space` 會關掉任何 popup —— K5、F6

- **現況**：`app.go` `handleKey()` 第 683 行，`msg.Type == tea.KeySpace && m.popupOpen() && !m.textFloat()`
  就 `closeTop()`：confirm（等於取消）、明細、viewer、help、Jobs、Alt+Enter 的 lock menu……全部會被 `Space` 關掉。
- **規則**：`Space` 只開關它自己開的 Space menu；其他 popup 上按 `Space` 不作用，它們由 `Esc` 或自己的流程關閉。
- **怎麼改**：這個分支只在最上層是 `spaceMenu` 時關閉；其他 popup 上吃掉 `Space`、不作用。
  README 兩份「五個鍵」表的 `Space`（「也用來關掉任何浮層」）、`sshu-ui-design.md` §4.2.1 一起改。

## 4. 沒有 host 的那一側，`Space` 直接開 host 清單 —— K5、M7

- **現況**：`app.go` `panelKey()` 第 948 行，`tabFT` 且 `m.sftp.cur().fs == nil` 時 `Space` 轉呼
  `sftpKey(keySelectHost)`，不開 Space menu（「一列的 menu 不是 menu」，`sshu-ui-design.md` §11.16）。
- **規則**：panel 上 `Space` 一律打開 Space menu；只有一列時照樣是 menu，選一列 `Enter` 才打開下一個 popup。
- **怎麼改**：拿掉這個捷徑，`Space` 開出只有 `[H]ost`（加上 global operation，見第 7 條）的 Space menu；
  `H` 熱鍵照舊直接開 host 清單。`dev-remarks.md`「`[F]ile transfer`」那一條與 §11.16 一起改。

## 5. `?` 疊在 popup 上時顯示整個 app 的 help —— K6

- **現況**：`app.go` 第 686–693 行，`?` 不論最上層是什麼都打開同一個 `helpPopup`，內容是 `helpContent`
  （整個 app 的按鍵）；註解明說「It opens from ON TOP of another float too」。
- **規則**：focus 在 popup 上時，`?` 只顯示**這個 popup** 的 help：這個框裡能按什麼、做什麼。
- **怎麼改**：`?` 先看最上層的 popup，各給一份自己的 help（Space menu / picker：`j/k`、`Enter`、熱鍵、`Esc`；
  confirm：`Enter <動詞>`、`Esc`；表單：`Tab` / `Shift-Tab`、`Enter` save、`←` `→`（Auth）、`Backspace`、`Esc`；
  viewer / 明細：捲動鍵、`Enter`（有 offer 時）、`Esc`；Jobs：`j/k`、取消鍵、`Esc`）。

## 6. `?` 是唯讀 viewport，沒有可執行的 global operation 區 —— M4、K9

- **現況**：`helppopup.go` 是唯讀 viewport（`update()` 只捲動），內容是 `helpContent`：Core keys、Global
  （`q quit`、`Ctrl+C force quit`）、ssh grid、Navigate。
- **規則**：focus 在 panel 上時，`?` 打開的是 `?` menu：
  1. `global operation` 區，可以直接執行（`j/k`、`Enter`、熱鍵），離開 app 必須在這裡；
  2. `key reference` 區，唯讀，至少列出 core key。
- **怎麼改**：`?` menu 改成「上半可執行、下半唯讀」。sshu 的 global operation：`[M]anage`、`[F]ile transfer`、
  `[S]SH`（切 tab）與 `[q]uit`。`helpContent` 的 Core keys、ssh grid、Navigate 留在 `key reference`
  （ssh grid 那組照舊要列：格子裡按不出 help）。建議把全域動作定義成一份清單，與第 7 條共用。

## 7. Space menu 沒有 global operation 區 —— M2、M7

- **現況**：各 panel 的 Space menu 只組 `item operation` 與 `panel operation`：`app.go` `menuItems()`（第 1558 行）、
  `sftpkeys.go` `sftpMenuItems()`（第 196 行）、`sshkeys.go` `sshMenuItems()`（第 253 行）、
  `credkeys.go` `credsMenuItems()`（第 66 行）、`knownkeys.go` `knownMenuItems()`（第 71 行）、
  `sshcfgkeys.go` `sshcfgMenuItems()`（第 68 行）。manage 的 nav、`[2]` layout、空的 Logs 只有說明用的 header 列；
  沒有 session 時 `[1]` sessions 的 menu 是空的，沒有一句「沒有可做的事」。
- **規則**：Space menu 第三區 `global operation`，列出全部全域動作，與 `?` menu 的 global operation 同一份清單、
  同一順序；沒有可執行的動作時照樣打開，顯示「沒有可做的事」與關閉方式。
- **怎麼改**：每個 menu 最後接上 `global operation` 區（切 tab 三列與 `[q]uit`）。panel 上的 Space menu
  **一律加區塊標題**（tdp v0.1.1 M2：global 區永遠在，至少兩區），原本只有一區的扁平 menu 要補上自己那一區的標題。
  加上 global 區之後不會再有空 menu，但 item / panel 都沒有時，前面仍要有一句「這裡沒有可做的事」。

## 8. 不能執行的列按了會跳原因 —— M6

- **現況**：`sftpkeys.go` `sftpKey()` 第 165–167 行，傳輸進行中對 `needsIdle` 的列（`[H]ost`、`[D]isconnect`）
  按 `Enter` 或熱鍵，toast 顯示 `transferBusy()` 的原因（`… still moving — cancel in [J]obs first`）。
  `app.go` `lockMenuKey()` 第 1524–1535 行，lock menu 上 disabled 的 `Lock PTY` / `Release PTY` 按下去 toast
  `Already locked` / `Not locked`。`spacemenu.go` 第 20–26 行的註解把「still answers when pressed」寫成設計。
- **規則**：列照樣出現、變暗；說明欄維持原本那句，不另外寫原因；`Enter` 與熱鍵都不作用。
- **怎麼改**：disabled 的列在 `Enter` / 熱鍵時直接 `return m, nil`（`sftpKey` 的 guard、`lockMenuKey` 的兩個 case）。
  `spacemenu.go` 的註解、README 兩份「傳輸進行中不能用 `H` 和 `D` —— 先到 `J` 取消」、`dev-remarks.md`
  「`[F]ile transfer`」那一條、`sshu-ui-design.md` §11.15 一起改。

## 9. `Ctrl-C` 直接結束，不走離開流程 —— K9

- **現況**：`app.go` 第 650 行 `if msg.Type == tea.KeyCtrlC && !m.inPty() { return m.quit() }`：有 live session、
  巢狀 sshu 或進行中的傳輸也直接走，不經 `q` 的 confirm（`panelKey()` 第 889–905 行）。`helpContent` 寫
  `Ctrl+C  force quit`。
- **規則**：`Ctrl-C` 與 `q` 做同一件事 —— 進入離開流程（有 `quitCost()` 時先 confirm）；離開流程進行中再按一次
  `Ctrl-C` 才立刻離開。PTY 裡的 `Ctrl-C` 仍屬於遠端（K10），這部分不變。
- **怎麼改**：把 `q` 的 confirm 抽成一個離開函式，`q` 與 `Ctrl-C` 都呼叫它；quit confirm 開著時的 `Ctrl-C`
  直接 `m.quit()`（現在已是如此）。help 的 `Ctrl+C  force quit` 與 README 兩份的「`Ctrl+C` 強制離開」改成
  `quit (twice: at once)` 之類的說法。

## 10. `q` 只在沒有 popup 時有效 —— K1、K9

- **現況**：`q` 只在 `panelKey()`（第 889 行）處理；Space menu、明細、viewer、help、confirm、Jobs 開著時，
  按鍵先被 popup 路由吃掉，`q` 沒有作用（`sshu-ui-design.md` §A.0.K：「`q` 在任何浮層開著時不生效」）。
- **規則**：`q` 是 core key，除了輸入態（表單、input、picker 的輸入、搜尋列）與 PTY 以外，在每個 surface 都是
  「離開 app」。
- **怎麼改**：`q` 的處理提到 popup 路由之前（`typing()` 與 PTY 的判斷之後）。§A.0.K 那句一起改。

## 11. 連線中的格子：footer 列的鍵都不作用，出口沒揭露 —— K10、M1

- **現況**：`app.go` 第 671 行，格子拿到 focus、但遠端還沒送出任何 byte（`ptyFocused() && !inPty()`）時，
  除了 `Alt+Esc`、`Ctrl-C` 以外的鍵全被吞掉。`view.go` `footer()` 第 203 行只在 `inPty()` 時換成 PTY 的列，
  所以這段期間 footer 仍寫著 `space menu`、`? help`、`1-2 M/F/S`、`q quit`，按了都沒反應，唯一的出口
  `Alt+Esc` 卻不在上面（`connectingBody()` 也沒寫）。
- **規則**：focus 在 PTY 時常駐揭露離開 PTY 的那個鍵；非輸入態的畫面上揭露的入口要按得動。
- **怎麼改**：`footer()` 的判斷改成 `ptyFocused()`：連線中也顯示 `alt+esc leave pty`（`pgup/pgdn`、`alt+v` 等
  依現有條件自然不出現）。

## 12. README 的 `[M]anage` 按鍵表過時 —— S2、文件對齊

- **現況**：`README.md` 與 `README-zh_TW.md` 的 `[M]anage` 表有一列「`V`：View what this row holds /
  唯讀檢視這一列」；但 `internal/ui/app.go`（約 695 行）的註解寫「nothing claims the letter any more」，
  `V` 在 pty 與輸入框以外都是 splash，檢視已經改成 `Enter`（`credkeys.go`、`sshcfgkeys.go`、
  `knownkeys.go` 的 `View` 都綁 `enter`）。同一張表 `Enter` 的說明（Credentials: edit）也可能對不上。
- **規則**：splash 不寫進 README（tdp S2）；README 照 app 現在的行為寫。
- **怎麼改**：對照程式碼核對整張 `[M]anage` 表（兩份語言）：拿掉 `V` 那一列，修正 `Enter` 在各區段的說明。

## 13. 正在關的 popup 還會吃 `Esc` —— F3

- **現況**：`app.go` `closeTop()` 對**每一個** popup 都用 `isActive()` 判斷（askpass、toast、Jobs、viewer、
  detail、三個 picker、四個表單、input、confirm、editor、help、Space menu、lock menu），而 `isActive()` 是
  `phase != animClosed`，關閉中的也算；`popupAnimator.close()` 又把 frame 重設成滿格。所以任何 popup 關到一半
  再按 `Esc`，關閉動畫重來，那個 `Esc` 也傳不到下面那層。（清單原本只寫了 toast，並說其他 popup 用 `owns()`
  —— 用 `owns()` 的是按鍵路由的 `switch`，不是 `closeTop()`。）
- **規則**：已經在跑關閉動畫的 popup 不再理會 `Esc`，也不再接收其他按鍵（F3）。
- **怎麼改**：`closeTop()` 的判斷全部改成 `anim.owns()`（locku v0.1.3 同樣的修法）；`closeStack()` 不受影響
  （已關的 `close()` 回 nil）。測試：任一 popup 關閉中按 `Esc`，關的是底下那層。

## 14. 程式碼註解仍引用 VTP 的 § 編號 —— 文件對齊

- **現況**：`internal/ui` 的註解用 VTP 的 § 編號、「u-family」與「the principle」（不影響行為）。只換引用 VTP 的；
  指 `sshu-ui-design.md` 自己章節的不動：所有 `§11.x`、`§7.3.2`、`theme.go:5` 寫明的 `sshu-ui-design.md §2.1 / §B`，
  以及 `theme.go:107`、`:131`（§3.4、§3）、`viewer.go:34`（§4.2 導覽）、`askpass.go:292`（§6.3 host form）、
  `hosts.go:225`、`:296`（§1.1、§1.5）、`chrome.go:134`（§1.1）、`app.go:1611`、`sshkeys.go:287`（§6.2 Space menu）。
- **怎麼改**：照 [terminu `vtp/README.md` 的對照表](https://github.com/vulcanshen/terminu/blob/v0.1.1/vtp/README.md) 換成 tdp 編號：

| 檔案:行 | 現在 | 換成 |
|---|---|---|
| `app.go:38` | `§1.3` | `tdp L3` |
| `app.go:115`、`:118` | `§A.0.K` | `tdp K1` |
| `app.go:117` | `the principle capped` | `VTP capped`（歷史敘述，保留原意）或刪掉這句 |
| `app.go:265` | `§6.4` | `tdp F4` |
| `app.go:607` | `§4.3` | `tdp K4` |
| `app.go:621` | `§4.5` | `tdp K8` |
| `app.go:675` | `§A.1 / §A.2` | `tdp K5, K6`（第 3 條修完後這段註解要重寫） |
| `app.go:680` | `§4.3` | `tdp K4` |
| `app.go:682` | `§4.5` | `tdp K8` |
| `app.go:690` | `§A.2 promises` | `tdp K6`（第 5 條修完後這段註解要重寫） |
| `app.go:695` | `u-family easter egg` | `terminu family easter egg` |
| `app.go:718`、`:727` | `§4.5` | `tdp K8` |
| `app.go:810`、`:1630`、`:1830` | `§6.4` | `tdp F4` |
| `app.go:860`、`:1758` | `§7.1` | `tdp T1` |
| `app.go:947`、`:1234`、`:1265`、`:1461` | `§4.2` | `tdp M3` |
| `app.go:1145` | `§4.5` | `tdp K8` |
| `app.go:1496` | `§A.1` | `tdp K5` |
| `app.go:1526` | `§A.1` | `tdp M6`（第 8 條修完後這行消失） |
| `app.go:1557` | `the §A.1 contents` | `the Space menu contents (tdp M2)` |
| `askpass.go:225` | `§4.3` | `tdp K4` |
| `bundlepage.go:17`、`:79`、`:126` | `§4.5` | `tdp K8` |
| `bundlepage.go:68` | `§6.7` | `tdp F5` |
| `chrome.go:119` | `§1.1 — narrow must stay usable` | `tdp L1` |
| `chrome.go:136` | `§1.3` | `tdp L3` |
| `chrome.go:141` | `§7.2` | `tdp T2` |
| `chrome.go:176` | `§A.1 / §A.2` | `tdp M1` |
| `chrome.go:199`、`:263` | `§2.1` | `tdp D2` |
| `confirm.go:32` | `§6.1` | `tdp F1` |
| `confirm.go:71` | `§4.3` | `tdp K4` |
| `copymode.go:127` | `§4.3` | `tdp K4` |
| `credform.go:219` | `§4.5` | `tdp K8` |
| `credkeys.go:11` | `§4.2` | `tdp M3` |
| `credkeys.go:107` | `§6.4` | `tdp F4` |
| `credkeys.go:232` | `§6.7` | `tdp F5` |
| `detail.go:24` | `the §6.1 VIEWPORT class` | `the viewport class (tdp F1)` |
| `detail.go:29` | `§1.4`（已退役的 `sshu-implementation.md` §1.4 欄位收縮） | `sshu-ui-design.md §1.2` |
| `detail.go:41` | `§4.4's "bright key, dim description"` | `tdp M5 / D1 的「亮鍵暗述」` |
| `detail.go:110` | `§4.3` | `tdp K4` |
| `filepicker.go:23` | `§4.5` | `tdp K8` |
| `filepicker.go:273` | `§2.4 override colour` | `tdp D2` |
| `form.go:15` | `the hybrid float §6.1 forbids` | `the hybrid float tdp F1 forbids` |
| `form.go:18` | `the §A.1 entry key` | `the Space menu entry key (tdp K5)` |
| `form.go:19`、`:471`、`:474` | `§4.5` | `tdp K8` |
| `form.go:431` | `§6.7` | `tdp F5` |
| `helppopup.go:8` | `the §A.2 non-contextual entry point` | `the ? entry point (tdp K6, M4)` |
| `helppopup.go:34` | `§A.0.K` | `tdp K1` |
| `highlight.go:12` | `the point of the u-family` | `the point of the terminu family` |
| `hosts.go:83`、`:171` | `§4.5` | `tdp K8` |
| `hosts.go:255` | `§1.3` | `tdp L3` |
| `inputpopup.go:24` | `§6.1` | `tdp F1` |
| `inputpopup.go:94` | `§4.3` | `tdp K4` |
| `inputpopup.go:123` | `§B` | `tdp P4` |
| `knownkeys.go:140` | `§6.1` | `tdp F1` |
| `knownkeys.go:166` | `§6.4` | `tdp F4` |
| `popup.go:12` | `§2.2 / §6.3` | `tdp D2` |
| `popup.go:14` | `§B` | `tdp P4` |
| `popup.go:31` | `§6.2 … inside the 100-200ms band` | `tdp F2 … the family default (tdp D3)` |
| `popup.go:71` | `§6.2` | `tdp F2` |
| `popup.go:151` | `a hole in the principle (§4.5)` | `a hole in tdp K8` |
| `popup.go:208`、`:214`、`:262` | `§4.4` | `tdp M5` |
| `preftab.go:108` | `§1.2` | `tdp L2` |
| `sftpkeys.go:77` | `§4.4` | `tdp M5` |
| `sftpkeys.go:142`、`:161` | `§4.2` | `tdp M3` |
| `sftpkeys.go:187` | `tab [2]'s §A.1 contents` | `tab [2]'s Space menu contents (tdp M2)` |
| `sftpsearch.go:191` | `§4.5` | `tdp K8` |
| `sftptab.go:369` | `§4.5` | `tdp K8` |
| `sftpview.go:11` | `§4.4` | `tdp M5` |
| `spacemenu.go:12` | `§4.2` | `tdp M3` |
| `spacemenu.go:29` | `the §A.1 contextual entry point` | `the Space menu (tdp K5, M2)` |
| `spacemenu.go:150` | `§4.4` | `tdp M5` |
| `splash.go:19` | `The u-family mark` | `The terminu family mark` |
| `sshcfgkeys.go:13`、`sshkeys.go:13` | `§4.2` | `tdp M3` |
| `sshkeys.go:251` | `tab [3]'s §A.1 contents` | `tab [3]'s Space menu contents (tdp M2)` |
| `sshtab.go:50` | `§1.2` | `tdp L2` |
| `sshtab.go:372` | `§A.1` | `tdp M5` |
| `sshtab.go:1035` | `§1.2` | `tdp L4` |
| `theme.go:10` | `§4.4` | `tdp M5` |
| `theme.go:21`、`:25`、`:39`、`:61`、`:80`、`:152`、`:162` | `§B` | `tdp P4` |
| `theme.go:35` | `§2.4` | `tdp D2` |
| `toast.go:26` | `§6.5` | `tdp F3` |
| `transfer.go:196` | `§7.2` | `tdp T2` |
| `view.go:13`、`:30` | `§1.3` | `tdp L3` |
| `view.go:188`–`189` | `§A.1 / §A.2` | `tdp M1` |
| `view.go:193`–`194` | `the principle's disclosure score` | `VTP's disclosure score`（歷史敘述） |
| `view.go:264` | `§4.4` | `tdp M5` |
| `width.go:11` | `§1.2` | `tdp L2` |

## 15. 選取模式（`Alt+v`）的 core key 都被吞掉 —— K11、M1

- **現況**：`app.go` `copyModeKey()` 只認 `Alt+Esc` / `Alt+v`（離開）與 `copyState.key()` 的移動、選取、`y`、
  `Esc`；`Space`、`?`、`q`、`Ctrl-C`、`Tab` 全部靜默吞掉。footer（`view.go` `copyLegendPairs()`）列的是模式的鍵，
  沒有 `space` 與 `?`。
- **規則**：模式裡 `Space` 開關**模式的按鍵清單**（每一列可直接執行，執行後關掉，不分區）；`?` 是模式的 help
  （唯讀）；`Esc` 離開模式；`q`、`Ctrl-C` 照 K9 進離開流程；`Tab` 可以暫停但要有回應（toast 說先 `Esc`）。
  footer 照樣顯示 `Space` 與 `?`。
- **怎麼改**：`copyModeKey()` 在交給 `copy.key()` 之前處理這五個鍵；按鍵清單的列就是 `copyLegendPairs()` 那幾組
  （一份來源，footer 與清單共用）。`Esc` 維持兩段（先取消選取，再離開模式），符合 K4 的「一次一層」。
  footer 前面固定 `space` 與 `?`，其餘照舊從尾端捨棄。README 兩份的選取模式小表、`sshu-ui-design.md` §11.33 一起改。

## 16. help 疊在 popup 上時，`Esc` 關到底下那層 —— D3、K6

- **現況**：`?` 現在就能疊在任何 popup 上（`app.go` `handleKey()` 的 `?` 分支），但 `closeTop()` 把 `help` 排在
  confirm、表單、picker 之後：help 開在 confirm 上時按 `Esc`，關掉的是底下的 confirm，help 還留著。
- **規則**：`?` 的 help 疊在其他 popup 上時，按鍵路由、`closeTop()` 與繪製都把它放在最上層（tdp D3）。
- **怎麼改**：跟第 5 條一起做：`closeTop()`、按鍵路由的 `switch` 與 overlay 的繪製順序都把 help 移到最前（askpass
  除外 —— ssh 在等它，它本來就搶走整個鍵盤）。測試：confirm 上開 help，`Esc` 只關 help、`Enter` 不確認 confirm。
