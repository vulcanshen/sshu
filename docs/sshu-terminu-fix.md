# sshu：尚未符合 tdp 的地方

tdp E7：逐條待修，全部修完就刪掉這份。對照 tdp v0.8.0（清單 `checklist/items-zh_TW.md`）。每一條都已定案、等 terminu-go 的下一版
（第三批）；lib 發版後 go.mod 升一次、逐條修掉。刻意不照做、寫了原因的在 `dev-remarks.md` 的「偏離 tdp」，不在這裡（目前沒有）。

| # | 違反哪一條 | 在哪裡 | 現況 | 該怎麼改 |
|---|---|---|---|---|
| 1 | file-picker-18–64（finder 預覽檔案時照 file-picker 的預覽） | `internal/tui/fileops.go` 的 `search` | 遠端那一側的搜尋，預覽只有一列 `in <目錄>`：沒有內容、樹、metadata；lib 的檔案預覽只讀本機磁碟 | lib 的預覽接 app 給的檔案系統後接上 SFTP（SS18） |
| 2 | file-picker-35（預覽的樹縮排） | lib `preview.go` | 第二層的 `├─` 畫在第 0 欄 | lib 已修，升級後就好（SS17） |
| 3 | input/number（數字用 number input；可以空的欄位空值不檢查範圍） | `internal/tui/hosts.go` 的 `optionalPort` | sshconfig host 與 `~/.ssh/config` 區塊的 Port 可以空，lib 的 number input 不收空值，暫時用 text input 加同樣的範圍檢查 | lib 的 number input 加「可以空」後換回 number（SS16） |
| 4 | panel-4、panel-75（loading 的揭露）、K10（連線中不轉送打字） | `internal/tui/ssh.go` | 格子在連線中沒有 loading icon、也沒有「connecting」那一句，打的字照送 | lib 告訴 app 程式什麼時候第一次輸出後做（SS11） |
| 5 | F6-2（confirm 的 `Esc` 取消） | `internal/tui/transfer.go` 的 `askpassArrived` | ssh 問的問題按 `Esc` 只關掉框，ssh 一直等，要按 `[D]isconnect` 叫停 | lib 的 confirm／input 有取消的 hook 後回答 ssh「不」（SS8） |
| 6 | M9-1（同一個鍵兩處指示，會觸發的亮） | lib `panel.go` | 串流／PTY 的 panel 失焦時 hint 不淡化，格子的 `Enter:type` 與 [1] 的 `Enter:open` 同時亮 | lib 照 tdp T2 讓失焦的串流 panel 的 hint 變暗（SS32） |
| 7 | popup-69、note-69（動作失敗開 error popup，source 留著） | lib input | Rename、Add、Trusted for 寫入失敗時 input popup 已經關了，值要重打 | lib 的 Done 能回報失敗後改用它（SS33） |
| 8 | M5-1（熱鍵在 label 裡原地包，不分大小寫；label 照本來的大小寫） | lib `MarkLabel`、`internal/tui/transfer.go` | lib 比對分大小寫：sshu 小寫熱鍵的列只好用小寫開頭（`[r]ename`），lib 的離開列畫成 `[q] Quit` | lib 不分大小寫後，label 改回 `Rename`、`View`…（SS31） |
| 9 | M5-16（幾個鍵做同一件事用 `\|` 寫成一組） | `internal/tui/ssh.go` 的格子鍵 | footer 是四組 `Alt-←:cell Alt-→:cell…` | lib 的 TerminalKey 收好幾個鍵後寫成 `Alt-←\|→\|↑\|↓`（SS12） |
| 10 | M6-6（key reference 不列對象不存在的鍵） | lib `panelKeyRef` | 還沒有任何傳輸時，`?` 的 app-wide 段照樣列 `J Jobs` | lib 已修，升級後就好（SS36） |
| 11 | components-26（panel 標題裡的名稱照原樣） | `internal/tui/transfer.go`、`ssh.go` 的 label | host 名稱、`user@host` 被轉成大寫（`[3] DEPLOY@10.0.3.14 #1`） | lib 的 Panel 多一個名稱欄位後改用它（SS13） |
| 12 | terminal-18–20（PTY 的選取模式） | `internal/tui/ssh.go` | 格子裡沒有選取模式（以前的 `Alt-v`） | lib 的 Terminal 做好選取模式後接回 `Alt-v`（SS4） |
| 13 | dialog/menu（工作清單的列跟著工作更新） | `internal/tui/xferjobs.go` 的 `jobsMenu` | Jobs 的進度是打開那一刻的快照 | lib 的 menu 每畫一次重問那一列的字後接上（SS9） |
