# sshu：尚未符合 tdp 的地方

tdp Rules E7：逐條待修，全部修完就刪掉這份。對照 tdp v0.7.4（清單 `checklist/items-zh_TW.md`）。刻意不照做、寫了原因的在
`dev-remarks.md` 的「偏離 tdp」，不在這裡。

| # | 違反哪一條 | 在哪裡 | 現況 | 該怎麼改 |
|---|---|---|---|---|
| 1 | E7-6（README 放一張代表性的 demo gif）、E6（gif 用 VHS 錄） | `docs/demo-grid.gif` | 錄的是改用 terminu-go 以前的畫面（v2.0.0）：框、膠囊、footer、格子的樣子都跟現在不一樣 | 在 `.local/demos/` 把 driver 照現在的鍵改寫（格子是 panel、`Enter` 進格子、`Alt-Esc` 先問），先在 tmux 乾跑，再用 demo 的 sshd 容器重錄 |
| 2 | file-picker-18–64（finder 預覽檔案時照 file-picker 的預覽） | `internal/tui/fileops.go` 的 `search` | 遠端那一側的搜尋，預覽只有一列 `in <目錄>`：沒有內容、樹、metadata；lib 的檔案預覽只讀本機磁碟 | 等 lib 的預覽能接 app 給的檔案系統（SS18），再接上 SFTP |
| 3 | file-picker-35（預覽的樹縮排） | lib `preview.go` | 第二層的 `├─` 畫在第 0 欄 | lib 修（SS17） |
| 4 | input number（數字用 number input） | `internal/tui/hosts.go` 的 `optionalPort` | sshconfig host 與 `~/.ssh/config` 區塊的 Port 可以空，lib 的 number input 不收空值，暫時用 text input 加同樣的範圍檢查 | 等 lib 的 number input 能空（SS16），換回 number |
| 5 | input-2（錯誤列點名值） | lib `terminu.ErrEmpty` | sshu 的名稱欄已經自己寫 `A name can't be empty`；lib 的 `ErrEmpty` 本身不帶名字，用它的地方都會違反 | lib 修（SS15） |
