# sshu 開發者備忘

開發 sshu 時要提醒自己、以及與 AI 協作時記下的決策。sshu 遵循 [terminu design principle](https://github.com/vulcanshen/terminu/tree/v0.7.4/principle)（tdp），畫面建在家族共用的 lib [terminu-go](https://github.com/vulcanshen/terminu-go) 上；使用者要知道的在 README，這裡收的是另一半 —— 運作的細節、背後的理由、以及走過來的歷史。

靈感來自 [Termius](https://termius.com/) —— hosts、sessions、檔案傳輸收在同一個屋簷下 —— 不是照單全收它的功能清單。

---

## 運作方式

### 畫面由 terminu-go 畫

2026-10-18 起 sshu 的 UI 是 `internal/tui`，建在 terminu-go v0.1.13 上（tdp E8）：sshu 登記畫面、panel、每一個動作（鍵、名稱、說明、範圍、要不要先問、做什麼），lib 照這一份登記畫出畫面 chip 列、panel 框、Space menu、global operation popup、`?` 的 key reference、下框 hint、footer、confirm、表單、每一種 input、viewer、toast、開關動畫與淡化，也處理所有 core key。以前 sshu 自己畫這一切的 `internal/ui`（約三萬行，含測試）整個刪掉。

- `app.go`：`Data`（main 載入的檔案與寫回的函式）、三個畫面、statusbar（未讀錯誤、傳輸進度、活著的 session 數，各自固定寬度）、chip 列下的進度條、離開流程（有 session 或傳輸時才先問，並說出會失去什麼）、三本紀錄的寫入。
- `manage.go` 與各節的檔（`hosts.go`、`creds.go`、`sshcfg.go`、`known.go`、`journals.go`）：Manage 畫面。
- `transfer.go`、`fileops.go`、`xferjobs.go`：File transfer 畫面。
- `ssh.go`、`sshcmd.go`、`askpass.go`：SSH 畫面、ssh 的命令列、askpass 中繼。
- `detail.go`：把 host、credential、區塊、金鑰寫成一行一行給 confirm 與 viewer 讀。
- `proc.go`：`KillChildren`，每一種離開方式（`q`、`Ctrl-C`、SIGHUP）都帶走子行程。

### Manage

`[1] SECTIONS` 是分組的清單（SSHU：Hosts、Credentials；SSH：Config、KnownHosts；LOGS：Errors、Connections、Changes），`[2]` 是 cursor 那一節的明細（components/layout/panel 的「明細」）：cursor 一動，`[2]` 的 label、內容與動作就換成那一節的；`Enter` 把 focus 送到 `[2]`（tdp K3 唯一可以用 `Enter` 換 panel 的地方）。

- **Hosts**：`hosts.yaml` 的表格，`/` 是 panel filter（名稱、user、host、port、tags 一起比對）。`Enter` 是連線：先開一個 confirm，上面是這台 host 的明細（連線、驗證、`~/.ssh/config` 會給它什麼，以及哪些被 sshu 的命令列蓋掉），問句在最後（tdp F1：confirm 可以帶一段回答前要看的內容）。表單是 lib 的：`Auth` 是畫在表單上的 radio，`Credential` 開 select，`Identity file` 開 file-picker（從 `~/.ssh` 開始），`Password` 開 password input。
- **Config**：`~/.ssh/config` 一個 `Host` 區塊一列，`Include` 會跟進去。編一個區塊只動它自己的那幾行：HostName、User、Port、IdentityFile 各一欄，其他關鍵字是一個 textarea（一行一個 `Keyword value`）；每個選項記得自己原本在哪一行（`reuseLines`），沒改的一行原樣留著，清空的那一行拿掉。存檔時檔案被別的編輯器改過，就拒絕並換成磁碟上的版本。
- **KnownHosts**：`~/.ssh/known_hosts`。`[A]dd` 是一個表單（host、port），送出時去問那台機器要金鑰（在認證之前停下），再開 confirm 給你看指紋，`Enter` 才寫。
- **三本紀錄**：Errors、Connections、Changes 各自一個表格，最新的在最上面。Errors 的 `Enter` 打開那一筆的全文；另外兩本的 `Enter` 打開整本。`[C]lear` 先問，再清空那一本的檔案。

### File transfer

`[1]` 左側檔案、`[2]` 左側標記、`[3]` 右側檔案、`[4]` 右側標記。兩側都是「某個檔案系統」，不分主從：`remote.FS` 一個介面兩種實作（本機與 SFTP），上傳、下載、遠端對遠端是同一條路徑。

- **檔案清單的第一列是目錄**（`sideBody`：sshu 自己的 panel body，畫一列 Lavender 的路徑，再接 lib 的清單）—— 「我在哪」是內容，不是框。
- **`h|l` 跨到另一側同一列的 panel**（tdp K12：左右結構）。`Esc` 回上一層目錄（tdp K4：「上一層」由 app 定義）。
- **標記照路徑記**，換目錄也留著；lib 的清單照列記標記，所以每次重畫時把這一側的標記同步進清單（`syncMarks`）。
- **`/` 是 finder**：整棵子樹邊走邊列，結果一直進來（components/input/finder：Loading 時可以一直加），`Enter` 到那一項所在的目錄、cursor 停在它上面。本機那一側的預覽是 lib 的檔案預覽。
- **傳輸先規劃再開始**：幾個檔、多少 bytes、對面哪些已經存在都先算好，要覆蓋就問一次整批。傳輸在自己的 goroutine 跑、用 atomic 回報，畫面從不等網路；正在寫進來的那個檔，在清單上用 loading 的 glyph 標出來（tdp F7：一筆持續進來的資料流怎麼揭露由 app 決定）。
- **`[e]dit`**：遠端的檔先抓到本機暫存，`$VISUAL`／`$EDITOR`（沒有就 `vi`）在 lib 的 terminal popup 裡跑；存回去之前比對內容，沒改就不寫；遠端在這段時間被改過就先問。用 `Alt-Esc` 離開編輯器等於放棄這次編輯（`Ends: true`：程式結束，不寫回）。編輯器的環境裡把外層終端機的名字（`TERM_PROGRAM` 等）清掉：它跑在 sshu 的模擬器裡，不是那台終端機。
- **目錄每兩秒看一次時間戳**（SFTP 不會通知），變了才重讀；畫面不在 File transfer 時不看。

### SSH

`[1] SESSIONS`、`[2] LAYOUT`（一列一個值：Arrange 的 radio、Columns），之後每個 session 一個 panel（`[3]` 起），裡面是 lib 的 `Terminal`（components/dialog/terminal：PTY 放在 panel 裡）。

- **連線**：從 Hosts 連線時，sshu 切到 SSH 畫面、focus 新的格子、鍵盤直接進去（tdp T1：長時間的 session 清掉 source）。鍵盤在格子裡時左欄收起，網格佔滿寬度；`Alt-z` 讓這一格佔滿整個 panel 區。
- **離開**：`Alt-Esc` 先問（tdp K10），`Enter` 離開、`Esc` 回到 PTY；離開後 focus 留在那一格，`Enter` 再進去。
- **換格**：`Alt-←|→|↑|↓` 照目前的排法走到那個方向的格子。
- **ssh 的原話**：ssh 以 `-E` 把自己的訊息寫進暫存檔。結束碼 255（ssh 自己放棄）時讀最後一行當原因：toast、Errors、Connections 都記；檔案隨即刪掉。遠端程式自己的輸出不受影響。
- **密碼**：password host 的 ssh 把 `SSH_ASKPASS` 指回 sshu 自己（`SSHU__ASKPASS_HOST`），helper 重讀 `hosts.yaml` 印出密碼 —— 密碼不進環境變數、不上命令列。
- **顏色深度**：ssh 只轉送 `TERM`，所以 sshu 用 `LC_SSHU_COLORTERM`（sshd 預設接受 `LC_*`）並加 `-o SendEnv`；遠端的 sshu 啟動時把它收進 `COLORTERM`。

### `auth: sshconfig`

只有名字與 destination，port、user 選填，其餘都交給 `~/.ssh/config`。SSH 畫面本來就跑真的 `ssh`。File transfer 為這種 host 跑 `ssh -s <host> sftp`（沒有終端機，`SSH_ASKPASS_REQUIRE=force`），所以 ProxyJump、agent、有 passphrase 的私鑰都能用；ssh 要問的（密碼、passphrase、未知的 host key）經 `SSHU__ASKPASS_SOCK` 的 socket 交給畫畫面的那個 sshu，開 confirm 或 password input 問使用者，答案直接回給 ssh、不存。

另外三種 auth 的 SFTP 走 `golang.org/x/crypto/ssh` 自己講協定：未知的 host key 直接拒絕、變過的 key 也拒絕（不把它變成 yes/no 題）。

### 資料與檔案

- **位置（tdp E3）**：設定目錄是 `$SSHU__CONFIG`，否則 `$XDG_CONFIG_HOME/sshu`，否則 `~/.config/sshu`，每個平台同一套。`hosts.yaml`、`credentials.yaml`、`config.yaml`、`.sshukey` 都在裡面；三本紀錄與 `state.yaml`（SSH 網格怎麼排，一列一個值的 panel 改了就存）是狀態，預設也在裡面，`SSHU__STATE` 可以搬走。
- **macOS 的舊位置**：以前照 `os.UserConfigDir`，macOS 上是 `~/Library/Application Support/sshu`。`~/.config/sshu` 不存在、舊目錄在時，暫時照舊讀舊目錄（TODO(tdp SS1)，等 user 定怎麼搬）；`uninstall.sh` 在 macOS 上另外問要不要刪舊目錄。
- 每次寫入都是原子的（暫存檔 + rename），固定 `0600`，檔案開頭帶一段警告。
- 舊格式的 `hosts.yaml`、`credentials.yaml` 在啟動時改寫成當前格式；更新版 sshu 寫的檔絕不覆蓋。明文密碼也在啟動時就地加密。
- `config.yaml` sshu 永遠不寫；壞掉時用預設值跑，在 Errors 裡說出來。啟動時讀檔的每一個問題都寫進 Errors，toast 說一聲有幾個。

### icon 與 powerline 符號的寬度

探測與量寬度交給 lib（tdp E5、tdp E8）：啟動時探測 icon 與 powerline 符號各佔幾格，`SSHU__ICON_WIDTH`／`TERMINU__ICON_WIDTH`、`SSHU__POWERLINE_WIDTH`／`TERMINU__POWERLINE_WIDTH` 可以覆寫；lib 開的每一個 PTY 都設好兩個 `TERMINU__` 變數。

隔著 ssh 的巢狀是 sshu 自己的事（tdp E5：環境變數過不了 ssh，要靠 app 自己的通道）。格子裡的 ssh 帶上 `LC_SSHU_ICON_WIDTH`、`LC_SSHU_POWERLINE_WIDTH`（這一層自己的格數），跟顏色深度一樣加 `-o SendEnv`；遠端的 sshu 啟動時（`AdoptForwarded`）把它們收進 `SSHU__ICON_WIDTH`、`SSHU__POWERLINE_WIDTH`，lib 的探測就照用 —— 遠端的探測由這一層的模擬器回答、量不到真的寬度。使用者在遠端自己設了 `SSHU__` 的那一個照用自己的。以前的指令通道（OSC 7181）做的其他事 —— 看到整條巢狀鏈、鏈上的鎖定與放大 —— lib 做不到（TODO(lib SS5)）。

## 設計決定

- **零學習成本是結構保證的。** 每個動作登記一次，Space menu、熱鍵、`?`、hint 都從同一份登記來，「menu 裡沒有的熱鍵」不可能存在。
- **三本紀錄，不是一本 log。** 出了什麼事、連過哪些機器、動過什麼，三件事該長的樣子不一樣：連線紀錄一列一筆好往下數，失敗訊息卻是遠端吐出來的十五行。Errors 的表格只有原因那一行，全文在 `Enter` 後面。只有 error 算未讀，warning 不算。
- **tag 只顯示、只被搜，sshu 從不解讀它。** 空白分開，其餘都是字面。理由：一個會被程式解讀的欄位，使用者就得學它的規則；tag 是使用者拿來分群的字，能被篩選找到就是它全部的用途。
- **密碼加密是把祕密拆成兩份。** `hosts.yaml` 或 `credentials.yaml` 單獨流出去不會漏密碼；整個設定目錄被複製就擋不住，因為鑰匙在裡面。`SSHU__KEY_FILE` 不做成預設：鑰匙離開設定目錄，備份就得自己記得。
- **`D` 一律是複製、`x`/`X` 一律是刪除。** 刪除原本在 `D`；同一個字母在不同 panel 一個複製、一個刪除，按錯的代價是資料，所以統一成整個 sshu 一個字母一個意思。File transfer 的刪除不放在 `d`：`d` 是半頁（tdp K12）。
- **檔案上的 `Enter`**：目錄是進去，檔案是用 viewer 讀（以前檔案上的 `Enter` 什麼都不做）。理由：tdp K3 要 `Enter` 做最直觀的那件事；在一個檔案上按 `Enter` 什麼都不發生，看起來像鍵壞了，而「讀它」不會改到任何東西。
- **Jobs 是 menu**：進行中的那一列 `Enter` 問要不要取消，結束的那一列 `Enter` 看全文（tdp F1：工作清單）。以前的 `c` 取消游標那一筆，menu 沒有「作用在游標那一列」的熱鍵，所以改成這樣。
- **暫定（SS2，等 user 確認）**：tdp E3 分設定、狀態、資料、快取四種位置。sshu 把 `hosts.yaml`、`credentials.yaml`、`.sshukey` 當設定（跟 `~/.ssh/config` 一樣，「要連哪裡」是設定），三本紀錄當狀態，都在設定目錄；沒有資料目錄 `~/.sshu`、沒有快取。

## 已否決，不要重提

每一條的理由在 `sshu-ui-design.md` 對應的章節。

- tab 鍵掛在 `1`/`2`/`3`、`Alt-p/f/s` 和絃（§11.21）
- `Alt-1–9` 在格子間跳（本地的 window manager 先吃掉；改 `Alt-←|→|↑|↓`）
- 開 mouse tracking 做網格選字；剪貼簿走 OSC 52（§11.33）
- 常駐的 history panel（§7.1.4）
- 把 `(sshu)` 注入遠端的 prompt（§11.47）
- password / privatekey / credential host 的 SFTP 也走真的 `ssh`（user 裁定不做，§11.52）
- 遠端內容搜尋（要在對面跑指令）
- sshu 自己畫畫面（2026-10-18 起全家族用 terminu-go）

## 已知的牆與未做

- **平台**：macOS 與 Linux（Unix PTY），沒有 Windows build。
- **terminu-go v0.1.13 還做不到、已回報的**（程式裡標 `TODO(lib SS<n>)`）：
  - SS3：沒有移動 focus、換畫面、增減 panel 的呼叫。sshu 自己送按鍵（數字、`Tab`、畫面鍵）；結束的 session 的格子只在排在 focus 後面時拿掉，在前面的先留著，等 focus 回到前面。
  - SS4：拿不到 PTY 畫面的字 —— 格子的選取複製（`Alt-v`）暫時拿掉。
  - SS5：巢狀指令通道（巢狀鏈、鏈上的鎖定與放大）拿掉；遠端那一層的 icon 寬度改走 `LC_SSHU_*` 變數（見「運作方式」）。
  - SS6：`Alt-Enter` 鎖定（把每個鍵交給內層 sshu）拿掉。
  - SS7：`Alt-z` 的最後一段（格子佔滿整個終端機、不畫框）拿掉，只剩「佔滿 panel 區」。
  - SS8：confirm 與 input 被 `Esc` 取消時 sshu 不知道 —— askpass 的問題按 `Esc` 之後 ssh 會一直等，`[D]isconnect` 叫停；編輯的兩個問題按 `Esc` 會把暫存的版本留著（說明裡寫了在哪）。
  - SS9：menu 的列打開時就定了，Jobs 裡的進度不會動。
  - SS10：lib 的 `Terminal` 在畫面那一邊改 PTY 大小、讀取那一邊關 PTY，兩者沒同步；跑 PTY 的測試加 `//go:build !race`。
  - SS11：不知道程式什麼時候開始輸出 —— 連線中照樣轉送打字，也沒有「connecting」的樣子。
  - SS12：PTY 的保留鍵一個只收一個鍵，`Alt-←|→|↑|↓` 在 footer 是四組。
  - SS13（盲點）：panel 膠囊把名稱轉大寫（`[3] DEPLOY@10.0.3.14 #1`）。
  - SS14：單欄的清單一定有表頭（`SECTION`、`SESSION`、`MARKED`）。
  - SS15–SS18、SS22、SS32、SS33：`ErrEmpty` 不點名值、number input 不能空（Port 暫時用 text input）、預覽的樹縮排、預覽只讀本機
    （遠端搜尋沒有預覽）、metadata 的單數、串流 panel 的 hint 不淡化、input 的 Done 不能回報失敗。逐條在 `sshu-terminu-fix.md`。
  - SS29：`Action.Ask` 是固定的字；連線與 Duplicate 用 `liveAsk` 在動作執行前把它換成 cursor 那一項的。
  - SS31：lib 比對熱鍵與 label 第一個字母時分大小寫，所以小寫熱鍵的列用小寫開頭（`[r]ename`）。
- **File transfer 的 host key**：password、privatekey、credential 的 host 遇到未知的 host key 一律拒絕；先從 SSH 畫面連一次，或在 KnownHosts 按 `[A]`。
- **加密私鑰**：privatekey host 的 SFTP 用不了有 passphrase 的私鑰；改用 sshconfig host（走 ssh 與 agent）。
- **`SSH_ASKPASS_PROMPT=none` 的通知**：FIDO 觸碰確認會讓 helper 掛著，sshconfig host 用 FIDO key 在 File transfer 會卡住。
- **`Alt-Esc` 誤觸**：遠端跑 vim 時快速連按兩次 `Esc` 會被讀成 `Alt-Esc`；離開格子與放棄編輯都先問，誤觸按 `Esc` 就回去。
- **未做**：Mouse；fsnotify 重讀 `hosts.yaml`；keychain 存密碼；Export / Import（以前實作了但一直遮罩著，搬到 lib 時沒有搬）。

## 偏離 tdp

- **tdp E3：macOS 的舊設定目錄（暫時）。** `~/.config/sshu` 不存在、`~/Library/Application Support/sshu` 在的時候，讀的是舊目錄
  （`internal/store/store.go` 的 `Dir`）。理由：舊目錄裡有使用者的 host 與解開密碼的鑰匙，直接換位置等於讓人一打開就是空的、密碼也解不開。
  怎麼搬等 user 定（SS1）。
- **components/layout/panel：File transfer 的檔案清單上面有一列目前目錄**（`internal/tui/transfer.go` 的 `sideBody`）。panel 的方案
  （一般清單、分組、一列一個值、明細）沒有這一列。理由：兩側可能是同一台機器的不同目錄，也可能是兩台機器的同一個路徑，不寫出
  「我在哪」就分不出來；它是內容，所以在框裡、用 Lavender（使用者足跡），不擠進框線。已回報成 tdp 的缺漏（SS20）。
- **tdp K3：SESSIONS 的 `Enter` 把 focus 送進那一格**（`internal/tui/ssh.go` 的 `open`）。tdp K3 說 `Enter` 不移動 focus、換 panel 只用在
  明細；格子不是清單那一列的明細，是那個 session 本身。理由：這張清單存在就是為了「進去那個 session」，按 `Enter` 停在清單上什麼都
  沒發生，正是 tdp K3 要避免的「按了沒反應」；從 Hosts 連線也一樣，連上就把鍵盤交給新的格子（tdp T1：長時間的 session 清掉 source）。
  已回報成 tdp 的盲點（SS30）。
- **components/input/textarea：`~/.ssh/config` 區塊的其他選項用 textarea**（`internal/tui/sshcfg.go`）。規範說設定檔這種一行就是一行的
  內容交給 `$EDITOR`。理由：這裡只是一個 `Host` 區塊裡 sshu 沒有專門欄位的那幾行，跟 HostName、User 等欄位在同一張表單上一起存；
  開 `$EDITOR` 要離開表單、也沒辦法把結果寫回表單的一欄（lib 的表單欄位只能開 input）。要編整個檔，File transfer 的 `[e]dit` 對
  `~/.ssh/config` 就是 `$EDITOR`。已回報成 tdp 的缺漏（SS19）。

以前唯一的一條 —— SSH tab 的 `Tab` 不作用（tdp K2）—— 隨著改用 lib 不再存在：SSH 畫面的 `Tab` 照 tdp K2 在 panel 之間輪替，格子也是 panel；鍵盤在格子裡時 `Tab` 屬於遠端（tdp K10）。

## 設計文件導讀

| 檔案 | 回答什麼 |
|---|---|
| [`sshu-ui-design.md`](sshu-ui-design.md) | 改用 terminu-go 以前（到 v2.0.0、tdp v0.1.23）的完整設計紀錄：每一個看得見的行為為什麼是這樣，以及試過而被否決的做法（§A、§B、§1–§11.73）。§12 起記改用 terminu-go 之後的事 |
| [`icon.svg`](icon.svg) | 家族的 mark：藍色 U 框裡兩個 S 與一個 H，一個 XML 註解一個字（U、S、S、H，tdp S5）；splash 照它逐格畫，`internal/tui/icon.svg` 是 `go:embed` 用的副本，`TestTheIconIsTheSplash` 守住兩份一樣 |
| `~/Documents/sideproj/terminu/.local/family-fix/REFACTOR-2026-10-18.md` | 這次改用 terminu-go 的交代（本機才有） |

Go、[terminu-go](https://github.com/vulcanshen/terminu-go)（建在 Bubble Tea v1 與 Lip Gloss v1 上），檔案傳輸用 [pkg/sftp](https://github.com/pkg/sftp) + `golang.org/x/crypto/ssh`。配色是 catppuccin-mocha（lib 的）。

## 建置與開發

從原始碼：`go install github.com/vulcanshen/sshu/cmd/sshu@latest`。clone 之後跑 `make` 列出所有指令（家族共用的在 `family.mk`，sshu 自己的只有 `make demo`：用 `demo/hosts.yaml` 跑，不碰你的設定）。

- **lib 的版本**：`go.mod` 寫 terminu-go 發布的 tag。本機開發用 git 不收的 `go.work`（`go work init . ../terminu-go`）接上 lib 的工作目錄；`make check` 有 `go.work` 時另外跑 `check-lib`（關掉 `go.work`、照 `go.mod` 編譯與測試），過不了就是用到了 lib 還沒發版的東西。
- **`make check`**：`fmt-check`、`vet`、`test-race`、`check-lib`，commit 前跑。
- **看畫面**：`SSHU__DUMP="100x30:2,Enter" go test ./internal/tui -run TestDump -v` 印出按了那串鍵之後的畫面（假資料、假的 ssh）。在真的終端機裡看：`.local/shot.sh -s 100x30 -- Escape 2 Space`（git 不收；在 tmux 裡用 demo 資料跑真的 binary，HOME 是丟棄式的，`-i 2`、`-p 2` 讓 icon、powerline 符號佔兩格）。
- **畫面測試**：每一個畫面在 icon 與 powerline 符號各種寬度、好幾種終端機尺寸下，每一列都剛好是終端機寬（tdp L4、tdp E5、tdp E6）。popup 的開關動畫每次約 0.6 秒，所以 popup 只在其中兩種寬度打開。
- **跑 PTY 的測試**（`internal/tui/ssh_pty_test.go`）用一支假的 ssh 腳本（`fakeSSH`），加了 `//go:build !race`（見 SS10）。
- **頂層的其他目錄**：`demo/`（`make demo` 的 hosts）、`.local/demos/`（VHS 的 tape 與 demo 用的 sshd 容器，git 不收）、`CLAUDE.md`（指向這次重構的交代）、`.claude/`（Claude Code 的專案設定：`agents/`、`commands/` 裡的 social-preview 用來從 `docs/icon.svg` 產生 `docs/social-preview.png`；`.claude/.gitignore` 擋掉個人的檔）。

## 發布

照 tdp project 的「發布」：`make check` → CHANGELOG 的 `## Unreleased` 改成 `## vX.Y.Z — tdp vA.B.C` → commit、打 tag → push commit 與 tag，`release.yml` 跑測試、goreleaser 發 release 並推 Homebrew tap `vulcanshen/homebrew-tap`，再用 CHANGELOG 那一段蓋過 release 的內文。版本號與發版時機由 user 定。

踩過的坑：

- **auto mode 會擋推 tag**（v2.0.0）：推 `v*` tag 會觸發發版，分類器把它判成對外公開；不要換寫法繞過，請 user 換 edit mode 或自己推。
- **環境裡的 `GITHUB_TOKEN` 蓋過 keyring 的 credential**：推 workflow 檔要 `env -u GITHUB_TOKEN git push …`。
- **新 repo 的第一個 tag 可能不觸發 workflow**：刪掉 tag 再推一次。
- **macOS runner 在 `-race` 下慢**：等子行程的測試要給到 10 秒；release job 在測試紅了時是 skipped，`gh run rerun <id> --failed` 重跑就好，不用重打 tag。
- **撤回一版要連 tap 一起 revert**，不然 `brew install` 404。
- **驗證**：tap 的 formula 在 repo 根目錄（`sshu.rb`）；用 API 讀（raw 有 CDN 快取）；sha256 對三方（自己算、`checksums.txt`、formula）；煙霧測試是 `sshu version`。
- tdp v0.7.0 以前發的版本，CHANGELOG 的標題照當時的寫法（`## [2.0.0] — 2026-10-06`），不改。
