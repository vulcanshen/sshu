# sshu 開發者備忘

開發 sshu 時要提醒自己、以及與 AI 協作時記下的決策。sshu 遵循 [terminu design principle](https://github.com/vulcanshen/terminu/tree/v1.1.0/principle)（tdp），畫面建在家族共用的 lib [terminu-go](https://github.com/vulcanshen/terminu-go) 上；使用者要知道的在 README，這裡收的是另一半 —— 運作的細節、背後的理由、以及走過來的歷史。

靈感來自 [Termius](https://termius.com/) —— hosts、sessions、檔案傳輸收在同一個屋簷下 —— 不是照單全收它的功能清單。

---

## 運作方式

### 畫面由 terminu-go 畫

2026-10-18 起 sshu 的 UI 是 `internal/tui`，建在 terminu-go 上（2026-10-28 起 v1.1.0，tdp E8）：sshu 登記畫面、panel、每一個動作（鍵、名稱、說明、範圍、要不要先問、做什麼），lib 照這一份登記畫出畫面 chip 列、panel 框、Space menu、global operation popup、`?` 的 key reference、下框 hint、footer、confirm、表單、每一種 input、viewer、toast、開關動畫與淡化，也處理所有 core key。以前 sshu 自己畫這一切的 `internal/ui`（約三萬行，含測試）整個刪掉。

- `app.go`：`Data`（main 載入的檔案與寫回的函式）、三個畫面、啟動時讀不了的檔（MOVE ASIDE）、statusbar（傳輸進度、活著的 session 數，各自固定寬度）、chip 列下的進度條、離開流程（有 session 或傳輸時才先問，並說出會失去什麼）、兩本紀錄與 App Log 的寫入。
- `manage.go` 與各節的檔（`hosts.go`、`creds.go`、`sshcfg.go`、`known.go`、`journals.go`）：Manage 畫面。
- `transfer.go`、`fileops.go`、`xferjobs.go`：File transfer 畫面。
- `ssh.go`、`sshcmd.go`、`askpass.go`：SSH 畫面、ssh 的命令列、askpass 中繼。
- `detail.go`：把 host、credential、區塊、金鑰寫成一行一行給 confirm 與 viewer 讀。
- `proc.go`：`KillChildren`，每一種離開方式（`q`、`Ctrl-C`、SIGHUP）都帶走子行程。

### Manage

`[1] SECTIONS` 是分組的清單（SSHU：Hosts、Credentials；SSH：Config、KnownHosts；LOGS：Connections、Changes），`[2]` 是 cursor 那一節的明細（components/layout/panel 的「明細」）：cursor 一動，`[2]` 的 label、內容與動作就換成那一節的；`Enter` 把 focus 送到 `[2]`（tdp K3 的明細）。開 app 時 focus 就在 `[2]` 的 Hosts 表上（`App.Focus`，在 `started()`）：README 的 Quick start 第一步是按 `A` 加 host，focus 在 `[1]` 時 `A` 沒有作用。

- **Hosts**：`hosts.yaml` 的表格，`/` 是 panel filter（名稱、user、host、port、tags 一起比對）。`Enter` 是連線：先開一個 confirm，上面是這台 host 的明細（連線、驗證、`~/.ssh/config` 會給它什麼，以及哪些被 sshu 的命令列蓋掉），問句在最後（tdp F1：confirm 可以帶一段回答前要看的內容）。表單是 lib 的：`Auth` 是畫在表單上的 radio，`Credential` 開 select，`Identity file` 開 file-picker（從 `~/.ssh` 開始），`Password` 開 password input。password 必填 Password、privatekey 必填 Identity file（credential 表單也是）。Port 的 22 是預設值不是填的值：非 sshconfig 時顯示 22、存成 22；Auth 換成 sshconfig 時就不見了，空的 Port 讓 `~/.ssh/config` 決定，帶 `-p 22` 會蓋掉它（lib 的表單沒有換選項的回呼，所以用「沒填」加上顯示的預設值來做）。
- **Config**：`~/.ssh/config` 一個 `Host` 區塊一列，`Include` 會跟進去。編一個區塊只動它自己的那幾行：HostName、User、Port、IdentityFile 各一欄，每個選項記得自己原本在哪一行，沒改的一行原樣留著，清空的那一行拿掉；其他關鍵字列在 `Others`，只顯示（focus 跳過它）。表單的 `Edit file` 按鈕（`FormButton.Open`）在表單上面用 `$EDITOR` 打開那個區塊所在的檔（整個檔，`+<Host 那一行>`）：編輯器讀的是磁碟上的檔，所以表單有改動、或是還沒寫進檔的新區塊，先開 confirm 問要不要存（`Enter` 存了再開編輯器，`Esc` 回到表單）。編輯器結束時（terminal 的 `OnExit`）重讀檔案（`SSHConfigFile.Reload`），把這個區塊的值放回表單；區塊不見了，表單就當成新的區塊。重讀不用 `FormButton.Back`：它跟著 Open 打開的那一個 popup，先問要不要存時那是 confirm，confirm 一關它就跑了，編輯器還沒開。存檔時檔案被別的編輯器改過，就拒絕並換成磁碟上的版本。
- **KnownHosts**：`~/.ssh/known_hosts`。`[A]dd` 是一個表單（host、port），送出時去問那台機器要金鑰（在認證之前停下），再開 confirm 給你看指紋，`Enter` 才寫。
- **兩本紀錄**：Connections、Changes 各自一個表格，最新的在最上面，`Enter` 打開整本。`[C]lear` 先問，再清空那一本的檔案。失敗的事不在這裡，在 App Log。

### File transfer

`[1]` 左側檔案、`[2]` 左側標記、`[3]` 右側檔案、`[4]` 右側標記。兩側都是「某個檔案系統」，不分主從：`remote.FS` 一個介面兩種實作（本機與 SFTP），上傳、下載、遠端對遠端是同一條路徑。

- **檔案清單的第一列是目錄**：components/layout/panel 的位置列（`List.Location`）。panel 的 label 是 host 的名字，照原樣（`Panel.Name`）。
- **`h|l` 跨到另一側同一列的 panel**（tdp K12：左右結構）。`Esc` 回上一層目錄（tdp K4：「上一層」由 app 定義）。
- **標記照路徑記**，換目錄也留著；每次重畫時把這一側的標記照 ID 交給清單（`List.SetMarked`）。
- **`/` 是 finder**：整棵子樹邊走邊列，結果一直進來（components/input/finder：Loading 時可以一直加），`Enter` 到那一項所在的目錄、cursor 停在它上面。預覽是 lib 的檔案預覽；遠端那一側交給 lib 一個 `PreviewFS`（`previewfs.go`：把 `remote.FS` 轉成 lib 要的樣子，開的檔能從任何位置讀，zip 的目錄在檔尾、PDF 的頁數都讀得到），經 SFTP 讀。lib 在背景讀、讀過的在搜尋開著時記住（tdp v0.8.2），打字與移動不等遠端。
- **傳輸先規劃再開始**：幾個檔、多少 bytes、對面哪些已經存在都先算好，要覆蓋就問一次整批。傳輸在自己的 goroutine 跑、用 atomic 回報，畫面從不等網路；正在寫進來的那個檔，在清單上用 lib 的 loading 圖示標出來（`LoadingFrame`，全家族同一個會轉的圖示）。Jobs 的列跟著傳輸變（`Action.Live`），進行中那一列的 hint 是 `Enter:cancel`（`EnterHint`）。
- **`[e]dit`**：遠端的檔先抓到本機暫存，`$VISUAL`／`$EDITOR`（沒有就 `vi`）在 lib 的 terminal popup 裡跑；存回去之前比對內容，沒改就不寫；遠端在這段時間被改過就先問。編輯器怎麼結束都一樣看檔案內容，不看結束碼：vim 的 `:cq` 寫了檔再以 1 結束；`Alt-Esc` 停掉編輯器（lib 送 SIGHUP），vim 也以 1 結束、但沒寫檔，所以就是沒改。以前結束碼不是 0 就當放棄、刪掉暫存副本，`:cq` 的修改會丟。編輯器的環境裡把外層終端機的名字（`TERM_PROGRAM` 等）清掉：它跑在 sshu 的模擬器裡，不是那台終端機。
- **目錄每兩秒看一次時間戳**（SFTP 不會通知），變了才重讀；畫面不在 File transfer 時不看。

### SSH

`[1] SESSIONS`、`[2] LAYOUT`（一列一個值：Arrange 的 radio、Columns），之後每個 session 一個 panel（`[3]` 起），裡面是 lib 的 `Terminal`（components/dialog/terminal：PTY 放在 panel 裡）。

- **連線**：從 Hosts 連線時，sshu 切到 SSH 畫面、focus 新的格子、鍵盤直接進去（`App.Focus`；tdp T1：開 shell 清掉 source，`Action.Clear`）。鍵盤在格子裡時左欄收起，網格佔滿寬度；`Alt-z` 讓這一格佔滿整個 panel 區。
- **連線中**：ssh 第一次輸出之前（`Terminal.OnFirstOutput`），格子的框上轉 loading 圖示（`Panel.Loading`），打的字不送（`NotReady`，tdp K10）—— 免得幾分鐘後才落到遠端。格子中間寫「connecting to <host>」（`Panel.LoadingText`，components/layout/panel 的 loading）。
- **游標指著它的格子**（layout/panel 的「被指到的 panel」，`List.Points`）：`[1]` 有 focus 時，游標那個 session 在網格上的格子，框從 Surface2 換成 Lavender，仍是失焦的細線直角，膠囊串跟著換。session 沒上網格、或 focus 在別的 panel 時，沒有格子被指；鍵盤進到那一格就照 focus 畫。清單列和格子是同一個 session 的兩面：掃清單不用讀格子標題就知道是哪一格，Duplicate 之後新格子一出現就被指著。
- **離開**：`Alt-Esc` 先問（tdp K10），`Enter` 離開、`Esc` 回到 PTY；離開後 focus 回到 `[1]`、游標停在那個 session，它就指著剛離開的格子（`Terminal.OnLeave`，terminu-go v0.4.22；user 2026-10-25：v2.0.0 就是這樣，改建時掉了）。
- **換格**：`Alt-←|→|↑|↓`（一個 `TerminalKey`，`Keys` 收四個鍵）照目前的排法走到那個方向的格子。
- **選取、歷史**：ssh 的格子是互動 terminal（`Terminal.Interactive`，components/dialog/terminal：使用者在裡面打自己的指令）：`Alt-v` 是 lib 的選取模式（複製走系統剪貼簿的工具，`pbcopy`、`wl-copy`、`xclip`、`xsel`，跟 viewer 一樣），`PgUp` 進 lib 的歷史模式。`$EDITOR` 開的 PTY（File transfer 的 `e`、Config 的 *Edit file*）不標，按鍵連 `PgUp` 全給編輯器。
- **鎖定**：`Alt-Enter` 把每一個鍵交給格子 —— `Alt-Esc`、`Alt-z`、`Alt-` 方向鍵、`Alt-v`、`PgUp` 都是 —— 給格子裡巢狀的 sshu（`ssh.go` 的 `lock`）。鎖住時出口鍵換成 `Alt-Enter`（`Terminal.ExitKey`，tdp K10：出口永遠有一個是 app 的），其他保留鍵拿掉，`Terminal.AllKeys` 把選取與歷史也讓給格子裡的程式 —— `Alt-v`、`PgUp|PgDn` 送進去，最裡面那一層自己選、自己捲（components/dialog/terminal）；鍵盤離開格子就解鎖（`place` 看到鍵盤不在了就呼叫 `unlock`）。
- **格子增減**：結束、藏起來的 session 的格子當下就從畫面拿掉，lib 會把 focus 留在原本的 panel；只有有 focus 的那一格留著，寫出 session 怎麼結束，等 focus 離開才拿掉 —— session 結束是背景的事，不可以移動 focus（tdp K3）。
- **ssh 的原話**：ssh 以 `-E` 把自己的訊息寫進暫存檔。結束碼 255（ssh 自己放棄）時讀最後一行當原因：toast、App Log、Connections 都記；檔案隨即刪掉。遠端程式自己的輸出不受影響。
- **密碼**：password host 的 ssh 把 `SSH_ASKPASS` 指回 sshu 自己（`SSHU__ASKPASS_HOST`），helper 重讀 `hosts.yaml` 印出密碼 —— 密碼不進環境變數、不上命令列。
- **顏色深度**：ssh 只轉送 `TERM`，所以 sshu 用 `LC_SSHU_COLORTERM`（sshd 預設接受 `LC_*`）並加 `-o SendEnv`；遠端的 sshu 啟動時把它收進 `COLORTERM`。

### `auth: sshconfig`

只有名字與 destination，port、user 選填，其餘都交給 `~/.ssh/config`。SSH 畫面本來就跑真的 `ssh`。File transfer 為這種 host 跑 `ssh -s <host> sftp`（沒有終端機，`SSH_ASKPASS_REQUIRE=force`），所以 ProxyJump、agent、有 passphrase 的私鑰都能用；ssh 要問的（密碼、passphrase、未知的 host key）經 `SSHU__ASKPASS_SOCK` 的 socket 交給畫畫面的那個 sshu，開 confirm 或 password input 問使用者，答案直接回給 ssh、不存。

另外三種 auth 的 SFTP 走 `golang.org/x/crypto/ssh` 自己講協定：未知的 host key 直接拒絕、變過的 key 也拒絕（不把它變成 yes/no 題）。

### 資料與檔案

- **位置（tdp E3：看刪掉會怎樣）**：設定目錄是 `$SSHU__CONFIG`，否則 `$XDG_CONFIG_HOME/sshu`，否則 `~/.config/sshu`，每個平台同一套，裡面只有 sshu 自己的設定檔 `config.yaml`。`state.yaml`（SSH 網格怎麼排，一列一個值的 panel 改了就存）刪了只是回到預設，是狀態，也在設定目錄，`SSHU__STATE` 可以搬走。`hosts.yaml`、`credentials.yaml` 是使用者寫給 sshu 的清單，刪了就找不回來，是資料（tdp v0.9.0 起，以前跟 `~/.ssh/config` 一樣算設定）；`.sshukey` 跟著它加密的兩個檔；兩本紀錄也是資料。資料放 `~/.sshu`，`SSHU__DATA` 可以搬走，`SSHU__KEY_FILE` 另外搬鑰匙。
- **換位置時自己搬**（`internal/store/migrate.go` 的 `Migrate`，啟動時在讀任何檔之前；tdp E3 的「換了位置時」，v0.8.1 起逐項搬）：macOS 上以前照 `os.UserConfigDir` 放在 `~/Library/Application Support/sshu`，裡面的東西一項一項搬到 `~/.config/sshu`；資料（`hosts.yaml`、`credentials.yaml`、`.sshukey`、紀錄，連同舊版留下的 `errors.yaml`）以前在設定目錄，一項一項搬到 `~/.sshu`；`SSHU__DATA` 有設也搬過去 —— 變數說的是資料要放哪，不是資料已經在那裡。新位置已經有的那一項留在舊位置，開 error popup 寫出兩個路徑請使用者自己合併；`.sshukey` 與它加密的 `hosts.yaml`、`credentials.yaml` 一起搬或一起留：新位置已經有三個裡的任一個，三個都留，免得密碼跟一把解不開它的鑰匙放在一起。error popup 分開說兩種原因：新位置已經有的（`Move.Left`），與新位置沒有、只是跟著鑰匙留下的（`Move.Kept`）—— 以前混在一起，寫成「`~/.sshu` already has .sshukey, hosts.yaml」，其實 `~/.sshu` 只有鑰匙。有一項搬不動，就把搬過去的搬回來、照舊用舊位置（`Dir`、`DataDir` 記得它，`SSHU__DATA` 也讓位），開 error popup 寫出兩個路徑請使用者自己搬。搬完空了的舊目錄拿掉。搬好了用 Info toast 說從哪搬到哪（跟啟動時的問題 toast 合成一則：第二則 toast 會蓋掉第一則），搬了哪幾項記進 App Log。`uninstall.sh` 在 macOS 上另外問舊目錄（有東西留在那裡時它還在）。
- 每次寫入都是原子的（暫存檔 + rename），固定 `0600`，檔案開頭帶一段警告。
- 舊格式的 `hosts.yaml`、`credentials.yaml` 在啟動時改寫成當前格式；更新版 sshu 寫的檔絕不覆蓋。明文密碼也在啟動時就地加密。
- `config.yaml` sshu 永遠不寫。
- **讀不了時**（tdp E3「讀不了時」）：sshu 自己的檔 —— `config.yaml`、`state.yaml`、`.sshukey`、`hosts.yaml`、`credentials.yaml`、兩本紀錄 —— 讀不了（格式錯、壞掉、沒有權限），讀不了的先當空的，啟動時照這個順序一個一個問 MOVE ASIDE（`App.MoveAside`）：`Enter` 改名成 `.bak`（`.bak.1`……，lib 找空的名字），照第一次打開那樣開始（tdp v0.9.2 的 E3-18）：第一次打開就會建的鑰匙（`EnsureKey`）馬上再建一把，其他的檔第一次打開也不會建，等第一次存檔時才有 —— `TestMovingAsideStartsAsOnTheFirstStart` 把七個檔逐一弄壞，比對按 `Enter` 之後磁碟上的檔跟第一次打開時一樣；`Esc` 離開，`Run` 的錯誤由 main 印到 stderr。全部問完才做啟動的其他事（還原網格、搬家的 toast），還沒問到的檔不會先被存檔蓋掉；`reconcileFiles` 也不改寫讀不了的檔。
- **鑰匙讀不了**：問的是鑰匙，不是 `hosts.yaml`。`decryptInto` 對讀不了的鑰匙跟沒有鑰匙一樣：密文原樣留著、名字列進「解不開的密碼」，所以 `hosts.yaml` 照樣讀得進來；以前它回錯誤，壞掉的鑰匙會讓 `hosts.yaml` 被當成讀不了的檔。`Enter` 之後 `EnsureKey` 產生新的鑰匙；舊的在 `.sshukey.bak`，放回去就打得開。鑰匙讀不了時存檔一律拒絕（`EnsureKey` 回錯誤），不會用一把讀不了的鑰匙封東西。
- **`~/.ssh` 不是 sshu 的**：`~/.ssh/config`、`known_hosts` 讀不了時照常開著，那一節中間寫原因（`Panel.Fail`，紅字），細節進 App Log。Config 的 *Edit file* 之後重讀失敗也一樣，另外跳 Error toast，舊的內容留著。
- **App Log**（tdp F5，lib 的 `!`）：toast 與 error popup 由 lib 自動記。sshu 另外記：啟動時的問題（解不開的密碼、重複的名字、新版 sshu 寫的檔、`~/.ssh` 讀不了），搬了哪幾項，失敗的事（在哪台、對誰，帶著遠端的原話），以及寫進 Changes 的每一筆。一次失敗也有 toast 或 error popup，所以在 App Log 是兩行（toast 那一句與 sshu 補上在哪、原話的那一句），footer 的未讀錯誤也算兩個；sshu 那一句沒有多說什麼的（存 `~/.ssh` 失敗、credential 改名後還有 host 用舊名字），只留 lib 記的那一句 —— 以前多寫一次是為了 Errors 那本。
- **使用者執行的每個動作都在 App Log**（tdp note.7，v0.11.0）：Space menu 列的每一個動作（item、panel、global operation，
  從 menu、熱鍵或 `Enter` 執行）都留一筆。結果本來就有 toast、error popup 或 `logChange` 的，那一句就是；沒有的由
  `logOp` 記：只看的（`Viewed credential ops`、`Viewed ~/a/x.txt on local`、`Opened Jobs`）、
  只改畫面的（`Marked …`、`Hid deploy@10.0.3.14 #1`、`Set Arrange to vertical`）、連上的（`Opened session …`、
  `Opened local on the left side`）。切換畫面由 lib 記（`Switched to SSH`），sshu 自己沒記過，不用拿掉。不記：移動 cursor、
  換 focus、`h|l` 換邊、Sections 按 `Enter` 進那一節（進明細只是換 focus，tdp v0.11.3 的 note.7）、
  `Esc` 回上一層目錄（core key，不是 P3 的動作）、lib 的 panel filter（tdp v0.11.1 起寫明不記）。在動作前面的 confirm、填值的表單或 input popup 上按 `Esc`
  取消的也不記：動作沒做（tdp v0.11.2 的 note.7）。Edit 存回去時的 Overwrite 按 `Esc` 不算：Edit 已經做了，
  沒存回去、改過的那份留在哪是它的結果，照樣有 toast（見設計文件 §12.17）。傳檔畫面的路徑照位置列寫成 `~/…`。
- **短字裡不放 `\n`**（tdp components「控制字元」）：toast、error popup、confirm 的 Question、panel 中間的 `Failure` 都是「名字與短字」，`\n` 照字面畫成紅色的 `\n`，不換行。sshu 自己的字不帶換行，幾句的一句一列（「Not moved」、「Edit failed」、啟動時的 toast；lib 的 `Error`、`Toast` 一列收一個字串）；外面來的多行字只放最後一行 —— ssh 在 stderr 印的 banner 加原因，最後一行是原因（`dialReason`），全文進 App Log（拿來讀的內容，可以多行）。confirm 要幾行的放 `Details`（ssh 問的 host key 問句就是拆成幾行放那裡）。這些 `\n` 是 tdp v0.9.0 以前寫的，那時 lib 會換行；升級時規則變了，編譯與 tdptest 都抓不到，error popup 的字又是紅的，紅色的 `\n` 不顯眼，直到 user 手測才看到。測試在這些畫面另外看有沒有 `\n` 這兩個字。回報 tdp 後定成 tdp v0.9.15（components-30～32）：app 自己的字一律不帶換行，要好幾列的地方（error popup 的訊息、toast、confirm 的內容）由 app 一列一列給，不空列；外面來的好幾行只放說原因的那一行，全文進 App Log。terminu-go v0.4.19 起 error popup 與 toast 一列一列收，tdptest 的 Driver 也預設抓畫面上紅色的 `\n`；sshu 只有故意拿 `tdptest.Tricky` 當名字的測試放行（`AllowLineBreaks`）。
- **App Log 大寫開頭**（tdp v0.9.2 的 note-78：跟 toast 一樣；toast 一字不改記進去，所以 toast 也算）：名字與路徑是內容、不改大小寫（components「大寫」），所以開頭是名字的句子改寫成動詞或名詞在前、名字在後，照 tdp 的例子 `Scaled deployment.apps/nginx`：`Session to db-replica failed: …`（以前 `db-replica: …`）、`Can't read ~/.ssh/config: …`、`Ignored 2 duplicate entries in hosts.yaml …`、`Deleted Host "x" from ~/.ssh/config`。整句是 Go error 的（`err.Error()`）由 `capital` 把第一個字母改大寫。Changes 那一本：以路徑開頭的幾種跟著改寫，其他照原樣存（`credential "x" deleted`），大寫只在 App Log（`logChange` 的 `capital`）。

### icon 與 powerline 符號的寬度

探測與量寬度交給 lib（tdp E5、tdp E8）：啟動時探測 icon 與 powerline 符號各佔幾格，`SSHU__ICON_WIDTH`／`TERMINU__ICON_WIDTH`、`SSHU__POWERLINE_WIDTH`／`TERMINU__POWERLINE_WIDTH` 可以覆寫；lib 開的每一個 PTY 都設好兩個 `TERMINU__` 變數。

隔著 ssh 時送是 sshu 的事、收是 lib 的事（tdp E5、tdp E8）：格子裡的 ssh 帶上 `LC_TERMINU_ICON_WIDTH`、`LC_TERMINU_POWERLINE_WIDTH`（這一層自己的格數），跟顏色深度一樣加 `-o SendEnv`；遠端不管跑哪一個家族 app 都讀得到 —— 遠端的探測由這一層的模擬器回答、量不到真的寬度。以前的指令通道（OSC 7181）做的其他事 —— 看到整條巢狀鏈、鏈上的鎖定與放大 —— 定案不做。

## 設計決定

- **零學習成本是結構保證的。** 每個動作登記一次，Space menu、熱鍵、`?`、hint 都從同一份登記來，「menu 裡沒有的熱鍵」不可能存在。
- **分開的紀錄，不是一本 log。** 連過哪些機器、動過什麼，該長的樣子不一樣：連線紀錄一列一筆好往下數，改動是一句話。出了什麼事（遠端吐出來的十五行）以前是第三本 Errors，現在是 App Log 的。
- **tag 只顯示、只被搜，sshu 從不解讀它。** 空白分開，其餘都是字面。理由：一個會被程式解讀的欄位，使用者就得學它的規則；tag 是使用者拿來分群的字，能被篩選找到就是它全部的用途。
- **密碼加密是把祕密拆成兩份。** `hosts.yaml` 或 `credentials.yaml` 單獨流出去不會漏密碼；整個資料目錄被複製就擋不住，因為鑰匙在裡面。`SSHU__KEY_FILE` 不做成預設：鑰匙離開資料目錄，備份就得自己記得。
- **Errors 併進 App Log，不再存在磁碟上**（user 2026-10-21 改判，原話「errors 合併到 app log 裡面, 就不在獨立存 errors」）：Manage 的 Errors 與 `errors.yaml` 拿掉，失敗只記進 App Log。之前暫定兩個並存，理由是 App Log 只在記憶體、只記這一次，Errors 跨次留著「上週連那台為什麼失敗」；terminu 說那是它判錯了，升級交代的「錯誤歷史改用 App Log」原本就是這個意思。代價是關掉 sshu，失敗的原話就沒了；連過哪台、成功還是失敗，Connections 照樣留著。statusbar 的未讀錯誤數也拿掉，交給 footer 的 `!:N errors`：兩個紅色的計數，使用者不知道該看哪一個。
- **不再用的檔原樣留著，每次啟動在 App Log 說一聲**（user 2026-10-21 選了只說一聲的做法，原話「我記得 app log 是不寫檔案的，每一次開啟只紀錄在記憶體中，所以應該是 B, 告知使用者狀況，要怎麼處理是使用者的事情」；tdp v0.9.3 的 E3-23）：舊的 `errors.yaml`（v1.6.0 起到上一版寫的）與 `applogs.yaml`（v1.5.1 以前唯一的 log）不讀、不刪、不跳 toast。還在的，每次啟動由 lib 的 `App.NoLongerUsed` 在 App Log 記一筆 INFO，寫出路徑；`store.NoLongerUsed` 給出它們會在的地方 —— `applogs.yaml` 在設定目錄，`errors.yaml` 在資料目錄，搬家時留在設定目錄的也算。`errors.yaml` 照 E3「換了位置時」照舊跟其他資料一起搬到 `~/.sshu`，舊目錄才會空、才拿得掉。CHANGELOG 也寫了。否決：讀進來放進 App Log —— App Log 只記這次開 app 的事。`history.yaml`、`activity.yaml` 是 `connections.yaml`、`changes.yaml` 的舊名字，在同一版（v1.6.0）發版前就改了，沒有使用者有這兩個檔。
- **`D` 一律是複製、`x`/`X` 一律是刪除。** 刪除原本在 `D`；同一個字母在不同 panel 一個複製、一個刪除，按錯的代價是資料，所以統一成整個 sshu 一個字母一個意思。File transfer 的刪除不放在 `d`：`d` 是半頁（tdp K12）。
- **檔案上的 `Enter`**：目錄是進去，檔案是用 viewer 讀（以前檔案上的 `Enter` 什麼都不做）。理由：tdp K3 要 `Enter` 做最直觀的那件事；在一個檔案上按 `Enter` 什麼都不發生，看起來像鍵壞了，而「讀它」不會改到任何東西。
- **Jobs 是 menu**：進行中的那一列 `Enter` 打開帶全文的 confirm、hint 寫 `Enter:cancel`，結束的那一列 `Enter` 看全文（tdp F1：工作清單）。以前的 `c` 取消游標那一筆，menu 沒有「作用在游標那一列」的熱鍵，所以改成這樣。
- **Space menu 留不留照 tdp T1 的表**：lib 預設打開東西就留著 menu（看東西、改一個值、用 `$EDITOR` 改），什麼都沒打開的動作做完 menu 就關。連線、Duplicate（開 shell、換畫面）與搜尋設 `Action.Clear`；搜尋清掉是因為選了結果就到別的目錄，跟進目錄一樣。Delete、Close、Clear 的 confirm 是 `Action.AskFor`（`app.go` 的 `gated`，動作執行時才問 cursor 那一項）：confirm 只是門，`Esc` 回到 menu，`Enter` 之後什麼都沒打開，menu 就關。傳輸（`t`、`T`）的覆蓋也是動作執行時才規劃、有同名檔才問（`asked`），問不問都一樣收掉 menu。`e` 的編輯器要等檔案抓下來才開，所以設 `Stay`，menu 在這段時間留著（表上「用編輯器改那一項」保留）。
- **focus 只在使用者預期的時候換**（tdp K3）：從 Hosts 連線後鍵盤在新的格子裡、SESSIONS 的 `Enter` 進那一格，都是按下去就預期會換的；背景的事（session 結束、傳輸做完）不搶 focus。
- **`Alt-z` 兩段**：佔滿 panel 區、還原。以前的第三段（格子佔滿整個終端機、不畫框）定案不做：那是 sshu 自己的需求，不變成通用的版面。
- **最低尺寸 60×20**（tdp L1：app 可以調低，不能高於 60×32；tdp v0.11.0 以前是 80×40）：一個 80×24 的終端機裡，放大的格子有 78×20，巢狀在裡面的 sshu 要畫得出來；60 欄時每個畫面與表單都還完整好用（長欄 —— 名稱、位址、路徑、一行文字 —— 標 `Column.Shrink` 先截短，短欄照最寬的值；還放不下才照 `Column.Drop` 整欄收，Hosts 先收 Tags、Auth），畫面測試也跑 60×20。60 欄時每個畫面的 panel 都並排（tdp L1、
  screen.5，v0.11.0）：File transfer 原本 72 欄以下才一次只畫一個 panel，現在門檻是 60，兩邊各 30 欄，檔名從前面截短。
- **放大只在鍵盤在格子裡時有效**（components/layout/screen 的 zoom：不照預設的寫在這裡）。預設是放大一直留著、`Tab` 與數字鍵換到的 panel 也放大、再按一次放大的鍵才還原；sshu 的 `Alt-z` 是格子裡的保留鍵（tdp K10），`Alt-←|→|↑|↓` 換到的格子照樣放大，但 `Alt-Esc` 把鍵盤收回來時放大就還原（`ssh.go` 的 `place`）。理由：鍵盤離開格子之後 `Alt-z` 就不在任何地方了 —— panel 上沒有這個鍵 —— 放大留著，網格與 SESSIONS 都被蓋住，卻沒有一個看得到的鍵能還原。

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
- **等 lib 的**：目前沒有。改用 terminu-go 以來回報給 terminu 的問題都已定案，`docs/sshu-terminu-fix.md` 全部修完、刪掉（tdp E7）。
- **File transfer 的 host key**：password、privatekey、credential 的 host 遇到未知的 host key 一律拒絕；先從 SSH 畫面連一次，或在 KnownHosts 按 `[A]`。
- **加密私鑰**：privatekey host 的 SFTP 用不了有 passphrase 的私鑰；改用 sshconfig host（走 ssh 與 agent）。
- **`SSH_ASKPASS_PROMPT=none` 的通知**：FIDO 觸碰確認會讓 helper 掛著，sshconfig host 用 FIDO key 在 File transfer 會卡住。
- **`Alt-Esc` 誤觸**：遠端跑 vim 時快速連按兩次 `Esc` 會被讀成 `Alt-Esc`；離開格子與結束編輯器都先問，誤觸按 `Esc` 就回去。
- **未做**：Mouse；fsnotify 重讀 `hosts.yaml`；keychain 存密碼；Export / Import（以前實作了但一直遮罩著，搬到 lib 時沒有搬）。

## 偏離 tdp

目前沒有。改用 terminu-go 時寫的四條都已定案、不再是偏離（2026-10-19，tdp v0.8.0）：macOS 的舊設定目錄改成第一次啟動時搬過去（tdp E3）；File transfer 的目前目錄是 components/layout/panel 的位置列；SESSIONS 的 `Enter` 進格子是使用者預期的 focus 移動（tdp K3）；`~/.ssh/config` 區塊的其他選項不再用 textarea，改成只顯示加 `Edit file`（components/input/textarea：設定檔交給 `$EDITOR`）。

以前唯一的一條 —— SSH tab 的 `Tab` 不作用（tdp K2）—— 隨著改用 lib 不再存在：SSH 畫面的 `Tab` 照 tdp K2 在 panel 之間輪替，格子也是 panel；鍵盤在格子裡時 `Tab` 屬於遠端（tdp K10）。

## 設計文件導讀

| 檔案 | 回答什麼 |
|---|---|
| [`sshu-ui-design.md`](sshu-ui-design.md) | 改用 terminu-go 以前（到 v2.0.0、tdp v0.1.23）的完整設計紀錄：每一個看得見的行為為什麼是這樣，以及試過而被否決的做法（§A、§B、§1–§11.73）。§12 起記改用 terminu-go 之後的事 |
| [`icon.svg`](icon.svg) | 家族的 mark：藍色 U 框裡兩個 S 與一個 H，一個 XML 註解一個字（U、S、S、H，tdp S5）；splash 照它逐格畫，`internal/tui/icon.svg` 是 `go:embed` 用的副本，`TestTheIconIsTheSplash` 守住兩份一樣 |

Go、[terminu-go](https://github.com/vulcanshen/terminu-go)（建在 Bubble Tea v1 與 Lip Gloss v1 上），檔案傳輸用 [pkg/sftp](https://github.com/pkg/sftp) + `golang.org/x/crypto/ssh`。配色是 catppuccin-mocha（lib 的）。

## 建置與開發

從原始碼：`go install github.com/vulcanshen/sshu/v3/cmd/sshu@latest`。clone 之後跑 `make` 列出所有指令（家族共用的在 `family.mk`，像 `make dev` 編譯再跑 `./sshu`；sshu 自己的只有 `make demo`：用 `demo/hosts.yaml` 跑，不碰你的設定）。

- **lib 的版本**：`go.mod` 寫 terminu-go 發布的 tag。本機開發用 git 不收的 `go.work`（`go work init . ../terminu-go`）接上 lib 的工作目錄；`make check` 有 `go.work` 時另外跑 `check-lib`（關掉 `go.work`、照 `go.mod` 編譯與測試），過不了就是用到了 lib 還沒發版的東西。
- **`make check`**：`fmt-check`、`vet`、`test-race`、`check-lib`，commit 前跑。
- **看畫面**：`SSHU__DUMP="100x30:2,Enter" go test ./internal/tui -run TestDump -v` 印出按了那串鍵之後的畫面（假資料、假的 ssh）。在真的終端機裡看：`.local/shot.sh -s 100x30 -- Escape 2 Space`（git 不收；在 tmux 裡用 demo 資料跑真的 binary，HOME 是丟棄式的，`-i 2`、`-p 2` 讓 icon、powerline 符號佔兩格）。
- **畫面測試**：每一個畫面在 icon 與 powerline 符號各種寬度、好幾種終端機尺寸下，每一列都剛好是終端機寬（tdp L4、tdp E5、tdp E6）；sshu 會開的每一種 popup 疊在它的畫面上也一樣（`popups_test.go`，terminal popup 在 `editor_pty_test.go`）；比最低尺寸小、一直到 1×1，不當掉、畫出名稱與提示，拉大後開著的 popup 還在（tdp E8 的分工：單獨量 popup 的框是 lib 的）。popup 的開關動畫每次約 0.6 秒，所以 popup 只在其中幾種尺寸打開。
- **跑 PTY 的測試**（`internal/tui/ssh_pty_test.go`、`editor_pty_test.go`）用假的 ssh 與編輯器腳本（`fakeSSH`），也在 `-race` 底下跑。
- **README 的 gif**（`docs/demo-grid.gif`）：`bash .local/demos/setup.sh` 起 demo 用的 sshd 容器、寫好假的 HOME 與資料，`make gif` 用 VHS 跑 `.local/demos/demo-grid.tape`（tape 在 tmux 裡開 sshu，`drive-grid.sh` 照時間送鍵），再跑 `sh .local/demos/optimize.sh` 縮小檔案。改了畫面或按鍵，先在 tmux 裡照同一個 driver 空跑、每秒存一張畫面，確認每一步都按對了再錄。`bash .local/demos/teardown.sh` 停掉容器。
- **頂層的其他目錄**：`demo/`（`make demo` 的 hosts）、`.local/demos/`（VHS 的 tape 與 demo 用的 sshd 容器，git 不收）。`CLAUDE.md` 與 `.claude/` 是維護者自己的 Claude Code 檔，git 不收（tdp project.3）。

## 發布

照 tdp project 的「發布」：`make check` → CHANGELOG 的 `## Unreleased` 改成 `## vX.Y.Z — tdp vA.B.C` → commit → 打 tag、push commit 與 tag，`release.yml` 跑測試、goreleaser 發 release 並推 Homebrew tap `vulcanshen/homebrew-tap`，再用 CHANGELOG 那一段蓋過 release 的內文。版本號與發版時機由 user 定；2026-10-27 起 sshu 只做到 commit，打 tag、push、發版由 terminu 在 user 下令後做。

**beta**（tdp project.6、project.7）：tag 是 `vX.Y.Z-beta.N`，CHANGELOG 的標題也是（`## v3.0.1-beta.1 — tdp v1.1.0-beta.2`）。goreleaser 把它發成 GitHub 的 pre-release、不推 tap（`prerelease: auto`、`skip_upload: auto`），brew 與 `install.sh` 照舊拿最新的正式版。之後的正式版寫從上一個正式版以來的全部改動，beta 的也算進去。

踩過的坑：

- **auto mode 會擋推 tag**（v2.0.0）：推 `v*` tag 會觸發發版，分類器把它判成對外公開；不要換寫法繞過，請 user 換 edit mode 或自己推。
- **環境裡的 `GITHUB_TOKEN` 蓋過 keyring 的 credential**：推 workflow 檔要 `env -u GITHUB_TOKEN git push …`。
- **新 repo 的第一個 tag 可能不觸發 workflow**：刪掉 tag 再推一次。
- **macOS runner 在 `-race` 下慢**：等子行程的測試要給到 10 秒；release job 在測試紅了時是 skipped，`gh run rerun <id> --failed` 重跑就好，不用重打 tag。
- **撤回一版要連 tap 一起 revert**，不然 `brew install` 404。
- **驗證**：tap 的 formula 在 repo 根目錄（`sshu.rb`）；用 API 讀（raw 有 CDN 快取）；sha256 對三方（自己算、`checksums.txt`、formula）；煙霧測試是 `sshu version`。
- **module path 要帶大版號**（tdp project.3）：到 v3.0.1-beta.1 為止 module path 是 `github.com/vulcanshen/sshu`，沒有 `/v3`，Go 不把 v2 以上的 tag 當成它的版本 —— 執行檔的 build info 寫的是 pseudo-version（v3.0.0 的是 `v1.7.2-0.20261027094627-9a0ace964f76+dirty`），舊路徑的 `go install …@latest` 也只找得到 v1。v3.0.1-beta.2 起是 `github.com/vulcanshen/sshu/v3`，import 跟著改，goreleaser 的 ldflags 用 `{{ .ModulePath }}`；以後跳 major 時在同一個 commit 改 module path。`sshu version` 照 ldflags 印 tag，一直是對的。`+dirty` 是另一件事：release notes 寫在 checkout 裡，tdp v1.1.0-beta.2 改寫到 `$RUNNER_TEMP`。
- tdp v0.7.0 以前發的版本，CHANGELOG 的標題照當時的寫法（`## [2.0.0] — 2026-10-06`），不改。
