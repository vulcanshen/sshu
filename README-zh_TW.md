# sshu

[![GitHub Release](https://img.shields.io/github/v/release/vulcanshen/sshu)](https://github.com/vulcanshen/sshu/releases)
[![Go Version](https://img.shields.io/github/go-mod/go-version/vulcanshen/sshu)](https://go.dev/)
[![License](https://img.shields.io/badge/license-GPL--3.0-blue)](LICENSE)

**語言**: [English](README.md) · 繁體中文

**ssh 與 sftp 的終端機前端** —— `Tab` / `Enter` / `Esc` / `Space` / `?` 驅動一切。host 收在一個檔案裡,想開幾個 shell 就開幾個,任兩台機器之間的檔案並排搬。不用背快捷鍵、不用設定、零學習成本。

> _不確定的時候,就按_ **`Space`**。

sshu 是 `u`-family 的成員,也是 [Vulcan's TUI Design Principle](https://github.com/vulcanshen/thoughts/blob/main/vtp.md) 在 ssh 領域的實作 —— 跟 [kbu](https://github.com/vulcanshen/kbu)(Kubernetes)、[filu](https://github.com/vulcanshen/filu)(filesystem)同一套設計系統。逐條對照的紀錄在 [`docs/sshu-implementation.md`](docs/sshu-implementation.md),背後的判斷過程(**包含試過而被否決的做法**)在 [`docs/sshu-ui-design.md`](docs/sshu-ui-design.md)。

## 五個鍵就能驅動 sshu

| 鍵 | 行為 |
|---|---|
| **`Tab`** | 移到當前 tab 的下一個 panel(`1`–`3` 換 tab,`4`–`7` 直達某個 panel) |
| **`Enter`** | 連線 / 進入目錄 / 確認選擇 |
| **`Space`** | *我在這裡能做什麼?* —— 當前 focus 的 contextual menu。也用來關掉任何浮層 |
| **`Esc`** | 退一層 —— 離開搜尋、回上層目錄、關掉最上面的浮層 |
| **`?`** | 全域說明 —— 所有跨 surface 的動作列在一起 |

不確定就按 `Space`。字母快捷鍵是給熟了之後求快用的,而**每一個都同時是 `Space` menu 裡的一列** —— 所以除非你想背,否則沒有任何東西需要背。

## 三個 tab

```
 ([1] hosts)  ([2] sftp)  ([3] ssh)
```

**`[1]` hosts** —— 一張蓋在 `hosts.yaml` 上的表格:name、user、host、port、auth,一列一台,終端機變窄就逐欄收起。`[A]dd` / `[E]dit` 打開帶即時驗證的表單;IdentityFile 欄位按 `Tab` 會開一個 fuzzy 檔案選擇器,顯示權限並標出「別人讀得到」的金鑰。`Enter` 連線。

**`[2]` sftp** —— 兩個各自獨立的檔案系統並排,1:1。`local` 開在你啟動 sshu 的目錄,所以 `cd ~/release && sshu` 一進去就在那批東西上。任一側可以是本機或某台已存的 host,而且**兩側都可以是遠端**,所以上傳、下載、遠端對遠端是同一個操作而不是三個。標記你要的、跨到另一邊、送出。`/` 搜尋的是**整棵子樹**,不是螢幕上那個目錄;`v` 不用抓下來就能讀,`e` 直接用你自己的編輯器開。

**`[3]` ssh** —— 多個並存的 session,每一個都是跑在 embedded terminal 裡的真 `ssh`,一次顯示一個。遠端拿著鍵盤時 `[5]` 佔滿整個 tab;`Alt+Esc` 把鍵盤收回來。

## 安裝

> sshu **只支援 macOS / Linux** —— 它用 Unix PTY,沒有原生 Windows build。

目前還沒有 release binary,請從原始碼編:

```bash
go install github.com/vulcanshen/sshu/cmd/sshu@latest
```

或 clone 下來編:

```bash
git clone https://github.com/vulcanshen/sshu.git
cd sshu
make build     # → ./sshu   (CGO_ENABLED=0、-trimpath、已 strip)
./sshu
```

`Makefile` 包好了常用的事 —— `make build`、`make install`(→ `$GOBIN`)/ `make uninstall`、`make demo`(用 `demo/hosts.yaml` 跑,不碰你真正的設定)、`make package`(產 `dist/` 底下的 `.tar.gz`)、`make check`(fmt + vet + test)。直接跑 `make` 會列出全部。

**Nerd Font 是必要條件、不是選配**:auth 方式、檔案型別、marks 都用 Nerd Font glyph 畫,而且版面會去量它們的寬度。

## 快速開始

```bash
sshu
```

開在 `[1]` hosts。按 `[A]` 加第一台 host,`Enter` 連線,`2` 切到檔案瀏覽器。第一次用的話,在任何 panel 上按 `Space` 讀一下那個 menu —— 它列的就是這個 panel 能做的全部。

## 你的資料放在哪

一個檔案 `hosts.yaml`,依這個順序解析:

| | |
|---|---|
| `$SSHU_CONFIG` | 直接指定目錄(`make demo` 和測試用的就是它) |
| `$XDG_CONFIG_HOME/sshu` | 有設就用 —— macOS 上也一樣,所以你可以不要 `~/Library/Application Support` |
| 都沒有 | `os.UserConfigDir()/sshu` |

它是可以手改的 YAML,而且 sshu 在檔案自己的標頭裡就這樣說。寫入是原子的(暫存檔 + rename),而且**每次寫入都重新確立 `0600`**。

### 密碼是明文存的 —— 請讀這段

`auth: password` 的 host 會把密碼**明文**放在 `hosts.yaml` 裡。這是一個刻意的取捨,以下是配套:

- 檔案維持 `0600`,每次寫入重新確立,而且帶一段警告標頭
- 密碼永遠不會被畫出來 —— 表單顯示的是 `••••`
- 用 `SSH_ASKPASS` 把它交給 `ssh`,所以這個祕密**不會進到子行程的環境變數**,也不會出現在 `ps`

`0600` 撐不過「被複製進備份、dotfiles repo 或同步資料夾」。如果你在意這件事,請用 `auth: privatekey` —— 它只存一條路徑。keychain 撐腰的 `secretStore` 是規劃中的替代方案。

### Host key

`[3]` ssh tab 是把真的 `ssh` 執行檔叫起來,所以那邊的 host key 處理就是 OpenSSH 的,連同你的 `~/.ssh/config` 和 `known_hosts`。

`[2]` sftp tab 自己講協定,而它的政策更嚴:**未知的 host 直接拒絕**而不是放行,**變過的 key 直接拒絕**而且不會拿出來當問題問你。要接受一台新的機器,先用 `[3]` 連一次 —— 那是 OpenSSH 的提示,用 OpenSSH 的指紋。

## 按鍵

底下每一個字母快捷鍵,同時都是那個 panel 的 `Space` menu 裡的一列。**方括號印的大小寫就是你要按的那個鍵**:`[A]dd` 是 shift+A、`[t]ransfer` 是裸的 `t`,沒有標出來的東西不會動。

### 到處都通

```
 tab       1 2 3                      panel     4 5 6 7  ·  Tab(只在當前 tab 內)
 游標      j k    u d(半頁)          gg G      方向鍵同義
 全域      Space menu    ? help    q 離開    Ctrl+C 強制離開
```

### `[1]` hosts

| 鍵 | 動作 |
|---|---|
| `Enter` | 連線(先問) |
| `A` | 新增 host |
| `E` | 編輯游標這一台 |
| `D` | 刪除(先問) |
| `/` | 搜尋 —— name / user / host / port 一起比對(**不含** auth 欄),依分數排序 |

表單裡:`Tab` / `Shift+Tab` / `↑` `↓` 換欄位,`←` `→` 切 Auth,**IdentityFile 欄位按 `Tab` 開檔案選擇器**,`Enter` 送出,`Esc` 取消。

### `[2]` sftp —— 小寫是游標那一列,大寫是整個 panel

| 鍵 | 動作 |
|---|---|
| `h` `l` | 跨到另外半邊,保持同一列(`[5]`↔`[7]`) |
| `Enter` | 進入游標所在的目錄 |
| `m` | 標記 / 取消標記(在 marks panel 上,`m` 就是取消) |
| `r` | 就地改名 |
| `v` | **讀它** —— 文字帶語法上色與行號,二進位轉 hex,目錄列出內容 |
| `e` | 用 `$EDITOR` **編它** —— 抓下來、編、寫回去 |
| `t` | 傳到另一側的當前目錄 |
| `x` | 刪掉(先問) |
| `/` | **搜尋整棵子樹** —— 結果就是普通的列,所以 `m` / `t` / `x` 在上面照樣能用 |
| `A` | **新增** —— `name` 建空檔,`name/` 建目錄 |
| `T` | 傳這一側全部的 marks |
| `X` | 刪這一側全部的 marks(先問) |
| `C` | 清空 marks —— 只是忘記它們,磁碟上什麼都不動 |
| `S` | 選 host —— `local` 排第一,而且開在**你啟動 sshu 的那個目錄** |
| `P` | 進度 —— 進行中的傳輸,可逐條取消 |

### `[3]` ssh

| 鍵 | 動作 |
|---|---|
| `Enter` | 在 `[5]` 顯示這個 session(不確認 —— 切換不花任何代價) |
| `C` | 關掉這個 session(先問) |
| `D` | 複製 —— 對同一台再開一個 session(先問) |
| `H` | 歷史 —— 已經結束的 session,以及為什麼結束 |
| **`Alt+Esc`** | **從遠端手上把鍵盤收回來** |

列上寫的是 `<user>@<host>` 加右邊界的 port —— 這條連線**是什麼**,而不是它叫什麼 —— 而 `[5]` 正在顯示的那一個是綠色的。

`Alt+Esc` 是 sshu 自己的鍵,只為一個情況存在:`[5]` 把每一個按鍵都交給遠端,所以總得有東西能把它收回來。其他地方,單純的 `Esc` 就夠了。

## 特色

- **零學習成本** —— 每一個動作都在 `Space` menu 裡、依當下情境、每個 panel 都有。menu 和字母快捷鍵是同一張表產生的,所以「menu 裡沒有的快捷鍵」不可能存在。
- **menu 分兩區** —— `item`(對游標那一列做什麼,標題就是那一列)和 `panel`(對這一側做什麼)。只有一區的時候維持扁平。
- **多個並存的 ssh session** —— 每一個都是 embedded PTY 裡的真 `ssh`。`[5]` 拿到 focus 就佔滿整個 tab;結束的 session 立刻放掉它的終端機模擬器,而不是凍在一張死掉的畫面上。
- **兩側對等的 sftp** —— local ↔ remote ↔ remote 走同一個 `FS` 介面。marks 是分側的;一個 mark 是一條絕對路徑,所以改名它會跟著走,刪掉它會被拿掉。
- **遞迴子樹搜尋** —— `/` 走遍當前目錄底下整棵樹,**廣度優先**(SFTP 上每一層目錄都是一次 round trip,所以近的先到),串流、可取消、有上限,而且**就地畫出來**:一個結果就是一列普通的列,所以標記它、傳它,不需要學任何新東西。
- **抓下來之前先讀** —— `v` 顯示游標那一項:文字帶語法上色與行號(chroma + catppuccin-mocha,跟 filu 同一套),二進位是 xxd 風格的 hex dump,目錄是它的第一層列表。最多讀 64 KiB,因為在遠端那一側每一個 byte 都要過網路。檔案裡的跳脫序列會被剝掉:那些 bytes 是從別人的機器上來的,不處理的話會重畫你的終端機。
- **用你自己的編輯器編** —— `e` 用 `$VISUAL` / `$EDITOR` 打開游標那一項(`vi` 只是地板,不是依賴),跑在 embedded terminal 裡所以框還在。遠端的檔案抓下來、編、寫回去;本機的檔案就地編,所以它的 inode —— 以及指向它的每一個 hard link —— 都還在。內容沒有真的變就不會寫回去;寫入是原子的,斷線不會留下一份被截斷的設定檔;而在你開著它的時候被別人改過的檔案,絕不會不問一聲就蓋掉。
- **真的傳輸引擎** —— 整個 plan 在動手之前就算完,所以進度條的分母從第一格就是對的,而覆寫在一開始就一次問完。可逐條取消;取消或失敗的檔案會被移除,而不是留在那裡看起來像完成了。
- **目錄保持最新,而且很便宜** —— SFTP 沒有變更通知,所以 sshu 去 stat 目錄、比對 mtime,只有動了才重新列。每隔幾秒一次很小的 round trip,而不是整份重列,而且只在這個 tab 在畫面上的時候做。
- **沒有東西會無聲死掉** —— 不正常結束的 session 會升起一個 toast,說是哪一台、為什麼;乾淨的 `exit` 什麼都不說,因為那本來就是你要的。
- **frame 不變量** —— 每一條畫出來的線都剛好是終端機的寬度,任何尺寸、任何內容。從遠端來的寬字元、量起來不一樣的 Nerd Font glyph、CJK 檔名,全部靠「量」而不是「猜」;而且有一個測試橫跨尺寸、focus 狀態與資料在檢查它。
- **unix-first、靜態執行檔** —— macOS + Linux;`CGO_ENABLED=0`。

## 現況

**0.1.0** —— 第一個 release。三個 tab 都能端到端運作,236 個測試,`make check` 綠、`-race` 乾淨。見 [CHANGELOG.md](CHANGELOG.md)。

還沒有的:

- 沒有 release binary、Homebrew tap 或安裝腳本 —— 目前請從原始碼編
- **`[2]` 未知 host key 的互動確認** —— 今天是直接拒絕,要先用 `[3]` 接受
- **`[2]` 的加密私鑰** —— 會如實回報,但還不能用;agent 支援是可能的解法
- 遠端內容搜尋(那需要在對面跑指令,而這個 tab 刻意不做這件事)
- `[1]` 的 `[S]ftp` 捷徑,把游標那一台直接接到 `[2]` 當前 focus 的那一側
- 滑鼠、`hosts.yaml` 的 `fsnotify` reload、session 保存、keychain 撐腰的密碼儲存

## 用什麼做的

Go、[Bubble Tea](https://github.com/charmbracelet/bubbletea) 與 [Lip Gloss](https://github.com/charmbracelet/lipgloss),embedded terminal 用 [creack/pty](https://github.com/creack/pty) + [hinshun/vt10x](https://github.com/hinshun/vt10x),檔案傳輸用 [pkg/sftp](https://github.com/pkg/sftp) + `golang.org/x/crypto/ssh`,`v` 的語法上色用 [chroma](https://github.com/alecthomas/chroma)。配色是 catppuccin-mocha。
