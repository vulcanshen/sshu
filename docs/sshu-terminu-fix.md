# sshu：尚未符合 tdp 的地方

tdp Rules E7：逐條待修，全部修完就刪掉這份。對照 tdp v0.7.4（清單 `checklist/items-zh_TW.md`）。刻意不照做、寫了原因的在
`dev-remarks.md` 的「偏離 tdp」，不在這裡。

| # | 違反哪一條 | 在哪裡 | 現況 | 該怎麼改 |
|---|---|---|---|---|
| 1 | file-picker-18–64（finder 預覽檔案時照 file-picker 的預覽） | `internal/tui/fileops.go` 的 `search` | 遠端那一側的搜尋，預覽只有一列 `in <目錄>`：沒有內容、樹、metadata；lib 的檔案預覽只讀本機磁碟 | 等 lib 的預覽能接 app 給的檔案系統（SS18），再接上 SFTP |
| 2 | file-picker-35（預覽的樹縮排） | lib `preview.go` | 第二層的 `├─` 畫在第 0 欄 | lib 修（SS17） |
| 3 | input number（數字用 number input） | `internal/tui/hosts.go` 的 `optionalPort` | sshconfig host 與 `~/.ssh/config` 區塊的 Port 可以空，lib 的 number input 不收空值，暫時用 text input 加同樣的範圍檢查 | 等 lib 的 number input 能空（SS16），換回 number |
| 4 | panel-4、panel-75（loading 的揭露）、K10（連線中不轉送打字） | `internal/tui/ssh.go` | 格子在連線中沒有 loading icon、也沒有「connecting」那一句，打的字照送 | 等 lib 告訴 app 程式什麼時候第一次輸出（SS11） |
| 5 | F6-2（confirm 的 `Esc` 取消） | `internal/tui/transfer.go` 的 `askpassArrived` | ssh 問的問題按 `Esc` 只關掉框，ssh 一直等，要按 `[D]isconnect` 叫停 | 等 lib 的 confirm／input 有取消的 hook（SS8） |
| 6 | M9-1（同一個鍵兩處指示，會觸發的亮） | lib `panel.go` | 串流／PTY 的 panel 失焦時 hint 不淡化，格子的 `Enter:type` 與 [1] 的 `Enter:open` 同時亮 | lib 修（SS32） |
| 7 | popup-69、note-69（動作失敗開 error popup，source 留著） | lib input | Rename、Add、Trusted for 寫入失敗時 input popup 已經關了，值要重打 | 等 lib 的 Done 能回報失敗（SS33） |
| 8 | M5-1（熱鍵在 label 裡原地標出） | lib `MarkLabel` | lib 的離開列畫成 `[q] Quit`；sshu 自己的列已改成小寫開頭（`[r]ename`） | lib 修（SS31） |
| 9 | M5-16（幾個鍵做同一件事用 `\|` 寫成一組） | `internal/tui/ssh.go` 的格子鍵 | footer 是四組 `Alt-←:cell Alt-→:cell…` | 等 lib 的 TerminalKey 收好幾個鍵（SS12） |
| 10 | panel-13（每個 panel 的下框都有 hint） | 空的 marks、SESSIONS、紀錄 | 清單空了又沒有對整個 panel 的鍵時 hint 是空的；規範兩條互相矛盾 | 等規範定案（SS35） |
| 11 | panel-24（panel 的內容離框一格） | lib `Terminal` | 格子裡的 PTY 貼著框畫 | 等規範定案（SS34） |
| 12 | M6-6（key reference 不列對象不存在的鍵） | lib `panelKeyRef` | 還沒有任何傳輸時，`?` 的 app-wide 段照樣列 `J Jobs`：lib 列 global 動作時沒看 `Hidden` | lib 修（SS36） |
