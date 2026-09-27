# sshu 開發者備忘

開發 sshu 時要提醒自己、以及與 AI 協作時記下的決策。sshu 遵循
[terminu design principle](https://github.com/vulcanshen/terminu/tree/v0.1.5/principle)（tdp）；
使用者要知道的在 README,這裡收的是另一半 —— 行為的細節、背後的理由、以及一路走過來的歷史。完整的設計紀錄(包含被否決的做法)在 [`sshu-ui-design.md`](sshu-ui-design.md)。

靈感來自 [Termius](https://termius.com/) —— 一款 GUI 的 SSH client,而不是哪個終端機工具。sshu 借的是它的精神 —— hosts、sessions、檔案傳輸收在同一個屋簷下 —— 不是照單全收它的功能清單。

---

## 運作方式

### `Alt+Esc` 與 `Alt+Enter`

`Alt+Esc` 是 sshu 自己的鍵,只為一個情況存在:網格的格子把每一個按鍵都交給遠端,所以總得有東西能把它收回來。其他地方,單純的 `Esc` 就夠了。`Alt+Enter` 是它的對手:`Esc` 從這一層出去,`Enter` 進入層的管理 —— 巢狀 sshu 時,lock 這一層,鍵就穿到下一層。

終端機要真的送出 Alt:bubbletea 認得 `\x1b\x1b` → `{KeyEscape, Alt}`,macOS Terminal.app 要開「Use Option as Meta key」、iTerm2 要把 Option 設成 Esc+,kitty / Alacritty / WezTerm 預設就送。已知誤觸:bubbletea 靠「ESC 後緊跟另一個 byte」判斷 Alt,所以遠端跑 vim 時快速連按兩次 Esc 會被讀成 `Alt+Esc`、意外跳出 pty;按 `Enter` 回得去。(詳見設計文件 §4)

### `[M]anage`

- nav 分 SSHU / SSH / Logs 三個 header,游標會直接跳過 header。鍵盤一交給內容,整片 nav 就暗下來變成「`[2]` 在顯示什麼」的圖例;唯一還亮著的是未讀錯誤數。
- **Hosts 一台兩列** —— 上面那列照舊,下面那列是 tag —— 終端機變窄就逐欄收起。
- **Config** 就是 `~/.ssh/config` 本身 —— ssh tab 早就在讀它了,因為 sshu 是去啟動真的 `ssh`。一列一個 `Host` 區塊,`Include` 會跟進去,所以清單涵蓋這棵樹真正有的每一個檔案;表單帶上那個區塊剛好用到的每一個關鍵字。編一個區塊只會動到它自己的那幾行:註解、`Match` 區塊、以及 sshu 沒聽過的關鍵字都原樣通過。每一台 host 自己的明細會說出這份檔案對它做了什麼 —— 所有命中區塊的聚集、每個關鍵字取第一個值,並標出哪些被 sshu 的命令列蓋掉了。
- **KnownHosts** 就是 `~/.ssh/known_hosts` —— 決定「你講話的對象是不是你以為的那台機器」的檔案,也是 sshu 在金鑰變了時直接拒絕連線的依據。`[X]` 就是那個拒絕的出路;`[A]` 去問那台機器要金鑰、**在認證之前停下**、把指紋給你看過才寫。
- **`V`(View)**:host 分「連線」與「認證」兩段,credential 只有認證那一段。存起來的密碼一律是固定寬度的遮罩、永遠不顯示值;credential host 就地解析 —— 包括它指向的 credential 已經不在時,直接說出來。
- **`Enter` 連 credential host 時在確認框就解析**:確認框顯示的就是實際要用的身分,斷掉的引用在那一步就用一句話失敗,不會走進 ssh 裡才爆。
- **`D`(複製)**開一個每一欄都已經從這一列填好的 Add 表單。什麼都還沒寫進檔案,也沒有發明任何名字:表單是完整的,所以 `Enter` 就是存檔,而它帶著的名字被它自己複製的那一列佔著。第一次 `Enter` 一定被擋在 Name 上,而游標本來就在 Name 上。
- **刪 credential 時**確認框會數還有幾台 host 引用它。`[C]lear` 確認時會指名它要清掉哪一個檔。

#### 表單的 `Enter`

**`Enter` 在每一欄上都是送出**(tdp K3,§11.59)。沒填完或填錯就不存:focus 跳到**第一個**有問題的欄位,紅色的錯誤列說出缺什麼、錯在哪。這看起來像 `Tab`,但不是 —— `Tab` 一欄一欄走,`Enter` 指出問題;再按一次 `Enter` 還是停在那一欄。(§11.59 以前沒填完的 `Enter` 是「下一欄」、hint 在 `next` / `save` 之間翻面。)「填完」的定義跟著 Auth 走,因為它就是 Auth 留著亮的那幾列:`password` 要 Password、`privatekey` 要 IdentityFile、`credential` 要 Credential 而且不再要 User;`sshconfig` 除了 Host 什麼都不要 —— Port 和 User 還亮著但變成選填,空著就是 ssh 決定;一個還停在預設 `22` 的 Port 會被清空,因為送出去的 `-p 22` 會蓋掉 config 裡的。浮層底部的 hint 固定是 `Enter save`。

「有沒有填」與「填得對不對」是同一個送出的兩半:`missing()` 找第一個空著的必填欄,各表單的 `check…` 找其他錯誤(Port 範圍、名字重複、credential 不存在),`firstError` 取欄位位置在前的那一個;同一欄兩邊都有話說時,用比較具體的那句(「Choose a credential…」而不是「Credential is required」)。送出一次之後,錯誤列隨著編輯即時更新。

兩個「選值欄位」保留一個例外:**空著的 IdentityFile 或 Credential 上按 `Enter` 是開選單** —— 對那一欄的送出(K3 允許 submit 單一欄位),因為選單是那一列唯一填得進去的方式。`~/.ssh/config` 表單的 `+ add option` 列打了字時,`Enter` 是加入那一列,不是存整個 block。有值之後它們就是普通的列,`Backspace` 整行清除。

選了 `credential`,User 列會變暗:user 由 credential 供應,而選單會直接說出它要用哪把金鑰 —— 換成密碼的話,那裡是一個固定長度的遮罩。**Tags** 空白分隔,其餘字元一律 literal(`k8s:prod` 是一個 tag),留空就是一份填完的表單。

### `[F]ile transfer`

- **兩側對等** —— local ↔ remote ↔ remote 走同一個 `FS` 介面。marks 分側;一個 mark 是一條絕對路徑,所以改名它會跟著走,刪掉它會被拿掉。
- **正在被寫入的檔案不能 mark**:mark 是「這個路徑可以拿來操作」的承諾,半個檔案不是。這種檔案的 **mark 欄會顯示 spinner** —— 它存在,但還沒到齊 —— 它正在落進去的那個目錄也一樣,因為傳一整棵樹的時候,你看得見的那一列就是目錄。兩者都在 job 結束的那一刻消失,清單同時重讀。
- **傳輸進行中 `H` 與 `D` 會凍結**:兩者都是把某一側底下的檔案系統抽掉,而每一筆傳輸都同時掛著兩側。那兩列留在 Space menu 裡、整列暗掉(不是消失 —— 它們屬於這個 panel,只是此刻不能做),按下去沒有反應 —— 不另外說原因(tdp M6),README 寫了要先去 `J` 取消。右上角的 summary 在傳輸期間會轉。
- **還沒有 host 的那一側,`Space` 照樣開 menu**,只有 `[H]ost` 一列(加上 global 區)。以前直接開 host 清單(「一列的 menu 不是 menu」,§11.16),但那讓 `Space` 在一個 panel 上有兩種意思;tdp K5、M7 定為一律開 menu(§11.55)。
- **進度**:右上角 `<done>/<files> · <pct>%` 用綠色報告,tab 列下方那條分隔線兼職進度條 —— 綠色從左往右隨百分比推進,在每個 tab 都看得到,傳完瞬間恢復成普通的線。
- **遞迴子樹搜尋** —— `/` 走遍當前目錄底下整棵樹,**廣度優先**(SFTP 上每一層目錄都是一次 round trip,所以近的先到),串流、可取消、有上限,而且就地畫出來。`Enter` 把你帶到結果所在的位置、游標已經停在它上面,從那裡 `a` / `t` / `v` / `e` / `x` 全部能用。遠端**內容**搜尋刻意不做:那需要在對面跑指令,而這個 tab 不做這件事。
- **抓下來之前先讀** —— `v` 的語法上色是 chroma + catppuccin-mocha,跟 filu 同一套;二進位是 xxd 風格的 hex dump。最多讀 64 KiB,因為在遠端那一側每一個 byte 都要過網路。檔案裡的跳脫序列會被剝掉:那些 bytes 是從別人的機器上來的,不處理的話會重畫你的終端機。
- **用你自己的編輯器編** —— `e` 用 `$VISUAL` / `$EDITOR`(`vi` 只是地板,不是依賴),跑在 embedded terminal 裡所以框還在。本機的檔案就地編,所以它的 inode —— 以及指向它的每一個 hard link —— 都還在。內容沒有真的變就不會寫回去;寫入是原子的,斷線不會留下一份被截斷的設定檔;在你開著它的時候被別人改過的檔案,絕不會不問一聲就蓋掉。
- **真的傳輸引擎** —— 整個 plan 在動手之前就算完,所以進度條的分母從第一格就是對的,而覆寫在一開始就一次問完。取消或失敗的檔案會被移除,而不是留在那裡看起來像完成了。
- **目錄保持最新,而且很便宜** —— SFTP 沒有變更通知,所以 sshu 去 stat 目錄、比對 mtime,只有動了才重新列。每隔幾秒一次很小的 round trip,而不是整份重列,而且只在這個 tab 在畫面上的時候做。`R` 存在是因為 mtime 不是承諾。

### `[S]SH`

- **網格** —— 每一格是 embedded PTY 裡的真 `ssh`。每一格的遠端只在尺寸真的變了才收到通知。結束的 session 立刻離開網格並放掉模擬器;鍵盤絕不會默默落進另一台遠端。
- **session 一連上就自動在網格上**,所以 `H` 絕大多數時候在做的是「拿掉」。(`Tab` 以前也做同一件事,§11.56 拿掉了。)
- **Close all sessions 刻意不給字母**:關掉每一條連線是破壞性且罕見的,而字母就是那個會被一隻只想捲清單的手按到的東西。
- **`D` 複製 session 後鍵盤留在清單上**、游標落在新的那一條:你按的那個 Enter 是對確認框按的,只有對一列按 Enter 才是「帶我進去」。
- **清單的項目**是兩行:上面是你叫它什麼,下面是 `<user>@<host>:<port>` —— ssh 自己的拼法。第一行開頭是顯示欄 —— 有格子的是 monitor glyph、沒有的是劃線的那個。兩行都不折:名字太長就截,位址太長就在保留的 `@` 兩側各自縮,所以一個項目永遠剛好兩行。游標移動時,對應格子的外框同步亮 —— 亮的是**游標自己的顏色**,不是 focus 藍:藍色的意思是「鍵盤在這裡」,螢幕上出現兩個藍框只會讓你得停下來找哪一個才是活的。
- **layout**:自訂時問的是**欄數**(1–9),列數由 session 數自己推出來。

#### zoom 與巢狀

`Alt+Z` 分階段:*zoom panel* 佔滿網格區,*zoom max* 連 sshu 自己的 chrome 和邊框都不畫,第三下回到正常。不會改變畫面的那一階自動跳過,所以只有一格時第一下就直接到 zoom max。

zoom max 這一階正是巢狀不再收費的原因:在那之前每層要吃 5 列,80×24 的終端機上第三層只剩 4 列可用。在 server 上裝 sshu 是一行指令的事,所以巢狀是自然會發生的事,不是要迴避的情境。`Alt+Enter` 鎖住一層,所有和絃就穿透到裡面那一層,而 Alt+Enter 是 lock 唯一吞不掉的鍵;每一層會往外自我通報,所以最外層列得出整條鏈,可以直接對其中任何一層下鎖,不必自己走進去。有鏈的時候選單另外帶兩列整鏈動作。

#### 選取模式(`Alt+v`)

終端機自己的選字是最明顯的那條路,而網格正好是破壞它的東西:拖曳是沿著螢幕的實體列走的,一拖就順手把邊框和隔壁格子同一列的輸出一起抓進來。業界的標準解法是開滑鼠、自己接管拖曳 —— sshu 不做,因為開了 mouse tracking 就等於把**整個 app** 的原生選字拿掉,包括清單和浮層這些原生選字仍然好用的地方。所以這是一個鍵盤模式,而它在自己以外不花任何東西。

進去之後那一格**停止跟著遠端跑** —— session 沒有停、照樣在讀,只是一個會在你選到一半時重排的頁面,是沒有人選得起來的頁面 —— 而外框轉黃就是在說這件事。鍵是 vim 的,word 的規則也照搬 vim(§11.53)。什麼都沒選時 `y` 拿游標那一行,於是「複製剛剛印出來的東西」是三個鍵的事。字進**系統**剪貼簿,所以貼進編輯器不需要終端機幫任何忙;而且它一定會回報複製了幾行,或者在做不到時告訴你該裝什麼。

它是 tdp 所說的「模式」(K11):`Space` 列出模式的鍵(每一列可直接執行;清單用方向鍵移動,因為 `j`/`k` 本身就是列),`?` 是模式的 help,`q` / `Ctrl-C` 照樣走離開流程,`Tab` 跳 toast 說先 `Esc`。footer 最前面固定 `space` 與 `?`,放不下的 motion 由 `Space` 的清單負責揭露(§11.57)。

#### 歷史與 `PgUp` / `PgDown`

`PgUp` / `PgDown` 是**借**來的,不是拿走的:全螢幕程式自己就用這兩個鍵翻頁,而它進場時會切到 alt screen —— 那件事本身就是宣告,所以它在的時候鍵原封不動送過去。純 shell 輸出不會翻頁,那正好是需要有人提供捲動的時候。正在放歷史的格子會在 title 說出來(`󰋚` 加往回幾行),因為一個在放歷史的格子和一個遠端已經沒聲音的格子,是同一張靜止的畫面。

vt10x 不留歷史:模擬器是一塊固定的 grid,離開頂端的列會被它清掉,所以每一塊從 PTY 讀進來的 bytes 在進模擬器的同時就被切成行存起來,顏色一起留著,最多 10000 行。alt screen 期間不收:全螢幕程式每按一個鍵就重畫整個視窗,照單全收會把真正值得捲回去的 shell 歷史沖掉。`\x1b[3J`(遠端明確要求清掉 scrollback)會清,`\x1b[2J` 不會。`clear` 送哪一個由 `TERM` 決定、不是由作業系統決定,而 sshu 把 pty 的 `TERM` 釘死成 `xterm-256color`,它的 terminfo 有那個 erase —— 所以在遠端打 `clear` 一定會清掉該 session 的歷史。`Ctrl+L` 只送 `\x1b[2J`,歷史留著:兩個手勢,兩種意思。

#### 連線與失敗

- **還沒接通的連線會說自己在連** —— 格子畫的是 PTY,而 ssh 等 TCP 的時候什麼都不印,所以連不上的主機以前就是一個空框、空到作業系統放棄為止。判準是**對面有沒有送出過 byte**,不是網格空不空:在那之前,panel 會說出對方是誰、以及等了幾秒。
- **沒有東西會無聲死掉,也沒有東西只講一次** —— 不正常結束的 session 會跳 toast,說是哪一台、以及 **ssh 自己說了什麼**(`Connection refused`,不是 `disconnected`);網格會留著那句話而不是變回空框;完整內容在 Errors。還沒讀的錯誤數會掛在 nav 和底部,直到你去看為止。
- **任何離開方式都不留孤兒** —— 每個 ssh 子行程都在自己的 PTY session 上,訊號自己到不了它。一個 registry 認得它們全部,而每一條出路 —— `q`、`Ctrl+C`、外部的 SIGINT/SIGTERM、甚至關掉終端機視窗(SIGHUP)—— 都會順路帶走它們。

### `auth: sshconfig`

`auth: sshconfig` 只有一個名字和一個 destination;port 和 user 選填,其餘都是 `~/.ssh/config` 的事。ssh tab 本來就是這樣。file transfer tab 為這種 host 啟動真的 `ssh -s sftp`,所以 ProxyJump、agent、有 passphrase 的私鑰在那裡也都能用 —— 而 ssh 要問的東西,不管是密碼還是沒見過的 host key,都會以 ssh 的原句跳成 popup,答案直接回給 ssh、不存。在那個 popup 上按 `Esc` 取消的是整條連線,不只是那個框。

另外三種 auth 的 sftp 側走 `golang.org/x/crypto/ssh` 自己講協定,所以:未知 host key 直接拒絕(不做互動確認)、變過的 key 直接拒絕而且不拿出來當問題問 —— 把它變成 yes/no 題,就是在訓練人按 yes;加密私鑰會如實回報但還不能用,agent 支援是可能的解法。

### 資料與檔案

- 每次寫入都是原子的(暫存檔 + rename),並重新確立 `0600`,檔案開頭帶一段警告。
- **知道自己版本的設定檔** —— 舊版的 `hosts.yaml` 會在 sshu 啟動時改寫成當前格式,而更新版的 sshu 寫出來的檔絕不會被覆蓋。兩個檔各自計數,所以動了其中一個,不會害舊版拒絕另一個。
- **`config.yaml` sshu 永遠不寫**:你手改過的檔案不會被重排,註解也活得下來。`connect_timeout` 在 ssh tab 交給 ssh 當 `-o ConnectTimeout`,sftp 側拿它當 dial timeout。超出 1–600 的值一律當成小數點打錯,改用預設。設定檔壞掉不會擋著不讓 sshu 啟動 —— 它用預設值跑,並且在 manage → Errors 裡說出來。
- `$SSHU_CONFIG` 是 `make demo` 和測試用來指定目錄的;`$XDG_CONFIG_HOME` 在 macOS 上也吃,讓人可以不要 `~/Library/Application Support`。

### 畫面

- **frame 不變量** —— 每一條畫出來的線都剛好是終端機的寬度,任何尺寸、任何內容。從遠端來的寬字元、量起來不一樣的 Nerd Font glyph、CJK 檔名,全部靠「量」而不是「猜」;有一個測試橫跨尺寸、focus 狀態與資料在檢查它。這也是 Nerd Font 是必要條件的原因:版面會去量它們。
- **量並補齊純文字,再上色。** `lipgloss.Width` 會跳過 ANSI,但對已上色的字串補空白會讓那些空白落進樣式範圍內、吃到背景色;已上色的字串要裁切一律走 `clipANSI`。遠端的寬字元(vt10x 一個 rune 算一格)也是 `ptyTerm.render` 每一行先 `clipANSI` 再補齊,代價是這種行被切掉最後一兩欄。
- **chrome 固定三列**:頂部膠囊 tab 列、其下一條整寬分隔線、底部 footer,`chromeRows = 3`。窄寬門檻是推導的:`sshNarrowW = sshLeftW + 28`(pty 至少留 28 欄)、`sftpNarrowW = 72`;`panes()` 是唯一決定版面形狀的地方。
- **導覽詞彙只有一份**(`internal/ui/nav.go`):`j`/`k` 繞、`u`/`d` 半頁不繞、沒有游標的 viewport 不繞(`moveScroll`);導覽字母不被任何動作佔用,由 `TestNoActionClaimsANavigationKey` 擋。
- **letter hotkey 與 Space menu 同一張表**:`hostActions` / `sshActions` / `sftpActions` 是熱鍵與 menu 列的唯一宣告,兩者走同一個 `dispatchKey`。
- **pty emulator 會回答終端機查詢**(`CSI 6n`、`OSC 11`):`startPty` 先開 pty master、再以它當 vt10x 的 writer;writer 不能碰 `p.mu`,否則在第一個查詢就死鎖(`sshu-ui-design.md` §11.36)。

## 設計決定

### 按鍵與 menu

- **零學習成本是結構保證的,不是靠自律。** menu 和字母快捷鍵是同一張表產生的,所以「menu 裡沒有的快捷鍵」不可能存在。
- **Space menu 分三區** —— `item operation`(對游標那一列做什麼)、`panel operation`(對這一側做什麼)、`global operation`。global 區永遠在,所以每一區都帶標題(tdp M2);沒有 item 與 panel 動作的 panel,前面寫一句 `nothing to do here`。global 區只有一列 `Global operation`,`Enter` 打開 global operation popup(`[M]anage`、`[F]ile transfer`、`[S]SH`、`[q]uit`,宣告在 `globalActions`;tdp M2、M4)。這是 sshu 先試、tdp v0.1.2 採納的做法。
- **`?` 只拿來讀**:在 panel 上是 key reference(`panelKeyReference()`):先是這個 panel 的鍵 —— 從它的 Space menu 讀出來,所以兩邊不會不一致 —— 再接 core key、SSH tab 上接網格的和絃、最後是導覽鍵(tdp M4、K6)。在浮層上是那個浮層自己的按鍵(`popupHelp()`)。網格的和絃在格子裡問不到(`?` 屬於遠端),所以 SSH tab 的 key reference 一定要列。
- **global operation popup 裡,目前所在的 tab 那一列變暗**:它在、只是你已經在那裡了。
- **方括號印的大小寫就是你要按的那個鍵**:`[A]dd` 是 shift+A、`[t]ransfer` 是裸的 `t`,沒有標出來的東西不會動。
- **`D` 一律是複製、`x`/`X` 一律是刪除。** 刪除原本是 `D`;統一之後整個 sshu 裡同一個字母只有一個意思。
- tab 用一個 shift 過的裸字母切換,裸數字 `1`–`9` 全部用來直達當前 tab 的 panel。pty 裡這三個字母跟其他裸鍵一樣屬於遠端,所以要先 `Alt+Esc`。

### 三本紀錄,不是一本 log

以前那本 log 想同時回答三個問題 —— 出了什麼事、連過哪些機器、動過哪些東西 —— 結果三個都答不好。這三件事該長的樣子根本不一樣:連線紀錄要整齊、一列一筆、寬度固定,同一台機器的紀錄才能順著欄位一路往下數;失敗訊息卻是遠端吐出來的十五行。全部塞在一起,你想找的那種永遠被另外兩種蓋住。

**Errors** 一列一筆(時間、哪台、用誰的身分、原因),列表本身維持一列,所以整頁失敗是拿來掃的,不是拿來讀的;按 `Enter` 才展開遠端**最後那整個畫面** —— 連線被拒絕只有一行,但 host key 對不上會吐十五行,而你要的指紋剛好在中間。**Changes** 記 host、credential、`~/.ssh` 底下的檔案、傳輸、改完存回去的檔。三本各有各的檔案,`[C]lear` 也是各清各的。

### tag 與顏色

- **sshu 從不解讀 tag。** 它只做兩件事:顯示它、讓它可以被搜。凡是程式會去解讀的欄位,你就得學一套規則,而這個欄位只有一條規則:空白會斷開,其餘都是字面。
- **顏色只標例外、不裝飾常態。** name / user / host 共用一個色調,因為它們是同一件事;一列上唯一的顏色是**不是 22 的 port** —— 二十台裡只有兩台開在奇怪的 port,那兩個正是一眼該落上去的地方。auth 靠 glyph 區分而不是顏色:每一列都有 auth,而標記每一列的顏色等於什麼都沒標。

### 密碼加密:拆成兩份,不是藏起來

加密做的是**把祕密拆成兩份**,不是把祕密藏起來。以前一個檔案漏掉就全完了,現在要兩個一起漏才算 —— 風險換了形狀,力道沒變,所以「那個資料夾不要進版控、不要進雲端同步、不要讓備份掃到」一個字都不用改。

`SSHU_KEY_FILE` 不做成預設是有原因的:鑰匙一旦離開 config 資料夾,備份就得靠你自己記得,而鑰匙掉了,所有密碼就一起沒了。

以前用來保護明文的措施一項都沒少:檔案固定 `0600`;密碼從來不畫在畫面上,表單顯示 `••••`,credential 選單的遮罩是固定長度,連幾個字都看不出來;交給 `ssh` 的方式是 `SSH_ASKPASS`,所以密碼不會跑進子行程的環境變數,`ps` 也看不到。舊的明文密碼會在下次啟動時就地鎖起來,沒有遷移步驟。

### 其他

- **session 完全不落地**:`[S]SH` 的 session 只存在記憶體,最後一個畫面可能有遠端印出來的任何東西。
- **pty 在 sshu 是 panel,不是浮層**:session 是長時的、同時可以有很多個,所以它是常駐 panel 的內容(filu 的 pty 是短時浮層)。
- **Connect 之後清掉 source**:明細腳底的 Connect 按下去,明細與 Space menu 一起收掉、切到 `[S]SH`、開 session —— ssh session 是長時 target(tdp T1)。

### PTY 裡的鍵

- **格子裡除了出口鍵,還留著 sshu 自己的和絃(tdp K10、M3)。** tdp v0.1.4 起,K10 只要求 PTY 至少有一個出口鍵,
  其他組合鍵由 app 決定(原本 v0.1.2 的 K10 只准出口鍵,這條曾是偏離)。sshu 在格子裡保留:`Alt+Z`(zoom)、`Alt`+方向鍵(換格子)、
  `Alt+v`(選取模式)、`Alt+Enter`(巢狀的 lock)、`PgUp` / `PgDown`(遠端不是全螢幕程式時翻歷史)。理由:
  - 它們作用的對象就是**正在用的那一格**。sshu 的網格是好幾個同時活著的 session,使用者在格子之間來回、放大
    其中一格、從某一格複製 —— 每做一次都要先 `Alt+Esc` 退到清單、開 menu、再進回去,等於把網格的用法拆成
    三段。
  - 它們跟出口鍵是同一類鍵:`Alt` 和絃是遠端程式幾乎不用的組合(K10 自己選出口鍵的理由);`PgUp` / `PgDown`
    只在遠端沒進 alt screen 時才借,全螢幕程式在的時候原封不動送過去(§11.19)。
  - 巢狀 sshu 靠和絃穿透到內層才操作得了(見下一條)。
  - 揭露:格子有鍵盤時 footer 常駐列出這些和絃(M1),`?` 的 key reference 有 `ssh grid` 一段。
- **鎖住的格子,出口是 `Alt+Enter`,不是 `Alt+Esc`(tdp K10)。** K10 只要求至少一個出口鍵,換成哪一個由 app
  決定(曾列為偏離,v0.1.4 起不是)。格子被 lock(巢狀 sshu 用)之後,`Alt+Esc`、`Alt+Z`、`Alt+v`、方向鍵
  全部穿透到內層,只有 `Alt+Enter` 留著(打開 lock menu,`Release PTY`)。理由
  (§11.43):lock 的意思就是「這一層是透明的管子」,內層 sshu 要收到完整的和絃才操作得了;footer 在鎖住時只寫
  `alt+enter release`,出口仍然常駐揭露。

## 已否決,不要重提

每一條的理由在 `sshu-ui-design.md` 對應的章節。

- tab 鍵掛在 `1`/`2`/`3`、`Alt+p/f/s` 和絃與固定亮的 `[Alt]` 鏈頭(§11.21,改裸的 `M`/`F`/`S`)
- `Alt+1..9` 在格子間跳(本地的 window manager 先吃掉;改 `Alt`+方向鍵)
- 開 mouse tracking 做網格選字(會拿掉整個 app 的原生選字;改鍵盤的選取模式,§11.33);剪貼簿走 OSC 52(§11.33)
- 常駐的 history panel(§7.1.4,改 toast + Errors / Connections)
- Connect 自己的確認框(§11.29,併進明細的腳)
- tab 列沒亮的段用 crust 或 surface0 當凹槽(§1.1)
- 把 `(sshu)` 注入遠端的 prompt(§11.47)
- password / privatekey / credential host 的 sftp 也走真 `ssh`(使用者裁定不做,§11.52)
- 遠端內容搜尋(要在對面跑指令,超出 file transfer tab 的授權範圍)

## 已知的牆與未做

- **平台**:macOS 與 Linux(用 Unix PTY),沒有原生 Windows build。
- **file transfer tab 未知 host key 的互動確認**:sshconfig host 已解(ssh 自己問,經 askpass 中繼跳成 yes/no popup);其他三種 auth 仍一律拒絕。
- **加密私鑰**:sshconfig host 已解;其他三種仍如實回報做不到。
- **`SSH_ASKPASS_PROMPT=none` 通知類提示**:FIDO 觸碰確認會讓 helper 掛著等 ssh 殺它,畫面上什麼都不出現;sshconfig host 用 FIDO key 在 file transfer tab 會卡住。
- **`Alt+Esc` 誤觸**:遠端跑 vim 時快速連按兩次 Esc 會被讀成 `Alt+Esc`(見上方「運作方式」)。
- **未做**:Mouse;`[1]` 的 `[S]ftp` 捷徑(從表格直接把游標那台接到 file transfer 當前 focus 的那一側);fsnotify 重讀 `hosts.yaml`;keychain 存密碼;Export / Import 已實作但遮罩中(設計未定案,§11.12)。
- **尚未符合 tdp 的地方**:目前沒有。2026-09-26 盤點的清單已於 2026-09-27 修完並刪除,經過記在設計文件 §11.54–§11.57;刻意不照做的在下方「偏離 tdp」。

## 偏離 tdp

依 tdp P0(規則服務 UX),下面兩條保留 sshu 的做法(使用者 2026-09-27 裁定,對照 tdp v0.1.5)。

sshu 同時管很多個目標,畫面中央又是一格一格的 PTY,原本撞上 tdp 的地方比家族其他成員多。回饋給 tdp 的幾條
已經採納,不再是偏離:`?` 只讀、global operation popup、K11 的方向鍵、K2 拿掉 grid 的例子(v0.1.2);
格子裡的和絃與鎖住時的出口(v0.1.4 的 K10 改成「至少一個出口鍵,其餘由 app 決定」,移到「設計決定」的
「PTY 裡的鍵」,§11.60)。表單的 `Enter` 則是 sshu 照 v0.1.3 的 K3 改了(§11.59)。

- **SSH tab 的 `Tab` 不作用(K2)。** 照 K2,這個 tab 的兩個 panel `[1]` sessions、`[2]` layout 之間應該用
  `Tab` 輪替。sshu 不做:畫面中央那一大塊是 PTY 的網格,`Tab` 在這個 tab 上一跳,使用者的直覺是「進格子」,
  而格子裡的 `Tab` 屬於遠端(K10);在旁邊兩個小 panel 之間跳,正好是最不會被想到的那個意思。要換 panel 用
  數字鍵,進格子用 `Enter`,格子之間用 `Alt`+方向鍵。`Tab` 以前是 `[H]ide` 的第二個拼法,§11.56 拿掉了。
  (tdp K2 原本拿 sshu 的 grid 當「`Tab` 在 cell 之間切換」的例子,v0.1.2 已刪。)
- **明細浮層是 viewport,腳底可以掛一個 offer(F1)。** `detailPopup`(`internal/ui/detail.go`)是唯讀
  viewport,但 hosts / credentials / Config 的明細腳底掛著 `Connect to "<name>"?` / `Edit "<name>"?` 的問句,
  `Enter` 就執行 —— 等於 viewport 兼 confirm。理由(`sshu-ui-design.md` §11.29):以前 Connect 有自己的確認框,
  但連線不是破壞性動作,那個確認框真正在做的是「先讓你看清楚要去哪」,而明細本來就說得更完整;「看」和「決定」
  分成兩個浮層,只是讓使用者多按一次、多看一次同樣的東西。

## 設計文件導讀

| 檔案 | 回答什麼 |
|---|---|
| [`sshu-ui-design.md`](sshu-ui-design.md) | 完整的設計紀錄:每一個看得見的行為為什麼是這樣,以及試過而被否決的做法。§A、§B、§1–§7 沿用 VTP 時期的分章(各章標出對應的 tdp 條目),§8 資料層、§9 檔案骨架、§10 開發順序、§11 之後的每一次改動(§11.1–§11.60),最後是按鍵全表 |
| [`icon.svg`](icon.svg) | 圖示:家族的 mark,藍 U 框住拼出 SSH 的方塊字;`V` splash 照它畫 |

Go、[Bubble Tea](https://github.com/charmbracelet/bubbletea) 與 [Lip Gloss](https://github.com/charmbracelet/lipgloss),embedded terminal 用 [creack/pty](https://github.com/creack/pty) + [hinshun/vt10x](https://github.com/hinshun/vt10x),檔案傳輸用 [pkg/sftp](https://github.com/pkg/sftp) + `golang.org/x/crypto/ssh`,語法上色用 [chroma](https://github.com/alecthomas/chroma)。配色是 catppuccin-mocha。

## 建置與開發

從原始碼:

```bash
go install github.com/vulcanshen/sshu/cmd/sshu@latest
```

或 clone 之後:`make build`、`make check`(fmt + vet + test),`make demo` 用 `demo/hosts.yaml` 跑、不碰你的設定 —— 直接跑 `make` 會列出其他的。

**unix-first、靜態執行檔** —— macOS + Linux;`CGO_ENABLED=0`、`-trimpath`、已 strip。`make package` 產 `dist/` 底下的 `.tar.gz`,`make install` / `make uninstall` 對 `$GOBIN`。

## 發布

push 一個 `v*` tag,GitHub Actions(`.github/workflows/release.yml`)跑 `go test -race`,過了由 goreleaser 打包並更新 homebrew tap;release notes 從 `CHANGELOG.md` 對應的那一節取出。每一版做了什麼,以 [CHANGELOG.md](../CHANGELOG.md) 為準。
