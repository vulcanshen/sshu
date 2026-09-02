# sshu — Implementation

[**VTP** — Vulcan's TUI Design Principle](https://github.com/vulcanshen/thoughts/blob/main/vtp.md)
是一套**與領域無關的通用 TUI 設計原則** —— 目標:不看文件、不背 hotkey,靠一套
跨 surface 不變的基礎操作就能用完整個 app。VTP **不屬於任何單一 app**:kbu 是它
在 K8s domain 的一個實現、filu 是 filesystem domain 的另一個平行實現、**sshu 是
ssh domain 的第三個**。三者是 sibling、共用同一套 VTP,不是誰派生自誰。

本文件是 sshu 對 VTP 的**具體落地紀錄**,結構鏡射同為實現的
`filu-implementation.md` / `kbu-implementation.md`(平行參照、非上位),逐節對照
sshu 的實作 —— VTP 是 **interface**、本文件是 sshu 這個 **implementation
class**。想知道**為什麼**這樣做、看 VTP;想知道 sshu **怎麼**做,看這裡。

> **與 `sshu-ui-design.md` 的分工**
>
> | 文件 | 回答 |
> |---|---|
> | 本檔 | **現在是怎麼做的** —— 逐條對照 VTP、參數、不變量、按鍵全表 |
> | `sshu-ui-design.md` | **為什麼是這樣** —— mockup、判斷過程,以及**試過而被否決的做法** |
>
> 兩份都跟著程式碼走。要改一個看得見的行為,兩份都要改;被否決的做法留在設計稿
> 裡不刪 —— 那份紀錄本身就是重點。

> **狀態標記**:本文件描述**當前已落地**的實作。尚未完成者標 `(planned)`,不宣稱
> 未落地的行為。狀態總表見 §9。

---

## §A. Implementation in sshu

### §A.0 sshu score 對照

| 軸 / 結果 | sshu 值 | 計算 |
|---|---|---|
| **X. 揭露程度** | ~1.0 | Space menu 列出當前 focus panel 的 contextual 動作 100%、`?` help 列出全域動作 100%。**menu 與 hotkey 由同一張 action table 產生**,所以「有 hotkey 卻不在 menu」在結構上不可能發生 |
| **Y. core-key role 數量** | 5 | `Tab` / `Enter` / `Esc` / `Space` / `?`。`Ctrl+C`(硬退)、`q`(離開)、`Alt+Esc`(從 pty 收回鍵盤)不另計 role —— 見 §A.0.Y |
| `min(1, 5/Y)` 係數 | 1.0 | Y = 5、無 penalty |
| **Score** | `~1.0 × 1.0` = **~100%** | |

**完整性是結構保證,不是紀律。** `hostActions` / `sshActions` / `sftpActions`
三張表是 letter hotkey 與 Space menu 列的**唯一宣告**:新增一個動作就是往表裡加
一列,兩邊同時生效。而且 hotkey 與 menu 走同一個 `dispatchKey` —— 兩條路曾經分岔
過,結果是 tab [3] 的 menu 列跑了 tab [1] 同字母的動作(見設計稿)。

### §A.0.Y sshu core-key 集合(5 個)

| Core-key | sshu 語意 | 對應通用條款 |
|---|---|---|
| `Tab` | focus 切到**當前 tab** 的下個 panel(`1`-`9` 直達當前 tab 的 panel;tab 切換是 `Alt+p/f/s` 和絃;**ssh tab 的 `Tab` 是顯示開關**) | §4.1 |
| `Enter` | 連線 / 進入目錄 / popup 內確認 | §4.1 |
| `Esc` | 關最上層浮層 / 退出搜尋 / 回上層目錄(LIFO back) | §4.3 |
| `Space` | §A.1 contextual 入口(Space menu);**在浮層上按 = 關掉它** | §A.1 |
| `?` | §A.2 non-contextual 入口(help popup);**再按一次關掉** | §A.2 |

**入口鍵會關掉自己開的東西。** 一個只有單向的入口鍵是陷阱:使用者伸手按同一個鍵
想出來,結果沒反應。這條在 `AppModel.handleKey` **一個地方**解決,跟 `Esc` 同樣的
理由(§4.3)—— 才不會有某一個浮層是「忘記做」的那個。唯一例外是**正在被打字的
浮層**(host form、file picker、Rename 輸入框):那裡的空白就是空白。

`Alt+Esc` 不計 core-key:它只在網格拿著鍵盤時有意義(§4.6)。`Alt+p/f/s`
與 `Alt+方向鍵` 同理 —— 它們是「裸鍵活不下來的地方」的專用和絃,不佔 role。

### §A.1 Contextual track — Space menu

`Space` 在任何 panel 都開,內容是**那個 panel** 能做的事。

**列有三種狀態。** 一般 / 不列出(`appliesTo`)/ `disabled`(暗、游標條改用
`baseHex`+`borderDim`、按下去由 panel 說明原因)。`disabled` 不影響游標能不能
停上去 —— 停得上去才問得到原因。

**盒子量得出自己的內容。** 寬度取「動作列 + header 列 + 標題 + 底邊
legend」的最大值。header 曾經被跳過不量,整份都是說明的 menu(空 log、
nav)於是量到 0、字被切掉;legend 也一樣要量。沒有任何可執行列時 legend
只留 `Esc close`(design doc §11.13 追記)。

**標題是 focus 的 panel,不是 tab。** 分割的 tab 裡,「我在這裡能做什麼」取決於
站在哪個 panel —— 一個只寫 `[2] sftp` 的標題分不出 `[4]` 和 `[6]`,而這兩邊接的
可能是完全不同的兩台機器。標題字串與 panel 邊框上的膠囊**來自同一個函式**
(`sftpModel.panelTitle` / `sshModel.panelTitle`)。

**兩個 region**(tab [1]/[2]/[3] 皆然,鏡射 kbu 的 panel-2 menu):

```
 item operation
 Enter                       Enter . open directory
 [a]ppend to marks               again takes it off
 [r]ename                           this item, here
 [t]ransfer            this item, to the other side
 [x] Delete                 this item, on this host
 ───────────────────────────────────────────────────
 panel operation
 [/] Search                   everything under here
 [A]dd             a file, or name/ for a directory
 [R]efresh              re-read this directory now
 [T]ransfer all marks             to the other side
 [X] Delete all marks      erase them, on this host
 [C]lear marks          forget them, change nothing
 [S]elect host                local or a saved host
 [D]isconnect              close it, back to no host
 [P]rogress                   transfers, and cancel
```

- **兩個 region 的標題是 `item operation` / `panel operation`**,三個 tab 共用
  同兩個字串 —— 措辭不一樣的 menu 會讀成另一**種**選單
- **只有一個 region 就保持扁平**:標題壓在單一群組上是雜訊
- **沒有目標就沒有那一區**:空目錄沒有 item 動作、沒選 host 的那一側只剩
  `[S]elect host`,而且**字母跟著一起消失**(hotkey 與 menu 走同一個
  `sftpApplicable()`)
- **一列的 menu 不開**:該側沒有 host 時 Space 直接開 host 清單,實作是
  `app.go` 的 `" "` 分支轉呼 `sftpKey(keySelectHost)` —— 等同於按 `S`,連
  §11.15 的守衛都同一條(design doc §11.16)
- **正在被寫入的列**:`transferJob.cur`(atomic,寫在 `CopyItem` 前、defer
  清掉)→ `transferModel.arrivals()` → `sftp.view(arrivals)` 一路傳進
  `renderFileRow` 的 `landing` 參數,佔 mark 欄。`receiving()` 同時判祖先
  目錄(前綴必須帶 `/`)。`a` 在這種列上被 `sftpToggleMark` 拒絕。刷新綁在
  `logFinishedTransfers()` 回報的「剛結束」上,不綁畫格(design doc §11.17)

### §A.2 Non-contextual track — `?` help

全域動作一張表:core key、`q` / `Ctrl+C`、導覽詞彙。**可以疊在別的浮層上開** ——
§A.2 承諾 help 在任何 surface 都到得了,而一個迷路的使用者最可能站的地方,正是他
剛打開的那個 menu。

---

## §B. 元素專職化 in sshu

一個元素一個意思,跨 surface 不變。

| 元素 | 專職 | 不可挪用 |
|---|---|---|
| **blue `#89b4fa`** | panel border、tab 帶上亮著那段的底、hosts 表格選中列的 bar | 不拿去表達 user state |
| **subtext1 `#bac2de`** | 清單游標 bar(menu / picker / session) | 不當 panel chrome |
| **lavender `#b4befe`** | **正在編輯的東西**:form 的當前欄位、sftp 的 cwd、Rename 輸入框 | 不當 popup border |
| **green `#a6e3a1`** | 正在進行 / 成功:`[5]` 顯示中的 session、sftp marks | 不當裝飾 |
| **red / peach** | warning / error | **不拿去標 auth method** |
| **glyph** | 型別訊號(auth 方式、檔案 vs 目錄、mark) | 不拿去表達狀態 |
| **`[4]` 的前景 / 背景** | 前景 = 正在 `[5]` 顯示;背景 = 游標 | 兩條獨立通道,不互相代替 |
| **大小寫**(tab [2]) | 作用範圍:小寫 = 游標那一列、大寫 = 整個 panel | 其他 tab 不用它表達範圍 |

**auth method 用 glyph 不用顏色**(鑰匙 / 鎖),因為紅與桃色留給警告。
**app log 的顏色只染 level 那一段字**,不染時間、更不染整列 —— 「這是一則錯誤」是
那一則的屬性,而一份紀錄裡多數行都不是錯誤。

---

## §1. 空間結構 in sshu

### 1.1 固定 chrome 三行

頂部 1 行膠囊 tab bar(獨立 content row,**不是** panel border title)、其下 1 行
整寬分隔線、底部 1 行 footer。中間全部給 panel。`chromeRows = 3`,**選定即鎖死**。

傳輸進行時那條分隔線**兼職進度條**:ink 從左緣起依 blended percent 換成
liveColor(數字與右上 summary 同源 `transferModel.progress()`),任何 tab
都報,沒東西在動就整條恢復 borderDim;summary 本身同時改 liveColor(進行
中的動作不 dim),其他 tab 的靜止 status 照舊 dim。

那條分隔線是**把膠囊還給 panel title 的前提**:tab 帶與 panel 膠囊上下相鄰時,
兩排填色形狀會讀成同一條 chrome。

### 1.2 三個 tab 的版面

| tab | panel | 分割 |
|---|---|---|
| `[Alt+p]reference` | `[1] sshu` nav(SSH / Events 分類)+ `[2]` 內容(Hosts / Credentials / Logs;Operation 類遮罩中) | 左欄固定 **18 欄**,窄寬(<60)只畫 focus 側 |
| `[Alt+f]ile transfer` | 四個 `[1]`-`[4]` | 左右 **1:1**,每側上下 2:1(檔案 / marks) |
| `[Alt+s]sh` | `[1]` sessions + `[2]` layout strip + 終端網格 | 左欄固定 **26 欄**,strip(5 行、選項直排)在左欄**底部**;右側整片是網格,依 layout 等分(splitEven 攤餘數,欄寬總和恰等於總寬) |

**窄寬門檻是推導的,不是另一個會忘記同步的常數**:`sshNarrowW = sshLeftW + 28`
(pty 至少留 28 欄才值得留 split)、`sftpNarrowW = 72`(1:1 分割低於此只畫 focus
的那一側)。

**鍵盤在網格裡時整個左欄(清單 + strip)收起、網格佔滿 tab**:遠端拿著
鍵盤時它們都碰不到,把四分之一的寬度花在碰不到的東西上不划算。`panes()` 是**唯一**決定版面形狀的地方
—— 這條是踩出來的:`view()` 自己重複了一次窄寬判斷,等 focus 變成第二個收合理由
之後兩邊就不同步,結果是 `make([]string, 0, -2)` panic。

### 1.3 空狀態

**所有 panel 同一個形狀**(`ui/empty.go`):置中的**事實**(這裡沒有什麼)+ 置中的
**提示**(該按什麼,鍵用 `handColor`)。沒有事可做就只有事實 —— 空目錄是一個事實,
不是一個提示。文字不必統一,形狀必須一致。

**提示會折行**,逐字做:換行之後那個鍵還是鍵,所以樣式跟著**字**走。原本不折,而
`centerLine` 裝不下就截斷 —— 在 26 欄的 panel 上,那句告訴人該按什麼的話會被切掉。

**panel 太矮就依序讓步**:空行 → 提示行(從尾端)→ 事實留到最後。

`[1]` 空表格必須同時揭露 `[A]` 與 `Space`。一個新使用者面對空 panel 無路可走,
X 直接掉(§A.0)。

### 1.4 欄位收縮

hosts 表格依序捨 Auth → Port → User/Host,**Name 最後才讓**。sftp 檔案列的 size
是固定 8 欄的右側 slot,名字撞它就先折。`[4]` session 列的 port **永不截斷**。

---

## §2. 色彩 in sshu

catppuccin-mocha,沿用 u-family 錨點(`internal/ui/theme.go`)。

| 名稱 | 值 | 用途 |
|---|---|---|
| base | `#1e1e2e` | 亮底膠囊上的深字、cursor 上的前景 |
| crust | `#11111b` | inactive 膠囊凹陷底 |
| structural blue | `#89b4fa` | panel border、active 膠囊、選中列 bar |
| surface2 | `#585b70` | unfocus panel border、inactive 膠囊字 |
| subtext1 | `#bac2de` | 清單游標 |
| lavender | `#b4befe` | 編輯中的欄位、cwd |
| green | `#a6e3a1` | 進行中 / 成功 |
| red / peach | — | warning / error |

**明度作 z-axis**:浮層 border 隨疊層變亮(`popupLayerColor`),使用者看得出哪一
層在上面。lavender 刻意不在那條色階裡 —— 那個色帶屬於「使用者的足跡」。

---

## §3. 符號語彙 in sshu

### 3.1 Nerd Font 是設計、必裝

auth 方式、檔案型別、mark 都用 Nerd Font glyph,而且**版面會量它**。原始碼裡
**永不寫 PUA 字面值**,一律 `string(rune(0xf084))` 這種形式 —— 那些碼位在編輯器
與 diff 裡看不出是什麼。

### 3.2 glyph 不是每個都一格寬

folder 與 file 的 icon 可以差一格。所以列是**量自己的固定前綴**算出名字寬度,不是
假設「glyph 一格」。假設過,結果是**只有目錄那幾列**把邊框推歪 —— 這種偏差看起來
像框壞了,不像 bug。

### 3.2.1 codepoint 一律查字型,不憑記憶

補 Auth 的 radio glyph(`nf-md-radiobox_blank` `U+F043D` /
`nf-md-radiobox_marked` `U+F043E`)時,順手把已經在用的常數對過字型的 cmap,查到
兩個對不上自己註解的:`glyphMark` 寫著 `nf-md-check_bold` 但指到
`md-alpha_m_box`(一個框起來的字母 M),`glyphUpload` 寫著 `nf-md-transfer` 但指到
`md-upload`。兩個都已改成註解說的那個。

`nf-md-radiobox_blank` 在字型裡是以別名 `checkbox-blank-circle-outline` 登記的
(MDI 本來就把兩者當同一個圖)—— 下一個去查 cmap 的人會看到另一個名字,那不是錯。

### 3.3 遠端的寬字元撞不破邊框

vt10x 一個 rune 算一格,但終端機把 emoji 與 CJK 畫成兩格。`ptyTerm.render` 每一行
先 `clipANSI` 再補齊。代價是這種行被切掉最後一兩欄;不切的代價是整個框壞掉。

### 3.4 Surface 標籤

- **tab 帶**:每段 `[N] label`(型別訊號 + 內容訊號)
- **panel**:**有兩個以上 panel 的 tab**,每個 panel 都有 `[N] label`,同形圓角
  膠囊嵌在上邊框。**只有一個 panel 的 tab 不戴**(tab [1]):title 是用來把 panel
  彼此分開的,一張表底下再掛一顆寫著 `hosts` 的膠囊,是在回答沒有人會問的問題
- **panel title 不帶 glyph** —— 全 app 沒有第二個帶 icon 的 title,一個帶了就讀成
  特例而不是裝飾
- **popup**:glyph + text 嵌上邊框、hint 嵌下邊框

---

## §4. 互動 in sshu

### 4.1 Core 5 鍵

見 §A.0.Y。

### 4.2 清單導覽詞彙(一份,所有清單共用)

| 鍵 | 動作 |
|---|---|
| `j` / `k` | 上 / 下一列,**會繞**(清單是一個環) |
| `u` / `d`(或 `Ctrl+U` / `Ctrl+D`) | 上 / 下半頁,**不繞** |
| `gg` / `G` | 第一列 / 最後一列 |
| `h` / `l` | tab [2]:切左半 / 右半,保持同一列 · tab [3]:`l` 進 `[5]` |
| 方向鍵 | 與 `j`/`k`/`h`/`l` 同義 |

實作在 `internal/ui/nav.go`,**四個清單面全部走它**(hosts 表格、`[4]` sessions、
sftp 的檔案與 marks),所以往詞彙裡加一個鍵是一次加到所有清單上。

- **`j`/`k` 繞**:最後一列離第一列只有一個鍵。短清單上最有感 —— 不繞的話替代方案
  是按著 `k` 看畫面完全沒反應。**所有有游標的面都繞**,panel 與 popup 一視同仁。
- **`u`/`d` 不繞**:半頁是「瞄準」的移動,會無聲傳送到另一端的瞄準比停下來更糟。
- **沒有游標的東西也不繞**(`moveScroll`):`!` app log 與 `?` help 是 viewport,
  捲到底又跳回頂端會讀成故障 —— 根本沒有游標可以繞回去。
- **導覽字母不被任何動作佔用**,一條例外都沒有(`navKeys`,由
  `TestNoActionClaimsANavigationKey` 擋)。所以 `[D]elete host` / `[D]uplicate` /
  `[U]nmark` **只認大寫**。

### 4.3 letter hotkey ⊆ Space menu

見 §A.0。新增任何 contextual 動作,就是往 action table 加一列 —— 沒有「只綁 hotkey
忘了加 menu」這個可能。

### 4.4 hotkey 揭露 = bracket + 亮鍵暗述

**bracket 印的就是要按的那個鍵,一字不差,而且是唯一按得動的鍵。**
配對是完全比對,大小寫算數(`hotkeyIndex`)。

- `[A]dd` = shift+A;裸的 `a` 不是綁定
- `[t]ransfer` 與 `[T]ransfer all marks` 是**兩個動作**,分得出來因為 bracket 印的
  就是那個大小寫

**亮鍵暗述**:凡「鍵 + 說明」成對出現(footer legend、popup 下邊框 hint),鍵用
`handColor`、說明用 `dimColor`。學一次、走全 app。

### 4.4.1 `Tab` 只在當前 tab 裡輪詢

到底繞回第一個,**不跨 tab**。換 tab 是 `1`/`2`/`3`。tab [1] 只有一個 panel、
tab [3] 也只剩一個(`[4]`),所以 `Tab` 在那裡唯一的作用是**從 pty 出來**;tab [2]
是 `[4]` → `[5]` → `[6]` → `[7]` → 繞回。

**`Tab` 永不進 tab [3] 的 `[5]`** —— 進去就被遠端吞掉,等於把帶你進去的鑰匙鎖在
門內。進 pty 有三條路:`[4]` 上按 `Enter`、按 `5`、按 `l`;出來一律 `Alt+Esc`。

### 4.5 文字輸入 surface 的例外

form / file picker / Rename 輸入框裡,**空白就是空白、問號就是問號**。這是 §A.0.Y
入口鍵規則的唯一例外,而且判斷收在一個 `m.textFloat()` 裡。

### 4.6 `Alt+Esc` / 按住 Alt+方向鍵 —— 網格的專用和絃

網格的格子把整個鍵盤交給遠端(`Esc`、`Tab`、`q` 都是遠端的)。`Alt+Esc`
收回鍵盤、focus 回 `[1]`(side 欄同時回來);**按住 Alt、方向鍵往鄰格走**
—— 空間移動,不用記編號、重排無感,邊緣 clamp 不繞(空間移動絕不瞬移),
ragged 網格的短列之外沒有格子。pty 內也有效:`Alt+方向鍵` 在裸終端裡本來
就是無效輸入,拿走沒有代價 ——(`Alt+1..9` 試過又拆掉:本地的 window
manager 先把它吃了,和絃根本到不了 sshu。)其他地方 `Alt+Esc` 就是普通的
`Esc`,不是死鍵。

### 4.7 數字只定址「當前 tab 裡看得見的 panel」

每個 tab 的 panel 從 `1` 編號:preference `1`-`2`、file transfer `1`-`4`、
ssh `1`-`2`(格子走 Alt+方向鍵)。畫面上沒有那個編號,按下去就沒反應 ——
規則直接從畫面讀得出來,不必記。pty 內裸數字屬於遠端。

### 4.8 tab 切換 —— `Alt+p/f/s` 和絃

tab 帶開頭是**固定亮的 `[Alt]` 鏈頭**,標籤 `[p]reference` 亮起的字母段
補上和絃的另一半 —— 亮著的兩段合起來就是按的:小寫是日常拼法,pty 外大小寫皆通
(死鍵離活鍵一個 shift 是沒有回報的陷阱);**pty 內**只認 shift 加大寫 ——
`M-f` 是 readline 的 forward-word,不是 sshu 的。popup 開著時和絃不作用:tab 在
form 底下換掉,form 就懸在一個它不認識的 surface 上。

---

## §5. Mouse in sshu

`(planned)` —— 沿用通用 §5 mapping(左鍵 focus + select、雙擊 = `Enter`、滾輪捲
清單)。目前完全鍵盤驅動。

---

## §6. 浮層 in sshu — Popup Convention

### 6.1 taxonomy — sshu 有 **6 類**

| 類型 | sshu 實例 | 特徵 |
|---|---|---|
| **menu** | Space menu、host picker、identity file picker、credential picker | 分 region / 清單、cursor-first、選一個執行 |
| **message** | Connect / Delete / Quit 確認、Toast | 短、確認 / auto-dismiss |
| **viewport** | `?` help、`[v]iew`(app log 已從浮層變成 preference → logs 的**內容 panel**,同為 viewport 語彙) | 可捲、沒有游標 |
| **form** | Add / Edit host、Add / Edit credential(共用 editField / formBody 欄位引擎) | 多欄位、逐欄位 focus、一次提交 |
| **input** | Rename、**Add** | **一行**文字、一個問題、Enter 送出;Add 的 Enter 動詞跟著輸入變 |
| **pty** | **tab [3] 的 panel `[5]`**、tab [2] 的 **`[e]dit`** | 外部程式在 sshu 內 render,鍵盤整個交出去 |

**`input` 不是單欄位的 form**:form 是「填 N 個欄位、一次提交」,input 是「回答一個
問題」—— 跟 confirm 同家族,差別只在答案是文字不是 yes/no。做成單欄位 form 會讓
`Tab`(切欄位)在只有一欄的地方變成死鍵。

**pty 在 sshu 不是浮層、是 panel `[5]` 本身** —— 這是與 filu 的分歧點:filu 的 pty
是「開 `$EDITOR`,關掉就結束」的短時浮層;sshu 的 session 是長時的、而且同時可以有
很多個,所以它是常駐 panel 的內容。

**sftp 的傳輸進度也不是 pty**:sshu 自己說 SFTP 協定,進度是自己畫的。

### 6.2 開關動畫 / 疊層 / 取消

- 動畫 `animFrames = 8` × `animStep = 16ms` ≈ 128ms,落在動作看得出來又不拖慢的
  區間
- **正在關閉的浮層不再握著鍵盤**(`popupAnimator.owns()`):動作一旦 commit,鍵盤
  就回到 panel 手上。曾經不是這樣,結果是 menu commit 之後那 128ms 內的下一個鍵被
  一個正在退場的浮層吃掉
- **`Esc` 只在一個地方解析**(`closeTop`):沒有任何 popup 自己重新實作取消
- **取消 target 會留下 source**(§6.4):從 Space menu 開的 form,`Esc` 回到 menu

### 6.3 破壞性動作一律先問

Delete host、Close session、Quit(有 live session 或進行中傳輸時)、sftp 的 `x` /
`X`、覆寫。問句一律說**數量與哪一台**——兩邊長得很像,只說「2 個檔案」不夠。

---

## §7. 時間軸 UX in sshu

### 7.1 context shift 之後清除 source

Connect 確認 `Enter` 之後:confirm 收掉 → Space menu 收掉 → 切到 tab [3] → 開
session。ssh session 是**長時 target**,使用者出來時注意力早已轉移。同一判準適用
sftp 傳輸。

### 7.2 session 完全不落地

`[3]` 的 session 與 app log **只存在記憶體**,沒有 `history.yaml`。最後一個畫面
可能有遠端印出來的任何東西 —— 那不是可以隨手寫進磁碟的資料。

### 7.3 資訊在需要的時候出現,不常駐

同一個形狀用在兩個地方:

| | 常駐 | 隨手看 | 事件當下 |
|---|---|---|---|
| 傳輸 | tab 列 `󰕒 3/12 · 42%` | `[P]rogress` popup | — |
| session 結束 | tab 列 `3 live sessions` | **`!` app log**(footer 報未讀數) | **error toast** |

`[6]` 曾經是常駐的 history panel,佔掉左欄三分之一、不能操作、大部分時間是空的。
真正有價值的是「哪一條斷了、為什麼」,而那件事以前是**完全靜默**的。詳見設計稿
§7.1.4。

**乾淨離開不出聲**:`exited 0` 是你打 `exit` 要的結果。

### 7.4 目錄怎麼保持最新 —— SFTP 沒有 watch

協定裡沒有變更通知。每一拍(**2 秒**)只 stat 目錄、比對 mtime,**只有動了才重列**。
兩半都是網路呼叫,所以跑在 goroutine 上用訊息送回來 —— 背景刷新永遠不該讓畫面等。
**沒人在看的 tab 不會問**。

游標跟著**同一個檔案**走,不是同一列:上方多出一個檔案就把游標滑到別的名字上,而
使用者沒碰過任何鍵 —— 下一個 `t` 就傳錯東西了。

### 7.5 `/` 搜尋:廣度優先、串流、可取消

SFTP 每個目錄是一次 round trip,所以結果抵達的順序就是使用者等待的順序。深度優先
會把最初幾秒花在剛好排最前面的那棵子樹裡,從外面看跟卡住沒兩樣。

- **畫在原地不開 popup**:結果是普通一列,`a`/`t`/`x`/`Enter` 直接可用
- **空 query = 當前目錄**,不是「底下全部」
- **到達順序就是順序,不重排** —— 游標全程活著,每來一批就重排等於把使用者手底下
  那一列抽掉
- 上限 **20000**,停在上限會說 `capped`

### 7.6 傳輸:先算完整個 plan 再問

`remote.Plan` 遞迴展開要建立的每一項(目錄也是一項),回總位元組數。覆寫因此問在
開始之前 —— 複製到一半才發現要覆寫,那時候問已經不算問了。進度條的分母從第一格就
是對的。**取消或失敗會刪掉半個檔案**:一個看起來像真貨的半截檔是這裡最糟的結果。

### 7.7 離開時三樣一起放掉 —— 而且沒有一扇門漏掉子行程

ssh session、進行中的傳輸、sftp 連線。app 內的出口(`q`、quit 確認、
`Ctrl+C`)走同一個 `AppModel.quit()`。app **外**的出口 —— 外部 SIGINT
(bubbletea 以 ErrInterrupted 收場、不經 model)、外部 SIGTERM(裸
QuitMsg)、SIGHUP(關終端視窗)—— 走 `ui.KillChildren()`:startPty 是所有
子行程的出生地,registry 記在那裡,`KillChildren` 是 Run 返回後的下一行、
也是 SIGHUP handler 的全部。子行程各自領著自己的 PTY session,訊號自己到
不了它們 —— 這就是 registry 必須存在的原因。

---

## §8. Panel chrome in sshu

### 8.1 powerline tab bar

**一條連起來的帶子**(filu 的 chain 語彙):圓帽開頭、**三角**分段、圓帽收尾,恰好
一段亮著。三角朝右,`pl-left_hard_divider`(U+E0B0)實心、承載顏色改變,
`pl-left_soft_divider`(U+E0B1)是它的外框、在兩側同色時只畫線。

- **active**:blue 底 + base 深字 + bold
- **inactive**:**畫布色(base)**底 + surface2 字 —— crust 與 surface0 都試過當
  「凹槽」並且都被否決,理由與另外兩種被否決的形狀見設計稿 §1.1
- 右端是**狀態 slot**:preference 依區(`1/5 hosts` / `2 credentials` /
  `13 entries`)、file transfer `2 marks` 或傳輸進度、ssh
  `2 live sessions · 2 on grid`

### 8.2 panel border title

每個 panel 一顆同形圓角膠囊,`[N] label`。tab 列與 panel 列之間隔一條整寬分隔線,
兩排膠囊才不會讀成同一條 chrome。

### 8.3 cwd 在 panel 內第一行

不是下邊框:那是**內容**(「我在哪」),看的頻率跟清單本身一樣高,塞進邊框會讀成
chrome。純文字、lavender 段落 + dim 斜線,**沒有 chip 底色** —— 上面就是 panel
膠囊,再來一排填色形狀會打架。

搜尋中同一列改放 query,提示符是**搜尋 glyph 不是 `/`**:把開搜尋的鍵原樣回顯,會
讓含斜線的 query 讀不出來(`/tmp` 畫成 `//tmp`)。右端是 `<符合> of <已看到>`,
走的時候加 `…`;位置不夠**整段丟掉不切一半** —— `12 of 840` 切成 `12 of 8` 不是
縮短,是另一個數字。

### 8.4 frame 不變量

**每一行的顯示寬度必須剛好等於終端寬度**,在任何尺寸、任何 focus、有無資料。這條
由測試釘住(`TestSFTPTabPreservesFrame` / `TestSearchPreservesFrame` /
`TestPopupPreservesFrame` …),而且它抓到的 bug 比任何其他手段都多:寬字元、PUA
glyph 寬度差、被重複扣掉的間隔格、ANSI 被切斷。

**規則:量並補齊「純文字」,再上色。** `lipgloss.Width` 會跳過 ANSI,但對已上色的
字串補空白會讓那些空白落進樣式範圍內、吃到背景色。已上色的字串要裁切一律走
`clipANSI`。

---

## §9. 實作狀態

### 已落地

| 項 | 落在哪 |
|---|---|
| 膠囊 tab bar + 分隔線 + footer,`chromeRows` 鎖死 3 | `ui/chrome.go` `ui/view.go` |
| `[1]` hosts 表格、responsive 收縮、form + 驗證、identity file picker | `ui/hosts.go` `ui/table.go` `ui/form.go` `ui/filepicker.go` |
| `[1]` `/` 跨欄 fuzzy 搜尋(不含 auth)、依分數排序 | `ui/hosts.go refilter` |
| `[2]` 四 panel、`remote.FS` 一介面兩實作、marks;本機側開在啟動目錄 | `ui/sftptab.go` `remote/fs.go` `remote/edit.go StartDir` |
| `[2]` `/` 遞迴搜尋:串流、廣度優先、可取消、上限;`Enter` 前往結果 | `remote/search.go` `ui/sftpsearch.go` `ui/sftptab.go enter` |
| `[2]` 傳輸:先 plan、進度、逐條 cancel、半檔清除 | `remote/copy.go` `ui/transfer.go` |
| `[2]` rename / delete / **add**(結尾 `/` 建目錄,否則建空檔;遞迴刪除不跟隨 symlink) | `ui/sftpkeys.go` `remote/fs.go RemoveAll` |
| `[2]` `[v]iew`:文字(chroma 上色 + 行號)/ hex / 目錄一層,64 KiB 上限,ESC 一律吃掉 | `ui/viewer.go` `ui/highlight.go` `remote/peek.go` |
| `[2]` `[e]dit`:`$VISUAL`/`$EDITOR`/`vi`,遠端抓下來→編→原子寫回,沒改不寫、被改過先問 | `ui/edit.go` `ui/editorcmd.go` `remote/edit.go` |
| `[2]` mtime 目錄刷新 | `ui/sftpwatch.go` |
| ssh **終端網格**:Tab 開關格子、Enter 進入、Alt+方向鍵走格、layout strip(horizontal / vertical / custom R×C)、每格獨立 SIGWINCH | `ui/sshtab.go` `ui/pty_unix.go` |
| ssh 連線中 spinner(判準是 PTY 有沒有說過話);失敗時網格顯示遠端原話、app log 收**整個最終畫面**(每則 40 行 / 4000 字) | `ui/sshtab.go` `ui/applog.go` `ui/pty_unix.go` |
| preference:nav(分類 header;鍵盤在 `[2]` 時整片 dim)+ Hosts / Credentials / Logs(Export / Import 已實作、遮罩中);logs 上畫面即已讀,nav 與 footer 掛未讀數(未讀數不 dim) | `ui/preftab.go` `ui/applog.go` `ui/bundlepage.go` |
| credentials CRUD + host form 三選 auth + credential picker;連線各入口統一 `store.Resolve` | `ui/credlist.go` `ui/credform.go` `ui/credkeys.go` `store/hosts.go Resolve` |
| 「選值欄位」互動:空欄 Enter 開選單、有值 Enter 下一欄、Backspace 整行清除 | `ui/form.go` `ui/credform.go` |
| app log 落地 applogs.yaml(append-only、自我修剪、0600);tail 開機讀回;`[C]lear logs` 先清檔(留警告標頭)再清記憶體 | `store/applog.go` `ui/applog.go` `ui/preftab.go` |
| 子行程 registry:任何退出路徑(含 SIGINT/SIGTERM/SIGHUP)不留孤兒 ssh | `ui/procreg.go` `cmd/sshu/main.go` |
| 浮層六類、動畫、疊層色、單一 `Esc`、`Space` 關閉 | `ui/popup.go` `ui/app.go` |
| 導覽詞彙(繞 / 半頁 / 保留字母) | `ui/nav.go` |
| `hosts.yaml`:XDG 解析、atomic 0600 寫入、警告標頭;`auth: credential` + `Resolve` | `store/store.go` `store/hosts.go` |
| `credentials.yaml`:與 hosts 同一組緩解;name 唯一、credential 不能再指 credential | `store/credentials.go` |
| `config.yaml`:唯讀設定,`connect_timeout` 兩個 tab 共用;缺檔用預設、壞檔進 app log | `store/config.go` |
| `SSH_ASKPASS` 供密碼(不進子行程環境) | `cmd/sshu/main.go` `ui/session.go` |

### `(planned)`

| 項 | 現況 |
|---|---|
| **`[2]` 未知 host key 的互動確認** | `remote.Dial` 收到 `nil` prompt、一律拒絕;要先用 `[3]` 連一次寫進 `known_hosts`。缺一個能在 dial 途中升起的對話框 |
| **加密私鑰** | `remote.authMethods` 如實回報做不到;agent 支援是可能的解法 |
| 遠端內容搜尋 | 需要在對面跑 grep,是「執行指令」不是「列目錄」,超出這個 tab 的授權範圍 |
| Mouse | §5 mapping |
| `[1]` 的 `[S]ftp` 捷徑 | 從表格直接把游標那台接到 `[2]` 當前 focus 的那一側 |
| fsnotify reload `hosts.yaml` / `state.yaml` / keychain `secretStore` | — |
| release binary / Homebrew tap / install script | — |

---

## 附錄 — sshu hotkey 全表(v0.2)

**bracket 印的那個大小寫就是唯一按得動的鍵**(§4.4)。

### Tab / panel / core key

| 鍵 | 語意 |
|---|---|
| `Alt+P` / `Alt+F` / `Alt+S`(pty 外小寫也通) | 切 tab —— pty 內也有效 |
| `1`-`9` | 當前 tab 的 panel 直達(pty 內屬於遠端) |
| `Tab` | 下個 panel;**ssh tab:顯示開關** |
| `Enter` · `Esc` · `Space` · `?` | 確認 / 取消 / menu / help |

### [Alt+p]reference

| Surface | 鍵 | 動作 |
|---|---|---|
| `[1]` nav | `j`/`k` · `Enter` | 選條目(分類 header 直接跳過;內容即換)/ 鍵盤給內容 |
| `[2]` Hosts | `Enter` · `A` · `E` · `D` · `/` | Connect(先問;credential 在此解析)/ Add / Edit / Delete / Search |
| `[2]` Credentials | `Enter` · `A` · `D` | Edit / Add / Delete(先問,列出引用數) |
| `[2]` Logs | 導覽鍵 · `C` | 捲動;上畫面即已讀 / Clear logs(先問,連 applogs.yaml;空 log 時沒有這個鍵) |
| ~~`[2]` Export / Import~~ | (遮罩中) | Operation 頁已實作但未上架 —— 設計未定案(design doc §11.12 追記) |

### form(host / credential 共通)

| 鍵 | 動作 |
|---|---|
| `Tab`/`Shift+Tab`/`↑``↓` | 換欄(所有欄位一致) |
| `←` `→`(Auth) | password / privatekey / credential(host form) |
| `Enter`(IdentityFile / Credential 欄) | **空欄開選單、有值送出**(與其他欄一致) |
| `Backspace`(同上) | **整行清除** |
| `Enter`(其他欄)· `Esc` | 送出 / 取消 |

### [Alt+f]ile transfer(小寫=游標列,大寫=整個 panel)

| Surface | 鍵 | 動作 |
|---|---|---|
| 全部 | `1`-`4` · `h`/`l` | panel 直達 / 切左右半 |
| 全部 | `t`/`T` · `x`/`X` · `r` · `v` · `e` · `S` · `c`/`C` · `P` | 傳(項/marks)/ 刪(項/marks)/ Rename / View / Edit / Select host / Clear(marks 單項/全部)/ Progress |
| 檔案 panel | `R` | **Refresh** —— `sftpRefresh` 直接叫 `reload()`,不看 mtime、不等 poll;成功報數量,失敗報 `s.err`(design doc §11.18) |
| 檔案 panel | `D` | **Disconnect** —— 重建 side 的零值(保留並 +1 `dialGen`) |
| 全部 | `S` · `D` 傳輸中 | `sftpAction.needsIdle` + `transferBusy()`:menu 列 `disabled`(暗)、`sftpKey` 擋下並 toast,**不關 menu** |
| 檔案 panel | `Enter` · `Esc` · `a` · `/` · `A` | 進目錄或去到搜尋結果 / 退搜尋→上層 / append 到 marks(再按取消)/ 搜尋子樹 / Add |

### [Alt+s]sh

| Surface | 鍵 | 動作 |
|---|---|---|
| `[1]` sessions | `Tab` · `Enter` · `C` · `D` | **顯示開關** / 顯示並進入(side 收起)/ Close(先問)/ Duplicate(先問);j/k 掃過時,游標 session 的格子外框在網格上同步亮 |
| `[2]` layout | `j`/`k`(`h`/`l` 也通)· `Enter` | 換排列(即生效)/ custom 問**列 × 行**(R×C) |
| 格子(pty) | 所有裸鍵 | 送給遠端 |
| 格子(pty) | 按住 `Alt`+`←→↑↓` | 往鄰格移動(邊緣 clamp) |
| 格子(pty) | `Alt+Esc` | 收回鍵盤、回 `[1]` |

### 全域

| 鍵 | 動作 |
|---|---|
| `q` | 離開(有 live session 或進行中傳輸時先問) |
| `Ctrl+C` | 強制離開(session / 傳輸 / sftp 連線 / 子行程一併帶走) |

---

## 結語

sshu 的定位不是「再做一個 ssh 管理器」,而是**第一次開就能不看文件開到底**。

三個 tab 之間沒有共用的資料流 —— hosts 是清單、ssh 是長時 session、sftp 是兩個檔案
系統 —— 但它們共用同一套 core key、同一份導覽詞彙、同一種浮層規則、同一條
「bracket 印的就是要按的」約定。學會其中一個 tab,另外兩個就已經會了大半。

那才是 VTP 想換到的東西。
