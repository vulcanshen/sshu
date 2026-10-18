# sshu：尚未符合 tdp 的地方

tdp Rules E7：逐條待修，全部修完就刪掉這份。對照 tdp v0.7.4（清單 `checklist/items-zh_TW.md`）。刻意不照做、寫了原因的在
`dev-remarks.md` 的「偏離 tdp」，不在這裡。

| # | 違反哪一條 | 在哪裡 | 現況 | 該怎麼改 |
|---|---|---|---|---|
| 2 | file-picker-18–64（finder 預覽檔案時照 file-picker 的預覽） | `internal/tui/fileops.go` 的 `search` | 遠端那一側的搜尋，預覽只有一列 `in <目錄>`：沒有內容、樹、metadata；lib 的檔案預覽只讀本機磁碟 | 等 lib 的預覽能接 app 給的檔案系統（SS18），再接上 SFTP |
| 3 | file-picker-35（預覽的樹縮排） | lib `preview.go` | 第二層的 `├─` 畫在第 0 欄 | lib 修（SS17） |
| 4 | input number（數字用 number input） | `internal/tui/hosts.go` 的 `optionalPort` | sshconfig host 與 `~/.ssh/config` 區塊的 Port 可以空，lib 的 number input 不收空值，暫時用 text input 加同樣的範圍檢查 | 等 lib 的 number input 能空（SS16），換回 number |
| 5 | input-2（錯誤列點名值） | lib `terminu.ErrEmpty` | sshu 的名稱欄已經自己寫 `A name can't be empty`；lib 的 `ErrEmpty` 本身不帶名字，用它的地方都會違反 | lib 修（SS15） |
| 6 | panel-4、panel-75（loading 的揭露）、K10（連線中不轉送打字） | `internal/tui/ssh.go` | 格子在連線中沒有 loading icon、也沒有「connecting」那一句，打的字照送 | 等 lib 告訴 app 程式什麼時候第一次輸出（SS11） |
| 7 | F6-2（confirm 的 `Esc` 取消） | `internal/tui/transfer.go` 的 `askpassArrived` | ssh 問的問題按 `Esc` 只關掉框，ssh 一直等，要按 `[D]isconnect` 叫停 | 等 lib 的 confirm／input 有取消的 hook（SS8） |
| 8 | M9-1（同一個鍵兩處指示，會觸發的亮） | lib `panel.go` | 串流／PTY 的 panel 失焦時 hint 不淡化，格子的 `Enter:type` 與 [1] 的 `Enter:open` 同時亮 | lib 修（SS32） |
| 9 | popup-69、note-69（動作失敗開 error popup，source 留著） | lib input | Rename、Add、Trusted for 寫入失敗時 input popup 已經關了，值要重打 | 等 lib 的 Done 能回報失敗（SS33） |
| 10 | note-66（error popup 的標題寫出名稱） | lib `App.Error` | 標題是 `DELETE FAILED`，寫不出 `: <名稱>` | 等 lib 的 Error 收 Subject（SS33） |
