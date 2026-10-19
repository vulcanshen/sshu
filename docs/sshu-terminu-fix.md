# sshu：尚未符合 tdp 的地方

tdp E7：逐條待修，全部修完就刪掉這份。對照 tdp v0.8.1（清單 `checklist/items-zh_TW.md`）、terminu-go v0.2.0。刻意不照做、寫了原因的在
`dev-remarks.md` 的「偏離 tdp」，不在這裡（目前沒有）。

| # | 違反哪一條 | 在哪裡 | 現況 | 該怎麼改 |
|---|---|---|---|---|
| 1 | panel-75（panel 還沒有任何內容時，中間照空狀態的樣子寫一句 loading 的話） | `internal/tui/ssh.go` 的 `start1` | 連線中的格子框上有 loading 圖示、打的字也擋住，但中間沒有「connecting to <host>…」：lib 的 `Panel.LoadingText` 只給有空狀態的 body，`Terminal` 沒有 | lib 的 Terminal 加 `Empty()` 後設 `LoadingText`（SS39，terminu 已收進 lib 的修正版） |
