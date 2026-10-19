# sshu：尚未符合 tdp 的地方

tdp E7：逐條待修，全部修完就刪掉這份。對照 tdp v0.8.0（清單 `checklist/items-zh_TW.md`）。每一條都已定案、等 terminu-go 的下一版
（第三批；有些 lib 已經修好、只是還沒發版）；lib 發版後 go.mod 升一次、逐條修掉。刻意不照做、寫了原因的在 `dev-remarks.md` 的
「偏離 tdp」，不在這裡（目前沒有）。

| # | 違反哪一條 | 在哪裡 | 現況 | 該怎麼改 |
|---|---|---|---|---|
| 1 | file-picker-18–64（finder 預覽檔案時照 file-picker 的預覽） | `internal/tui/fileops.go` 的 `search` | 遠端那一側的搜尋，預覽只有一列 `in <目錄>`：沒有內容、樹、metadata；lib 的檔案預覽只讀本機磁碟 | lib 的預覽接 app 給的檔案系統後接上 SFTP（SS18） |
| 2 | file-picker-25、35、51、53、91（本機的預覽） | lib `preview.go` | 樹的第二層畫在第 0 欄；多字元的 escape 序列漏字；一項的目錄寫 `1 items`；MODE 不是 `ls -l` 的寫法；列出後才不見的檔 metadata 是空白、不是 `—` | lib 修（SS17、SS22、SS38；樹縮排已修），升級後就好 |
| 3 | text-2、number-9（數字用 number；可以空的欄位空值不檢查範圍） | `internal/tui/hosts.go` 的 `optionalPort` | sshconfig host 與 `~/.ssh/config` 區塊的 Port 可以空，lib 的 number input 不收空值，暫時用 text input 加同樣的範圍檢查 | lib 的 number input 加「可以空」後換回 number（SS16） |
| 4 | panel-4、panel-75（loading 的揭露）、tdp K10（連線中不轉送打字） | `internal/tui/ssh.go` | 格子在連線中沒有 loading icon、也沒有「connecting」那一句，打的字照送 | lib 告訴 app 程式什麼時候第一次輸出後做（SS11） |
| 5 | F6-2（confirm 的 `Esc` 取消） | `internal/tui/transfer.go` 的 `askpassArrived` | ssh 問的問題按 `Esc` 只關掉框，ssh 一直等，要按 `[D]isconnect` 叫停 | lib 的 confirm／input 有取消的 hook 後回答 ssh「不」（SS8） |
| 6 | M9-1、T2-3（串流 panel 失焦時 hint 照樣變暗） | lib `panel.go` | 串流／PTY 的 panel 失焦時 hint 不淡化，格子的 `Enter:type` 與 [1] 的 `Enter:open` 同時亮 | lib 修（SS32） |
| 7 | popup-69、note-69（動作失敗開 error popup，source 留著） | lib input | Rename、Add、Trusted for 寫入失敗時 input popup 已經關了，值要重打 | lib 的 Done 能回報失敗後改用它（SS33） |
| 8 | M5-1、M5-25（熱鍵在 label 裡原地包，不分大小寫；label 照本來的大小寫） | lib `MarkLabel`、`internal/tui/transfer.go` | lib 比對分大小寫：sshu 小寫熱鍵的列只好用小寫開頭（`[r]ename`），lib 的離開列畫成 `[q] Quit` | lib 不分大小寫後，label 改回 `Rename`、`View`…（SS31） |
| 9 | M5-16（幾個鍵做同一件事用 `\|` 寫成一組） | `internal/tui/ssh.go` 的格子鍵 | footer 是四組 `Alt-←:cell Alt-→:cell…` | lib 的 TerminalKey 收好幾個鍵後寫成 `Alt-←\|→\|↑\|↓`（SS12） |
| 10 | M6-6（key reference 不列對象不存在的鍵） | lib `panelKeyRef` | 還沒有任何傳輸時，`?` 的 app-wide 段照樣列 `J Jobs` | lib 已修，升級後就好（SS36） |
| 11 | components-26（panel 標題裡的名稱照原樣） | `internal/tui/transfer.go`、`ssh.go` 的 label | host 名稱、`user@host` 被轉成大寫（`[3] DEPLOY@10.0.3.14 #1`） | lib 的 Panel 多一個名稱欄位後改用它（SS13） |
| 12 | terminal-18–20（PTY 的選取模式） | `internal/tui/ssh.go` | 格子裡沒有選取模式（以前的 `Alt-v`） | lib 的 Terminal 做好選取模式後接回 `Alt-v`（SS4） |
| 13 | menu-21、menu-22（工作清單：hint 寫那個動作；列跟著工作更新） | `internal/tui/xferjobs.go` 的 `jobsMenu` | 進行中那一列的 hint 是 `Enter:run`；進度是打開那一刻的快照 | lib 的 menu 能給 Enter 的字、每畫一次重問那一列後接上（SS9） |
| 14 | K11-12、terminal-15–17（PTY 的歷史模式） | lib `Terminal` | `PgUp` 捲歷史，但不是有框色與 `History` 模式名的模式 | lib 的 Terminal 做好歷史模式，升級後就好 |
| 15 | toast-15、toast-16、color-12、note-36（成功的訊息用 Success） | `Saved`、`Deleted`、`Copied`…… 的 toast | lib 沒有 Success，成功的訊息用 Info | lib 有 Success 後改用它 |
| 16 | L1-2–L1-6、E5-13（比最低尺寸小時只畫名稱與提示；測試到 1×1） | lib、`internal/tui/manage_test.go` | v0.1.13 照樣硬畫；在 80×5 打開 Space menu 會 panic，所以極小尺寸的測試暫時不開 popup | lib 的最低尺寸畫面與 popup 的截斷（已修）升級後，測試補上 popup 與名稱、提示 |
| 17 | K6-7、K6-8（key reference 的移動鍵：一維清單寫 `h\|k`、`j\|l`；沒東西可移、可捲時不列） | lib `MoveKeys` | 表單、menu、LAYOUT 寫 `j\|k`；清單空了、內容放得下照樣列移動與捲動鍵 | lib 修，升級後就好 |
| 18 | K3-8（打開時是空的欄位打了又刪掉，按 `Enter` 不寫回） | lib input | 照樣寫回，表單被記成改過，`Esc` 會問要不要放棄 | lib 修，升級後就好 |
| 19 | K9-2、form-95（表單的打字說明 note 只收 `Enter`、`Esc`） | lib `note.go` | 在 note 上按 `q` 進入離開流程，`?` 打開 key reference | lib 修，升級後就好 |
| 20 | F2-3、T1-6（好幾層一起收起時每一層一起播） | lib `closeAll` | 只有最上面那一層播收起動畫，底下的直接消失 | lib 修，升級後就好 |
| 21 | screen-34、components-5、panel-125、popup-12（放不下時：空狀態折行、膠囊截尾加 `…`、popup 的 hint 留住 `Esc`） | lib | 80 欄時空狀態的句子被截成 `…`；三個 session 時選中的膠囊被硬切；Search 的 hint 丟掉 `Esc:cancel` | lib 已修，升級後就好 |
| 22 | panel-127、panel-128（欄放不下時整欄收掉） | lib `List`、`internal/tui/hosts.go` | 80 欄的 Hosts 表格只縮欄寬 | lib 的 Column 有收掉的順序後替每一欄標上 |
| 23 | terminal-13（terminal popup 開著時不畫 footer） | lib `terminal.go` | 最後一列左端露出 footer 的字 | lib 修，升級後就好 |
| 24 | input-76、E3-11（表單、panel 上的值含控制字元時畫成紅色轉義；清單、menu 的 Tab 畫成空白） | lib `form.go`、`values.go`、`list.go` | 控制字元與 Tab 原樣送出 | lib 修，升級後就好 |
