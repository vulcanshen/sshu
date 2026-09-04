# sshu — UI 設計稿

sshu 是 u-family 的第三個成員(kbu = K8s domain、filu = filesystem domain、
**sshu = ssh/sftp domain**)。三者**平行**、共用同一套
[**VTP** — Vulcan's TUI Design Principle](../../thoughts/vtp.md),不是誰
派生自誰。

本檔是 sshu 的**設計紀錄**:每一個看得見的行為**為什麼**是這樣,以及**試過而被
否決的做法**。它跟著程式碼走 —— 改一個使用者看得見的東西,就在同一輪改這裡。

> **設計權威順序**:**VTP**。`filu-implementation.md` / `kbu-implementation.md`
> 是**平行實現的參照、不是上位權威**。
>
> **範圍**:三個 tab 都已完整設計並落地(§0)。開發順序是
> **1. hosts → 3. ssh → 2. sftp**;章節本身照 VTP 的條目排,不照那個順序。

---

> **與 `sshu-implementation.md` 的分工**
>
> | 文件 | 回答 |
> |---|---|
> | `sshu-implementation.md` | **現在是怎麼做的** —— 逐條對照 VTP、參數、不變量、按鍵全表(結構鏡射 filu / kbu 的同名文件) |
> | 本檔 | **為什麼是這樣** —— mockup、判斷過程,以及**試過而被否決的做法** |
>
> 兩份都跟著程式碼走。要改一個看得見的行為,兩份都要改;被否決的做法留在這裡不刪
> —— 那份紀錄本身就是重點。

## 0. 三個 tab 的定位

| tab | 職責 | 狀態 |
|---|---|---|
| **[M]anage** | sshu 自己的資料,nav 分類:SSH(Hosts、Credentials)、Events(Logs)—— 左側 nav `[1] sshu` + 右側內容 `[2]`(Operation 類已實作、遮罩中,見 §11.12) | 已落地 |
| **[F]ile transfer** | 兩個檔案系統之間的傳輸;內含 `[1]`-`[4]` 四個 panel | 已落地 |
| **[S]SH** | 多個互動式 session 的**終端網格**;`[1]` sessions、`[2]` layout,格子間按住 Alt 用方向鍵走 | 已落地 |

三個 tab 是**三個並存的 surface**(不是同一對象的三個視角),以**一個 shift
過的裸字母**切換:標籤印的就是按的。裸數字整組讓給「當前 tab 的 panel」。

tab 鍵走過三代:v0.2 之前掛在 `1`/`2`/`3`(換掉是為了把數字讓給 panel),
v0.2 到 v1.1.0 是 `Alt+p/f/s` 和絃(為了在 pty 內也能切),v1.2.0 起是裸的
`M`/`F`/`S` —— pty 內改為「先 `Alt+Esc` 出來」,理由見 §11.21。

---

## §A. VTP 對照

### §A.0 score

| 軸 | sshu 值 | 說明 |
|---|---|---|
| **X. 揭露程度** | ~1.0 | `Space` 列出當前 focus 的全部 contextual 動作;`?` 列出全部全域動作。兩個入口自身由 **footer 常駐揭露** |
| **Y. core-key role** | **5** | `Tab` / `Enter` / `Esc` / `Space` / `?` |
| `min(1, 5/Y)` | 1.0 | Y = 5,無 penalty |
| **Score** | **~100%** | 第一次開就能用完 |

### §A.0.Y core-key 集合(5 個,跨 surface 不變)

> **v0.2**:tab 切換移到 `Alt+p/f/s` 和絃,`1`-`9` 全數改為「當前 tab 的
> panel 直達」;ssh tab 的 `Tab` 改為顯示開關(§11.一、§11.六)。下表的
> 「`1`/`2`/`3` 直達 alias」是 v0.1 的形狀,保留當時推理。

| Core-key | sshu 語意 | 通用條款 |
|---|---|---|
| `Tab`(`1`/`2`/`3` 直達 alias) | 切 tab(surface 切換);popup 內切欄位 | §4.1 |
| `Enter` | 確認 / 進入(hosts:對 cursor host 發起 ssh 連線) | §4.1 |
| `Esc` | 取消 / 關閉最上層浮層 | §4.3 |
| `Space` | §A.1 contextual 入口(Space menu) | §A.1 |
| `?` | §A.2 non-contextual 入口(help popup) | §A.2 |

**`q` 與 `Ctrl+C` 不各記一個 role**(對齊 filu):`q` = 離開 app,是一個
**全域動作**(列在 footer + `?` help),不是「取消」role;取消 role 由 `Esc`
單獨承載。`Ctrl+C` 是逃生硬退 —— **除了鍵盤在遠端或編輯器手上的時候,那裡它
是它們的中斷鍵**(§11.20)。**`q` 在任何浮層開著時不生效**(浮層擁有鍵盤),
避免半套 alias 汙染取消 role。

**letter hotkey 不佔 core-key slot**:`e` / `d` / `c` / `s`、導覽 `h j k l`
`gg` `G` 都是入口內動作的加速捷徑。

### §A.1 Contextual track — Space menu

入口自身在 **footer** 揭露。hosts tab 只有一個 panel,所以 focus 恆定在
hosts 表格上。

| Region | 動作 | 觸發 |
|---|---|---|
| **item**(cursor 上那張卡) | Connect(ssh) | `Enter` |
| | Sftp `(planned)` | `s` |
| | Edit | `e` |
| | Delete | `d` |
| **panel** | Add | `A` |

§6.6 cursor-first:item region 在前、panel region 在後。

### §A.2 Non-contextual track — `?` help popup

| 全域動作 | key |
|---|---|
| 說明 | `?` |
| 離開 | `q` |
| 硬退 | `Ctrl+C` |
| 切 tab | `M` / `F` / `S`(§11.21) |

sshu 目前沒有 kbu 那種全域 toggle,§A.2 軌很薄(同 filu)。

**help popup 裡還有一組不全域的鍵**:網格格子的 `Alt+arrows` / `Alt+Enter` /
`Alt+Esc` / `PgUp`·`PgDn`(§4.6、§11.19、§11.25)。它們只在 panel `[5]` 有效,
照理不該進這張表 —— 但那個 panel 把 `?` 也送給遠端,**在裡面按不出 help**。
唯一學得到它們的地方,就是從外面打開的這一份。§A.2 的承諾是「沒讀過 README
的人在這裡找得到整套詞彙」,而一組進去就問不到的鍵,正是最需要先講的。

### contextual / non-contextual 邊界(audit 用)

| 動作 | contextual? | track |
|---|---|---|
| 對 cursor host 做 Connect / Sftp / Edit / Delete | ✓ 對 cursor 卡 | §A.1 |
| 對 hosts panel 做 Add | ✓ 對當前 panel | §A.1 |
| 切 tab / Help / Quit | ✗ 全域 | §A.2 |

---

## §B. 元素專職化

| 元素 | 專職語意 | 不准兼職 |
|---|---|---|
| **blue `#89b4fa`**(structural) | panel border、active tab 膠囊底色、**hosts 表格選中列的 bar**(`rowSelColor`) | 不拿去做 user state |
| **surface2 `#585b70`** | unfocus 的 panel border、inactive 膠囊前景 | — |
| **subtext1 `#bac2de`**(`handColor`) | Space menu / file picker / session 清單的游標 bar | 不當 panel chrome、不當 form 編輯列、不當 hosts 表格游標(那是 blue) |
| **lavender `#b4befe`**(`editColor`) | **正在編輯的 form 列**(label + value + caret 一起) | list cursor(那是 `handColor`)、popup border、panel border |
| **crust `#11111b`** | (不再使用 —— tab bar 沒亮的段用 base,見 §1.1) | — |
| **overlay0 `#6c7086`**(`dimColor`) | 次要文字、region header、停用欄位 | — |
| **Peach / Red** | warning / error override | 不參與 popup layer scale;**不拿去標 auth method** |
| popup border 色 | popup layer 明度(`popupLayerColor`) | 不 hardcode |
| `[X]label` bracket | letter hotkey 揭露 | 純 label 不加 bracket |
| `Esc` | 關閉 / 取消 | 永遠不當「確認」 |
| Nerd Font glyph | **欄位型別訊號**(這一列是什麼欄位) | 不當熱鍵 signal、不當純裝飾 |

**點名一個決定:auth method 不用顏色編碼。** 直覺會想把 `password` 染成
peach、`privatekey` 染成綠,但 peach/red 已被 warning/error override 專職
佔用(§2.4),借用會讓使用者看到 password 卡以為「這台有問題」。改由
**glyph 區分**(鎖 vs 鑰匙)+ 文字,兩者都是內容訊號、不動色彩層。

---

## §1. 空間結構

### 1.1 版面 grid

固定 chrome **3 行**:頂部 1 行膠囊 tab bar(獨立 content row,**不是**
panel border title)、其下 1 行整寬分隔線(傳輸進行時兼職進度條,見
11.9)、底部 1 行 footer。中間全部給 panel。

**tab bar 是一條連起來的 powerline 帶**,不是三顆各自獨立的膠囊:一個圓帽開頭、
中間用**三角**分段、一個圓帽收尾。三段 `[M]anage / [F]ile transfer / [S]SH`
**恰好一段亮** —— 就是你所在的那個。(v1.1.0 之前前面還有一個固定亮的 `[Alt]`
鏈頭,兩段亮著合起來拼出和絃;裸鍵沒有和絃可拼,見 §11.21。)三角一律朝右
(讀的方向):
`pl-left_hard_divider`(U+E0B0)是實心的,承載**顏色改變**;
`pl-left_soft_divider`(U+E0B1)是同一個形狀的外框,只在兩側同色時畫一條線。

**沒亮的段就用畫布色(base)填底,不另外給底色。** 先試 `crust`、再試
`surface0`,兩個都是想在沒亮的段後面墊一條「凹槽」,而兩個都更差 —— surface0 淺到
不像凹陷、crust 是一條混濁的帶子,加了重量沒加資訊。三角把結構的工作做完之後,沒亮
的段**不需要自己的底色**:讓這條帶子讀得懂的是「亮的那一段有形狀」,不是「其他段
有背景」。

原本是三顆獨立膠囊,而且沒亮的那兩顆用 `borderDim` 前景配 **`crust`** 底 —— crust
跟畫布只差一階,所以**沒亮的 tab 根本沒有形狀**;而同一個 app 裡沒 focus 的
**panel chip**(深字配 `borderDim` 底)卻清楚得很。同一個「未選中」,兩個地方畫法
相反,而看不見的那個是 tab。

> **試過而被否決(一)**:把沒亮的 tab 改成跟 panel chip 一樣的灰色實心。可見度
> 解決了,而且全 app 只有一種膠囊形狀 —— 但三顆實心藥丸並排就是「一排按鈕」,正是
> 下面那條分隔線當初要處理的問題。
>
> **試過而被否決(二)**:沒亮的用**細圓帽**(U+E0B7 / E0B5)畫成外框、不填色。
> 形狀有了、而且選中與否不再只靠顏色 —— 但實機上兩道細弧夾著暗字**讀起來是括號**,
> `([2] sftp)`,不是膠囊。終端機畫不出膠囊的上下邊,這是這條路的天花板。
>
> 兩次都在修「沒亮的那一段看不見」,而**連起來**同時解掉了兩個顧慮:它是**一個
> 物件**,所以沒亮的段明顯是「某個東西的一部分」而不是漂浮的字;而它**不可能**被
> 讀成一排按鈕,因為按鈕之間有縫,這條沒有。

> **改過一次**:這裡原本寫著 sshu 刻意**不**用 filu 的 chain,理由是「chain 說的是
> 『這些是同一個東西的分頁』,而 sshu 的三個 tab 是三個並存的 surface」。實機看過
> 之後判定那個理由站不住:**一個 app 的三個並存 surface**,正好就是這條帶子畫出來
> 的東西;而真正需要被分開的是「上面是 chrome、下面是 surface」,那是分隔線的工作。

分隔線是後補的,而且它是**把膠囊還給 panel title 的前提**:tab 膠囊與 panel
膠囊上下相鄰時,兩排填色形狀會讀成同一條 chrome。中間夾一條線,上面那排才明確
是「可以按的 tab」、下面那排是「這個框叫什麼」。

> **未決**:filu 在同一個位置試過一條 dim 實線、判定「太重」而改用 status bar。
> sshu 留著它是因為 sshu 下方有膠囊要分隔、filu 沒有 —— 但這條仍待實機複核。

```
  [1] hosts  [2] sftp  [3] ssh                                    1/5 hosts
──────────────────────────────────────────────────────────────────────────────
╭(hosts)─────────────────────────────────────────────────────────────────────╮
│ Name               User           Host                  Port  Auth         │
│ prod-web-01        deploy         10.0.3.14               22  ◆ privatekey │
│ db-replica-tokyo…  postgres       db.internal.corp      2222  ◆ password   │
│ bastion-eu-west-1  ec2-user       bastion.eu-west-1.…     22  ◆ privatekey │
│ staging-api        app            staging.example.com     22  ◆ password   │
│ jump               root           jump.corp               22  ◆ privatekey │
│                                                                            │
╰────────────────────────────────────────────────────────────────────────────╯
 space menu   ? help   1-3 tabs   q quit
```

> 圖例:`( )` = powerline 圓角 cap `U+E0B6` / `U+E0B4`;`◆` = auth 的 Nerd Font
> glyph(鑰匙 / 鎖)。上圖 terminal 寬 78、cursor 在第一列。

**[1] 是表格,不是卡片牆。** 卡片一次看一張很好看,但 host 清單是**沿著欄位往
下掃、互相比對**的東西,而且一張卡吃六列、一列只吃一列 —— host 一多就整個放不
進畫面。改成表格後欄位不變(Name / User / Host / Port / Auth),每台一列。

**tab [3] 是兩個 panel**(`[4]` sessions / `[5]` pty):

```
 ([1] hosts)  ([2] sftp)  ([3] ssh)                            3 live · 1 past
──────────────────────────────────────────────────────────────────────────────
╭([4] sessions)──────────╮╭([5] prod-web-01)─────────────────────────────────╮
│  prod-web-01        #1 ││ deploy@prod-web-01:~$ uptime                     │
│   db-replica-tokyo-ap- ││  14:02:11 up 42 days,  3:17,  2 users            │
│   northeast-1          ││ deploy@prod-web-01:~$ █                          │
│   prod-web-01       #2 ││                                                  │
│                        ││                                                  │
│                        ││                                                  │
│                        ││                                                  │
│                        ││                                                  │
│                        ││                                                  │
│                        ││                                                  │
╰────────────────────────╯╰──────────────────────────────────────────────────╯
 alt+esc leave pty
```

**數字只定址「當前 tab 裡看得見的 panel」**:`1`-`3` 永遠是 tab;`4`-`6` 在
tab [3] 是 sessions / pty,`4`-`7` 在 tab [2] 是左檔案 / 左 marks /
右檔案 / 右 marks。畫面上沒有那個編號,按下去就沒有反應 —— 規則直接從畫面讀得
出來,不必記。

**否決過的做法**:一開始做成「一條連續的全域定址空間」,在 tab [1] 按 `4` 會
跳到 tab [3] 並 focus sessions。看起來更一致,實際上是讓一個數字做了螢幕從未
顯示過的事。

**左欄固定 30 欄**(§1.2 的同一條理由:可調分割會讓 panel 寬度跟內容綁動),
整欄給 `[4]`。這個數字是 §11.28 定的:一列要完整說出
`<glyph><user>@<host>:<port> #<N>`,而 30 給位址留 16 欄 —— 剛好裝得下
`root@192.168.1.100`、`deploy@prod-web-01` 這類最常見的形狀。再寬也換不到
別的典型案例,再窄則連 `demo@localhost` 都要縮。

> 原本是 **26**,理由是「再窄版位開銷就會蓋過名字」。26 的問題不是太窄到不能
> 用,是**剛好差一格**:`demo@localhost:2222 #2` 要 22 欄而它只給 21。

**窄寬門檻是推導出來的**,不是另一個會忘記同步的常數:`sshNarrowW = sshLeftW + 28`
—— 只要 `[5]` 還剩得下 28 欄、split 就值得留著。所以把左欄縮窄也順帶讓 split
在更窄的終端機上活得下來(從 `w < 60` 降到 `w < 54`)。低於門檻時左欄整個收起、
`[5]` 佔滿畫面 —— 清單要 `Alt+Esc` 出來才切得到。

**tab [2] 是四個 panel**,左右各一半、寬度 1:1,每一半上面是檔案清單、下面是
它自己的 marks:

```
 ([1] hosts)  ([2] sftp)  ([3] ssh)                                   2 marks
──────────────────────────────────────────────────────────────────────────────
╭([4] local)──────────────────────────╮╭([6] prod-web-01)────────────────────╮
│ ~/Documents/sideproj                ││ /srv/www/releases                   │
│ ✓  deploy.sh                   1.2 K││    2026-08-30                      -│
│    README.md                   4.0 K││    2026-08-29                      -│
│ ✓  assets                          -││    current                         -│
│                                     ││                                     │
╰─────────────────────────────────────╯╰─────────────────────────────────────╯
╭([5] Marked files)───────────────────╮╭([7] Marked files)───────────────────╮
│  ✓ deploy.sh                        ││  (none)                             │
│  ✓ assets                           ││                                     │
╰─────────────────────────────────────╯╰─────────────────────────────────────╯
 space menu   ? help   1-7 surfaces   q quit
```

> 圖例:`✓` = 已 mark 的 Nerd Font glyph;`⌕` = 搜尋提示符(實際是
> `nf-fa-search`);size 欄目錄顯示 `-`。上圖寬 78。

**四個 panel 各自 focus,不是左右兩組。** 原本設計成「一邊的三個 panel 一起
focus」,但要對 marks 裡的某一項做事就得先切到另一邊 —— marks 需要自己的游標。
所以拆成 `[4]` `[5]` `[6]` `[7]`,`Tab` 依序走。

**兩邊都是「某個檔案系統」,不分主從。** 一邊可以是 local、可以是遠端,兩邊同
時遠端也可以 —— `remote.FS` 一個介面兩種實作,所以 upload / download /
remote-to-remote 是同一條路徑帶不同的值,不是三個功能。`[S]witch host` 的第一
項固定是 `local`,因為從這台機器上傳是最常見的事。

**cwd 在 panel 內的第一行**,不是下邊框:那是內容(「我在哪」),看的頻率跟清
單本身一樣高,塞進邊框會讀成 chrome。純文字、lavender 段落 + dim 斜線,沒有
chip 底色 —— 上面就是 panel 膠囊,再來一排填色形狀會打架(filu 的 `crumbRow`
同一個結論、同一個理由)。

**否決**:powerline 漸層麵包屑。照 filu 的**實作文件**做了一整套(blue→crust
漸層、WCAG 反白),然後發現 filu 的程式碼早就不是那樣了 —— 抄 UI 要讀對方的
程式碼,文件只拿來看理由。

**膠囊 tab bar**(§4.4 + §3.4):三顆**各自獨立**的圓角膠囊、彼此不相連
(不用 filu 那種 powerline 連鎖 chain —— 那條 chain 是「同一組分頁」的語彙,
這裡三個 tab 是三個並存 surface,分開才誠實)。

- **active**:blue `#89b4fa` 底 + base `#1e1e2e` 深字 + bold,兩端圓 cap 同 blue
- **inactive**:crust `#11111b` 底 + surface2 `#585b70` 字,兩端圓 cap 同 crust
- 標籤格式 `[N] label` —— `[N]` 同時是型別訊號與 hotkey 揭露(對齊 filu §3.4)
- **有兩個以上 panel 的 tab,每個 panel 都戴圓角膠囊 border title**:`[N] label`,
  與 tab 膠囊同形但語意不同 —— tab 膠囊是「可以按的按鈕」,panel 膠囊是「這個框
  叫什麼」。兩者能共存的前提是中間那條分隔線(見上)。

  **只有一個 panel 的 tab 不戴。** title 的作用是把 panel **彼此分開**;tab [1]
  只有一張表,而它正下方的 tab 膠囊已經寫著 `[1] hosts` —— 再掛一顆寫著 `hosts`
  的膠囊,是在回答一個沒有人會問的問題。

  **「不戴」是連膠囊都沒有,不是戴一顆空的。** `panelChip("")` 照樣會畫出左右
  兩個圓角 cap、中間什麼都沒有 —— 邊框上兩個沒來由的字元,正好是「沒有 title」
  最不該長成的樣子。

  這一項來回過兩次:先是「panel 不需要 border title,膠囊已經回答你在哪個
  tab」→ 但膠囊說不出「這是四個 panel 的哪一個」,所以 tab [3] 先加回純文字
  title → 最後統一成「全部 panel 都戴膠囊」並補上分隔線。

**右側狀態 slot**:膠囊列右端右對齊,hosts tab 顯示 `<cursor>/<total> hosts`。
這一列同時兼做膠囊與 panel 之間的視覺分隔。

### 1.1.1 `[1]` 的 `/` 搜尋

**haystack 是那一列的識別欄位串起來**:name、user、host、port ——
**不含 auth**。`password` / `privatekey` 是整張表大部分列共用的兩個字,拿去比對只會
把跟你打的東西無關的列拖進來。串起來而不是逐欄比對,是為了讓 query **跨欄**:
`prod 22` 找得到 port 22 的 prod-web-01。

**依分數排序,最佳的在最上面。** 對串起來的 haystack 做子序列比對是很寬鬆的 ——
`prod` 的四個字母也散落在 `db-replica-tokyo-ap-northeast-1` 裡的某處 —— 排序就是
讓這件事不要緊的辦法:你要的那一列在第 0 列。

> **這裡排序、tab [2] 不排序,不是不一致。** tab [2] 的結果是**串流進來**的,而
> 游標全程活著,每來一批就重排等於把使用者手底下那一列抽掉。這裡每一次按鍵清單
> 就已經完整,而且游標本來就會回到頂端,沒有東西可以被抽掉。

**query 佔掉欄位標題那一列**,不是把表格往下推:兩者回答同一個問題(「我在看
什麼」),共用一個 slot 才不會動到列數(§1.3)。右端是 `<符合> of <總數>`。提示符
是搜尋 glyph 不是 `/`,理由同 tab [2]。

**離開搜尋要留在同一列。** 游標在過濾期間指的是 matches 的索引,直接丟掉 filter
會把它變成完整清單裡的同一個索引 —— 你搜到了、按 Esc,然後站在別的地方。這一條在
`[1]` 特別要緊:letter 動作在搜尋中是被當字元吃掉的(§4.5),所以
**「搜尋 → Esc → 動作」是唯一能對搜尋結果動手的路徑**。

(同一個 bug 在 tab [2] 也在,一起修了。那邊多一種情況:三層底下的結果根本不在
當前目錄,查不到就回到頂端 —— 那是「這一列不在這裡」的誠實答案。)

**沒有 host 就沒有 `/`**:空表格沒有東西可搜,menu 列與字母一起消失。

### 1.2 表格欄位與收縮順序

| 欄 | 寬度 |
|---|---|
| **Name** | 剩餘寬的 35%(下限 8) |
| **User** | 剩餘寬(下限 6) |
| **Host** | 剩餘寬的 40%(下限 10) |
| **Port** | 固定 5(`65535`),右對齊 |
| **Auth** | 固定 12(glyph + 空格 + `privatekey`) |

**放不下就整欄拿掉,不把欄位削到沒有意義**。收縮順序是
**Auth → Port → User/Host**,`Name` 永遠留著 —— 叫不出名字的一列不算一列。
門檻由上面的下限推導:Auth 需要 `w ≥ 51`、Port 需要 `w ≥ 37`、User/Host 那組
需要 `w ≥ 30`,再窄就只剩 Name。

**header 與資料列走同一個 `tableRowText`**,所以欄位不可能對不齊 ——
`TestTableHeaderAndRowsAlign` 釘住這件事。

### 1.3 窄寬:表格自己就是 responsive 形式

卡片時代需要一個「`w < 38` 改單行清單」的 fallback 分支。表格不需要 ——
**收縮欄位就是它的窄寬形式**,一路降到只剩 Name 都還是同一個 widget、同一套
游標、同一套鍵。少一個分支、少一套要維護的版面。

### 1.4 Statusbar / footer 行數固定

膠囊列 N=1、footer N=1,**選定即鎖死**(通用 §1.3)。內容溢出就截斷,
絕不 reflow 多吃一行。

### 1.5 空狀態

第一次開、`hosts.yaml` 還不存在時,panel 置中顯示:

```
╭────────────────────────────────────────────────────────────────────────────╮
│                                                                            │
│                                No hosts yet                                │
│                                                                            │
│       Press [A] to add a host, or Space to see what you can do here        │
│                                                                            │
╰────────────────────────────────────────────────────────────────────────────╯
```

空狀態**必須**同時揭露 `[A]` 與 `Space` —— 否則新使用者第一次開 app 面對
空 panel 無路可走,X 直接掉。測試 `TestEmptyStateDisclosesEntryPoints` 盯著
這件事。

#### 所有 panel 的空狀態是**同一個形狀**

兩件事,兩行,都置中(水平與垂直):

| | |
|---|---|
| **事實** | 這裡沒有什麼 —— `No hosts yet` / `No host` / `Empty directory` / `Nothing marked` / `No sessions` |
| **提示** | 該按什麼(鍵用 `handColor`)。**沒有事可做就不寫** —— 空目錄是一個事實,不是一個提示 |

**文字不必統一,形狀必須一致。** 原本五個 panel 各自發明:`(empty)` 與 `(none)`
釘在左上角、一句沒有標題的置中句子、一個標題加一句。左上角那種還有另一個問題 ——
它讀起來像「一個清單,而它的第一**筆**寫著 (empty)」,那正是空狀態最不該長成的樣子。

**提示會折行。** 原本不會,而 `centerLine` 裝不下就截斷 —— 所以在 30 欄的 panel 上,
那句告訴使用者該按什麼的話會被切掉,而那正是他最需要它的寬度。折行是**逐字**做的
(`hintWord`),因為換行之後那個鍵還是鍵 —— 樣式必須跟著**字**走,不能套在整句上。

**panel 太矮就依序讓步**:先讓空行、再從尾端讓提示行,**事實留到最後**。一個什麼都
不說的 panel 正是這整套東西存在要防的狀態。

---

## §2. 色彩(catppuccin-mocha,沿用 u-family 錨點)

### 2.1 錨點

| 錨點 | hex | 用途 |
|---|---|---|
| base | `#1e1e2e` | 亮底膠囊上的深字、cursor 上的前景 |
| crust | `#11111b` | inactive 膠囊凹陷底 |
| structural blue | `#89b4fa` | panel border、active 膠囊 |
| surface2 | `#585b70` | unfocus panel border、inactive 膠囊字 |
| `handColor` subtext1 | `#bac2de` | menu / picker / session 清單的游標 bar |
| `editColor` lavender | `#b4befe` | 正在編輯的 form 列 |
| `dimColor` overlay0 | `#6c7086` | 次要文字 / region header / 停用欄位 |
| text | `#cdd6f4` | 一般欄位文字 |
| Peach / Red | `#fab387` / `#f38ba8` | warning / error override |

### 2.2 明度作 z-axis

- **popup**:border 走 `popupLayerColor(layer)`(lavenphire25 → sapphire),
  巢狀越上層越亮,不 hardcode(通用 §2.5 / §6.3)。直接沿用 filu 的實作。
- **表格選中列**:blue bar;未選中列的欄位退到 `dimColor`。這是
  z-axis 在 item 層的實例。

### 2.2.1 兩種「當前」分色:操作 vs 編輯

`handColor`(subtext1)與 `editColor`(lavender)都在講「這是你現在所在的
位置」,但**指的是兩件事**,所以分成兩條色帶(§B):

| 色 | 語意 | 出現在 |
|---|---|---|
| `handColor` subtext1 | 「游標停在這一列、按 `Enter` 會**對它做事**」 | Space menu / file picker / session 清單(hosts 表格用 blue,見 §2.3) |
| `editColor` lavender | 「這一欄你**正在改**」 | host form 的 focus 列 —— label、value、caret 一起 |

差別是**「作用於」vs「正在變更」**。清單游標指的是一個你可能操作的對象;
form 的 focus 列是一個你此刻正在改寫的值。共用一個顏色會讓 user 在 form 裡
以為「按 Enter 會對這一列做事」,但實際上 Enter 是送出整張表(§6.1 menu vs
form 的同一條分界)。

`TestFocusedFormRowIsLavender` / `TestListCursorIsNotLavender` 把這條分界釘住
—— 顏色指派是這份設計裡唯一沒有其他機制能擋住它漂移的規則。

### 2.3 cursor 的視覺形式:整列 bar

表格的列只有一列高,所以 cursor 終於可以用**填色 bar** —— 跟 Space menu /
file picker / session 清單同一種形式。(卡片時代做不到:一張卡四列高,整塊反白
會把四行文字都推進低對比區。)

| | 形式 |
|---|---|
| **選中列** | **blue `#89b4fa`** 背景 + base 深字 |
| **未選中列** | Name 用 `textColor`(要能掃)、其餘欄位 `dimColor` 退階 |

**為什麼是 blue 不是 `handColor`**:subtext1 `#bac2de` 跟 `textColor` `#cdd6f4`
太近,讀不出「這列被選中」。blue 是這套色盤裡唯一夠響的。

這條**跟 panel chrome 共用 structural 色帶**(§B)。可接受的理由:兩者是同一個
概念的不同尺度 —— panel 的「你在這個 surface」與列的「你在這一列」—— 而且
chrome 是外框、選中列在框內,不會貼在一起。

**已知的不一致**:hosts 表格的游標 bar 是 blue,但 Space menu / file picker /
session 清單的 bar 仍是 `handColor`。若之後覺得刺眼,收斂方向是把**那些也改成
blue**,而不是把這裡改回去 —— 那只會退回「看不出被選中」的原點。

`TestSelectedHostRowIsBlue` 釘住 hex 與「選中/未選中 render 出來不能一樣」。

---

## §3. 符號語彙

### 3.1 Nerd Font 是設計、必裝

同 kbu / filu(通用 §3.1),不做降級分支。source 內**不放 PUA 字面**,一律
`string(rune(0x...))`。

### 3.2 glyph 配置

| 位置 | 語意 | 候選碼位 |
|---|---|---|
| Auth 欄(privatekey) | 金鑰 | `nf-fa-key` `U+F084` |
| Auth 欄(password) | 密碼鎖 | `nf-fa-lock` `U+F023` |
| Space menu title | 選單 | `nf-fa-bars` `U+F0C9` |
| help title | 說明 | `nf-fa-question-circle` `U+F059` |
| confirm title | 警示 | `nf-fa-warning` `U+F071` |
| form title(create) | 新增 | `nf-fa-plus` `U+F067` |
| form title(edit) | 編輯 | `nf-fa-pencil` `U+F040` |
| 膠囊 cap | 圓角 | `U+E0B6` / `U+E0B4` |
| Auth radio | 空 / 實 | `nf-md-radiobox_blank` `U+F043D` / `nf-md-radiobox_marked` `U+F043E` |

> **codepoint 一律查字型,不憑記憶。** 補 radio glyph 時把已經在用的常數一起對過
> 字型的 cmap,查到兩個對不上自己註解的:`glyphMark` 寫著 `nf-md-check_bold` 但
> 指到 `md-alpha_m_box`(一個框起來的字母 M),`glyphUpload` 寫著 `nf-md-transfer`
> 但指到 `md-upload`。兩個都已改成註解說的那個。
>
> 另外 `nf-md-radiobox_blank` 在字型裡是以別名 `checkbox-blank-circle-outline`
> 登記的(MDI 本來就把兩者當同一個圖),所以下一個去查 cmap 的人會看到另一個
> 名字 —— 那不是錯,不要「修正」它。

> 碼位待實作時對照 Nerd Font cheat sheet 逐一驗證後鎖定。

**Auth 是唯一「glyph 隨值變」的欄** —— 鑰匙 vs 鎖同時是型別訊號(這欄是
auth)與內容訊號(哪一種 auth)。這是 §3.3「型別 + 內容」在單一 cell 上的壓縮,
可接受,因為文字就在旁邊補全內容訊號。

**表格的其他欄沒有 glyph**:column header 已經說了那欄是什麼,卡片時代的
per-field glyph 是在補一個表格天生就有的東西,留著只是雜訊。

### 3.3 CJK icon 寬度

沿用 filu 的 **CPR 偵測**(`\x1b[6n`,在 `tea.NewProgram` 之前實測 icon
實際格寬)。表格欄寬與 pty 版位都是固定格數,icon 寬度誤判會**直接破框**,所以這一層在
sshu 是必要而非可選。

### 3.4 Surface 標籤

- **tab 膠囊**:`[N] label`(型別訊號 `[N]` + 內容訊號 label)
- **panel**:**每一個 panel 都有** `[N] label`,做成與 tab 同形的圓角膠囊、
  嵌在上邊框。tab 列與 panel 列之間隔一條整寬分隔線,兩排膠囊才不會讀成同一
  條 chrome(§1.1)。膠囊只說得出「你在哪個 tab」、說不出「這是四個 panel 的
  哪一個」,所以 title 本身仍然必要(`ui/chrome.go panelChrome`)
- **panel title 不帶 glyph**:tab [2] 的檔案 panel 曾經在 host 名前放一個
  `nf-md-monitor`,拿掉了 —— 全 app 沒有第二個 panel title 帶 icon,一個帶了
  就讀成特例而不是裝飾(§B)
- **popup**:glyph + text 嵌上邊框、hint 嵌下邊框(kbu form)

---

## §4. 互動

### 4.1 Core 5 鍵(見 §A.0.Y)

語意在任何 surface 不變。

### 4.2 清單導覽

| 鍵 | 動作 |
|---|---|
| `j` / `k` | 上 / 下一列 |
| **`u` / `d`** | **上 / 下半頁**(`Ctrl+U` / `Ctrl+D` 同義) |
| `gg` / `G` | 第一列 / 最後一列 |
| 方向鍵 | 與 `j`/`k` 同義 |
| **`h` / `l`** | tab [2]:切到左半 / 右半,保持同一列。**tab [3] 沒有 `h`/`l`** |

**一份詞彙、一個實作**(`ui/nav.go moveCursor`)。四個清單面(hosts 表格、
`[4]` sessions、sftp 的檔案與 marks)全部走它,所以往詞彙裡加
一個鍵,是一次加到所有清單上,不是加四次。

**半頁而不是整頁**:落點與離開的地方要有重疊,眼睛才接得回去。整頁跳完要重新
找自己在哪。

**`j`/`k` 會繞**:清單是一個環,最後一列離第一列只有一個鍵。這在**短清單**上最
有感 —— 不繞的話,替代方案是按著 `k` 看畫面完全沒有反應。**所有有游標的面都繞**,
panel 與 popup 一視同仁(hosts 表格、`[4]` sessions、sftp 的檔案與 marks、Space
menu、host picker、file picker、Transfers)。

**`u`/`d` 不繞**,`gg`/`G` 更不用說。半頁是「瞄準」的移動,一個會無聲傳送到清單
另一端的瞄準比停下來更糟。

**沒有游標的東西也不繞**(`moveScroll`):`!` app log 與 `?` help 是 viewport,
捲到底又跳回頂端會讀成故障 —— 因為根本沒有游標可以「繞回去」。

**`u`/`d` 這兩個字母是有代價的**,而且代價落在別人身上 —— 見 §4.4 的保留規則:
`[U]nmark` 與 `[D]elete host` / `[D]uplicate` 從此**只認大寫**。

**導覽字母不會被動作拿走,一條例外都沒有。** tab [2] 的刪除曾經放在 `d` 上一輪,
代價是那個 tab 失去裸的半頁鍵、而且需要一條專屬規則來解釋自己
(「只有有第二拼法的字母能被要走」)。搬到 `x`/`X` 之後,那條規則整條刪掉,這句話
變回沒有註腳的一句話。

> **改過一次**:原本的判斷是「不另設 half-page,捲動由 `j`/`k` + cursor 驅動」,
> 理由正是要把 `d` 留給 Delete。清單變長(sftp 的遞迴搜尋一次可以吐出上千列)
> 之後這個取捨反過來了:一列一列走一份搜尋結果不是可行的操作方式。

**`h`/`l` 只在 tab [2] 有意義**:左半 / 右半,而且**保持同一列** —— `[5]` 去
`[7]` 而不是 `[6]`。兩邊是鏡像,「對面的同一個 panel」是唯一「只換了看哪台機器、
沒換在看什麼」的落點。`Tab` 是「下一個 panel」,`h`/`l` 是「我要哪一邊」;1:1
分割時後者才是常態。

> **改過一次**:原本 **tab [3] 的 `l` 也進 `[5]`**,理由是空間上的「往右」——
> `[4]` 是左欄、pty 是右欄,跟 tab [2] 的 `h`/`l` 同一個意思。**移除了**:`Enter`
> 在一個 session 上本來就同時「顯示它」和「把 focus 給 `[5]`」(`openSession`),
> 所以 `l` 是第二個鍵去做一個鍵已經做完的事。而它的代價不是零 —— 每一個會把鍵盤
> 交給遠端的鍵,都是一個你可能「路過」而不是「決定」進去的入口,而唯一的出路是
> `Alt+Esc`。**把鍵盤交出去應該是一個決定**,所以入口只留刻意的那兩個:`Enter`,
> 和直達任何 panel 的 `5`。

> **這跟「`Tab` 不進 `[5]`」(§4.4.1)不衝突。** `Tab` 進去會被遠端吞掉 ——
> 等於把帶你進去的那把鑰匙鎖在門內。`l` 不是任何地方的「出口鍵」,借給 pty 不
> 花任何成本;出來仍然是 `Alt+Esc`。兩個鍵做不同的事,正是要有兩個鍵的原因。
>
> 反方向沒有 `h`:`[5]` 把整個鍵盤交給遠端,那裡的 `h` 是遠端的 `h`。

在 [1],`h`/`l` **沒有綁定** —— 表格沒有欄可以左右移動(卡片網格時代它們是
「左右移一張卡」,改成表格後那個語意消失了)。但兩個字母**仍然被保留**(§4.4)
—— 一個在某個面是導覽的字母,不該在另一個面變成某個動作的 fallback。

### 4.2.1 入口鍵會關掉自己開的東西

**`Space` 關掉當前浮層,`?` 開關 help。** 一個只有單向的入口鍵是陷阱:使用者會
伸手去按同一個鍵想出來,結果沒反應,那個面看起來就像卡住了。filu 的 space menu
一直是 `case "esc", " "`,sshu 漏掉了。

**在一個地方解決,不是每個 popup 各寫一份** —— 跟 `Esc` 同樣的理由(§4.3):
一個角色一個地方,才不會有某一個浮層是「忘記做」的那個。實作在
`AppModel.handleKey`,`m.textFloat()` 是唯一的例外判斷。

**例外:正在被打字的浮層**(host form、file picker、Rename 輸入框)。那裡的空白
就是空白、問號就是問號(§4.5)。

`?` 還會**疊在別的浮層上開**:§A.2 承諾 help 在任何 surface 都到得了,而一個迷路
的使用者最可能站的地方,正是他剛打開的那個 menu。層級由 `m.layer()` 決定,所以
邊框顏色會跟著往上跳一階(§6.3),`Space` 再按一次只收掉最上面那層。

`TestSpaceDismissesEveryFloat` 用一張**列出全部浮層**的表釘住這件事 —— 針對被回報
的那一個寫測試沒有用,漏掉的一定是沒被想到的那一個。

### 4.3 letter hotkey ⊆ Space menu(完整性)

| key | 動作 | region |
|---|---|---|
| `e` | Edit | item |
| `d` | Delete | item |
| `s` | Sftp `(planned)` | item |
| `A` | Add | panel |

全部小寫(清單沒有 `d` = half-page-down 的衝突,因為捲動由 `j`/`k` +
cursor 驅動,不另設 half-page)。

**完整性 audit**:新增任何 contextual 動作,必須同步在 Space menu 加 entry。
只綁 letter hotkey = VTP 破洞。

**這個包含關係是單向的。** 「每一個 letter hotkey 都是 menu 的一列」不等於
「每一列都有 letter hotkey」—— core key 的列(`Enter` / `Tab`)本來就沒有,
而**刻意不給字母**也是一個合法的選擇:見 §11.26 的 `Close all sessions`。
menu 是慢路徑,而有些動作的正確速度就是慢。

### 4.4 hotkey 揭露 = bracket `[X]label` + 「亮鍵暗述」

**兩個層面、一套規則**:

1. **bracket `[X]label`** —— letter hotkey 專用(Space menu 的列)。core-key
   動作(如 Connect = `Enter`)不套 bracket,改在 hint 欄顯示鍵名 —— bracket
   專職 letter hotkey。

   **bracket 顯示的就是要按的那個鍵**,一字不差(`ui/popup.go bracketHotkey`)。
   **標記是契約、綁定跟著標記走**,不是反過來 —— 標記寫 `[A]dd`,使用者照著
   按 shift+A 卻沒反應,那個標記就是在說謊。

   **配對是完全比對,大小寫算數**(`ui/popup.go hotkeyIndex`)。畫面上那個
   bracket 就是全部的綁定:**它寫的一定按得動,它沒寫的一定按不動。**

   > **改過兩次,方向相反。** 最早是「一律大寫顯示、一律不分大小寫」,那是在修
   > 一個真的 bug —— 表裡宣告 `c`、顯示卻印成 `[C]`,照著標記按 shift+C 什麼都
   > 沒發生。但那個修法落在錯的地方:**讓 bracket 印出宣告的那個字母**才是治本,
   > 之後那條寬鬆比對就只剩下一個「畫面上沒有任何東西提過」的第二綁定 ——
   > `[C]lose` 會被裸的 `c` 觸發,而在 tab [2] 裡 `c` 會觸發 `[C]lear marks`,
   > 正好違背「小寫是這一列」那條規則。所以它被拿掉了。
   >
   > 拿掉之後有兩個附帶結果:`t`/`T`、`x`/`X` 不再需要「完全比對優先」這種特例
   > 說明(本來就只有完全比對),而導覽字母也不再需要 `hotkeyIndex` 裡的守衛 ——
   > **沒有東西會 fold 到 `d` 上,因為沒有東西會 fold**。剩下的唯一風險是某個動作
   > 直接宣告 `d`,那由 `TestNoActionClaimsANavigationKey` 擋。

   這條只適用 bracket 標的 letter hotkey;導覽鍵**仍然分大小寫**(`G` 跳底、
   `g` 是 `gg` chord 的前半,兩者不能混)。
2. **亮鍵暗述** —— 凡是「鍵 + 說明」成對出現的地方(footer legend、popup 下邊
   框 hint),**鍵用 `focusColor`(藍)、說明用 `dimColor`(暗)**。學一次、走
   全 app。實作:`ui/chrome.go keyLegend`(footer)與 `ui/popup.go hintLegend`
   (popup hint)共用同一條規則,只有間距不同 —— 邊框那行沒有 footer 那麼多
   餘裕,所以 hint 收緊成「pair 內 1 空格、pair 間 2 空格」。

   > **鍵原本是 `handColor`,換成藍。** 理由是 band(§2.1):`handColor` 是
   > **游標** —— 「你在這個清單的哪一列」。用同一個顏色講「這裡有個鍵可以按」,
   > 會讓 footer 讀起來像上面那片清單多長出來的一列。鍵名是 **app 在命名自己的
   > 控制項**,是 chrome 而不是 user state,那正是 structural band 畫的那條線;
   > `rowSelColor = focusColor` 早就開過同一個先例(同一個念頭、不同的尺度)。
   >
   > 兩個 renderer **必須一起換**:它們是同一條 legend 的兩個尺度,鍵在 app 底
   > 那一列是藍的、在 popup 底那一列卻是別的顏色,規則就不成立了。
   >
   > 這次也順手抓到一個**測錯東西的斷言**:`TestListCursorIsNotLavender` 宣稱
   > 「menu cursor 還是 handColor」,但 menu cursor 是**背景**色,它比對的卻是
   > 前景 —— 而畫面上那個前景 handColor 一直來自 popup 的 hint legend。也就是說
   > 游標塗成任何顏色它都會綠。legend 轉藍才把它逼出來,已改成比對背景。

**popup hint 是 contextual 的**:它顯示的是**當前欄位**能做什麼,不是一份
固定清單。這是 §4.5 用來換掉 `Space` 入口的那個「常駐揭露」—— 換掉了就得換
得夠準,否則等於沒揭露。

### 4.4.1 `Tab` 只在當前 tab 裡輪詢

> **v0.2**:本節「輪詢」在 preference / file transfer 照舊;**ssh tab 的
> `Tab` 改為「顯示開關」**(切換游標 session 在網格上的格子)—— 該 tab 的
> panel 輪詢本來就是空集合(網格不是 Tab 可以走進去的地方),鍵讓給清單
> 整天在做的那件事。見 §11.六。
>
> **v1.3.0**:同一件事**多了一個字母** `[H]ide`(§11.30)。`Tab` 沒有被拿掉
> —— 它是這個 panel 的鍵,跟 `1`-`9`、`Space` 同一層,由 footer 與 help 揭露;
> `H` 是 item operation 那一列的鍵,由 menu 的 bracket 揭露。兩個揭露管道,
> 一件事。

`Tab` 循環**當前 tab 看得見的 panel**,到底就繞回第一個,**不會跨到別的 tab**。
換 tab 是 `1`/`2`/`3` 的事 —— 一個鍵一個工作。

tab [1] 只有一個 panel,所以 `Tab` 在那裡不動;tab [3] 也只剩一個(`[4]`),
`Tab` 在那裡唯一的作用是**從 pty 出來**;
tab [2] 是 `[4]` → `[5]` → `[6]` → `[7]` → 繞回。

**`Tab` 仍然不進 tab [3] 的 `[5]`** —— 進去就被遠端吞掉,等於把帶你進去的鑰匙
鎖在門內。要進 pty 有三條路:`[4]` 上按 `Enter`、按 `5`、或按 `l`(§4.2);出來
一律 `Alt+Esc`。

> **改過一次**:原本是「`Tab` 走的是 surface,不是 tab」—— 走完當前 tab 的
> panel 就接著跳下一個 tab。聽起來一致,實際上同一個鍵在同一個循環裡會做**兩
> 種尺寸的移動**:大部分時候換一個框,偶爾整個畫面換掉,而且要數到第幾下才知
> 道是哪一種。

**`Tab` 刻意不會走進 `[5]`** —— 那個 panel 把鍵盤交給遠端,`Tab` 進去就被
吞了,等於把帶你進去的那把鑰匙鎖在門內。進 `[5]` 一律是明確動作(在 `[4]` 上
按 `Enter`、或按 `5`),出來一律是 `Alt+Esc`。

### 4.6 `Alt+Esc` —— sshu 專屬、只在 panel [5]

> **v0.2**:`[5]` 已成**網格**。`Alt+Esc` 的語意不變(把鍵盤收回來),落點
> 是 `[1]` sessions,side 欄同時回來;格子之間按住 Alt 用方向鍵走,tab 和絃
> `Alt+p/f/s` 在 pty 內也通。見 §11.六。
>
> **v1.2.0**:tab 鍵改成裸字母,**pty 內不再能切 tab** —— 這條路現在只有
> `Alt+Esc` 一個入口,它的重要性因此更高(§11.21)。同一輪 `Ctrl+C` 也還給了
> 遠端(§11.20),所以 pty 內屬於 sshu 的鍵只剩三組:`Alt+Esc`、`Alt+方向鍵`、
> 以及非 alt-screen 時的 `PgUp`/`PgDn`(§11.19)。

**這條不是 VTP core key,也不計入 §A.0.Y 的 5 個 role。** 理由:panel [5] 把
鍵盤整個交給遠端程式,五個 core key 在那裡全部失效(`Tab` `Enter` `Esc`
`Space` `?` 都會送出去),所以需要一把「把鍵盤要回來」的鑰匙。它的作用對象是
「sshu 對鍵盤的所有權」,不是任何 focus 裡的東西,也不是全域動作 —— 兩條 track
都不歸它管。對齊 filu 把 `Ctrl+C`(逃生硬退)排除在 Y 之外的處理。

| 情境 | `Alt+Esc` |
|---|---|
| focus 在 `[5]` 且 session 還活著 | 收回鍵盤、focus 回 `[4]` |
| 其他任何地方 | 等同 `Esc`(關最上層浮層)—— 不做成死鍵 |

**揭露(強制)**:focus 進 `[5]` 時 **footer 整條換成 `alt+esc leave pty`**。
這時 `space` / `?` / 數字 / `q` 全部會送給遠端,footer 再列它們就是說謊。留下
唯一還成立的那一條,而它剛好就是出口。

**技術前提與已知誤觸**(README 要寫):

- bubbletea 認得 `\x1b\x1b` → `{KeyEscape, Alt}`,但**終端機要真的送**。
  macOS Terminal.app 需開「Use Option as Meta key」、iTerm2 需把 Option 設成
  Esc+;kitty / Alacritty / WezTerm 預設就送。**沒設的人出不了 pty。**
- bubbletea 靠「ESC 後緊跟另一個 byte」判斷 Alt,所以遠端跑 vim 時**快速連按
  兩次 Esc 會被讀成 `Alt+Esc`**、意外跳出 pty。按 `Enter` 或 `5` 就回得去。

### 4.5 `Space` / `?` 在文字輸入 surface 內的例外(§0 規則擴充)

`Space` 是 §A.1 入口,但在 **host form 的文字欄位**內,`Space` 必須輸入
空白字元。這不是 VTP 破洞,而是規則擴充:

- **origin UX**:`Space` 入口要回答「我在這裡能做什麼」。
- **在 form 內**,這個問題由 **border hint 常駐揭露**回答
  (`Tab next   ←→ switch   Enter save   Esc cancel`),而且是**永久可見**
  (比按一次入口更強的揭露)。
- 所以 form 內不需要 `Space` 入口、X 不掉。

同理,form 內 `j`/`k` **不作導覽**(它們是字元),欄位切換一律走
`Tab` / `Shift+Tab` / `↑` `↓`。

---

## §5. Mouse

`(planned)` —— 沿用通用 §5 mapping(左鍵 focus+select 列、雙擊 =
`Enter`、右鍵 = `Space`、滾輪 = 捲 card row)。mouse 必為 keyboard 的
mapping、不引入新語意。

**但這件事在 sshu 有一個別人沒有的代價**(§11.19):開 mouse tracking 之後,
alt screen 下的滑鼠事件被程式攔走,終端機原生的**選字複製**就沒了 —— 要按住
Option 才選得到字。對一個 ssh 工具而言,把畫面上的輸出複製走是高頻動作。
所以 `[3]` 的捲動走 `PgUp`/`PgDown` 而不是滾輪,而整個 mouse 支援在還沒想清楚
怎麼跟選字共存之前,不會只為了捲動而開。

**後來想清楚了,答案是不開**(§11.33):grid 破壞掉的選字用**鍵盤**的選取模式
(`Alt+v`)補回來,而不是用滑鼠買回來。鍵盤模式在自己以外不花任何東西;開 mouse
則要拿**整個 app** 的原生選字去換,包括那些原生選字仍然好用的 panel。

---

## §6. 浮層(Popup Convention)

### 6.1 taxonomy — sshu 有 **5 類**(比 filu 多一個 `form`)

| 類型 | sshu 實例 | 特徵 |
|---|---|---|
| **menu** | Space menu、**Identity file picker** | 分 region / 清單、cursor-first、選一個執行 |
| **message** | Connect 確認、Delete 確認、Toast | 短、確認 / auto-dismiss |
| **viewport** | `?` help、**`!` app log**、`[v]iew`、**`[V]iew` 明細**(§11.29) | 可捲、沒有游標 |
| **form** ← **新** | Add host / Edit host | 多欄位、逐欄位 focus、一次提交 |
| **input** ← **新** | tab [2] 的 Rename | **一行**文字、一個問題、Enter 送出 |
| **pty** | **tab [3] 的 panel [5]**(ssh session) | 外部程式在 sshu 內 render |

前四類都已落地(`ui/spacemenu.go` / `ui/confirm.go` + `ui/toast.go` /
`ui/helppopup.go` / `ui/form.go`),共用 `drawPopupBox` 與 `popupAnimator`。

sftp 的傳輸進度**不是** pty:sshu 自己說 SFTP 協定,進度是自己畫的
(`ui/transfer.go`),沒有外部程式可以 render。

**pty 在 sshu 不是浮層、是 panel [5] 本身** —— 這是與 filu 的分歧點:filu 的
pty 是「開 `$EDITOR`,關掉就結束」的短時浮層;sshu 的 session 是長時的、而且
同時可以有很多個,所以它是常駐 panel 的內容,不是疊在上面的東西。

**為什麼 input 不算 form**:form 是「填 N 個欄位、一次提交」,input 是「回答一
個問題」—— 跟 confirm 是同一個家族(短、一問一答),差別只在答案是文字而不是
yes/no。做成單欄位的 form 會讓 `Tab` 這個「切欄位」的鍵在只有一欄的地方變成死鍵。

**為什麼 form 要獨立成一類、不塞進 menu**:menu 的語意是「從 N 個選項挑
一個執行」,form 的語意是「填 N 個欄位、一次提交」。混成一個浮層就是 §6.1
禁止的「混血」—— 使用者會分不清「按 Enter 是執行這一列、還是送出整張表」。
分家後語意乾淨:menu 的 `Enter` = 執行 cursor 那列;form 的 `Enter` = 送出
整張表(不論 cursor 在哪一欄)。

全部走共用 `drawPopupBox`(title 嵌上邊框、hint 嵌下邊框)。

### 6.2 Space menu(menu)

**標題是「當前 focus 的 panel」,不是 tab。** 分割的 tab 裡,「我在這裡能做
什麼」取決於站在哪一個 panel —— 一個只寫 `[2] sftp` 的標題分不出 `[4]` 和
`[6]`,而這兩邊接的可能是完全不同的兩台機器。標題字串與 panel 自己邊框上的膠囊
**來自同一個函式**(`sftpModel.panelTitle` / `sshModel.panelTitle`),所以浮層
不可能跟使用者正在看的框說法不一致。

**tab [2] 的 `[4]`/`[6]` menu 分成兩個 region**,跟 tab [1] 與 kbu 的
panel-2 menu 同一個形狀:

```
 item operation
 Enter                       Enter . open directory
 [a]ppend to marks               again takes it off
 [r]ename                           this item, here
 [v]iew                        read this item, here
 [e]dit                   open this item in $EDITOR
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
 [H]ost      switch this side — a directory, or a host
 [D]isconnect              close it, back to no host
 [J]obs             transfers in flight, and cancel
```

**大小寫本身就說明範圍**:**小寫作用在游標那一列,大寫作用在整個 panel。**
`[a]ppend` / `[r]ename` / `[t]ransfer` / `[x]` 對著一列;`[A]` / `[T]` / `[X]` /
`[C]` / `[R]` / `[S]` / `[D]` / `[P]` 對著這一側。tab [2] 是唯一需要這個區分的地方 —— 它是唯一
兩種範圍並存、而且同一個動詞出現兩次的 tab(`[t]ransfer` / `[T]ransfer all
marks`、`[x]` / `[X]`、`[c]lear mark` / `[C]lear marks`),讀的人必須不看 hint
欄就分得出來。

`/` 不在這條規則裡(不是字母),`Enter` 也不在(core key,鍵名放 hint 不套
bracket,§4.4)。

**把東西移出 marks 的字母幾經演進,落在 `[c]lear mark`。** 一開始 `[U]nmark`
併進 `[m]`(一鍵 toggle,`u` 屬於半頁上捲);v0.2 後使用者裁定拆開(見
11.10):檔案清單是 `[a]ppend to marks`(仍 toggle,hint 誠實寫「again takes
it off」),marks 清單的移出是 `[c]lear mark` —— 跟 `[C]lear marks` 同字母
成對,小寫單項、大寫整個 panel,同 `t`/`T`、`x`/`X` 的 case 文法。

**兩個 region 的標題是 `item operation` / `panel operation`**,三個 tab 共用同兩
個字串(`menuItemRegion` / `menuPanelRegion`)。標題措辭不一樣的 menu 會讀成另一
**種**選單,而不是同一種。

> **試過並否決**:讓 item 標題寫出那一列的名字(`item . deploy.sh`)。想法是讓
> `[x]` 讀在它會刪掉的東西旁邊。實際上它不直觀 —— 而且那個資訊並沒有消失:游標
> 就在那一列上、那一列還是反白的,popup 的標題也已經寫著是哪個 panel。

**只有一個 region 的時候整份保持扁平**(kbu 的規則):標題壓在單一群組上面是雜訊。
空目錄沒有 item region(下面那條),沒有 host 時只剩一列,兩種情況都不加標題。

**沒有列就沒有 item 動作。** 空目錄、或空的 marks panel,`Enter`/`M`/`R`/`t`/`d`
會整組消失 —— 連同它們的字母,因為 hotkey 與 menu 走同一個 `sftpApplicable()`。
列出一堆按了沒反應的東西,跟沒有 host 時列出 Transfer 是同一個謊。

**還沒選 host 的那一側,能做的動作只有 `[H]ost` 一個。** 沒有 host 就
沒有東西可以標記、傳送或清空,列出那些列只會教會使用者「這個 menu 說的不算
數」。hotkey 與 menu 走同一個 `sftpApplicable()`,所以兩邊不可能各說各話
(§4.2)。而既然只剩一個答案,**Space 就直接給那個答案** —— 開 host 清單本身,
不再繞一層只有一列的 menu(§11.16)。

```
        ╭─ ◆ [1] hosts ───────────────────────╮
        │                                     │
        │ host . prod-web-01                  │
        │ Connect         Enter . ssh session │
        │ [E]dit             change this host │
        │ [D]elete     remove from hosts.yaml │
        │ ─────────────────────────────────── │
        │ panel                               │
        │ [A]dd                    a new host │
        │                                     │
        ╰─ j/k move   Enter run   Esc close ──╯
```

- item region 標題帶 cursor host 的名字 —— 使用者一眼確認「這些動作打在誰身上」
- `Connect` 不套 bracket(core-key 動作),鍵名放 hint 欄
- 單一類動作時不分 region(通用 §6.6),但 hosts 有 item + panel 兩類,分

**實作上這條是「由結構保證」而不是靠自律**:letter hotkey 與 menu row 都從
同一張 `hostActions` 表展開(`ui/app.go`),所以不可能只加 hotkey 而漏掉 menu
entry —— 它們是同一個宣告。`TestSpaceMenuListsEveryAction` /
`TestEveryMenuRowRuns` 從兩個方向釘住。

**窄寬退化**:box 塞不下 label + hint 兩欄時,**hint 先讓位**(label 是動作
本身、hint 只是補充),`ui/spacemenu.go`。

**列有三種狀態**:能做(一般)、不適用(**整列不列出**,`appliesTo`)、
**現在不能做(整列暗掉、游標條降階、按下去說明原因)**。第三種是給「屬於
這個 panel、只是此刻被擋住」的動作用的 —— 讓它消失會被學成「這裡沒有這個
動作」。見 §11.15。

### 6.3 Host form(form)—— Add / Edit 共用

> **v0.2**:Auth 變三選(password / privatekey / **credential**),選
> credential 時 User 欄整列變暗(credential 整包供應 user);IdentityFile 與
> Credential 兩個「選值欄位」改為 **Enter 空欄開選單、Enter 有值直接送出、
> Backspace 整行清除**,`Tab` 回歸「下一欄」。見 §11.三、§11.五。

```
      ╭─ ◆ New host ──────────────────────────────────╮
      │                                               │
      │  Name                                         │
      │  Host                                         │
      │  Port          22                             │
      │  User                                         │
      │  Auth          ( ) password  (•) privatekey   │
      │  IdentityFile   tab to browse ~/.ssh          │
      │  Password      —                              │
      │                                               │
      │                                               │
      │                                               │
      ╰─ Tab browse  ↑↓ next  Enter save  Esc cancel ─╯
```

| 欄位 | 型別 | 說明 |
|---|---|---|
| Name | text | 必填、**全域唯一**(即 hosts.yaml 的 key) |
| Host | text | 必填、IP 或 domain |
| Port | text(只吃數字) | 預設 `22`,非數字直接不進欄位 |
| User | text | 必填 |
| Auth | **segmented toggle** | `password` / `privatekey`,`←` `→` 切換 |
| **IdentityFile** | text + **`Tab` 開檔案選擇器** | Auth = privatekey 時啟用;空的時候顯示 dim placeholder `tab to browse ~/.ssh` |
| Password | text(**遮罩 `••••`**) | Auth = password 時啟用;= privatekey 時 **dim + 跳過** |

**欄位叫 `IdentityFile` 不叫 `Identity`** —— 它存的是**檔案路徑**,不是身分;
名字跟 `hosts.yaml` 的 `identity_file` 對齊,使用者手改 yaml 時不用再猜對應。

**IdentityFile 與 Password 兩列永遠都在、只是其一 dim** —— popup 高度恆定
(不因切 auth 而跳動),而且使用者一眼看到「另一種 auth 也存在」。停用列以
`dimColor` 繪、內容顯示 `—`。`TestFormHeightIsStableAcrossAuth` 釘住高度。

**hint 隨 focus 換**:Auth 欄多一條 `←→ switch`;IdentityFile 欄的 hint 變成
`Tab browse  ↑↓ next`;其餘欄是 `Tab next`。`TestFormHintIsPerField` 釘住
「不能在做不到 browse 的欄位上宣傳 browse」,而且**只看 hint 那一行** ——
placeholder 上也有 browse 這個字,整份搜會因為錯的理由通過。

**`Tab` 在 IdentityFile 欄的意義不一樣**:它開檔案選擇器,就像 shell 裡對著
路徑按 `Tab` 會補全。這是全 form 唯一一個 `Tab` 不等於「下一欄」的地方,而
border hint **正好只在那一欄**這樣寫 —— 那就是文字輸入 surface 可以擁有自己
一把鍵的全部理由(§4.5)。

> **代價明講**:那一欄**沒辦法用 `Tab` 離開**。方向鍵與 `Shift+Tab` 仍然可以,
> hint 也列了。`TestPathFieldCanStillBeLeft` 釘住這條退路。

**驗證與錯誤**:`Enter` 送出時驗必填 / name 唯一 / port 為 1-65535。不通過
就**留在 form**、把該欄位標紅(Red override,§2.4),錯誤字顯示在最後一列
—— 那一列**永遠存在**(沒錯時是空白),所以驗證失敗不會讓 popup 抽動。
不另開 popup 疊上去(§6.7)。

**欄位歸屬**:`store` 驗的是「整份文件合不合法」,`ui` 驗的是「哪一欄錯了」
—— 後者 `store` 給不出來。`store.SaveTo` 寫入前仍會再驗一次,所以 store 仍是
權威,ui 這層只負責把錯誤指到正確的欄位上。

**窄寬退化**:toggle 塞不下兩個選項時只顯示當前選項(`(•) privatekey`)——
選項被切一半會讀成另一個值。label 欄在極窄時也會讓位,寧可截斷 label 也要
留住 value 欄 —— 截斷的 label 還讀得懂,消失的 value 不行。

### 6.3.1 Identity file picker(menu)—— `Tab`

> **v0.2**:開啟鍵由 `Tab` 改為**空欄位上的 `Enter`**(§11.五)。picker 本身
> 的行為不變。

私鑰路徑**用選的、不用打**。打錯的路徑要到連線那一刻才會失敗,離出錯的地方
太遠。

```
      ╭─ ◆ Identity file  ~/.ssh ───────────────────────────╮
      │  ed                                                 │
      │─────────────────────────────────────────────────────│
      │ id_ed25519                          0600     411 B  │
      │ id_ed25519.pub                      0644      98 B  │
      │ id_rsa                              0600    2.6 kB  │
      ╰─ ↑↓ select  Enter pick  Esc cancel ─────────────────╯
```

- **不分模式**:打字永遠是過濾、方向鍵永遠是移動。沒有「輸入態 / 清單態」要
  學 —— 跟 form 同一條規則(§4.5:文字輸入 surface 裡字母打字、方向鍵導覽)。
  這是刻意跟 filu finder 的 modal 設計分道:filu 要掃整個 `$HOME`、需要
  「Enter 交清單」當節流點;sshu 只掃 `~/.ssh`,沒有那個需求。
- **fuzzy 比對**:子序列比對,**連續命中**與**落在分隔符後**加權,所以打
  `ided` 會把 `id_ed25519` 排到偶然含這幾個字母的檔案前面。
- **列出權限與大小**:`0600` / `411 B`。**權限被 group / other 讀得到的 key
  用 Red override 標出來** —— ssh 本來就會拒絕這種 key,在「正在挑它」的當下
  講,比連線失敗時才講有用。
- **只掃 `~/.ssh`、不遞迴掃 `$HOME`**:掃整個家目錄會卡住 UI。`~/.ssh` 以外
  的 key 仍可直接在欄位打路徑 —— 選擇器是捷徑、不是唯一入口。
- **`~/.ssh` 不存在**時 picker 仍會開,但直說 `directory not found — type the
  path instead`。**入口不能沒回應**(§A.1 衍生規則),空盒子會被讀成壞掉。
- **上限 2000 檔**,撞到就在清單下方寫明「stopped at 2000 files」——
  不做無聲截斷。
- 選中即寫回欄位,**不再多一次確認**:選這個動作本身就是確認。
- 路徑寫回時**折回 `~` 形式**(`store.FoldHome`),`hosts.yaml` 才跨機器可讀。
  `store.ExpandTilde` 是它的反向。

**為什麼是 `Tab` 不是 `Ctrl+F` / `Alt+F`**:兩個修飾鍵版本都走過。`Ctrl+F` 在
terminal 生態裡太滿(tmux prefix、readline forward-char、pager 搜尋),踩到別人
的鍵會讓使用者以為 app 壞了;`Alt+F` 則要求終端機把 Option 當 Meta 送出,沒設
的人**根本按不到**(跟 `Alt+Esc` 同一個依賴)。`Tab` 兩個問題都沒有,而且
**對著路徑按 Tab** 本來就是 shell 使用者最熟的那個動作。

(v1.2.0 起裸的 `F` 是 file transfer tab 的鍵。它們不衝突:form 是浮層,而 tab
鍵在浮層開著時整組不作用 —— 一個 form 底下的 tab 換掉,form 就懸在一個它不認識
的 surface 上了。見 §11.21。)

form 裡所有 Alt 組合仍然**一律吞掉、不當字元** —— 否則 `Alt+x` 會把 `x` 打進
欄位(`TestTabBrowsesOnlyOnThePathField` 一併釘住)。

**stack**:picker 疊在 form 上(layer +1),form 留在底下 —— `Esc` 取消選檔
會回到那張還沒填完的 form,不是掉回面板(§6.4)。

### 6.4 Connect 確認(message)

`Enter` 對 cursor 卡按下 → 先跳確認、確認後才切到 `[3] ssh` 開 session:

```
            ╭─ ◆ Connect ─────────────────────────╮
            │                                     │
            │  Connect to prod-web-01?            │
            │  deploy@10.0.3.14:22  ·  privatekey │
            │                                     │
            ╰─ Enter connect   Esc cancel ────────╯
```

`Esc` = 取消、留在 hosts。`Enter` = 開 session。

### 6.5 Delete 確認(message)

```
            ╭─ ◆ Confirm ─────────────────╮
            │                             │
            │  Delete host "prod-web-01"? │
            │  This rewrites hosts.yaml.  │
            │                             │
            ╰─ Enter delete   Esc cancel ─╯
```

第一行走 Red override(§2.4)—— 這是不可逆的寫入。

### 6.6 `?` help(viewport)

```
      ╭─ ◆ Help ─────────────────────────╮
      │ Core keys                        │
      │  Tab · 1-3  switch tab           │
      │  Enter      confirm / connect    │
      │  Esc        close popup / cancel │
      │  Space      what can I do here   │
      │  ?          this help            │
      │ Global                           │
      │  q          quit                 │
      │  Ctrl+C     force quit           │
      │ Navigate                         │
      │  h j k l    move cursor          │
      │  gg · G     first / last host    │
      ╰─ Esc close ──────────────────────╯
```

清單超過畫面高度時 hint 變成 ` j/k scroll   Esc close `、`j`/`k` 捲動。

### 6.7 開關動畫 / border 色 / 取消鍵

- 每個 popup 有自己的 `popupAnimator`(name 不重複、避免 tick 互撞,§6.2)
- border 色 = `popupLayerColor(layer)`,不 hardcode(§6.3)
- `Esc` 通殺任何可見浮層,含 auto-dismiss toast(§6.5)
- popup 疊 popup 預設保留 source(§6.4)—— 例:Space menu 開 Edit form,
  form 是 layer 2、Space menu 留在底下,`Esc` 退回 Space menu

---

## §7. 時間軸 UX

### 7.1 Connect 確認後 → **清除 source**(§7.1 context-shift 例外)

通用 §7.1 預設保留 source,但 **ssh session 是長時 target**:使用者從
session 出來時注意力早已轉移,底下浮著的 Space menu / confirm 只會恍神。
所以 Connect 確認 `Enter` 之後:

```
confirm popup 收掉 → Space menu(若有)收掉 → 切 tab 到 [3] → 開 session
```

同一判準也適用 sftp 傳輸:`[t]ransfer` / `[T]ransfer all marks` 一旦確定就把
整疊浮層收掉(`closeStack`),回到 panel —— 送出就是這趟差事的結束。

**而且是立刻收掉,不是等動畫跑完。** 浮層的關閉動畫是視覺,不是模態狀態:動作
一旦 commit,鍵盤就已經回到 panel 手上。原本 `popupOpen()` 認的是「還在畫面
上」,於是 commit 之後那 128ms 內的下一個鍵會被一個正在退場的浮層吃掉 —— 從
Space menu 選了 Search、緊接著按 `Esc` 想退出搜尋,第一下沒有反應。
`popupAnimator.owns()` 現在把 closing 排除在外;opening 仍然算數,因為那是
刻意的(鍵不該落在畫到一半的表面上)。

對照:**Delete 確認**是短時 confirm、**保留 source**(從 Space menu 進來的,
刪完退回 Space menu)。

### 7.1.1 session 的一生

> **v0.2**:單一 `[5]` 已成**終端網格** —— 任意數量的 session 可同時各佔
> 一格;本節的 `[4]`/`[5]` 編號在新鍵位下是 `[1]` 與網格。一生的形狀
> (連線中 → 說話 → 結束讀最後畫面)不變。見 §11.六。

| 事件 | 發生什麼 |
|---|---|
| `[1]` 上 `Enter` → 確認 | 開新 session、切到 tab [3]、focus 直接進 `[5]` |
| `[4]` 上 `Enter`(任何 session) | **不確認**,`[5]` 換到它 + resize + focus 進去 |
| `[4]` 上 `C` | 確認 → Close,下一個 tick 收掉並記進 app log |
| `[4]` 上 `d` | 確認 → **Duplicate**:對同一台 host 再開一條,不必回 `[1]`;**焦點留在清單上**,游標落在新的那一條(§11.23) |
| ssh 自己結束 / 斷線 | 記進 app log 帶著**遠端自己說的那句話**;失敗時 `[5]` 顯示它並留著;同時跳 error toast |
| `!` app log | **不能選取,只能捲動** —— 它是視圖,不是清單(§7.1.5) |
| `q` 時還有活 session | 紅字確認,列出會被關掉幾條 |

**移動游標不會切 `[5]`。** 切 session 要 resize + 讓遠端重畫,把它綁在游標上
等於瀏覽清單就一直打斷遠端。所以「右邊現在是誰」由 **glyph + 綠色**回答,
換人是明確的 `Enter`。

**`Enter` 不問。** 切換不開任何連線也不關任何連線 —— 離開的那個照常跑,在另一
列再按一次 `Enter` 就回去了。對一個沒有代價、又能自己撤銷的動作跳確認,只是擋
在路上的一次按鍵。會問的是有副作用的那幾個:`[1]` 的連線、`C` 的中斷、
`q` 的離開。

**`[5]` 的 resize 只打給當前顯示的 session。** 背景 session 維持它啟動時的
幾何,直到被切到前景 —— 跟 terminal multiplexer 同樣的取捨:對沒人在看的遠端
送 SIGWINCH 只是讓它白重畫一次。

**結束的 session 立刻離開 `[5]`。** 不停在最後一屏 —— 那一屏長得跟活著的
prompt 一模一樣,停在那裡只會讓人以為還連著、對著死掉的 terminal 打字。
`[5]` 回到「Select a session in [4]」,而且 emulator 一併釋放:一整個終端機
grid 留給沒有東西會 render 它的資料,是白佔記憶體。

**`[5]` 被 focus 時 `[4]` 收起來、`[5]` 佔滿整個 tab。** 遠端拿著鍵盤的
時候那兩個清單本來就碰不到,留在畫面上等於拿四分之一的寬度換一個你按不到的
東西。`Alt+Esc` 出來時它們自動回來 —— 兩個方向都會重新 resize 遠端,否則遠端
會照著錯的幾何畫。

**scrollback 是自己攢的,不是 emulator 給的。** vt10x 只有「當前這一屏」,
離開頂端的列被它 `clear()` 掉。所以每一塊從 PTY 讀進來的 bytes 在寫進 emulator
的同時也被切成行、存進一個 10000 行的 ring,`PgUp`/`PgDown` 捲的是那個 ring
(§11.19)。上限之外、以及 alt screen 期間的畫面,仍然只有遠端自己的 `tmux` /
`less` 找得回來 —— 這裡不假裝有完整 log。

#### `[4]` 的列格式

```
<space><user>@<host>...<port>
```

**列說的是這條連線「是什麼」,不是它「叫什麼」。** 兩台存起來的 host 可以指向同一
台機器,而名字是人自己取的標籤;`deploy@10.0.3.14` 才是 ssh 實際做的那件事。

**port 永遠完整顯示**,位址撞到它就折行。理由:同一台 host 可以開多個 session,
而截掉一半的 port 是一個你無法據以辨認的欄位。所以 port 佔右邊固定 6 格
(1 格間距 + `65535`),位址在剩下的空間裡折。

ordinal(`#1` / `#2`)跟著位址走、不佔獨立版位:它是**唯一**能分開「同一台 host
的兩條 session」的東西,位址和 port 都做不到。

`#N` **跟著名字走、不另外佔版位** —— 它是唯一能分辨「同 host 的兩個 session」
的東西(port 分不出來,兩個 session 的 port 一樣),所以它必須跟名字一起折行,
而不是被擠到某個固定欄位裡。

> **代價**:左欄 26 欄扣掉 glyph 版位與 port 版位,名字只剩 **15 格**,長名字
> 會折成三列。要換的話把 `sshLeftW` 調到 30 就有 19 格。
>
> **v1.2.0:換了,而且連折行本身一起換掉。** `sshLeftW` 是 30;更重要的是位址
> 現在**縮減而不折行**,所以「折成三列」這件事不再會發生 —— 見 §11.28。
>
> **v1.3.0:名字拿回了自己的一行。** 一個 item 現在是兩行,第一行就是名字
> (截而不折),所以「名字只剩 15 格」這個代價整個消失了 —— 見 §11.32。

#### `[4]` 的一列:`<user>@<host>` + port

**列說的是這條連線「是什麼」,不是它「叫什麼」。** 兩台存起來的 host 可以指向同一
台機器,而名字是人自己取的標籤;`deploy@10.0.3.14` 才是 ssh 實際做的那件事。port
在名字列的右端 —— 那裡本來就是空的 —— 而且**永不截斷**,位址不夠就先折。

ordinal(`#1` / `#2`)跟著位址走、不佔獨立版位:它是**唯一**能分開「同一台 host
的兩條 session」的東西,位址和 port 都做不到。

#### 顏色是兩條獨立的通道

| 通道 | 說什麼 |
|---|---|
| **前景** | 綠 = 這條就是 `[5]` 正在顯示的 |
| **背景** | 游標 bar |

因為是不同通道,兩者不爭用,**沒有特例**。這同時拿掉了兩樣東西:原本「游標壓在
正在顯示那一列時整列反白成綠底」的 inverse,以及那個**重複表達同一件事**的終端機
glyph —— 顏色自己就說得完,那個 glyph 是第二次說。

兩者真的相遇時(游標就壓在那一列),**bar 贏,那一列的綠看不到**。這是可以接受的
代價:游標就在上面,而旁邊 `[5]` 的標題正寫著那條 session 的名字。

> **改過一次**:原本 inverse 是為了「一列只能有一個背景」而設計的妥協。把
> on-screen 換成純前景之後,那個前提就不存在了 —— 兩個訊號本來就不必搶同一個通道。

#### 結束原因用顏色 —— 另一個問題

`[4]` 回答「**哪一個正顯示在 `[5]`**」,app log 回答「**每一件事是怎麼結束的**」。
兩個問題各佔一個通道:

| 面 | 訊號 | 內容 |
|---|---|---|
| `[4]` | **前景色** | 綠 = 這個 session 正顯示於 `[5]`(原本另有一個 glyph 說同一件事,已移除) |
| app log | **顏色,只上在 level 那一段字** | `ERR` 上警示色;`INFO` 保持 dim |

log 的顏色**只染 level、不染時間、更不染整列背景** —— 「這是一則錯誤」是那一則的
屬性,整列上色會把話講得太滿,而一份紀錄裡多數行都不是錯誤。

#### app log 是視圖,不是清單

`[6]` **沒有游標、沒有可執行的動作**,`j`/`k` 捲的是視圖而不是選取。它回答
「發生過什麼」,不是「要對哪一筆做事」。

代價說清楚:原本掛在 `[6]` 上的 **Reconnect 與 Remove 一起沒了**。重新連線改從
`[1] hosts` 走(host 記錄本來就在那裡);app log 沒有手動清除,滿 500 筆自然
汰舊。`Space` 在 `[6]` 仍然會回應 —— 它直說這裡是視圖、並指向 `[1]`,因為
入口不能按下去沒反應(§A.1 衍生規則)。

**綠色不能同時是這兩件事**(§B)。既然 `[4]` 的訊號已經交給 glyph,綠色就完整
讓給 `[6]` 的「乾淨結束」。`[6]` 也不會有 on-screen glyph —— 它裡面沒有活的
session,那個標記在那裡沒有意義。

#### 列的四種著色 —— 游標 bar 與列本身的顏色會合併

一列只有一個背景。與其讓游標 bar 蓋掉「這是正在顯示的那一個」,兩個訊號**合併**:
**bar 直接吃掉那列本來要用的前景色**。

| 情況 | 呈現 |
|---|---|
| 游標 **且** 正在顯示 | **綠色背景** + 無色文字(反色) |
| 游標 | `handColor` 背景 + 無色文字 |
| 正在顯示 | glyph 與 name **綠色前景** |
| 其他 | 一般文字色 |

**反色只用在 `[4]`**。`[6]` 的游標就是一般的 `handColor` bar —— 壓上去時
reason 的顏色會被蓋掉,但字還在、還讀得到,不需要為此把整列變成綠底或紅底。

port 不吃這套色 —— 它在非游標列一律 `dimColor`。使用者要的是「glyph 和 name
用綠色」,port 是次要欄位、跟卡片時代一樣退一階。

`TestSessionRowColourCases` 把四種情況都釘住,特別是第四種(游標壓在正在顯示
的列上時,不能只剩普通 bar)。

### 7.1.2 session 完全不落地

> **v0.2**:**已反轉** —— 失敗連線的完整畫面現在寫進 applogs.yaml(使用者
> 裁定「全部落地」),0600 + 警告標頭 + 自我修剪。理由與緩解記在 §11.四;
> 本節保留當時的推理。

session 與 app log **只活在記憶體裡**,關掉 sshu 就沒了。沒有 `history.yaml`。

理由不只是省事:最後一屏可能含遠端印出的任何東西(token、金鑰、客戶資料),
把它寫進磁碟等於憑空造出一個新的外洩面,而 vt10x 本來就只有一屏、稱不上
「log」。想要真正的連線記錄,那是遠端 / `tmux` 的工作,不是 sshu 的。

app log 上限 **500 行**。session 結束時 emulator 就被放掉(整個 grid,留給沒有人畫
的東西),所以那個上限是關於文字、不是關於記憶體 —— 讀走最後一行、記進 log、grid 走
人。
超過砍最舊的並釋放它的 pty。

### 7.1.3 遠端的寬字元不能撞破邊框

vt10x 一個 rune 算一格,但終端機把 emoji 與 CJK 畫成**兩格**。所以遠端 prompt
裡有一個 🌐,那一行實際渲染出來就比 grid 說的寬,**把 `[5]` 的右邊框推出畫面**。

處理:`ptyTerm.render` 每一行都先 `clipANSI` 再補齊。代價是這種行會被切掉最後
一兩欄;不切的代價是整個框壞掉。`TestWideRemoteOutputCannotBreakTheFrame` 用真
的 pty 印 emoji 來釘住(拿掉 clip 就會量到 92 欄的終端機出現 94 欄的行)。

### 7.1.4 history:先從 panel 變成 popup,再整個變成 app log

**第一次(panel → popup)**:`[6]` 不能被操作、大部分時間是空的,卻永久佔掉左欄
三分之一。開了四五條 session 的時候,擠的是還在用的那個清單。真正有價值的從來不是
那個 panel,而是「**哪一條斷了、為什麼**」。

**第二次(popup → app log)**:`[H]istory` 答的問題其實是「它們怎麼結束的」,而那
個問題**不是 tab [3] 專屬的** —— 傳輸失敗、寫回失敗、host key 被拒是同一種消息,而
它們原本唯一的去處是一個兩秒就消失的 toast。所以清單本身也拿掉了,留下的是理由,
放進一份全 app 的紀錄(§7.1.5)。

兩次是同一個判斷推到底:**要留下的一直是原因,不是那份清單**。

| | 管道 |
|---|---|
| 常駐 | tab 列右側 `3 live sessions` |
| 出事當下 | **error toast**:`prod-web-01 · ssh: connect to ... Connection refused` |
| 事後 | **`!` app log**,footer 報未讀錯誤數 |
| 失敗那一刻的 `[5]` | 顯示原因並**留著**,不是變回空框 |

**乾淨離開不出聲**:`exited 0` 是你打 `exit` 要的結果 —— 它只進 log,不跳 toast。

### 7.1.5 app log —— 一則消息只出現兩秒,等於沒出現過

> **v0.2**:`!` popup 已移除,app log 變成 preference → logs 的**內容
> panel**,並且**落地到 applogs.yaml**(反轉 §7.1.2「session 完全不落地」——
> 使用者裁定,代價與緩解見 §11.四)。「看到即已讀」:logs 區上了畫面,未讀
> 計數就歸零;在那之前 nav 列與 footer 都掛著數字。

原本是 `[H]istory`:一份「結束過的 session」清單。它答的問題其實不是「有哪些
session」,而是「**它們怎麼結束的**」—— 而那個問題不是 tab [3] 專屬的。傳輸失敗、
寫回失敗、host key 被拒,是同一種消息,而它們原本唯一的去處是一個兩秒就消失的 toast。

所以 history 換成 **app log**:`!` 開、`!` 關(跟 `?` 同一個約定),viewport 類
—— 最新的在最上面、沒有游標、裡面沒有東西可以被選取或操作。

**toast 和 log 是兩個職務,不是兩個選項。** toast 是「剛剛發生了」,而它會消失是它
的功能不是缺陷;log 是「後來你想再看一次」。session 死掉原本只有前者,那表示**移開
視線一下,等於從來沒被通知過**。

**footer 會說有幾則沒讀過的錯誤**:平常是 `! log`,有未讀時變成 `! 2 errors`。一把
沒人按的鍵和一份沒人開的紀錄是同一回事,所以那個數字是這條紀錄唯一的入口。

**log 裡的訊息是折行、不是截斷。** 那些是別人機器寫的錯誤訊息,而它們把原因放在
**句尾** —— `…port 22: Connection refused`。截掉尾巴,等於截掉唯一有人想讀的那個字。

**一則事件記的是整個畫面,不是最後一行。** 連線被拒是一行,但 host key 不符是十五行
—— banner、指紋、`known_hosts` 的第幾行 —— 而**最後一行只是
`Host key verification failed.`**,那正是唯一沒有告訴你任何新東西的一行。你需要的
指紋在中間。

所以兩個份量各給各的:

| | 內容 | 為什麼 |
|---|---|---|
| toast / `[5]` 標題 | **最後一行** | 那是一行字塞得下的量 |
| app log | **整個最終畫面** | 那是你事後真的要讀的東西 |

兩者都在 reaper 丟掉 emulator **之前**讀走,因為再一個 tick 就沒有了。

上限是**每則 40 行 / 4000 字**(kbu 也對每則設上限,只是它的每則是單行)。一台決定
印一 MB 出來的遠端,不能把整份紀錄一起帶走。

`top` 數的是**畫出來的列**而不是「第幾則」:一則可以是四十行,用「則」捲動會變成
看得到開頭、永遠到不了結尾。

#### 連不上的時候要說**它說了什麼**,不是「disconnected」

`exitReason` 只看得到 exit code,而 ssh 的 255 對「連線被拒」「金鑰不對」「host key
變了」是同一個數字。真正有用的那句話 ssh **印在畫面上**,而 reaper 在下一個 tick 就
把 emulator 丟掉了 —— 那句話比它出現得還快地被扔掉。

所以 reaper 在丟掉 emulator 之前先讀走最後一行非空白的內容(`ptyTerm.lastWords`),
只在**不是乾淨結束**的時候拿它取代 reason。`[5]` 接著用它:

```
        nobody@127.0.0.1 · ssh: connect to host 127.0.0.1 port 1: Connection refused
                    Press ! for the app log, or [1] to try another host
```

**而且它會留在那裡**,直到有別的東西接管那個 panel。一個兩秒後把自己擦掉的錯誤訊息
是一個讀不到的錯誤訊息。

#### 連線中不接受打字

`[5]` 有 focus,但**還沒接通的時候按鍵不會送出去**。ssh 在等連線時根本沒有在讀
stdin,所以那些 bytes 會留在緩衝區,等連上之後**才送進遠端的 shell** —— 一個原本要
給 sshu 的 `q`,幾分鐘後在別人的機器上執行。

所以 `inPty`(鍵盤屬於某個**遠端**)跟 `ptyFocused`(panel 拿著鍵盤)分家了:前者
多要求一個條件,就是對面已經說過話。中間那段時間按鍵被**吃掉**,而不是被轉送,也不
是落回 panel 去觸發 `q` 離開 —— panel 確實拿著鍵盤,只是還沒有人可以聽。出口是
footer 一直在講的那個 `Alt+Esc`。

> **代價,講清楚**:判準是「對面有沒有送出過 byte」,所以一台**接通了但完全不說話**
> 的遠端會被顯示成「連線中」,而且在它說話之前不能對它打字。實務上 ssh 一接上就會有
> prompt 或 banner,但這個代價是真的;出口一樣是 `Alt+Esc`。

#### 連線有預算,而且**讓 ssh 自己去超時**

「連線中」不能是一個沒有盡頭的狀態。預算放在 `config.yaml` 的 `connect_timeout`
(秒,預設 15 —— 也就是這件事可設定之前兩個 tab 各自寫死的那個常數),tab [2] 拿它
當 SSH client 的 dial timeout,tab [3] 把它交給 ssh 當 `-o ConnectTimeout`。

**交給 ssh 而不是自己動手,是因為 ssh 會「講」。** 它自己超時的時候會印
`Operation timed out` 然後離開,那句話走的是每一種失敗都在走的同一條路(§7.1.4:
最後畫面 → reason + app log)。sshu 自己把行程殺掉,只會得到一具沒有附說明的屍體。

實測:`10.255.255.1` 原本會轉大約 75 秒(那是作業系統的 TCP 逾時),設成 3 之後三秒
就結束,而且畫面上寫的是 ssh 的原話。

**但 ssh 的 `ConnectTimeout` 只管 TCP 那一段。** 一台接了連線之後就不說話的主機
——、DNS 卡住、server accept 了卻不送 banner —— 從外面看跟「還在連」一模一樣。所以
另外有一個兜底:一條**完全沒說過話**的 session 超過「預算 + 5 秒寬限」就被停掉,
reason 寫成 `no answer after Ns`。

那 5 秒寬限就是兩者的分工:**先讓 ssh 用自己的話去超時,sshu 才伸手拔插頭**。

**`config.yaml` 對 sshu 是唯讀的。** 沒有任何 UI 會寫回它,所以手改過的檔案不會被
重排、不會被重新格式化,寫在裡面的註解也活得下來。檔案不存在不是錯誤:每個設定的
預設值就是「這個設定存在之前 sshu 的行為」,所以那個檔只需要寫你想改的那幾行。

**壞掉的設定檔會被講出來,但不會擋著不讓你開。** 解析失敗就用預設值繼續跑,並把
錯誤丟進 app log —— alt screen 之後 stderr 是看不見的,而「有人寫了東西而它沒有生
效」是一定要讓那個人知道的事。

**超出範圍的數字當打錯處理**(1–600 秒以外一律回到預設)。`connect_timeout: 0` 照字
面執行的話,是一個永遠連不上又不說為什麼的 tab。

### 7.2 hosts.yaml 外部變更

`(planned)` 用 fsnotify 監看 `hosts.yaml`,外部編輯後即時 reload
(沿用 filu 的做法)。v1 可先只在啟動時讀、存檔時寫。

### 7.3 tab [2] 的一次搜尋

`/` 搜的是**整棵子樹**,不是螢幕上這一層。

```
 ([1] hosts)  ([2] sftp)  ([3] ssh)                                   2 marks
──────────────────────────────────────────────────────────────────────────────
╭([4] local)──────────────────────────╮╭([6] prod-web-01)────────────────────╮
│ ⌕ /dep_                   3 of 840 …││ /srv/www/releases                   │
│    Documents/sideproj/app          -││    2026-08-30                      -│
│    Documents/…/deploy.sh       1.2 K││    2026-08-29                      -│
│    backups/deploy-old.sh       3.1 K││    current                         -│
│                                     ││                                     │
╰─────────────────────────────────────╯╰─────────────────────────────────────╯
```

**畫在原地,不開 popup。** 結果就是 `[4]`/`[6]` 裡的普通一列 —— `a` 標記它、
`t` 傳它、`Enter` 進它所在的目錄,同一批鍵做同一件事,只是這一列剛好來自三層
底下。做成 finder popup 的話,得先發明一個「reveal」步驟,再把使用者送回 panel
去做他本來就要做的那件事。

實作上,搜尋中一列的 `Name` 是**相對於 cwd 的路徑**,所以 `Join(cwd, Name)` 仍
然是真正的絕對路徑 —— mark / transfer / enter 全都不需要知道發生過搜尋
(`sftpSideModel.rowAt`)。

**代價是游標跟結果活在同一個清單裡**,而結果是串流進來的。兩條規則從這裡長出來:

- **空 query = 當前目錄**,不是「底下全部」。按了 `/` 還沒打字,清單必須跟原本
  一模一樣;有東西要找,子樹才浮出來。
- **到達順序就是順序,不重排。** filu 的 finder 依 fuzzy score 排,但它的游標在
  打字期間是停著的(要 `Enter` 才進 nav mode);sshu 的游標全程活著,每來一批就
  重排等於把使用者手底下那一列抽掉。廣度優先本身就是有意義的排序:近的先到,而
  近的通常就是要找的那個。

**廣度優先是對「等待」的承諾**,不是整齊考量。SFTP 每個目錄是一次 round trip,
結果抵達的順序就是使用者等待的順序 —— 深度優先會把最初幾秒花在剛好排最前面的
那棵子樹裡,從外面看跟卡住沒兩樣(`remote.Scan`)。走訪先把當前目錄的 entries
直接當成第一批結果(它們已經在手上了),walk 從下一層開始,所以按下 `/` 到畫面
有東西之間沒有 round trip,也不會有東西被列兩次。

**上限 20000 筆**,與 `planCap` 同一個精神:超過這個數量,搜尋就不是你要的工具。
停在上限時 query 列右端會說 `capped` —— 截斷了卻宣稱完整,比慢還糟。

**query 列的提示符是搜尋 glyph,不是一個 `/`。** 開搜尋的鍵是 `/`,但把它原樣
回顯當提示符,會讓**含斜線的 query 讀不出來** —— 搜 `/tmp` 會畫成 `//tmp`,沒有
辦法分辨哪個斜線是自己打的。glyph 說的是同一件事,而且不可能跟輸入撞在一起。

**query 列右端是 `<符合> of <已看到>`**,還在走時加 `…`。兩半都要:數字不再往上
跳代表走完了,而不是卡住。位置不夠時**整段丟掉、不切一半** —— `12 of 840` 切成
`12 of 8` 不是縮短,是另一個數字。

**`Esc` 先退搜尋,再退目錄。** 這是 filu 的兩段式 `Esc` 多長出來的一段:搜尋是
比目錄更內層的東西,所以它先被剝掉。第二下 `Esc` 才上一層。

**離開搜尋要真的停下來。** `Esc` / 換目錄 / 換 host 都會 cancel 那個 context,
不然「看起來閒著」的 panel 底下還在打那條連線(`searchScan.stop`)。

**沒做**:遠端的內容搜尋(filu `f` 的 rg)。那要在對面跑一個 grep,是「在遠端
執行指令」而不是「列目錄」,超出 SFTP 這條路徑該有的授權範圍。


#### `Enter` 是搜尋結果唯一到得了 panel 的鍵,所以它就是「去那裡」

搜尋中每一個可見字元都進 query(§4.5)—— 這是對的,不然打不出含 `a` 的檔名。
但它有一個沒被看見的後果:**找到的檔案沒有任何一個鍵動得了它**。`a`、`t`、`v`、
`e`、`x` 全部變成打字,而 `Esc` 把整批結果丟掉、游標回到當前目錄第一列。搜尋能告訴
你東西在哪,然後要你自己走過去 —— 那正是它本來要省掉的事。

`Enter` 是唯一不會被吞掉的鍵,所以它承擔這件事:**到那個東西所在的地方去,並把游標
停在它上面**,順手退出搜尋。之後 `a`/`t`/`v`/`e`/`x` 全部是普通的列上動作,不需要
新詞彙 —— 這正是「結果就是普通的列」原本承諾的那句話,現在才是真的。

目錄結果本來就會被 `Enter` 打開;現在它也會一起退出搜尋,而不是留著一個已經不描述
眼前這份清單的 query。

> **這是實機 dogfood 才抓到的**。所有測試都綠,四份文件都寫著「結果是普通的列,
> 所以 `a` / `t` / `x` 照樣能用」,而那句話是假的 —— 沒有任何一個測試去按過那些鍵。

### 7.3.1 目錄怎麼保持最新 —— SFTP 沒有 watch

**協定裡沒有變更通知**,所以沒有東西可以訂閱。剩下的選擇是:在對面跑
`inotifywait`(需要對方裝了那個工具,而且那是「在遠端執行指令」,這個 tab 刻意
不做),或者自己問。

sshu 自己問,但問**便宜的那一題**。每秒重列一次是真的有成本 —— `ReadDir` 會把
每一筆的屬性都帶回來,大目錄配慢線路就是一個大部分時間閒著的 tab 在持續佔用頻
寬。所以每一拍(2 秒)只 **stat 那個目錄、比對 mtime**,那是一次很小的往返;
**只有 mtime 動了才重列**。

- **買到的**:新增、刪除、改名會自己出現。
- **買不到的**:原地改寫一個檔案不會動到目錄的 mtime,所以一個持續長大的
  log 會維持舊的 size,直到有別的事觸發重列。離開目錄再回來就是手動 refresh。
- **時間戳要在列目錄「之前」取**(`dirMTime` 的註解):兩者之間發生的變更,這樣
  會表現成一次多餘的刷新 —— 那是無害的方向。反過來取會把新的時間戳配上舊的清
  單,那次變更就**永遠不會被發現**。

兩半都是網路呼叫,所以跑在 goroutine 上、用訊息送回來。背景刷新永遠不該讓畫面
等。而且**沒人在看的 tab 不會問** —— 迴圈只在 tab [2] 在畫面上時活著。

**游標跟著「同一個檔案」走,不是同一列**(`applyWatch`)。這不是講究:上方多出
一個檔案就會把游標滑到別的名字上,而使用者沒有碰過任何鍵 —— 下一個 `t` 就傳錯
東西了。

### 7.3.2 改名與刪除 —— 唯一會破壞資料的地方

**`[R]ename` 就地改名**,不是搬移。輸入框**預填舊名字**:多數改名是改名字的一
部分,從空白開始等於每次都要重打一遍。含 `/` 的名字被擋掉 —— 那是搬移,而搬移有
一整個 tab 在做。

**目的地存在就拒絕,不覆寫。** 這一條要自己做,因為兩端的行為不一樣:`os.Rename`
會直接蓋掉,SFTP 的 `Rename` 會拒絕。**同一個動作不該因為落在哪一端而不同**,所
以先 `Exists` 再動手,兩邊都拒絕。

**改名會帶著 mark 走**:mark 是一條路徑,改完名不動 mark 的話,那個 mark 就指著
一個已經不存在的東西。

**`[A]dd` 建在「正在瀏覽的那個目錄」裡**,是 panel 動作不是 item 動作 —— 游標在哪
跟它建在哪無關。建完游標會**停在新的那一項上**:新增幾乎都是「拿它做點什麼」的前
半段,在長清單裡再找一次它是沒人要求的後半段。三種拒絕跟改名一樣(空的、含 `/`、
名字已存在),因為那是同樣三種「說出跟你想的不一樣的東西」的方式。檔案或目錄由結
尾那條斜線決定,理由見 §7.3.5。

**`[x]` 刪游標這一項,`[X]` 刪這一側全部 marks** —— 跟 `[t]ransfer` /
`[T]ransfer all marks` 同一個大小寫分法。兩者都先問,而且問句都說**哪一個 / 幾個,
在哪一台**。刪掉的東西如果是被 mark 的,mark 會一起拿掉:指著已經不存在的路徑的
mark,會在稍後某個看不到原因的地方失敗。

**為什麼是 `x` 不是 `d`。** `d` 是半頁下捲。刪除放在 `d` 上試過一輪,兩個代價:
tab [2] 失去裸的半頁鍵,而且更糟的是,它讓**刪除的作用範圍**與**傳輸的作用範圍**
用了兩套不同的機制。

**被否決的替代方案:讓 panel 決定範圍**(`[4]`/`[6]` 的 `D` = 這一項,
`[5]`/`[7]` 的 `D` = 全部 marks)。它在 marks panel 上壞掉:那個 panel **有自己
的游標**,所以它沒辦法代表「全部」而不同時失去「只刪其中一個」的能力 —— 而一個從
子樹搜尋標記來的 mark,回到檔案清單根本看不到,沒有第二條路可以刪它。同一個
panel 上 `[U]nmark` 是「這一個」、`[C]lear marks` 是「全部」,再讓 `D` 變成「全部」
會跟這組既有的分法打架,而且畫面上沒有任何東西說得出差別。

**`[x]` 與 `[X]` 都會破壞資料**,所以:

- **先問**,而且問句要說**數量與哪一台**(`2 marked items on local.`)—— 兩邊長
  得很像,只說「2 個檔案」不夠。
- **遞迴刪除用 `Lstat`,不是 `Stat`**(`remote.RemoveAll`)。symlink 在 `Stat`
  底下看起來就是一個目錄,照著走下去會把**別人只是連結過去**的目錄清空。走訪一
  律用 lstat 語意:連結本身被 unlink,它指向的東西不動。這是這個 app 能製造出來
  最糟的意外,所以它有自己的測試。
- **失敗不中斷**:刪得掉的先刪掉,回報第一個錯誤。半途停下會讓清單顯示的東西和
  實際剩下的對不起來。

**`[D]elete marks` 與 `[C]lear marks` 是鄰居**,所以它們的 hint 用唯一重要的說法
把差別講明:一個**抹掉檔案**,一個只是**忘記你挑過哪些**。原本後者叫
`[R]eset marks` —— 跟 `Delete marks` 只差一個字,對一個不可逆的動作來說,那個距
離不夠。(`R` 現在是 Rename。)

### 7.3.3 `[v]iew` —— filu 的 preview,搬到 popup 裡

`[v]` 把游標那一項讀出來看。這是傳輸前你真正會問的那個問題:**這是不是我以為的
那個檔案**。

**filu 有的東西不是全部搬得過來,而且不是懶得搬:**

| filu | sshu | 為什麼 |
|---|---|---|
| 文字 + 語法上色 | ✅ | 唯一乾淨過得來的那個,連 chroma 與 catppuccin-mocha 一起搬 |
| 二進位 hex | ✅ | 認出「這是什麼」需要的資訊量很小 |
| 目錄樹 | ✅ 但**只有一層** | 每一層都是一次 round trip;「這裡面有什麼」第一層就答完了 |
| 圖片 | ❌ | 要整份下載,而且要一個這個 app 不會說的終端機協定 |
| 壓縮檔內容 | ❌ | 要把整個壓縮檔抓下來 |
| PDF 文字 | ❌ | 要抓下來,還要連一個 parser |

**上限 64 KiB**(`remote.PeekCap`)。這裡的上限不是記憶體考量而是**傳輸預算** ——
遠端那一側每一個 byte 都要過網路。64 KiB 足以回答「這是不是那個檔案」,又小到讓
「在一個好幾 GB 的 log 上按到 v」不是一個你得等完的錯誤。

**讀取與上色都不在 update loop 上。** 讀是因為它過網路;上色是因為 64 KiB 過一遍
lexer 不是免費的。畫面不該等其中任何一個。

**popup 先開、內容後到。** 跟連線的 spinner 同一個教訓:一個要等 bytes 到了才有反應
的鍵,看起來就是一個沒反應的鍵。

**ESC 一定要被吃掉**(`sanitizeLine`)。這一條在 sshu 比在 filu 更重要:那些 bytes
是從**別人的機器**上來的。一個含有跳脫序列的檔案,不處理的話可以重畫這個 popup、
重畫它後面的 panel,或者重畫整個終端機。tab 換成空白、CR 丟掉、其他控制字元換成
空白,單行長度也設上限 —— 一行病態的長行不該讓寬度計算變貴。

**它是 viewport,不是清單**:沒有游標、不能選,`j`/`k`/`u`/`d` 捲動而且不繞(§4.2)。

### 7.3.4 `[e]dit` —— 沒有 `sftp edit`,那支舞就是 sshu 自己跳

kbu 的 edit 是 `kubectl edit` 跑在 embedded PTY 裡:抓下來、開編輯器、送回去,
整套 dance 是 kubectl 做的,kbu 只負責設 `KUBE_EDITOR` 跟把它放進 PTY。SFTP 沒有
這個代打,所以這裡是 sshu 自己的責任:

```
remote   抓成暫存檔  ->  $EDITOR  ->  內容變了嗎  ->  寫回去
local    $EDITOR,對著真的那個檔
```

**local 那條不是最佳化。** 就地編輯保住 inode —— hard link、擁有者、xattr 都還在。
複製出去再 rename 回來會把這三個都弄斷,而且是為了防一個根本沒參與的網路。

**沒有大小上限,而且那是想清楚的結果。** edit 要的不變量是「絕不寫回一份讀不完整
的內容」,那是靠「整份讀完、否則失敗」買到的,不是靠拒絕大檔。它串流,所以大小的
代價是時間和磁碟、不是記憶體,而時間可以取消。拿「拒絕」去解一個「等待」的問題,
代價會是你真的想編那個 8 MB 的 log 時被自己的工具擋在門外。

#### 寫回去:先落在隔壁,再整個蓋過去

`remote.WriteBack` 先寫同目錄的 `.sshu-tmp` 兄弟檔,再一步蓋過目標。斷線斷在寫到
一半,原檔仍然完整 —— 而不是一份被截斷的設定檔,那種檔案下一個讀它的程式看不出來
是半份。

兩個例外,兩個都是刻意的:

| | 做法 | 為什麼 |
|---|---|---|
| symlink | 寫穿過去 | rename 蓋上去會把連結換成普通檔案,你編的那個 dotfile 悄悄不再指向它原本的 repo |
| 目錄不收新檔(你的檔、別人的目錄) | 原地寫 | 另一個選項是根本存不了 |

SFTP 上的「蓋過去」用 OpenSSH 的 `posix-rename@openssh.com`(`FS.Replace`),因為
基本協定的 rename 遇到目標存在會拒絕。**只有在伺服器明說沒有這個 extension 時才
退回 remove-then-rename** —— 把一個權限錯誤誤判成「沒有 extension」,那個 remove
就會變成毀掉檔案的那一步。

#### 三個「先問」

**「你改過嗎」用內容雜湊,不用 mtime。** 編輯器 `:w` 不管有沒有改都會重寫檔案,
mtime 回答的是另一個問題。打開看一看然後 `:q`,不該產生一次寫入。

**編輯期間別人改了那個檔 → 問。** 下載時記 size + mtime,寫回前再 stat 一次。這是
這個功能唯一絕對不能有的結果:默默蓋掉別人的工作。而且不管你怎麼答,**本機那份副
本都留著,並且告訴你它在哪** —— 拒絕覆寫不該是弄丟你剛打的字的那一步。

**看起來不是文字 → 問,不是拒絕。** 判準是「沒有 NUL 且是合法 UTF-8」,而一個
Latin-1 的設定檔過不了它、卻完全可以編輯。硬擋等於拿一個猜測去否決一個人對自己檔
案的判斷。反正只有內容真的變了才會寫回去,開起來 `:q` 什麼都不會發生。

#### 哪一個編輯器,以及怎麼把檔名交給它

順序是 `$VISUAL` → `$EDITOR` → `vi`。**`vi` 是地板不是依賴** —— POSIX 保證它在,
那是它被點名的唯一理由;三個都沒有就直說要設哪個變數,那句話就是整個修法。

`$EDITOR` 是 **shell 語法、不是程式名**:`code -w`、`vim -u NONE`、含空白的路徑都
得能動,所以交給 `sh -c`(git 跑編輯器也是這樣)。但**檔名是位置參數,絕不插進那
段 script** —— 那個名字是從別人的機器上來的,一個叫 `; rm -rf ~` 的檔案必須以引數
的身分抵達,不能變成第二道指令。

**不告訴編輯器外面是哪台終端機。** `TERM_PROGRAM` / `KITTY_*` / `ITERM_*` /
`COLORTERM` / `TERM` 全部剝掉,`TERM` 釘成 `xterm-256color`。編輯器跑在 vt10x
裡,而 vt10x 不回答 DA1 那類查詢;nvim 讀到那些變數會判定這是一台高階終端機、送出
查詢、等一個不會來的回覆,離開時卡住。sshu 沒辦法讓 emulator 回答,那就不要再自稱
是那台會被問的終端機。(這條直接抄 kbu,它跑 `kubectl edit` 用的是同一份剝除清單。)

#### 它在跑的時候,鍵盤整個是它的

跟 panel `[5]` 一樣:Esc 是 vim 的 Esc、`q` 是一個字母、Space 是一個空白、
**`Ctrl+C` 是編輯器自己的中斷鍵**。只留下一個鍵 —— `Alt+Esc` 放棄這次編輯
(tab [3] 那邊它是「把鍵盤拿回來」,這裡沒有別的 panel 可以拿回去,所以拿回來
就是離開,box 底下的 hint 就這樣寫)。

`Ctrl+C` 原本也留著、當作 sshu 到處都是的那個緊急出口,理由是「讓它在這一個
PTY 裡意思不一樣,緊急出口就不再是緊急出口了」。那個推論的方向反了,已經改掉
—— 見 §11.20。

**一次只准一個。** 兩個編輯器用沒人能推理的順序寫回去不是功能。box 開著的時候鍵盤
在它手上,`e` 根本到不了動作表;而它**正在關**的那段動畫裡鍵盤已經還回來了
(`popupAnimator.owns`),`sftpEdit` 裡那句拒絕就是為那個窗口存在的。

### 7.3.5 `[A]dd` —— 一個框做兩件事,結尾那條斜線說是哪一件

原本是 `[N]ew directory`。有了 `[e]dit` 之後,「開一個新檔然後編它」變成一個會反覆
做的動作,而當時要做到它得先去別的地方開檔再回來。

**一個框、兩種答案,差別是結尾那一個字元:**

```
logs      ->  檔案
logs/     ->  目錄
```

這不是新發明的慣例:shell 一直是這樣寫目錄的,這個 app 自己的列表也是這樣畫的。
另一個選項是兩個鍵、兩列 menu,去講同一件事。

**它是大寫,而且那不是筆誤。** §7.3.2 的規則是小寫對游標那一項、大寫對整個 panel,
而「在當前目錄裡新增」跟游標沒有關係 —— 游標停在哪裡都做同一件事。tab `[1]` 的
`[A]dd` 也已經是這個字,所以整個 app 裡「做一個新的」就是同一個鍵。

#### 一個藏在單一字元裡的規則,得說兩次

`/` 這種規則的風險是它是隱形的。所以它在兩個地方現身,而且分工不同:

1. **menu 的 hint 欄**:`[A]dd  ·  a file, or name/ for a directory` —— 這是你**學到**
   它的地方(§4.4:hint 就是這個 surface 能做什麼的常設揭露)。
2. **空框裡的 placeholder**:`name, or name/ for a directory` —— 直接長在你要打字的
   那一行上,打第一個字就消失。

而 **Enter 的動詞會跟著你打的字改**:空的時候是 `create`,打了 `logs` 變成
`create file`,補上 `/` 變成 `create directory`。這一條才是真正的保險:前兩個只能
*描述*規則,這一個是在你按下去之前**確認你在規則的哪一邊**。沒有它,你只能按下去
才知道剛才做出了什麼。

#### 三個拒絕,跟 rename 同一組

空的、名字裡有 `/`、名字已經被佔用。**只有最後那一個斜線是型別標記** —— 出現在別的
位置就是一條路徑,而路徑打進一個「名字」欄位,幾乎都是打錯而不是意圖:`a/b/` 要成立
就得默默建出兩層目錄,而「你是不是想這樣」的正確答案不是沉默。

「已經存在」這一條在有了檔案之後變得更重要:`Create` 會截斷。少了那個檢查,對一個
已存在的名字按 Add 不是被拒絕,而是把那個檔案清空 —— 所以測試比對的是**內容有沒有
被清掉**,不是目錄裡的項目數(數量在那個情況下根本不會變)。

#### 本機那一側從**啟動目錄**開始,不是從 `$HOME`

遠端那一側只能開在它的 home —— 沒有別的地方可以解釋成「這裡」。**本機不一樣**:
你是在某個目錄裡站著的時候決定需要 sshu 的。`cd ~/release && sshu` 應該一進去就
看著那批要送出去的東西,而不是先落在一個滿是 dotfile 的家目錄、再從那裡導航出來。

所以 `remote.StartDir` 跟 `remote.LocalPath` 問的是同一類問題(這個 FS 是不是這台
機器),而 host picker 第一列的說明也跟著從「this machine」改成「this directory」
—— 它連的不再只是一台機器,而是一個**位置**。

`s.home` 仍然是真正的家目錄:麵包屑把路徑折回 `~` 靠的是它,把它換掉會讓
`~/release` 被折成 `~`。**「開在哪」和「用什麼折」是兩個問題**,之前它們共用一個
值,所以看起來像同一個。

#### tab [3] 也要說它在連 —— 空的終端機和「還沒開始」長得一模一樣

`[5]` 畫的是 PTY 的網格,而 ssh 在等 TCP 連線的時候**什麼都不印**。所以連一台不會
回應的機器,那個 panel 就是一個空框,一直空到作業系統放棄為止 —— 可以超過一分鐘。
使用者看到的是「按了 Enter,然後 app 沒反應」,而且**分不出是主機慢還是程式壞了**。

判準是 **PTY 有沒有吐出過任何一個 byte**(`ptyTerm.spoke`),不是「網格是不是空
的」。ssh 一旦說話 —— banner、密碼提示、shell prompt、錯誤訊息 —— 對話就開始了,
終端機就把 panel 拿回去。在那之前是 spinner、對方是誰、以及等了幾秒。

這跟 §7.3 的 sftp dial spinner 是同一個抱怨,而這個 tab 當初沒做,理由是「`[5]` 顯示
遠端送來的東西」—— 那句話在**遠端還沒送任何東西**的時候正好是錯的。

### 7.4 tab [2] 的一次傳輸

**先算完整個 plan,再問。** `remote.Plan` 遞迴展開要建立的每一項(目錄也是一
項:空目錄要到,而且檔案不能比父目錄先寫),並回報總位元組數。覆寫的詢問因此
問在開始之前 —— 複製到一半才發現要覆寫,那時候問已經不算問了。

**進度條的分母從第一格就是對的**,正因為 plan 先跑完。job 用 atomic 回報、
render 只讀,所以畫一格 frame 不會等在網路上(`ui/transfer.go`)。

**取消會刪掉半個檔案。** 一個看起來像真貨的半截檔是這裡最糟的結果:下一個讀它
的東西拿到截斷資料,而且沒有任何東西說它被截斷過。

**進度佔膠囊列右邊那個 slot**(`󰕒 3/12 · 42%`),marks 數退位 —— 進度是會一直
瞄的東西,marks 在 `[5]`/`[7]` 本來就看得到。`[J]obs` 開詳細清單、可逐條
cancel。

**離開時三樣一起放掉**:ssh session、進行中的傳輸、sftp 連線。三條出口
(`q`、quit 確認、`Ctrl+C`)走同一個 `AppModel.quit()`,所以沒有一條會漏掉其中
一樣。而且**進行中的傳輸也會讓 `q` 先問** —— 半個檔案的損失不比一個閒置的 shell
小,對後者示警卻對前者沉默,那條線畫得很奇怪。

**`[t]ransfer` 傳游標這一項,`[T]ransfer all marks` 傳這一側全部 marks**,兩者
都送到「對面的當前目錄」。兩邊各自選好 host、各自切好目錄,傳輸就有明確的來源與
去處,不必再問一次目的地。

`t` 與 `T` 是這個 app 裡唯一一對用大小寫分辨的動作,也是 §4.4 那條「完全相同的
鍵優先」存在的原因。它們同時出現在四個 panel:游標在哪一個 panel 上,`t` 傳的就
是那裡的那一項(檔案清單的一列,或 marks 清單的一列)。

> **同一個字母的兩段歷史**:`[T]ransfers`(進度視窗)曾經跟 `[t]ransfer` 撞在
> 一起、而且是**悄悄**被蓋掉的。當時的解法是改名 `[P]rogress` —— 因為 `T` 這個
> 位置留給了「傳全部」,而進度視窗跟傳輸本來就不是同一件事,不該靠大小寫去分。
>
> 它後來又讓了一次:v1.2.0 tab 鍵搬回裸字母,`P` 被 `[M]anage` 之外的第二個
> 需求盯上前就先騰了出來,而 `[J]obs` 本來就比 `[P]rogress` 準 —— 那個視窗列的
> 是一件件工作、可以逐條取消,不是一根進度條。見 §11.21。

---

## §8. 資料層

### 8.1 config 根目錄(XDG 優先)

沿用 filu 的 `filuConfigDir` 邏輯:

```
XDG_CONFIG_HOME 有設   → $XDG_CONFIG_HOME/sshu/
否則                   → os.UserConfigDir()/sshu/
                          macOS: ~/Library/Application Support/sshu/
                          Linux: ~/.config/sshu/
```

`XDG_CONFIG_HOME` **在所有平台都優先** —— 讓 macOS 使用者可以主動選
`~/.config/sshu`。另留 `SSHU_CONFIG` 環境變數覆寫整個目錄(demo 錄製 /
隔離測試用,同 filu 的 `FILU_CONFIG`)。

| 檔 | 內容 | 誰維護 |
|---|---|---|
| `hosts.yaml` | host 清單 | **[1] hosts tab 的 CRUD** |
| `config.yaml` | 調校旋鈕 | 手改 `(planned)` |
| `state.yaml` | session 狀態(上次 tab / cursor) | app 自動 `(planned)` |

### 8.2 `hosts.yaml` schema

```yaml
# sshu hosts —— 由 [1] hosts tab 管理,手改也可以。
# 本檔權限固定 0600(內含連線密碼,見下方警告)。
version: 1
hosts:
  - name: prod-web-01
    host: 10.0.3.14
    port: 22
    user: deploy
    auth: privatekey                 # privatekey | password
    identity_file: ~/.ssh/id_ed25519

  - name: db-replica
    host: db.internal.corp
    port: 2222
    user: postgres
    auth: password
    password: "s3cr3t"               # 明碼
```

- **`name` 就是 key**,全域唯一;CRUD 以 name 定位,不另設 id(簡單優先)
- `auth` 是**扁平字串**、不是巢狀 map;`identity_file` / `password` 是
  依 `auth` 值二選一的兄弟欄位
- `identity_file` 支援 `~` 展開(`store.ExpandTilde`);由 form 的 `Tab`
  檔案選擇器寫入時會折回 `~` 形式(`store.FoldHome`),兩者互為反向
- 寫檔用 **atomic write**(寫 temp → `rename`),避免中途斷電毀掉整份清單

### 8.3 密碼儲存 —— 已決定存在 `hosts.yaml`

依你的決定,`auth: password` 的密碼**明碼存在 `hosts.yaml`**。這是明確的
取捨,設計上配套三件事把面積壓到最小:

1. **檔案權限固定 `0600`**,每次寫入後重新 `chmod`(即使使用者手動改寬)
2. **UI 永不顯示明碼** —— 表格的 Auth 欄只顯示 `password` 這個 method 名;form 內
   一律遮罩成 `••••••••`
3. **檔頭固定寫一行警告註解**,提醒此檔不可進版控 / 不可同步

> ⚠️ **殘留風險**(已知並接受):`hosts.yaml` 一旦被雲端同步、備份、或誤
> `git add`,密碼即外洩;0600 擋不了「檔案被整份複製走」。
>
**密碼怎麼送給 ssh**:走 `SSH_ASKPASS` + `SSH_ASKPASS_REQUIRE=force`
(OpenSSH 8.4+)。ssh 會把 sshu 自己再執行一次、環境變數帶
`SSHU_ASKPASS_HOST=<name>`,那個模式**只印出該 host 的密碼然後結束**,不啟動
TUI。

- **密碼不進子行程的環境變數** —— helper 自己重讀 `hosts.yaml`(0600),
  所以祕密只存在那一個檔裡,不會被複製進一個活著的行程環境中。
- **不去比對 pty 裡的 `password:` 提示然後自動打字**:提示文字會隨語系 /
  OpenSSH 版本變,而且那等於把密碼寫進 pty 的 input。
- helper 失敗(找不到 host、不是 password 認證、檔讀不到)就回非零,ssh 退回
  在 pty 裡提示、使用者自己打 —— 不會卡死。

> **升級路徑(`(planned)`)**:把密碼讀寫抽成一個 `secretStore` 介面,
> v1 實作 `yamlStore`,之後可直接掛 `keychainStore`(macOS Keychain /
> libsecret),`hosts.yaml` 改成只存 reference。介面現在就留、之後不用改
> schema 以外的東西。

---

### 8.4 `credentials.yaml` —— 可重用的身分(v0.2)

```yaml
# sshu credentials —— 由 manage → credentials 管理,手改也可以。
version: 1
credentials:
  - name: ops-pw          # host 以這個名字引用
    user: ops             # credential 整包供應 user(§11.三)
    auth: password        # password | privatekey(不能再是 credential)
    password: "..."       # 與 hosts.yaml 同一個明文決策、同一組緩解
```

host 端寫 `auth: credential` + `credential: ops-pw`,連線時經
`store.Resolve` 換成具體的 user+auth;引用斷掉在**確認框那一步**就報錯,
不會走到 ssh 裡才失敗。刪除/改名 credential 時會數還有幾台 host 引用它。

### 8.5 `applogs.yaml` —— app log 的落地(v0.2)

裸的 top-level YAML list:記一筆事件=往檔尾 append 一個單元素 list 的
bytes,熱路徑沒有 read-modify-write,寫到一半掛掉損失一筆而不是整檔。
超過 1 MiB 自我修剪(條數與位元組雙重上限)。失敗連線的完整畫面在裡面,
所以比照密碼檔:0600 每次重申 + 警告標頭。

### 8.6 訊號與孤兒(v0.2)

每個子行程都在自己的 PTY session 裡,殺 sshu 的訊號**到不了它們**。
startPty 是所有子行程的出生地,所以 registry 記在那裡;`KillChildren()`
是每條退出路徑的最後一行,外加自己接的 SIGHUP(關終端視窗)。外部
SIGINT/SIGTERM 走 bubbletea 內部路徑、不經 model 的 quit —— 這就是
registry 必須存在的原因。實測:SIGTERM 與 SIGHUP 前有 ssh 子行程,後無。

## §9. 檔案骨架

```
sshu/
├── cmd/sshu/main.go        進入點;也是 ssh 的 askpass helper
├── internal/
│   ├── ui/
│   │   ├── app.go          AppModel、tab 狀態、按鍵路由、單一出口
│   │   ├── view.go         compose、浮層疊放次序、footer legend
│   │   ├── theme.go        色彩錨點與 glyph 常數
│   │   ├── chrome.go       powerline tab 帶(自 filu 改)
│   │   ├── preftab.go      [M]anage nav + 內容切換 + 版面(v0.2)
│   │   ├── hosts.go        hosts 表格 model:cursor、捲動、跨欄 fuzzy 搜尋
│   │   ├── credlist.go     credentials 表格(v0.2)
│   │   ├── credform.go     credential form(v0.2;與 host form 共用欄位引擎)
│   │   ├── credkeys.go     credentials 的動作表 + credential picker(v0.2)
│   │   ├── table.go        hosts 表格:欄寬推導、列 render
│   │   ├── form.go         host form popup(form 類)
│   │   ├── filepicker.go   identity file picker(menu 類)
│   │   ├── sshtab.go       [S]SH 終端網格、layout strip、session 清單(v0.2 重寫)
│   │   ├── sshkeys.go      [S]SH 的動作表、layout 鍵、R×C 解析
│   │   ├── session.go      session model、ssh 指令、askpass 環境
│   │   ├── pty_unix.go     pty + vt10x + 按鍵轉 bytes(unix only)
│   │   ├── sftptab.go      [2] 四 panel 版面、兩側 model、marks
│   │   ├── sftpview.go     [2] 的 render:檔案列、marks 列、query 列
│   │   ├── sftpkeys.go     [2] 的動作表、host picker、rename/add/delete
│   │   ├── sftpsearch.go   [2] 的 `/`:串流 walk、比對、取消
│   │   ├── sftpdial.go     連線中的 spinner 與經過秒數
│   │   ├── sftpwatch.go    目錄刷新:stat mtime、變了才重列
│   │   ├── viewer.go       [v]iew:文字 / hex / 目錄一層,ESC 消毒
│   │   ├── highlight.go    chroma + catppuccin-mocha(自 filu 搬)
│   │   ├── edit.go         [e]dit:抓下來 → PTY → 比內容 → 原子寫回
│   │   ├── editorcmd.go    $VISUAL/$EDITOR/vi 解析、環境剝除
│   │   ├── inputpopup.go   一行文字的問句(Rename / Add)
│   │   ├── confirm.go      破壞性動作與離開的確認
│   │   ├── applog.go       app log([M]anage → logs 的內容 panel;寫穿 applogs.yaml)
│   │   ├── procreg.go      子行程 registry + KillChildren(v0.2)
│   │   ├── empty.go        沒有 item 時的統一空狀態(事實 + 可折行提示)
│   │   ├── nav.go          清單導覽詞彙:繞回、半頁、保留字母
│   │   ├── crumb.go        cwd 純文字麵包屑(lavender + dim 斜線)
│   │   ├── transfer.go     傳輸 job、進度條、Transfers popup
│   │   ├── path.go         fitPath 漸進縮短
│   │   ├── spacemenu.go    §A.1
│   │   ├── helppopup.go    §A.2
│   │   ├── popup.go        drawPopupBox + animator + hotkey 比對
│   │   ├── toast.go        回饋
│   │   └── width.go        display-width helper + ANSI-safe clip
│   ├── remote/
│   │   ├── fs.go           FS 介面(local | sftp)、排序、Join/Parent、RemoveAll
│   │   ├── local.go        本機實作
│   │   ├── sftp.go         SFTP 實作、認證、known_hosts 驗證、posix-rename
│   │   ├── copy.go         Plan / CopyItem / Conflicts / SameTree
│   │   ├── search.go       Scan:廣度優先子樹走訪、上限、可取消
│   │   ├── peek.go         Peek:讀開頭 n bytes(給 [v]iew)
│   │   ├── edit.go         Fetch / Digest / WriteBack / Stamp(給 [e]dit)
│   │   └── errors.go       兩端錯誤的共同說法
│   └── store/
│       ├── store.go        XDG 路徑解析
│       ├── config.go       config.yaml(唯讀:連線預算)
│       ├── hosts.go        hosts.yaml 讀寫 + 驗證 + Resolve(credential)
│       ├── credentials.go  credentials.yaml 讀寫 + 驗證(v0.2)
│       └── applog.go       applogs.yaml append/讀回/自我修剪(v0.2)
├── docs/
│   ├── sshu-ui-design.md       ← 本檔(為什麼)
│   └── sshu-implementation.md  現在是怎麼做的
├── README.md / README-zh_TW.md / CHANGELOG.md
├── go.mod
└── Makefile
```

相依:`bubbletea` / `lipgloss` / `bubbletea-overlay` / `yaml.v3` / `x/ansi`、
embedded terminal 的 `creack/pty` + `hinshun/vt10x`、連線的
`golang.org/x/crypto/ssh` + `pkg/sftp`,以及 `[v]iew` 上色用的
`alecthomas/chroma/v2`。
---

## §10. 開發順序與狀態

**1. hosts → 3. ssh → 2. sftp**(你指定的順序)。

### 第一階段:tab [1] hosts —— **完成**

| 項 | 狀態 | 落在哪 |
|---|---|---|
| 膠囊 tab bar + `Tab`/`1-3` 切換 | 已落地 | `ui/chrome.go` `ui/app.go` |
| hosts **表格**(Name/User/Host/Port/Auth)+ `j`/`k` + `gg`/`G` + 捲動 | 已落地 | `ui/hosts.go` `ui/table.go` |
| 窄寬:表格逐欄收縮(Auth → Port → User/Host) | 已落地 | `ui/table.go computeCols` |
| 空狀態(揭露 `[A]` + `Space`) | 已落地 | `ui/hosts.go emptyState` |
| 所有 panel 的空狀態同一個形狀,提示會折行 | 已落地 | `ui/empty.go` |
| `/` 搜尋:跨欄 fuzzy(不含 auth)、依分數排序、佔標題列 | 已落地 | `ui/hosts.go refilter` |
| panel 無 border title(單一 panel 的 tab 不戴) | 已落地 | `ui/hosts.go view` |
| `hosts.yaml` 讀寫(XDG、0600、atomic) | 已落地 | `store/store.go` `store/hosts.go` |
| footer(揭露兩個入口) | 已落地 | `ui/chrome.go keyLegend` |
| **Space menu**(item + panel region) | 已落地 | `ui/spacemenu.go` `ui/app.go hostActions` |
| **`?` help popup**(可捲) | 已落地 | `ui/helppopup.go` |
| **Add / Edit form popup + 驗證** | 已落地 | `ui/form.go` `ui/app.go validateForm` |
| **Delete 確認** | 已落地 | `ui/confirm.go` |
| **Connect 確認**(確認後 → tab [3]) | 已落地(接點待 tab [3]) | `ui/confirm.go` `ui/app.go doConnect` |
| **Toast**(generation guard + auto-dismiss) | 已落地 | `ui/toast.go` |
| **開關動畫**(~128ms、各自 animator) | 已落地 | `ui/popup.go popupAnimator` |
| **popup layer 色**(巢狀深度推導) | 已落地 | `ui/popup.go popupLayerColor` |
| **Identity file picker**(`Tab`、fuzzy、權限標示) | 已落地 | `ui/filepicker.go` |
| **亮鍵暗述 hint**(footer 與 popup 同一規則) | 已落地 | `ui/popup.go hintLegend` |
| **編輯列 lavender**(與清單 cursor 分色) | 已落地 | `ui/theme.go editColor` |

**VTP 破洞已補**:`Space` 與 `?` 現在在任何 tab(含未實作的 [2]/[3])都會回應
—— 沒有具體動作時 menu 仍列出「no actions here yet」,符合 §A.1 衍生規則。
X 回到 ~1.0。

**測試釘住的不變式**(`internal/ui/*_test.go`):

| 不變式 | 測試 |
|---|---|
| 任何 popup 疊上去,每一行仍等於終端寬、行數不變 | `TestPopupPreservesFrame` |
| 每個 contextual 動作都在 Space menu 現身(§4.2) | `TestSpaceMenuListsEveryAction` |
| Space menu 的每一列都真的會跑 | `TestEveryMenuRowRuns` |
| item region 在 panel region 之前(§6.6) | `TestMenuRegionsAreCursorFirst` |
| 未實作的 tab 上 `Space` 仍有回應 | `TestSpaceRespondsOnUnbuiltTabs` |
| `Esc` 一次只退一層、保留 source(§6.4) | `TestEscPopsOneLevel` |
| commit 收掉整個 stack(§7.1) | `TestCommitTearsDownTheStack` |
| popup 開著時 `q` 不生效 | `TestQuitIsInertUnderAPopup` |
| 動畫中的 popup 不吃鍵(§6.2) | `TestPopupIgnoresKeysWhileAnimating` |
| 切 auth 時 form 高度不變 | `TestFormHeightIsStableAcrossAuth` |
| 密碼永遠不出現在畫面上 | `TestPasswordIsMasked` |
| form 內 `Space` 打空白(§4.5) | `TestSpaceTypesInsideTheForm` |
| CRUD 真的寫對 / 取消不寫 | `TestCreateSavesTheNewHost` 等 5 個 |
| 窄 panel 上提示折行而不是被截斷 | `TestEmptyHintWrapsInsteadOfTruncating` |
| 折行之後鍵還是鍵 | `TestKeysStayLitAcrossAWrap` |
| 每個 panel 的空狀態同一個形狀(置中) | `TestEveryEmptyPanelIsTheSameShape` |
| panel 太矮時事實留到最後 | `TestAShortPanelKeepsTheFact` |
| 搜尋跨欄比對但**不含 auth**,排序最佳在前 | `TestHostsSearchMatchesAcrossColumns` / `TestHostsSearchIgnoresTheAuthColumn` |
| query 佔標題列,不改變列數 | `TestHostsSearchRowReplacesTheHeader` |
| 動作打在**畫面上那一列**,離開搜尋不換位置 | `TestHostsActionsFollowTheFilteredCursor` / `TestLeavingASearchKeepsTheRow` |
| 空表格不提供 `/` | `TestSearchNeedsHostsToSearch` |
| Auth 用 radio glyph、panel 不戴 title | `TestAuthFieldUsesRadioGlyphs` / `TestHostsPanelHasNoTitle` |
| 舊 toast timer 不會關掉新 toast | `TestToastGenerationGuard` |
| bracket 印的那個大小寫**是唯一**按得動的鍵 | `TestOnlyTheMarkedCaseFires` / `TestLowercaseDoesNotFireAnUppercaseAction` |
| tab [2] 小寫作用在游標列、大寫作用在 panel;`a` append(再按取消)、marks panel `c` 清單項 | `TestSFTPMenuHasItemAndPanelRegions` / `TestSFTPMarkToggles` |
| 導覽鍵不受大小寫折疊影響(`G` vs `g`) | `TestNavigationKeysStayCaseSensitive` |
| `Tab` 只在 IdentityFile 欄開 picker、hint 也只在那裡宣傳 | `TestBrowseOpensOnlyOnTheIdentityField` / `TestFormHintIsPerField` |
| `Tab` 不會打字;方向鍵仍能離開該欄 | `TestTabBrowsesOnlyOnThePathField` / `TestPathFieldCanStillBeLeft` |
| 選檔後直接寫回欄位、路徑折成 `~` | `TestPickFillsTheField` |
| picker `Esc` 回到那張還沒填完的 form | `TestPickerCancelKeepsTheForm` |
| picker 疊上去 frame 仍不變形 | `TestPickerFrameHolds` |
| `~/.ssh` 不存在時 picker 說得出原因 | `TestPickerWithNoRootExplainsItself` |
| fuzzy:連續命中勝過分散命中 | `TestFuzzyScore` |

| 編輯列是 lavender、清單 cursor 不是(§B) | `TestFocusedFormRowIsLavender` / `TestListCursorIsNotLavender` |
| 選中列是 blue,且與未選中 render 不同 | `TestSelectedHostRowIsBlue` |

### 第二階段:tab [3] ssh —— **完成**

| 項 | 落在哪 |
|---|---|
| 三 panel 版面 `[4]`/`[5]`/`[6]`、左欄固定 30、上下 2:1 | `ui/sshtab.go panes` |
| `[5]` 取得 focus 時 `[4]`/`[6]` 收起、`[5]` 佔滿;`Alt+Esc` 還原 | `ui/sshtab.go panes` / `setFocus` |
| 遠端寬字元(emoji/CJK)不會撞破邊框 | `ui/pty_unix.go render` |
| `[4]` 列格式 `<glyph> <name…> <port>`、port 永不截斷 | `ui/sshtab.go listItem` |
| 游標 bar 與列色合併(**僅 `[4]`** 正在顯示 → 綠底反色) | `ui/sshtab.go listItem` |
| 窄寬收起左欄、pty 佔滿(門檻由左欄寬推導) | `ui/sshtab.go narrow` |
| 嵌入式 pty(`creack/pty` + `vt10x`)、多 session 併行 | `ui/pty_unix.go` |
| 按鍵轉 raw bytes(含 Alt 重新編碼、DECCKM) | `ui/pty_unix.go ptyKeyBytes` |
| `Alt+Esc` 收回鍵盤 + footer 換成唯一出口 | `ui/app.go` / `ui/view.go footer` |
| `Tab` 走 surface、刻意不進 `[5]` | `ui/sshtab.go cycleFocus` |
| `4`/`5`/`6` 直達 panel | `ui/app.go panelKey` |
| `[4]` 用終端機 glyph 標「右邊現在是誰」;`[6]` 用綠/紅標結束方式 | `ui/sshtab.go listItem` |
| 名字折行(優先斷在分隔符)、`#N` 預留欄位 | `ui/sshtab.go wrapText` |
| session 結束 → history + 結束原因;`[5]` 立刻回空狀態並釋放 emulator | `ui/sshtab.go reap` |
| Close / Duplicate 確認;`[4]` 的 `Enter` 直接切換不問;Duplicate 完成後焦點留在清單(§11.23)| `ui/sshkeys.go` |
| `q` 時有活 session 走紅字確認、退出殺掉子行程 | `ui/app.go` |
| `SSH_ASKPASS` 密碼路徑(密碼不進子行程環境) | `ui/session.go` / `cmd/sshu/main.go` |

**測試釘住的不變式**:

| 不變式 | 測試 |
|---|---|
| 三 panel + pty 疊上去每行仍等於終端寬(含窄寬臨界 53/54/55) | `TestSSHTabPreservesFrame` |
| `Alt+Esc` 收回鍵盤;**裸 `Esc` 屬於遠端** | `TestAltEscLeavesThePty` |
| 打字真的送到遠端(用 `cat` 回音驗證) | `TestKeysReachTheRemote` |
| pty 有 focus 時 footer 只留出口、不列會被吞掉的鍵 | `TestFooterInPtyAdvertisesTheWayOut` |
| `Tab` 永遠不會走進 `[5]` | `TestTabNeverEntersThePty` |
| `4`/`5`/`6` 直達 panel | `TestDigitsAddressPanels` |
| 移動游標**不會**切換 `[5]` 顯示的 session | `TestCursorDoesNotSwitchTheSession` |
| 游標已在當前 session 時 `Enter` 不跳確認 | `TestEnterOnCurrentSessionAttachesDirectly` |
| 結束的 session 帶著原因退場、focus 不留在死掉的 pty | `TestExitedSessionLeavesWithItsReason` |
| `#N` 只在同 host 多 session 時出現 | `TestOrdinalOnlyWhenDuplicated` |
| `q` 只在有活 session 時才確認,且數字正確 | `TestQuitWarnsOnlyWithLiveSessions` |
| Space menu 只列**當前 focus panel** 的動作,不外洩 | `TestSSHMenuListsFocusedPanelActions` |
| 密碼走 askpass、**不進子行程環境** | `TestPasswordHostUsesAskpassNotTheEnvironment` |
| ssh 參數(port / `-i` / `IdentitiesOnly` / `~` 展開) | `TestBuildSSHCmdArgs` |
| 折行優先斷在分隔符、不掉字 | `TestWrapText` |
| 結束的 session 立刻離開 `[5]`、emulator 被釋放 | `TestEndedSessionLeavesThePanel` |
| app log 不畫游標、`j`/`k` 捲視圖、沒有可執行的動作 | `TestTheAppLogIsAViewNotAList` |
| **失敗會被說出來、留在 `[5]`、也進 log**;footer 報未讀 | `TestAFailedConnectionIsSaidAndKept` |
| 連線有預算,交給 ssh 講;它管不到的沉默由 sweep 兜底 | `TestTheTimeoutIsHandedToSSH` / `TestAStalledConnectionIsGivenUpOn` |
| `config.yaml` 缺檔不是錯、壞檔不致命、荒謬的值不照做 | `TestNoConfigIsNotAProblem` / `TestABrokenConfigIsReportedButNotFatal` / `TestAnAbsurdTimeoutFallsBackToTheDefault` |
| log 收的是**整個失敗畫面**,而且長的那則捲得到底 | `TestTheLogKeepsTheWholeFailureNotJustItsLastLine` / `TestTheLogScrollsThroughALongEntry` |
| **還沒接通就不接受打字**,而 `Alt+Esc` 仍然出得去 | `TestKeysAreNotSentToAConnectionThatHasNotAnswered` |
| panel title 是純文字,膠囊只留給 tab row | `TestPanelTitlesAreNotCapsules` |
| `[6]` 不帶 on-screen glyph | `TestHistoryHasNoOnScreenMarker` |
| tab [3] 只剩兩個 panel,`6` 不再定址任何東西 | `TestTabThreeHasTwoPanels` |
| history 搬進 popup,仍然是沒有游標的 view | `TestHistoryIsAViewNotAList` / `TestHistoryPopupListsEndedSessions` |
| session 失敗會出聲,乾淨離開不會 | `TestABadExitIsAnnounced` / `TestAFailedSessionRaisesAToast` |
| `[4]` 的 `Enter` 永不跳確認(同列或他列皆然) | `TestEnterOnSessionNeverAsks` |
| `[D]uplicate` 對同一台 host 再開一條、用 session 自己的連線資料 | `TestDuplicateOpensASecondSessionToTheSameHost` / `TestDuplicateUsesTheSessionHostNotTheHostsFile` |
| `[C]lose` 會先問,取消不殺 | `TestCloseEndsTheSession` |
| focus `[5]` 佔滿全 tab、離開時還原並重新 resize 遠端 | `TestFocusedPtyTakesTheWholeTab` |
| 遠端印 emoji 也撞不破邊框(真 pty) | `TestWideRemoteOutputCannotBreakTheFrame` |
| 前景說 on-screen、背景說游標,沒有 inverse | `TestSessionRowColourCases` |
| 列顯示 `<user>@<host>`,不是存起來的名字 | `TestSessionRowShowsUserAtHost` |
| 三個 tab 的 region 標題用同兩個字串 | `TestEveryTabWordsItsRegionsTheSameWay` |
| port 在任何寬度都不被截掉,名字折行讓位 | `TestSessionRowAlwaysShowsThePort` |
| `q` 走完整路徑會問(pty 內的 `q` 屬於遠端)、取消不殺 session | `TestQuitFromSessionsAsksAndThenStops` |

### 第三階段:tab [2] sftp —— **完成**

| 項 | 落在哪 |
|---|---|
| 四 panel 版面 `[4]`-`[7]`、左右 1:1、各自 focus | `ui/sftptab.go panes` |
| `remote.FS` 一個介面(local / sftp),upload = download = remote-to-remote | `remote/fs.go` |
| SFTP 自己做認證與 host key 驗證(**變更的 key 直接拒絕,不問**) | `remote/sftp.go` |
| cwd 在 panel 內第一行、純文字 lavender + dim 斜線 | `ui/crumb.go` |
| `a` 標記(再按取消)/ `c` 清單項 / `C` 清空,marks 換 host 時清掉 | `ui/sftptab.go` |
| `r` 就地改名(預填舊名、拒絕覆寫、mark 跟著走) | `ui/sftpkeys.go` `ui/inputpopup.go` |
| `v` 讀檔:文字(上色 + 行號)/ hex / 目錄一層,上限 64 KiB | `ui/viewer.go` `remote/peek.go` |
| 本機側開在啟動目錄(`home` 仍是家目錄,折麵包屑用) | `remote/edit.go StartDir` |
| `e` 在 `$EDITOR` 裡編輯:遠端抓下來再寫回、本機就地改 | `ui/edit.go` `ui/editorcmd.go` `remote/edit.go` |
| `x` 刪游標這一項 / `X` 刪全部 marks(都先問、遞迴、**不跟隨 symlink**) | `remote/fs.go RemoveAll` |
| `A` 在當前目錄建檔或建目錄(結尾 `/` 決定),游標停在新的那一項上 | `ui/sftpkeys.go doAdd` |
| Space menu 分 item / panel 兩區,單一區時保持扁平 | `ui/sftpkeys.go sftpMenuItems` |
| `t` / `T` 是兩個動作(游標這項 / 全部 marks),靠大小寫分 | `ui/sftpkeys.go` `ui/popup.go hotkeyIndex` |
| Space menu 的標題是 focus 的那個 panel,與邊框膠囊同源 | `ui/app.go menuTitle` |
| 沒有 host 的那一側只提供 `[H]ost`,而且 Space 直接開 host 清單 | `ui/sftpkeys.go appliesTo` · `ui/app.go` |
| **`/` 遞迴搜尋整棵子樹**,串流、廣度優先、可取消、上限 20000 | `remote/search.go` `ui/sftpsearch.go` |
| 先 plan 再問覆寫,進度條分母從第一格就正確 | `remote/copy.go` `ui/transfer.go` |
| 取消或失敗會刪掉半個檔案 | `remote/copy.go CopyItem` |
| 進度佔膠囊列右 slot,`[J]obs` 可逐條 cancel | `ui/transfer.go` |

| 不變量 | 釘它的測試 |
|---|---|
| 四個 panel 任一 focus、任一寬度都不破框 | `TestSFTPTabPreservesFrame` |
| 搜尋中(深路徑 + 右側計數)也不破框 | `TestSearchPreservesFrame` |
| `/` 找得到三層底下的檔案,mark 記的是真正的絕對路徑 | `TestSearchFindsFilesBelowTheDirectory` |
| 空 query 只顯示當前目錄(即使 walk 已經跑完) | `TestSearchEmptyQueryShowsOnlyTheCurrentDirectory` |
| 結果串流進來不會移動游標 | `TestStreamingResultsDoNotMoveTheCursor` |
| 廣度優先:兄弟目錄的淺項先於深項 | `TestScanIsBreadthFirst` |
| 離開搜尋會 cancel walk 的 context | `TestClearingTheFilterCancelsItsWalk` / `TestScanStopCancelsItsContext` |
| 讀不了的目錄只跳過、不終止整趟 walk | `TestScanSkipsAnUnreadableDirectory` |
| 上限會被回報,不是默默截斷 | `TestScanReportsItsCap` |
| 計數放不下時整段丟掉,不切成另一個數字 | `TestNarrowSearchRowDropsTheCountRatherThanCutIt` |
| `t` 傳游標那一項、`T` 傳所有 marks,互不代勞 | `TestTransferCursorAndTransferAllAreDifferentKeys` |
| 每個動作都被自己的鍵選中(重複鍵 / 被吃掉的大小寫兄弟都算失敗) | `TestNoHotkeyCollisions` |
| 沒有 host 時 menu 與 hotkey 都只剩 `S` | `TestASideWithNoHostOnlyOffersSelectHost` |
| menu 標題 = focus 的 panel,而且真的印在畫面上 | `TestMenuTitleNamesTheFocusedPanel` |
| 搜尋提示符是 glyph,含斜線的 query 讀得出來 | `TestSearchPromptIsAGlyphNotASlash` |
| `Esc` 先退搜尋、第二下才上一層(hotkey 與 menu 兩條路都是) | `TestEscLeavesTheSearchBeforeTheDirectory` |
| 正在關閉的浮層不會吃掉下一個鍵 | `TestAClosingPopupDoesNotEatTheNextKey` |
| 目錄變更會自己出現(stat mtime,只有動了才重列) | `TestWatchPicksUpANewFile` / `TestWatchDoesNotRelistAnUnchangedDirectory` |
| 背景刷新後游標仍在同一個檔案上 | `TestWatchKeepsTheCursorOnTheSameEntry` |
| 沒人在看的 tab 不刷新;過期的 probe 結果會丟掉 | `TestWatchStopsWhenTheTabIsNotOnScreen` / `TestWatchDropsAStaleResult` |
| `h`/`l` 切換左右且保持同一列 | `TestHLCrossesSides` |
| `q` 與 `Ctrl+C` 都會關掉兩側的 sftp 連線 | `TestQuitClosesTheSftpConnections` / `TestForceQuitClosesTheSftpConnections` |
| 進行中的傳輸會讓 `q` 先問 | `TestQuitAsksAboutARunningTransfer` |
| 改名會帶著 mark 走,且拒絕覆寫既有名字 | `TestRenameMovesTheFileAndItsMark` / `TestRenameRefusesToClobber` |
| 刪除先問;取消不動任何東西 | `TestDeleteMarksErasesThemAfterConfirming` |
| `Clear marks` 只忘記、不刪檔 | `TestClearMarksLeavesTheFilesAlone` |
| 遞迴刪除不會走進 symlink 的目標 | `TestRemoveAllDoesNotFollowASymlink` |
| 有游標的清單與浮層都會繞;viewport 不繞 | `TestCursorsWrapEverywhere` / `TestSpaceMenuWrapsPastItsHeaders` / `TestViewportsDoNotWrap` |
| 每一個浮層都關得掉(`Space`),但打字中的三個不受影響 | `TestSpaceDismissesEveryFloat` / `TestSpaceTypesIntoTheRenameBox` |
| `?` 開關 help,並且疊得到別的浮層上面 | `TestQuestionMarkTogglesTheHelp` |
| `x` 刪游標那一項、`X` 刪 marks,互不代勞,且都先問 | `TestDeleteCursorAndDeleteMarksAreDifferentKeys` |
| 刪掉的東西如果被 mark 過,mark 一起拿掉 | `TestDeletingAMarkedRowDropsItsMark` |
| 新目錄會拒絕空的 / 含 `/` / 已存在的名字,並停住游標 | `TestNewDirectoryRefusesBadNames` / `TestNewDirectoryLandsTheCursorOnIt` |
| menu 兩區、標題跟著游標;單一區時扁平 | `TestSFTPMenuHasItemAndPanelRegions` / `TestItemRegionFollowsTheCursor` / `TestSFTPMenuStaysFlatWithOneRegion` |
| 沒有列時 item 動作連同字母一起消失 | `TestSFTPItemActionsNeedARow` |
| 導覽字母不被任何動作佔用;`d` 在每個 tab 都捲半頁 | `TestNoActionClaimsANavigationKey` / `TestDScrollsInEveryTab` |
| `v` 顯示文字(行號)/ 二進位(hex)/ 目錄(一層) | `TestViewShowsTextWithLineNumbers` / `TestViewShowsBinaryAsHex` / `TestViewShowsADirectoryListing` |
| **遠端檔案裡的 ESC 到不了終端機** | `TestViewStripsControlSequences` |
| 讀取有上限;過期的 preview 不會蓋上來 | `TestViewIsCapped` / `TestASupersededViewCannotLand` |
| viewer 是 viewport:捲動、不繞 | `TestViewScrollsAndDoesNotWrap` |
| **搜尋找到的檔案,`Enter` 會帶你過去,然後它就是普通的列** | `TestEnterGoesToWhatTheSearchFound` / `TestEnterOnADirectoryResultOpensIt` |
| tab bar 是**一條帶子**,恰好一段亮著(三角種類就是證據) | `TestTheTabRowIsOneStripWithOneLitSegment` |
| 接縫屬於**左邊**那一段(顏色方向,render 出來的字串看不出來) | `TestTheSeamBelongsToTheTabOnItsLeft` |
| 本機側開在**啟動目錄**,而 `home` 仍是家目錄 | `TestTheLocalSideOpensWhereSshuWasLaunched` |
| viewer 不戴 `/` 的放大鏡 —— 一個 glyph 就是一個詞 | `TestTheViewerDoesNotWearTheSearchGlyph` |
| **沒有東西會「晃」進 `[5]`**(`l` 不再是入口) | `TestNothingWandersIntoThePty` |
| `A` 兩種都做得出來,游標停在新的那一項 | `TestAddMakesAFileOrADirectory` |
| **框在你打字時就說會做出哪一種** | `TestAddSaysWhichKindItWillMake` |
| 路徑 / 空的 / 已存在都拒絕,而且不會清空既有檔案 | `TestAddRefusesBadNames` |
| `e` 真的跑起編輯器,而且它寫的東西回得去 | `TestEditRunsTheEditorAndSavesWhatItChanged` |
| 本機的檔就地編輯(inode 不動,hard link 還在) | `TestALocalFileIsEditedWhereItLives` |
| **沒改就不寫回**(比內容,不比 mtime) | `TestAnEditThatChangedNothingIsNotWrittenBack` |
| **被別人改過的檔不會被默默蓋掉** | `TestAFileThatChangedUnderneathIsNotOverwritten` |
| 寫到一半失敗,原檔完好 / symlink 不被換掉 | `TestWriteBackLeavesTheOriginalWhenTheWriteFails` / `TestWriteBackWritesThroughASymlink` |
| **檔名是引數,不是 shell script 的一部分** | `TestTheFilenameIsAnArgumentNotAScript` |
| 編輯器問不到外層終端機的身分 | `TestTheEditorIsNotToldWhichTerminalThisIs` |
| 不是文字先問、不是檔案直接擋、一次只准一個 | `TestEditAsksBeforeOpeningSomethingThatIsNotText` / `TestEditRefusesWhatIsNotAFile` / `TestOnlyOneEditorAtATime` |
| 覆寫會先問 | `TestSFTPOverwriteAsksFirst` |
| 目錄複製進自己會被擋 | `TestSFTPRefusesSelfCopy` |
| 傳輸可以取消 | `TestTransferCanBeCancelled` |
| 三張 action table 沒有大小寫撞號 | `TestNoHotkeyCollisions` |

### 第四階段:v0.2 大改 —— **完成**

Alt 和絃 tab、preference(nav + hosts/credentials/logs)、credential 整包、
applog 落地、host form 三選 auth 與「選值欄位」互動、ssh 終端網格
(Tab 開關、Enter 進入、Alt+方向鍵走格、layout strip、custom R×C)、訊號清理。
決策紀錄見 §11。

### 之後

- **tab [1] 的 `[S]ftp`**:`(planned)` 從 hosts 表格把游標那台直接接到 tab [2]
  當前 focus 的那一側,省掉「切 tab → `s` → 在清單裡再找一次」
- **Mouse**:`(planned)` §5 mapping
- **未知 host key 的互動確認**:`(planned)` 現在 `remote.Dial` 收到 `nil`
  prompt、一律拒絕,要先用 tab [3]（走真的 `ssh`）連一次寫進 `known_hosts`。
  缺的是一個能在 dial 途中升起的對話框
- **加密私鑰**:`(planned)` `remote.authMethods` 今天只會如實說做不到
- **fsnotify reload**、**`state.yaml`**、**keychain secretStore**:`(planned)`

---

## §11. v0.2 大改 —— Alt 和絃、preference、credential、終端網格

(這一輪的內部代號是 v0.2;對外的首發版號定為 **v0.1.0** —— 0.1.0 從未單獨
發布,首個公開版本就是大改後的樣子。)

一次成塊的重排,整輪都是使用者點名的方向;四個當場裁定的分歧點原文照錄:
applog「全部落地」、credential「整包供應」、數字例外「只有 pty 內」、
ssh 清單「Enter 直接切到該 item 的 pty panel,使用 tab 來開關」。

### 11.1 tab 讓出數字,搬到 Alt 和絃

`1`/`2`/`3` 切 tab 的舊形狀有兩個天花板:數字不夠 panel 用(sftp 四個
panel 得從 4 編起,「畫面上的號碼」跟「按的號碼」隔了一個位移),而且
**pty 裡一個裸鍵都活不下來** —— 遠端拿著鍵盤時,你沒有任何辦法換 tab。
Alt 和絃兩個都解:每個 tab 的 panel 都從 1 編號,和絃在 pty 裡照樣被
sshu 攔下。

大小寫的分工是後來補的裁定:**pty 外**小寫 `alt+p/f/s` 也通(那裡它是
死鍵,離活鍵一個 shift 的死鍵是沒有回報的陷阱);**pty 內**只認大寫
(`M-f` 是每一個 readline 的 forward-word,不是 sshu 的)。標籤先拼成
`[Alt+p]reference` —— 一個括號裝整個和絃(`[Alt][p]` 的兩括號版試了一輪
就併掉;之後 `[Alt]` 又抽成固定亮的鏈頭,見 11.8)—— 同批把 nav 面板改名 `[1] Resources`(使用者裁定;各節內的
舊拼法屬歷史紀錄,不回改。之後再改名 `[1] sshu` 並分類,見 11.12)。

**VHS 送不出任何 Alt**(`Alt+F` 只送裸字元、`Alt+Esc` 更是把 "Esc" 三個
字母打出來,cat -v 實證)。和絃由單元測試覆蓋;tape 要壓 Alt 得繞
`tmux send-keys`(M-P / M-Escape),網格的 dogfood 截圖就是這樣拍的。

### 11.2 preference:sshu 自己的資料一個屋簷

hosts 表格從「一個 tab 就是一張表」變成 preference 的一個區,旁邊住進
credentials 與 logs。左側 nav 的游標**就是選擇**:j/k 移過去內容立刻換,
Enter 只負責把鍵盤移過去 —— 需要第二個鍵才有意義的游標,瀏覽起來像壞掉。
tab 仍然**開在內容上**(hosts 表格,舊肌肉記憶的落點),nav 是去了才去的
chrome。

### 11.3 credential 是整包,不是預設值

credential = user + auth 一組,host 寫 `auth: credential` 就整包拿走,
**User 欄同時變暗**。裁定選了整包而不是「host 的 user 可覆蓋」:後者讓
「這條連線到底用誰」要看兩個地方才有答案。連帶的誠實成本都在門口付:
表格的 user 欄顯示 credential 的 user(引用斷掉顯示 `?`,不編造名字)、
確認框顯示解析後的身分、斷掉的引用在確認框那一步就用一句話失敗。

### 11.4 applog 落地(反轉「session 不落地」)

0.1.0 的立場是「連線紀錄不值得那個洩漏」。使用者裁定反轉:**全部落地**,
含失敗連線的完整畫面。緩解照密碼檔那一套 —— 0600 每次重申、警告標頭、
自我修剪(1 MiB,條數與位元組雙重上限)。`!` popup 一併退場:log 變成
preference → logs 的內容 panel,**上了畫面就是已讀**;寫不進去的 log 自己
在 log 裡抱怨,而且只抱怨一次。事件面也放寬:不只失敗 —— host/credential
的增刪改、連線的開始與結束、sftp 撥號、傳輸結果、edit 寫回,全都是事件。

### 11.5 「選值欄位」:Enter 開、Enter 存、Backspace 整行

IdentityFile 的舊互動(`Tab` 開 picker)把 Tab 在唯一一個欄位上變成別的
意思。host form 的欄位順序同時前移了 Auth(Name、Host、Port、**Auth**、
Credential、User、IdentityFile、Password):Auth 決定後面哪些列存在,選了
credential 就不必填 user —— 先問 user 再把它變暗,是問了一個得收回的問題。

**「有值 Enter」後來從「跳下一欄」改成「直接送出」。** 上線後使用者實測:
Auth 選 credential、Enter 開選單、選好 credential —— 然後還要再按兩次
Enter 才存得下去,因為第一次只是跳到下一欄,而下一欄輪回 Name。選一個值
的代價變成三次 Enter 加一圈迴轉,這不是「選值」該有的價錢。

新規則其實是**少一條規則**:空欄 Enter 開選單(空欄的 Enter 本來就沒有別的
意思可用),**有值就跟 form 上其他每一欄一樣 —— Enter 送出**。原本的
「跳下一欄」是這兩欄自己發明的第三種 Enter,它讓使用者得先知道「這一欄的
Enter 跟別欄不同」才按得對。border hint 跟著只說現在成立的:空欄
`Enter browse` / `Enter choose`,有值 `Enter save`(§4.4、§4.5)。

被否決的替代:(a) **picker 選完就直接存** —— 那會把「填完一欄」和「整張表
送出」綁成同一個動作,使用者還沒看過 form 的其他欄就被存掉;(b) **Tab 才
是下一欄、Enter 永遠送出,連空欄也是** —— 那空欄的 picker 就沒有鍵可以
開,只剩 Tab 一條路,而 Tab 在全 form 都是「下一欄」,再拿它回去開 picker
就是繞回 §11.5 一開始要修掉的那個問題。要「跳下一欄」的人本來就有
`Tab` / `↓`,那是它們在全 form 一致的意思。

8 個 mutation 全數被抓(有值仍跳欄 ×2 form、空欄不開選單 ×2 form、
Backspace 不整行清、hint 仍寫 next)。

**credential picker 曾經開在 form 底下**:composite 順序讓 form 蓋掉了它,
而所有 isActive 斷言照樣綠 —— z-order bug 只有 render 出來的畫面抓得到,
釘住它的測試讀的就是畫面。無效送出則是「說兩次」:error row 標出是哪一欄,
toast 讓拒絕不可能被錯過,form 留在原地讓人填完。新互動是使用者描述的流程原樣:**空欄 Enter 開選單 → 選好 Enter →
有值的欄 Enter 跳下一欄;Backspace 整行清除**,選來的值是被替換的,不是
一個字母一個字母刮掉的。`Tab` 回歸全欄位一致的「下一欄」。host form 的
Credential 欄與 credForm 的 IdentityFile 欄同一套;兩張 form 共用同一個
欄位引擎(editField / formBody),不是兩份實作。

### 11.6 ssh 終端網格

單一 `[5]` 變成**網格**:每個 session 可以有一格,格與格等分。裁定的
操作語彙:**清單上 `Tab` 開關格子**(該 tab 的 panel 輪詢本來就沒有地方
可去 —— 網格不是 Tab 可以走進去的地方,鍵讓給清單整天做的事)、**Enter
顯示並把鍵盤交過去**(side 欄同時收起)、**按住 Alt、方向鍵在格子間走**、
**`Alt+Esc` 收回鍵盤**(side 欄回來)。

格子間的移動一開始是 `Alt+1..9`,上線即拆:使用者指出它**跟本地的
shortcut 與 window management 軟體衝突**(AeroSpace 等平鋪工具的
workspace 鍵正是 Alt+數字,和絃在本地層就被吃掉、到不了 sshu),而且
編號跟著顯示順序重排、肌肉記憶會抓錯格。換成**空間移動**:按住 Alt 用
方向鍵往鄰格走 —— `Alt+方向鍵` 在裸終端裡本來就是無效輸入(使用者裁定),
不用記編號、重排無感;邊緣 **clamp 不繞**(空間移動絕不瞬移,同 u/d 的
規則),ragged 網格短列之外沒有格子就是不動。格子標題隨之拿掉
`[Alt+N]` 前綴 —— 沒有編號好揭露,名字本身就是身分。清單列開頭多一個顯示欄:有格子
的是 monitor、沒有的是劃線 monitor —— 兩個**形狀**,不是一個形狀兩種
顏色,色弱也讀得出來(codepoint 照規矩查 cmap:f0379 / f0d90)。

layout strip `[2]` 住在**左欄底部**、sessions 清單之下 —— 一開始蓋在網格
上方,使用者裁定搬家:右側就該完整屬於 pty panel。三個選項直排(30 欄的
左欄坐不下三個並排的選項,而且清單下面接一個清單讀起來自然),j/k 走、
h/l 也還答應。horizontal / vertical 是兩個退化網格(一列 / 一欄),custom
是指定的 R×C(**先列後行**,提示、解析、條紋標籤、toast 四處同一個順序 ——
一開始是 C×R,使用者裁定改為列先)—— Enter 問形狀,兩個 1-9 的數字用什麼
隔開都行;**塞爆時長列不砍格**(格子絕不能默默不存在)。條紋在左欄高度
撐不起「它自己 + 可用清單」時讓位。

清單列同批改成**一行**:`<user>@<host>:<port>`,ssh 自己的拼法 —— port
不再擁有右緣的專屬 slot(使用者裁定)。位址照折行,但 `:port`(與 #N)是
**一個不可拆的尾巴**:掛不上最後一行就整個自己一行 —— 拆到兩行的 port
(「…:2 / 222」)是另一個數字,不是短一點的同一個。

幾何的紀律:每一格的遠端只在數字真的變了才收到 SIGWINCH(applied 幾何
記在 session 上);splitEven 把餘數攤開,欄寬總和**精確等於**總寬 ——
frame invariant 不容忍一條散落的直欄。結束的紀律:格子死了就離場重排,
**鍵盤絕不默默落進另一台遠端** —— focus 格死了就退回清單;失敗畫面只在
「這一死讓網格空了」時佔住網格說話。

### 11.7 訊號與孤兒

`若程式被 ctrl-c 砍掉,同時需要砍掉所有連線中的 process`。三條沒人管的
門:外部 SIGINT(bubbletea 以 ErrInterrupted 收場、不經 model)、外部
SIGTERM(裸 QuitMsg,同樣不經 model)、SIGHUP(關終端視窗,根本沒人
接)。解法見 §8.6;實測兩種訊號前有 ssh 子行程、後無。

### 11.8 亮的兩段拼出和絃;游標的格子回聲

兩個 live-use 裁定,同批落地。

**`[Alt]` 抽成鏈頭、固定亮**。標籤第三次演進:`[Alt+p]reference` →
`[Alt] ❯ [p]reference ❯ [f]ile transfer ❯ [s]sh`。Alt 是三個和絃共用的
一半,與其在每段重複拼寫,不如抽成一段恆亮的鏈頭 —— active 的字母段亮起
之後,**帶子上亮著的兩段合起來就是按的和絃**;active 緊鄰鏈頭時兩段同色、
中間只剩 soft divider,直接連讀成 `[Alt]・[p]`。窄階退化順帶變好:
`[Alt] [p] [f] [s]` 比舊的 `[Alt+p] [Alt+f] [Alt+s]` 更窄,單一階仍是
整座梯子。實作上 tabChain 的段 0 就是鏈頭(isLit:`i == 0 || i == active`),
divider 語彙不動 —— 哪些三角是實心的,本身就把「兩段亮」釘死在測試裡。

**`[1]` 游標的格子回聲**。j/k 掃過 sessions 清單時,游標所在 session 若在
網格上,**右側那一格的外框同步亮**(同 focusColor,連 chip 一起)。清單列
與格子是同一個 session 的兩個面,游標掃過去格子跟著亮,「這一列是哪一格」
不用讀 cell 標題就有答案;游標停在沒上網格的 session 上時右側什麼都不亮 ——
沒有回聲本身就是「它不在網格上」的訊號,和列首的劃線 monitor glyph 互為
印證。鍵盤真的進到網格後,回聲讓位給 keyboard focus(cellLit 先看
panelPty);focus 在 `[2]` layout 上時也沒有回聲 —— 那裡沒有游標指著任何
session。

### 11.9 分隔線兼職進度條;傳輸 summary 不再 dim

使用者自己標註「過分的 fancy 要求」,落地之後其實一點都不過分 —— 兩個
channel 都是本來就在的元素,一個換 ink、一個換色,不佔任何新空間。

**右上 summary 改 liveColor**。`<done>/<files> · <pct>%` 是進行中的動作,
不是靜止事實 —— summary 的程式註解本來就引著 §7.2「information arriving
is not dimmed」,render 端卻一直把它染成 dim;這一筆是把畫面拉回 app
自己寫下的規則。只有 file transfer tab 的 summary 是「動作」;其他 tab
的 status(no sessions、n marks)仍是靜止事實、照舊 dim。

**分隔線從左往右變綠**。tab 帶與 workspace 之間那條整寬分隔線,傳輸時
ink 從左緣起依 blended percent 換成 liveColor,其餘維持 borderDim;沒
東西在動的瞬間整條恢復。分隔的本職完全不動 —— 同樣的字形、同一行 ——
所以這條進度條不花任何版面。兩個 channel 讀同一個
`transferModel.progress()`(running jobs 的平均,也是 summary 印的那個
數),數字不可能吵架。**分隔線在每個 tab 都報**:傳輸不因你切去看別的
tab 就停,而這條線是三個 tab 唯一共用的 chrome —— 在 ssh tab 盯著遠端,
眼角那條綠線仍在報進度。

### 11.10 [a]ppend to marks 與 [c]lear mark

`[m]ark`(hint「toggle」)名不符實兩頭落空:名字說單向、行為是雙向,hint
又只剩一個孤零零的「toggle」。裁定改成:檔案 panel `a` = **Append to
marks**,行為維持 toggle(誤標的救援仍是再按一次,不用跑去 marks panel),
hint 誠實寫「again takes it off」;marks panel 的移出改成 `c` = **Clear
mark**,跟 `[C]lear marks` 同字母成對 —— 小寫單項、大寫整個 panel,與
`t`/`T`、`x`/`X` 同一套 case 文法,hint 沿用 C 的「forget it, change
nothing」(它跟 Delete 的距離就靠這句話)。hotkeyIndex 無大小寫回退,
`a`/`A`dd 各行其是、files panel 按 `c` 也不會誤射 `C` —— 後者本來就有
測試釘著。`u` 依然屬於半頁上捲。同批:marks panel 的標題
`[2]/[4] Marked files` 改成 `[2]/[4] Marks` —— 動作叫 Append to **marks**、
清的叫 Clear **mark**,panel 就叫它自己的名字,標題跟著語彙走。

### 11.11 icon 與 splash 彩蛋

`docs/icon.svg` 是 u-family mark 的 sshu 版:藍 U 框住一個**拼出 SSH 的
金色圖形** —— 兩個 S 疊在 H 的雙軌之間,rows 10-11 的整寬橫槓是 H 的
橫畫。`V`(pty 外、無 popup 時)觸發 splash 彩蛋,kbu / filu 的同款:
底片掃入 → 上 S 散點浮現 → 下 S → H 自上而下 → 藍 U 框自底升起 ——
**揭示順序唸出 s-s-h** —— 然後名稱 / 版本 / tagline / Esc 提示分兩拍
淡入,任意鍵放回原畫面。像素畫由 icon.svg 逐格生成(中心點取樣 ——
icon 含幾個 10×15 的 rect,固定 10px 網格會漏掉 S 的豎筆,實際漏過
一次);splash 期間鍵盤完全歸它,pty 內 V 屬於遠端(外層路由先擋,
條件裡的 inPty 是縱深防禦)。

### 11.12 [1] sshu:nav 分類與 Operation 頁(Export / Import)

**改名**:`[1] Resources` → `[1] sshu`(使用者要求)。這個 panel 裝的是
「屬於 sshu 自己、而不是屬於某台 host」的一切 —— 資料、事件、以及對設定
整體的操作 —— app 名是這件事最短的誠實標籤。同批把 nav 項目改成大寫
(`hosts`→`Hosts` 等):它們現在是**分類底下的條目**,跟 kbu sidebar 的
條目同一身分,拼法也跟上。

**分類**(kbu sidebar 的形狀):header 是裝飾列 —— 游標永遠不落在上面,
j/k 直接跨過去 —— 條目縮排一格站在它底下。三類:**SSH**(Hosts、
Credentials:連線跑在上面的資料)、**Events**(Logs:發生過的事)、
**Operation**(Export、Import:對 sshu 自己的設定整體做的動作)。header
用 dim:它是地標不是選項,跟游標搶眼就輸了。enum 與 moveCursor 原封不動
—— 分類只活在渲染層,「游標即選擇」的機制一行沒改。

**Operation 頁是 panel,不是 popup。** Export / Import 是 nav 游標落腳的
目的地,跟 Hosts 或 Logs 同級;它們唯一的內容就是一張小表單,再包一層
popup 等於多一個沒東西站的臺階。代價是 §4.5 的老帳進了 panel:focus 在頁
上時字母**與數字**都是字元 —— 數字直達、`q` quit、Space menu、`?` help、
`V` 彩蛋全部讓位(`textPage()` 給 ?/V 的全域 handler 各加一道 guard;
漏掉的話「檔名裡打個 V」會開 splash)。出口只剩 **Esc = 鍵盤還給 nav**;
footer 沿用 pty 列的誠實原則,切成 `tab field · enter run · esc back`。
頁內常駐一列 hint(§4.5 的 standing disclosure),status 列(錯誤**或**
上次成功)永遠保留一行 —— 頁的形狀不因驗證結果改變(§6.7 / §1.3)。

**Export**:Directory(預設 = 啟動目錄,tab [2] 的 local 同款慣例)+
Filename(預設 `sshu-export.sshu`,副檔名寫全 —— 欄位顯示什麼就寫什麼;
漏打 `.sshu` 自動補上而不是報錯,這頁只會寫這一種東西)。Enter 把**當前
清單**(畫面上的,不是重讀磁碟)打包成 zip:`hosts.yaml` +
`credentials.yaml` 原樣兩個 entry、各帶警告標頭、bundle 本身 0600 —— 它帶
著跟 YAML 一樣的明文密碼,頁上有一行常駐警語。**目錄不存在報錯**(擅自
mkdir 會吞掉打錯的路徑)、**檔案已存在拒絕**。被否決的替代:蓋掉前彈
confirm —— 又一層浮層,而改個檔名比回答一個問題便宜。

**Import**:一個路徑欄(**用打的,不是 picker** —— filepicker 刻意不走
$HOME,遞迴掃 home 會掛 UI;而剛跨完機器的 .sshu,路徑本來就在使用者手
上)。Enter 讀 zip、合併、**同名跳過**:Name 是兩個檔案的 key
(File.Validate 強制唯一),撞名的 incoming **整條跳過、絕不逐欄合併** ——
本機那份是使用者看得到、一直在信任的那份。無效條目同理跳過。credentials
先落地、hosts 後(imported host 可能引用 imported credential,反序在第二
個 save 失敗時會留 dangling reference)。結果句照實數:
`Imported 1 host · 1 credential (1 skipped)`,toast、頁上、app log 三處
同句。

**splash 落款**:caption 最下方新增一行 dim 的
`developed by vulcan.shen.2304@gmail.com`,跟 Esc 提示同拍浮現。

7 個 mutation 全數被抓(merge 不去重、export 蓋檔、副檔名不補、頁不攔鍵、
header 不畫、import 不落地、落款不顯示)。

> **同日追記(使用者裁定)**:
> 1. **header dim → 藍(focusColor)**:分類是 app 結構,跟 panel border /
>    title 同一種藍,不是次要資訊的 dim。條目維持 textColor,游標 bar 不變。
> 2. **Operation 整類遮罩**:Export / Import 的設計尚未定案 —— enum 尾值、
>    頁面程式與測試全部留著,但 `prefSections` 不列、nav 不畫也不停
>    (`navKey` 只在可見條目數內繞)。解除遮罩 = 把
>    `{"Operation", …}` 那行放回 prefSections。README / CHANGELOG 同步
>    不宣傳。
> 3. **splash 落款拆兩行、移到 Esc 提示上方**:`developed by` 與
>    `vulcan.shen.2304@gmail.com` 各一行,接著空一行才是
>    `Press Esc to close` —— 行動提示留在最後一行。
> 4. 順手修一個既有 frame 缺陷:logs 內容的空狀態(`emptyBody`)回傳的
>    行數少於 panel 高,窄寬(<60)只畫 content 側時整個 frame 短三行 ——
>    logs branch 補上 `fitLines`,跟其他 content body 同款。frame 測試
>    的 `{"1","G","enter"}` 路徑現在守著它。

### 11.13 nav 在鍵盤不在它身上時整個暗下來;Logs 的 [C]lear

**nav dim**(使用者要求):focus 在 `[2]` 的時候,`[1]` 整片降一階 ——
分類 header 從藍(focusColor)變成邊框的 dim、條目從 textColor 變 dimColor、
游標 bar 從 handColor 換成 borderDim。理由:鍵盤不在它身上的時候,nav 是
「`[2]` 現在在顯示什麼」的圖例,不是正在工作的地方;一片游標動不了的清單
卻是畫面上最亮的東西,只會跟真正在用的 panel 搶。header 的顏色直接用
`borderColor(focused)` —— 它跟 panel 邊框、title chip 是同一支筆,所以
「分類是 app 結構」(§11.12 追記)在兩個狀態下都還成立,而不是只在亮的
那個。

**游標仍然是 bar**,只是換一階。被否決的做法是「沒 focus 就不畫 bar」——
那樣就沒有東西說 `[2]` 在顯示哪一節了(panel title 說得出名字,但 nav 說
的是它在清單裡的位置,以及上下還有什麼)。暗掉的 bar 用的是 **unfocused
panel chip 同一組配色**(baseHex 字 + borderDim 底):app 裡「沒被選到的
高亮」本來就長這樣,不是為這裡新發明的一階。

**未讀錯誤數不 dim**:整片暗下來的 nav 裡唯一還亮著的是 logs 那列的紅色
計數。它是消息不是裝飾,而且**你正在看別的地方**的時候才最需要它 —— 這正
是這個計數存在的理由(§7.1.5)。

**[C]lear logs**:logs 的 Space menu 以前只有兩列 header(「沒東西可以
做」),現在底下多一個動作。它同時是選單列與字母鍵(§4.2:每個字母鍵都是
一列,每一列都能不知道字母也跑得動);小寫 `c` 不接 —— hotkeyIndex 只精準
比對,大小寫不折疊。一個動作不開 action table:一列不是註冊表,menu 那列
與 panel 那個鍵各寫一次同一個條件就夠了。

**先問**(confirm popup,跟刪 host 同一個形狀):log 是「你沒在看的時候
發生了什麼」的唯一記錄,而且清掉會連 `applogs.yaml` 一起清。確認句把代價
寫出來:`38 entries erased, applogs.yaml too.`。

**先清檔案,再清記憶體**:順序反過來的話,檔案還在而畫面空了 —— 下次開
app 全部回來,這是 clear 唯一不能有的結果。檔案拒絕(唯讀、權限)就整批
不動,把錯誤原話說出來。

**清完不寫 log**:「app log cleared」當作剛清空的 log 的第一列,看起來像
沒清乾淨。這則消息歸 toast(`Cleared 38 entries`)。

**空 log 沒有這一列、也沒有這個鍵**:跟 hosts 表格沒有列可刪時 `D` 不出現
是同一條規則;menu 退回兩列 header(「nothing recorded yet」),按 `C` 是
沉默 —— 已經空了,沒有話要說。

**store 層**:`ClearLog` / `ClearLogTo` 把檔案寫回**只剩警告標頭** ——
警告講的是這個檔案而不是某一則 entry,而且清空後它仍然是下一則 append 的
目標(0600 照樣重新確立)。檔案不存在不算錯。注入方式跟 sink 同款
(`WithLog(tail, sink, clear)`),測試裡 clear 是 nil = 只清記憶體。

12 個 mutation 全數被抓(header 不 dim、條目不 dim、bar 不降階、不先問、
空 log 仍收鍵 / 仍出列、不清檔、檔案失敗仍清記憶體、清完又寫一則 log、
確認句不講代價、ClearLogTo 不清 / 清掉標頭)。

**追記:全是說明的 menu 也要量得出寬度。** 空 log 的 menu(以及 nav 的
menu)整份都是 header 列,而 `spaceMenu.view()` 量寬度時**跳過 header** ——
於是它量到 0,盒子縮成標題那麼寬,自己的字被切成
`nothing reco…`、底線的 legend 被切成 `j/k move  En`。一個回答「這裡沒事
可做」的盒子長得像壞掉,比沒有回答還糟。

修法是把「所有必須放得進去的東西」一起量:動作列(label 欄 + hint 欄)、
**單獨成列的 header**、標題、以及**底邊的 legend** —— legend 本來也沒被量
過,只是實務上每個 menu 都比它寬,所以一直沒現形。

同時把 legend 講誠實:沒有任何可執行列的時候,`j/k` 沒地方去、`Enter` 沒
東西可送,legend 只留 `Esc close`。跟 pty 那列 footer 同一條原則 —— 只說
現在還成立的鍵(§4.4)。

被否決的替代:沒事可做就**不開 menu**。Space 的承諾是「我在這裡能做什
麼」,「沒事」也是一個答案;按了什麼都沒發生,才是真的沒回答。

4 個 mutation 全數被抓(header 不量、legend 不量、legend 不誠實、legend
永遠只剩 Esc)。

### 11.14 tab [2] 的 [D]isconnect

`[H]ost`(當時叫 `[S]elect host`)一直是單向的:選得進去、退不出來。要讓一側回到「還沒選
host」,唯一的辦法是關掉整個 sshu。使用者要求補上反向的那一半 ——
**`[D]isconnect`,選了之後 panel 回到未選擇 host 的狀態。**

**它是重建零值,不是逐欄清空。** `*s = sftpSideModel{markedSet: …}` ——
因為要丟掉的東西全部屬於那台正在離開的主機:mark 是某個檔案系統上的路徑、
cwd 是它上面的一個位置、listing 是它回答過的話。逐欄清空的版本會在有人加
新欄位時漏掉那一欄,而漏掉的樣子是「這台主機的 marks 疊在另一台主機的檔案
上」。唯一被保留(且 +1)的是 `dialGen`:還在路上的 dial 不能在斷線之後才
落到這一側,而歸零反而會讓**下一次** dial 跟那個還在路上的撞號。

**傳輸進行中會被拒絕,而且說出去哪裡停。** 這個 tab 的每一筆傳輸都同時掛著
兩側(一邊來源一邊目的),所以關掉任何一側都會弄斷它 —— 而且是以「沒人喊
停的複製突然 EOF」的樣子弄斷。停傳輸的地方是 `[J]obs`,所以拒絕的話直接
講那件事,而不只是說不行。**`[H]ost` 吃同一條規則**(見 §11.15):
換 host 也是把這一側底下的檔案系統抽掉,對進行中的傳輸是一樣的傷害。

**只放在 `[1]`/`[3]` 兩個檔案 panel**(使用者指定):host 是檔案 panel 在顯示
的東西,marks panel 顯示的是 marks,在那裡放一個會把自己清空的動作會讀成
mark 動作。沒有 host 的那一側則連這一列都不存在 —— 沒有東西可以斷(同
`appliesTo` 的老規則,hotkey 跟著一起消失)。

**字母 `D` 在 `[M]anage` 的 `[1]`/`[2]` 是 Delete**,但 tab [2] 的刪除從來
不掛在字母 `D` 上(它是 `x`/`X`),所以這裡的 `D` 沒有主人,讀起來也不會
跟刪除搞混。`d` 仍然是半頁下捲 —— `hotkeyIndex` 精準比對大小寫,兩者不會
互相搶(同 `t`/`T`、`c`/`C` 的老規矩)。

7 個 mutation 全數被抓(掛到 marks panel、menu 少一列、marks 沒清、dialGen
沒進位、順手把另一側也斷掉、傳輸中不拒絕、拒絕不講去哪裡停)。

### 11.15 傳輸中凍結:menu 的第三種列,與會轉的 summary

`[H]ost` 與 `[D]isconnect` 做的是同一件事 —— **把這一側底下的檔案
系統換掉**。而這個 tab 的每一筆傳輸都同時掛著兩側,所以傳輸進行中做這件事
會把複製弄斷。使用者裁定:**傳輸中不允許換 host**。

**menu 多了第三種列的狀態:`disabled`。** 在此之前 menu 只有兩種答案 ——
列出來(能做)、不列出來(不適用,`appliesTo`)。這裡需要第三種:**這個
動作屬於這個 panel,只是「現在」不能做**。不列出來會教錯事 —— 使用者會學
成「這個 panel 沒有換 host 這件事」,以後就不會再來找;整列消失也讓「為什麼
不見了」沒有地方可問。所以它**留在原位、整列暗掉**,按下去會說明原因。

這跟 §11.13 的 nav dim 是同一個判斷:降一階代表「還在,只是不是現在」,消失
代表「不存在」。兩者不可混用。

**游標仍然可以停在暗掉的列上** —— 那正是你查明「為什麼是暗的」的方式。停上
去時游標條改用 app 既有的「有反白但不是活的」那組色(`baseHex` + `borderDim`,
就是 unfocused panel chip 與 §11.13 nav 游標用的那組),不用亮色條 —— 亮色條
是在承諾「按下去會跑」。

**拒絕不關 menu。** 拒絕不是 commit:被拒的那一列還在畫面上、還是暗的、還在
解釋自己,把 menu 收掉等於把答案一起收掉。訊息由 `transferBusy()` 一個地方
產出,所以裸熱鍵與 menu 裡的 Enter 講的是同一句話(§4.2 的老規矩:兩條路同一
個真相)—— 而且守衛就放在 `sftpKey`,兩條路都會經過那裡,不可能只擋到一條。

**summary 加了 spinner。** 凍結的原因必須在同一眼看得到,而右上角原本只有
`󰁥 3/7 · 42%` —— 大檔案卡在 99% 很久的時候,不動的數字看起來就是當掉。改成
**轉動的點在前、傳輸 glyph 在後**(`⠋ 󰁥 3/7 · 42%`),跟撥號那顆 spinner 同
一套 frame 與同一個理由(§11.x dial)。它吃的是 `xferTickMsg` —— 本來就是為了
重畫這一行而存在的 120ms tick,不新增計時器;傳輸結束、tick 停,summary 也
一併消失。

12 個 mutation 全數被抓(S/D 沒標 needsIdle、守衛不觸發、列沒標 disabled、
全部列都暗、閒置也暗、拒絕不講 [J]obs、render 不理 disabled、游標條沒
降階、summary 沒 spinner、spinner 不前進、summary 掉了傳輸 glyph)。

### 11.16 一列的 menu 不是 menu

沒有 host 的那一側,Space 會開出一個**只有 `[H]ost` 一列**的盒子。
使用者裁定拿掉那一層:**Space 直接開 host 清單。**

§A.1 對 Space 的承諾是「我在這裡能做什麼」。當答案只有一個的時候,先給一張
只寫著那個答案的清單、再要求按第二次 Enter 才拿到它,那一層沒有回答任何問題
—— 它只是把答案往後推了一格。(注意這跟 §11.13 的「沒事可做仍然開 menu」不
衝突:那裡的答案是「沒有」,而「沒有」需要被說出來;這裡的答案是一個具體
動作,說出來的方式就是做它。)

**條件掛在「這一側沒有 host」,不是掛在 panel 編號上。** 使用者指名的是
`[1]`/`[3]`,但沒有 host 的是那一**側**,它的 marks panel 同樣只有那一個動作
可做 —— 只改檔案 panel 會讓 `[2]`/`[4]` 變成全 app 唯一還會開出一列 menu 的
地方。

**走 `sftpKey(keySelectHost)` 而不是直接呼叫 `sftpSwitchHost()`。** 兩者今天
行為相同(沒有 host 的一側不可能有傳輸在跑,所以 §11.15 的守衛不會有差),
但前者讓「Space 在這裡 == 按 `S`」是定義上的事實,而不是兩份會各自漂移的實作
(§4.2 的老規矩)。這也是唯一一個沒被測試殺掉的 mutation —— 它殺不掉,因為
今天兩者確實等價;留著的是明天的保險。

4 / 5 mutation 被抓(捷徑不觸發、連上了也觸發、看錯側、每個 tab 都觸發;
第五個見上)。

### 11.17 還沒到齊的檔案:mark 欄的 spinner

傳輸把目的檔案**先建起來、再慢慢寫**。所以在複製途中,target 目錄的清單裡
已經看得到那一列了 —— 但它不是完整的檔案。使用者裁定:**這種列不能被
mark,而且要在 mark 欄用 loading 圖示標出來,傳完消失並刷新 target 目錄。**

**不能 mark 的理由不是「怕出錯」,是 mark 的語意。** mark 是「這個路徑是一個
你可以拿來操作的東西」的承諾,而半個檔案不是那個東西 —— mark 它、`[T]` 送
出去,遠端收到的是一個沒人告知過的截斷檔。所以是拒絕,而且說明原因
(`Still arriving — not all of it is here yet`),不是靜靜地沒反應。

**spinner 佔的是 mark 欄,不是新的一欄。** 那一格在這個狀態下**依定義是空
的**(不能 mark 就不會有 mark 圖示),而且傳輸途中長出一欄會把旁邊每個檔名
往右推 —— 一個「正在動」的提示不該讓不動的東西也跟著動。兩者同時成立時
(已經 mark 的檔案被傳輸覆寫)**arrival 贏** —— 「還沒到齊」比「你標過它」
急,而且那個 mark 現在也已經不能用了。

**「正在寫」的集合是每個 running job 的當前那一項**(`transferJob.cur`,
`atomic.Pointer[string]`,寫在 `CopyItem` 前、`defer` 清掉)。這個定義剛好
就是「存在但不完整」的那一組:還沒開始的檔案根本還不存在,寫完的則是完整
的。判定同時涵蓋**祖先目錄** —— 傳一整個目錄的時候,看得見的那一列是目錄
本身,正在寫的檔案在它下面三層;只比對完整路徑的話,這個功能在大家實際
用 marks 的場景裡是看不見的。前綴比對必須帶 `/`,否則 `/dst/blob` 會把
`/dst/blobby` 一起吃掉。

**view 是被「交」進去的,不是自己去拿的。** `sftp.view(arrivals)` —— sftp
model 不認識 transfer engine,同 `status(xfer string)` 的老安排。

**刷新綁在「job 結束」那一刻,不是綁在畫格上。** `logFinishedTransfers()`
本來就用 `logged` 旗標精準抓到「這一輪剛結束」,現在讓它回傳這件事,結束時
才 re-list ——「spinner 停」與「清單補上」是同一幀。綁在 tick 上會變成每
120ms 對著正在忙的連線多送一次 `List`;watch loop 之所以是 2 秒一次、而且
mtime 沒動就不重列,就是這個原因。

13 個 mutation 全數被抓(engine 不公布 / 不清除當前項、結束了仍回報、只比對
完整路徑、前綴少了 `/`、拒絕不觸發 / 擋掉全部、列不轉、spinner 用錯色、mark
壓過 arrival、結束不刷新、每格都刷新、還在跑就回報結束)。

### 11.18 [R]efresh —— 對 poll 的懷疑要有一個按鍵可以解

watch loop 兩秒一次、而且**只有目錄自己的 mtime 動過才重列**。mtime 不是
承諾:檔案系統可以在底下改了而不動它,有些 server 還把它捨進到秒。所以
「我知道有東西變了,但畫面沒變」這個懷疑,poll 本身解不掉 —— 得有一個鍵
無條件重讀。那就是 `[R]efresh`。

**它是 panel op(大寫),不是 item op。** 它作用在整個目錄,不是游標那一列
—— 大小寫在這個 tab 就是範圍的宣告(§6.2)。`r` 仍然是 Rename,
`hotkeyIndex` 精準比對大小寫,兩者不會互搶。(附錄原本把 Rename 寫成大寫
`R`,那是舊筆誤 —— 程式從來都是小寫 `r`;這次一併改正,否則同一張表裡 `R`
會有兩個意思。)

**它不吃 §11.15 的凍結。** 讀是傳輸進行中唯一仍然安全的事,而且傳輸中正是
最想再看一眼清單的時候。凍結的是「換掉這一側底下的檔案系統」,不是「看它」。

**toast 要報數量。** 重讀之後什麼都沒變的畫面,跟按鍵沒生效的畫面長得一模
一樣 —— 而這個鍵被按下去的時機,正好就是使用者已經在懷疑畫面的時候。所以
它說 `Refreshed — 12 items`,而不是默默地什麼都不說。

**游標跟著名字走,不是跟著索引。** 重讀後有東西排到前面去,索引會指向別
的檔案;`applyWatch` 記的是名字,所以你不會因為看一眼而失去位置。

8 個 mutation 全數被抓(表裡沒有 R、掛到 marks panel、被傳輸凍結、沒真的
重列、重列了另一側、成功不出聲、失敗仍報成功、游標沒跟著名字)。

### 11.19 pty 的歷史 —— 捲不上去是因為從來沒存過

`[3]` 的格子捲不動。看起來像「鍵沒接」,其實不是:**那些行已經不存在了。**

vt10x 是一塊固定大小的 grid,它的 `scrollUp()` 把列往上搬、然後 `clear()` 掉
頂端讓出來的那幾列。離開畫面的內容不是被收到某處,是被抹掉。`render()` 只是
把當下的 grid 抄出來,所以無論接上什麼鍵,都沒有東西可以捲回去。

要補的不是鍵,是**歷史本身**:唯一還來得及留下一行的時刻,是它進 emulator
之前。`readLoop` 每讀到一塊 bytes,寫進 emulator 的同一個 mutex 區塊裡,也
把它切成行存進一個 10000 行的 ring。

**存 raw,不存 strip 過的。** 會想捲回去看的行,多半正是有顏色的那行 ——
紅色的錯誤、highlight 過的 diff。把顏色洗掉的 scrollback 回答的是另一個問題。
`ansi.Strip` 只用來**判斷**這行值不值得留(shell prompt 重繪是十幾個控制序列
配零個字形,存進去只會得到一堆看起來空白的列),留下來的是原字串。

**`\r` 要等下一個 byte 才知道它是什麼。** 遠端送的是 CRLF,而進度條用的是
孤立的 `\r` 原地重畫。看到 `\r` 就當換行,每一行會被存兩次;看到 `\r` 就當
重畫,CRLF 的機器上一行都存不到。所以掛一個 `pendingCR`,由下一個 byte 決定。

#### PgUp / PgDown 是借來的,不是拿走的

這兩個鍵原本整個轉送給遠端,`less`、`vim`、`man` 都靠它們翻頁。全域搶走會
在最需要它們的地方把它們弄壞。

判準是 **alt screen**:全螢幕程式進場時會切過去,而它切過去這件事本身就是
「我自己會翻頁」的宣告。alt screen 亮著,鍵是遠端的;純 shell 輸出不會翻頁,
那正好就是需要有人提供捲動的時候,而能提供的只有 sshu。

**alt screen 期間也不 capture。** vim 每一次按鍵都重畫整個視窗,照單全收會
把幾千格 vim 的畫面推過 ring、把真正值得捲回去的 shell 歷史沖掉。真實終端機
在 alt screen 期間同樣凍結自己的 scrollback,理由一樣。(kbu 的 `PtyView` 有
scrollback 但沒有這一條 —— 它的 Alterm 幾乎不開全螢幕程式,踩不到;sshu 是
ssh 工具,vim 是日常。)

**已知代價:**不切 alt screen 的 pager(busybox `less`、某些嵌入式環境)會
被搶走 PgUp。判準只有一個信號可用,而這是最好的那一個;誤搶的後果也有出口
—— 打任何一個字就回到 live。

#### 三件被否決的事

- **滑鼠滾輪。** 要開 `tea.WithMouseCellMotion()`,而 alt screen 下滑鼠事件
  被程式攔走,終端機原生的**選字複製**就沒了(要按住 Option 才選得到)。對一個
  ssh 工具來說「把畫面上的輸出複製走」是高頻動作,拿它換捲動是虧的。
- **獨立的歷史檢視模式(tmux copy-mode 式)。** 語意零歧義、還能加搜尋,但
  看一眼歷史要先進一個模式,比按一下 PgUp 慢一步。這個 tab 的動作是「瞄一眼
  剛剛刷過去的東西」,不是「研讀」。(使用者當場定案。)

  > 後來 sshu 還是長出了一個模式(§11.33 的 `Alt+v`),但它**不是**為了看歷史,
  > 是為了**複製**。這兩件事對「先進一個模式」的容忍度不一樣:瞄一眼是反射動作,
  > 複製本來就是慎重的動作。看歷史仍然是一個 `PgUp`,沒有模式。
- **`Home` / `End` 跳到兩端。** 在 shell 裡它們是**行首 / 行尾**,是正在打字時
  的即時編輯手勢。拿它換「跳到歷史的兩端」是壞交易。回到 live 有兩條路:
  `PgDown` 按到底,或直接打字。

#### `clear` 之後還捲不捲得回去,由遠端決定

`\x1b[3J`(明確要求清除 scrollback)和 `\x1bc`(RIS 全重置)清空歷史;
`\x1b[2J`(清除**畫面**)不清 —— 捲過 `clear` 看到之前的東西,正是 scrollback
存在的理由。`Ctrl+L` 走的是 shell 自己的 clear-screen,只送 `2J`,歷史留著。

**送不送 `3J` 由 `TERM` 決定,不是由作業系統決定。** `clear` 這支程式問的是
terminfo 的 `E3`:`xterm-256color` 有(`E3=\E[3J`),`linux`、`tmux-256color`
沒有。而 sshu 給 pty 子行程的 `TERM` 是釘死的 `xterm-256color`
(`internal/ui/session.go`),ssh 又把它帶去遠端 —— 所以**在 sshu 裡,遠端打
`clear` 一定送 `3J`、一定清空這個 session 的歷史**,不分平台。

> 這一段原本寫的是「macOS 的 `clear` 兩個都送、Linux 的只送 `2J`」,那是錯的:
> 同一台機器換個 `TERM` 就換個答案,而 sshu 根本不讓它換。順帶一提,「跟你自己
> 的終端機同一個行為」這個理由也因此比原本弱 —— 使用者在 tmux 裡
> (`TERM=tmux-256color`)打 `clear` 是**不會**掉歷史的,sshu 卻一定掉。

**理由換掉,行為維持清空**(使用者當場定案)。撐住這件事的不是「跟你的終端機
一樣」,而是**已經有一個不破壞歷史的清畫面手勢**:`Ctrl+L`(shell 的
clear-screen)和 `tput clear` 都只送 `2J`。兩個手勢兩種意思 —— `clear` 是
「忘掉」,`Ctrl+L` 是「挪開」。被否決的兩條:只讓 `\x1bc`(RIS)清空;以及
兩個都不清 —— 那樣 `reset` 之後還捲得到 reset 前的畫面,語意上說不過去。

#### 一次 read 裡,erase 只清掉它前面的東西

`clear && ls` 是同一個 chunk:erase 之後緊接著就是 `ls` 的輸出。原本的寫法
`bytes.Contains` 命中就 `return`,整塊都不進歷史 —— 那次 `ls` 的輸出從來沒被
存過,panel 於是誠實地回報「沒有歷史可捲」,而使用者看到的是「`clear` 把
`PgUp` 弄壞了」。**清空的是它前面的東西,後面的是新歷史。**

所以改成找**最後一個** erase 結束的位置(`eraseEnd`),清空之後從那裡繼續
capture。要最後一個,因為在它前面的都已經被要求丟掉了;取第一個會把兩次
`clear` 之間那段又收進來。

已知邊界:`\x1b[3J` 剛好被切在兩次 read 中間就認不出來 —— 4 個 byte 落在 4096
的交界,罕見,沒有為它加跨 chunk 的狀態機。

#### 正在看歷史,要看得出來

格子的 title 補上 `󰋚 N`(nf-md-history + 往回幾行)。理由跟 `hasSpoken` 那條
一模一樣:**一個在放歷史的格子,和一個遠端已經沒聲音的格子,是同一張靜止的
畫面** —— 只有一個是問題。用往上的箭頭是第一個念頭也是錯的念頭:箭頭說的是
「畫面往哪個方向動了」,而必須毫無疑義的是「現在螢幕上是什麼」——過去。

marker 接在名字**被截斷之後**,不是之前:格子窄的時候,該活下來的是狀態。

footer 也只在**真的能捲**的時候才提供這兩個鍵(有 alt screen 或歷史還不滿一頁
都不提)。一個按下去不會動的鍵,第一次按就會被讀成壞掉。

**但 footer 那份是 contextual 的,而它出現的地方正好是問不到 help 的地方。**
所以 `?` help 的 ssh grid 區塊也列這兩個鍵 —— 跟三個 `Alt` 和絃並排。那份清單
不挑時機、不看 `canScroll()`:它回答的是「這個 app 有什麼」,不是「現在按下去
會發生什麼」。兩種揭露問的是不同的問題,所以條件也不同(§A.2 追記)。

16 個 mutation 全數被抓(readLoop 不 capture、alt screen 照捲、`3J` 不清、
`2J` 也清、空白行也存、ring 不設上限、孤立 `\r` 當換行、`\r` 不重設、打字不回
live、scrollPage 不 clamp、render 忽略 offset、render 不 re-clamp、alt screen
下搶鍵、鍵完全不接、title 不說、footer 無條件提供)。其中會捲的那半段是靠
**真的 fakeSSH 吐 200 行、真的按鍵、真的讀 `View()`** 抓的 —— 只餵合成資料的
測試,把 `readLoop` 裡那一行 `capture` 刪掉照樣全綠。

---

### 11.20 `Ctrl+C` 在遠端手上時是遠端的

在格子裡跑一個指令、想中斷它、按下 `Ctrl+C` —— **整個 sshu 沒了**,連同其他
每一條 session,而且連 `q` 會問的那句確認都沒有。

原本的理由寫在 §7 的 editor 那段:「讓它在這一個 PTY 裡意思不一樣,緊急出口
就不再是緊急出口了」。這個推論的方向反了。`Ctrl+C` **在 shell 裡本來就有意思**
—— 它是全世界最反射性的一個鍵,手比腦快。把它接到「殺掉整個 app」上,不是讓
緊急出口保持一致,是**把最容易誤按的鍵接到最貴的動作**。

所以在遠端手上時(session 的格子)和在編輯器手上時,`Ctrl+C` 原封不動送過去。
其他每一個地方它仍然是硬退,連浮層底下都是。

**沒有東西因此被困住:**

- `Alt+Esc` 是 sshu 自己處理的,不會送給遠端,所以遠端再怎麼卡死它都有效 ——
  收回鍵盤,`Ctrl+C` 在另一邊立刻又是它自己。footer 從頭到尾都在宣傳這個鍵。
- app **外面**的出口沒動:外部的 SIGINT / SIGTERM / SIGHUP 仍然經過 procreg,
  一個子行程都不留(§7)。
- 編輯器那邊 `Alt+Esc` 本來就是「放棄這次編輯」,同一個鍵、同一個意思。

代價是硬退在 pty 內變成兩個鍵。那正是它該有的距離:離開遠端這件事本身,本來就
是一個動作。

4 個 mutation 全數被抓(session 內改回硬退、硬退整個消失、editor 內改回硬退、
editor 的 `Alt+Esc` 失效)。

---

### 11.21 tab 鍵搬回裸字母 —— 和絃買的東西已經不要了

`Alt+p/f/s` 是 v0.2 的決定,它買的是**一件很具體的事**:鍵盤在遠端手上時也能切
tab。裸鍵做不到 —— 在 pty 裡每一個裸鍵都屬於對面。

現在不要那件事了(使用者裁定):**pty 內就先 `Alt+Esc` 出來,再做別的**。切 tab
本來就是「離開這個遠端去看別的東西」,而離開這個遠端本身就是一個動作,它有自己
的鍵。和絃於是只剩下成本 —— 三個 app 裡最好按的鍵,永遠得多壓一個修飾鍵。

所以三個 tab 改成**一個 shift 過的裸字母**:`M` / `F` / `S`。

#### 三件事一起動,因為它們是同一個約束的三面

| | 之前 | 之後 | 為什麼 |
|---|---|---|---|
| tab | `Alt+p` `Alt+f` `Alt+s` | **`M` `F` `S`** | 裸鍵不必壓修飾鍵 |
| `[Alt+p]reference` | Preference | **`[M]anage`** | 這個 tab 裝的是 hosts / credentials / logs —— **記錄,不是設定**。名字一直答錯了問題,順手改對;`P` 也因此不必再爭 |
| `[S]elect host` | `S` | **`[H]ost`** | `S` 給了 SSH tab。而這一列本來就是「這一側的 **host**」,動詞搬進 hint(`switch this side — a directory, or a saved host`) |
| `[P]rogress` | `P` | **`[J]obs`** | 那個視窗列的是一件件工作、可以逐條取消,不是一根進度條。`P` 就此完全空出來 |

`S` 是死結,這點值得寫下來:ssh tab 不可能改名(它就叫 ssh),所以 `S` 要嘛給
tab、要嘛給 Select host,**一定有一個要讓**。改 Preference 的名字解不掉它 ——
那隻手解的是 `P`。

改完之後全 app 已用的大寫是 `A C D E G H J R T V X` 加 tab 的 `M F S`,`P` 空著。

#### 亮的兩段沒有第二段了

tab 帶開頭那個**固定亮的 `[Alt]` 鏈頭**(§11.8)一起拿掉。它存在的理由是「亮著
的兩段合起來就是你按的和絃」;裸鍵沒有和絃可拼,留著就只是一個**不指向任何
surface 的亮塊**,跟真正在說「你在這裡」的那一段搶注意力。現在恰好一段亮。

窄階退化跟著變好:`[M] [F] [S]` 比 `[Alt] [p] [f] [s]` 又短一截。

#### 「有人在打字」必須是一個問題,問一次

裸字母當全域鍵,踩到的第一件事是**打字**。而掃描的時候發現這件事**已經壞了**:

```
搜尋中按 V  →  query = ""，splash 跳出來
搜尋中按 ?  →  query = ""，help 打開
```

`V`(彩蛋)和 `?`(help)各自帶著自己的守衛 `!m.textFloat() && !m.textPage()`,
而 `textFloat()` **只涵蓋浮層** —— 兩個 `/` 搜尋是 panel 內的 `filtering` 旗標,
不在裡面。每個鍵各記一份例外清單,結果就是每一份都漏了同一塊。

改成一個判準:

```go
func (m AppModel) typing() bool {
	if m.textFloat() || m.textPage() { return true }
	if m.popupOpen() { return false } // 浮層拿著鍵盤,底下的 panel 不算在被打字
	if m.tab == tabFT && m.sftp.cur().filtering { return true }
	return m.tab == tabPref && m.pref.item == prefHosts && m.hosts.filtering
}
```

tab 鍵、`V`、`?` 全部問它。新增一個全域鍵時只要記得問這一個問題,而不是記得補
四個地方 —— 那正是原本失敗的方式。

`popupOpen()` 那條 early-return 有順序意義:**它排在 `textFloat()` 後面**。form
開著時 `?` 是字元(form 在被打字),confirm 開著時 `?` 開 help(§A.2 保證 help
從任何 surface 都到得了)。

#### 鍵的位置也搬了

和絃的分派點在 `handleKey` 最上面 —— 它必須在那裡,才能搶在 pty 之前。裸鍵**不
該**在那裡,而且不能只靠加一個 `!m.inPty()` 守衛:那是把「pty 優先」重寫成一句
容易漏掉的條件。所以整段移到 **pty 分支下面**,跟 `?` / `V` 同一區。順序因此讀
得出來:

```
遠端 > 浮層 > 全域裸鍵(問 typing()) > 正在打字的 panel > panel 動作表
```

#### footer 不再提它

pty 內的 footer 拿掉了 `alt+P/F/S tab` 那一格 —— 那個鍵在那裡已經不作用,而**一個
被宣傳在它不生效之處的鍵,比一個沒人提的鍵更糟**。Operation 頁的 footer 同理。
其餘地方改成 `1-4 M/F/S panel tab`。

13 個 mutation 全數被抓(小寫也切、鍵搬回 pty 之上、無視浮層、`typing()` 漏掉
sftp 搜尋 / hosts 搜尋 / 浮層與 text page、`?` 與 `V` 不問 typing、鏈頭復活、
`H` 改回 `S`、`J` 改回 `P`、pty footer 提供 tab 鍵、panel footer 不再揭露)。

---

### 11.23 確認框的 Enter,不是那一列的 Enter

`[1]` 上按 `D` 開 Duplicate 的確認框,按 Enter 之後 —— 鍵盤直接跳進了新開的
那一格 pty。

問題不在「跳」,在**憑什麼跳**。使用者按下的兩個 Enter 意思不一樣:

- **在一列上按 Enter** = 「帶我進去這條 session」。這是 `[1]` 的 `Enter` 動作
  (`Open`),它就是為此存在的。
- **在確認框上按 Enter** = 「對,做那件事」。那件事是**複製一條連線**,不是
  「進去它」。

把兩者接到同一個結果,等於讓確認框的 Enter 偷偷多做了一件沒被要求的事。使用者
的裁定就是這句話:**popup 的 enter 不算對 item 按下 enter**。

所以規則是:**在 `[1]` 上,只有直接對一列按 Enter 會把鍵盤交給遠端。** 其他
每一個操作 —— Duplicate、Close、Display —— 做完都留在清單上。

#### 留在清單上,不是什麼都沒發生

`connect()` 本來就把新 session 接在清單末端、並把游標移過去。所以 Duplicate
完成的那一刻,畫面同時有三件事在說它成功了:

1. 游標**落在新的那一列**上(cursor bar 移動)
2. 那一列對應的格子出現在網格上,外框是**游標的白色回聲**(§11.22)
3. 膠囊列右側從 `1 live session` 變成 `2 live sessions · 2 on grid`

沒有另外加 toast:螢幕已經用三個 channel 說完了,再加一句是重複。

#### 從 hosts 表格連線**仍然**直接進去

那不是同一件事,也不在 `[1]` 上。在 `[M]anage` 的 hosts 表格按 Enter,要的就是
「到那台機器上」—— 中途停在一張清單只是路上多一個按鍵(§7.1 context shift)。

實作上 `startSession` 因此多一個參數 `land sshPanel`,每個呼叫點都得把落點講
出來,而不是由函式自己猜。兩個呼叫點,兩個不同的答案,都寫在呼叫的那一行。

5 個 mutation 全數被抓(Duplicate 又跳進 pty、hosts 連線不再進 pty、
`startSession` 忽略落點、`connect` 不再移動游標、列上的 Enter 不再進 pty)。

---

### 11.24 `[1]` 的清單自己會捲了

§11.23 讓 Duplicate 把游標留在新的那一條上,並且**把「游標移過去」當成主要
回饋**。然後就發現那個回饋在清單一長就看不見:`[1]` 的視窗 `topSess`
**從來沒有跟著游標走過**。`clampCursors` 只把它夾進合法範圍,`handleListKey`
只動 `curSess` —— 所以 j/k 走到第七、八條,游標就消失在框底下,再也不回來。

同一段程式還藏著第二個 bug:

```go
func (m sshModel) listRows() int { return max(1, m.sessionsH()-2) }   // 行數
m.curSess = moveCursor(m.curSess, len(m.sessions), k, m.listRows())    // 當成項數用
```

`listRows` 回的是**框的行數**,而 `moveCursor` 的 page 參數要的是**項數**。
一項佔兩行,所以 `u`/`d` 名義上跳半頁、實際上跳了一整頁。

#### 不能除,要數

直覺的修法是 `innerH / 2`。那是錯的,因為**列高不固定**:

```go
lines := wrapText(s.host.User+"@"+s.host.Host, nameW)
if last := …; dispW(last)+dispW(tail) <= nameW {
    lines[len(lines)-1] = last + tail      // 一行
} else if dispW(tail) <= nameW {
    lines = append(lines, tail)            // 兩行,或更多
}
```

位址長就 wrap,而 `:port #N` 的尾巴**不拆**(拆過的 port 是另一個號碼,不是
一個縮短的號碼),所以它寧可自己佔一整行。同一個 `[1]` 裡,`root@jump:22` 是
一行、`ec2-user@bastion.eu-west-1.compute.internal:22 #2` 是三行。

所以 `listRows(top)` 改成**從 top 往下累加真實列高,數到裝不下為止**,而
`revealCursor` 從**游標往回**累加 —— 往回走才找得到「讓游標整列完整留在畫面
上的最高 top」。半截在框外的那一列,是一列讀不到 port 的列。

行高的唯一真相是 `listItem` 自己,所以兩邊都呼叫它來數,而不是複製一份 wrap
規則出來(複製出來的那份會分歧)。

#### 跟隨掛在 `clampCursors` 上

因為每一條會動到游標的路徑本來就都經過它:j/k/u/d/gg/G、resize、`connect`
新增一條、`reap` 收掉一條。掛在那裡,「視窗跟著游標」就是一條規則、一個地方,
而不是每個呼叫點要各自記得的事。

側欄收起來的時候(鍵盤在 pty、或 narrow)什麼都不捲 —— `listInner` 回報寬度
為 0,那不是「一個很窄的清單」,是**沒有清單**。

7 個 mutation 全數被抓(不跟隨、只往下捲、只往上捲、容許半截列、折疊時照捲、
`listRows` 改回除法、`listRows` 改回回傳行數)。

> **v1.2.0 追記:一半的複雜度後來消失了**(而 §11.32 又把常數從 1 換成 2,做法不變)。 上面那套「往回量、不能除」的邏輯
> 存在的唯一理由是**列高不固定**。§11.28 讓一列永遠是一列(位址縮減而不折行),
> 於是 `listRows` 變回「框有幾行」、`revealCursor` 變回 sftp 清單也在用的
> `scrollTo`。留下來的是**跟隨本身**:那才是這一節真正修好的東西。

---

### 11.25 zoom —— `Alt+Enter` 進去,`Alt+Esc` 一次剝一層

session 一多,右側就擠。那本身是對的(網格就是要同時看得到),但**專心處理
其中一格**的時候需要它整片。

#### 鍵位:為什麼不是 `Alt+z`

`Alt+z` 是最好記的(tmux 的 zoom 就是 `prefix z`),但使用者的顧慮是它**可能**
撞到遠端上跑的東西 —— 而在 pty 裡,任何裸鍵都是遠端的,所以 zoom 鍵一定得是
Alt 組合,也就一定會跟遠端的 Meta 綁定競爭。

先把事實查清楚。readline / emacs 綁掉的 Meta 鍵是:

```
M-b M-f M-d M-t M-u M-l M-c M-y M-. M-_ M-< M-> M-n M-p M-r M-~ M-# M-* M-? M-= M-\ M-BackSpace
```

`M-z`、`M-m`、`M-o`、`M-Enter` 都不在裡面。四個都實測過能到得了 bubbletea
(`Alt+Enter` 特別值得測:`ESC` 後面接的是控制字元 `\r`,不是一般 rune):

```
type=13 alt=true str="alt+enter"
type=-1 alt=true str="alt+m"     (KeyRunes)
type=-1 alt=true str="alt+o"
type=-1 alt=true str="alt+z"
```

使用者選了 **`Alt+Enter`**。它有兩個別人沒有的好處:

1. **跟 `Alt+Esc` 天然成對。** `Enter` / `Esc` 本來就是 sshu 的一對 core key
   (確認 / 取消),加上 Alt 就是這一對在 pty 層的版本:**進去 / 出來**。
   使用者要的「第一次 Alt+Esc 離開 zoom」正好讓它們讀成一組。
2. **不花掉任何字母。** 字母空間才剛因為 tab 鍵擠過一輪(§11.21)。

已知風險:Windows Terminal 把 `Alt+Enter` 當全螢幕切換,某些 Linux WM 拿它開
終端機。兩者都是**本地層**,會在 sshu 之前吃掉它 —— 跟 `Alt+digits` 當年被
AeroSpace 攔掉是同一類問題(§11.6)。sshu 是 unix-first,主流的 iTerm2 /
Alacritty / kitty / GNOME Terminal 都沒有綁。

#### `Alt+Esc` 一次剝一層

zoom 是使用者自己疊上去的一層,所以出去的鍵**一次拿掉一層**:

| 按第幾次 | 從哪裡 | 到哪裡 |
|---|---|---|
| 一 | zoom 中 | 回到網格,鍵盤**仍在那一格** |
| 二 | 網格中 | 鍵盤回 `[1]`,側欄回來 |

按一次跳兩層,是一個「出口」鍵不再可預測的方式。

#### 只有一格時,這個鍵是遠端的

一格已經填滿網格,zoom 什麼都不會改變。而**一個按下去看不出變化的鍵,第一次
按就會被讀成壞掉**(同 §11.19 的判斷)。所以 `canZoom()` 為 false 時 sshu
**不攔截**它 —— `Alt+Enter` 原封不動送給遠端,跟任何其他和絃一樣。footer 那格
也跟著不出現。

#### zoom 只在鍵盤在格子裡的時候存在

`setFocus(p != panelPty)` 直接清掉它。理由:zoom 講的是「我正在用的這個終端機
要整片」,而鍵盤回到 `[1]` 之後就沒有「正在用的終端機」了 —— 側欄還會回來、
網格重新排版,zoom 這個狀態失去了它的對象。

#### 換格保持 zoom

`Alt+方向鍵` 在 zoom 中照樣走,而且**留在 zoom 裡**。tmux 的 zoom 在切 pane 時
會自動取消;這裡不那樣做,因為 zoom 進去是為了看清楚一個終端機,而切到下一個
終端機通常是為了同樣看清楚它。走的規則不變(仍然是網格上的空間移動),只是網格
暫時只畫一格 —— 佈局還在,退出 zoom 就看得到自己在哪。

實作上 `moveCell` 因此要在移動後重新 `applyGeometry()`:zoom 中「哪一格佔滿
螢幕」就是幾何本身,新接手的那一格得先知道自己的新尺寸才畫得對。不 zoom 時
這一行是免費的 —— `applyGeometry` 只對數字真的變了的 session 發 SIGWINCH。

#### 不需要 marker

一個只畫著一個終端機的網格,本身就是它自己的狀態顯示。這跟 §11.22 的
scrollback marker 不同:那裡「在看歷史」和「遠端沒聲音」是同一張靜止畫面,
非說不可;這裡沒有任何別的狀態長得像它。

footer 仍然變:它說的是**現在按 `Alt+Esc` 會拿掉哪一層**(`leave zoom` /
`leave pty`),以及 `Alt+Enter` 現在是 `zoom` 還是 `unzoom`。

10 個 mutation 全數被抓(不 zoom、無條件吞掉這個鍵、一格也能 zoom、zoom 時
照畫全部格子、zoom 的格子不 resize、`Alt+Esc` 一次剝兩層、離開網格不清 zoom、
換格不重算幾何、footer 不說層、footer 無條件提供)。

---

### 11.26 `Close all sessions` —— 一列**故意**沒有熱鍵

`[1]` 的 Space menu 多一列 panel operation:`Close all sessions`,一次關掉全部。
使用者指定**不給它熱鍵**。

#### 這不違反 §4.3,它是 §4.3 的另一半

§4.3 說的是「每一個 letter hotkey 都必須是 menu 的一列」,**反過來不成立**。
menu 裡本來就有沒有字母的列 —— `Enter` 的 Open、`Tab` 的 Display,它們的鍵名
寫在 hint 欄而不是 bracket 裡(§4.4)。

而這一列連 core key 都沒有,是因為**它不該快**:

- 關掉每一條連線是**破壞性且罕見**的。
- 給它一個字母,那個字母就會在某天被一隻只是想捲清單的手按到。`[1]` 上已經
  有 `C`(關一條)—— 再放一個相鄰語意的大寫字母,誤觸的代價是全部。
- menu 是慢路徑:開 menu → 走到那一列 → `Enter`,三個動作。對這件事,三個
  動作正好。

#### 機制:key 的形狀就是宣告

```go
const keyCloseAll = "close-all"
```

不需要新的旗標,兩件事自己成立:

- `bracketHotkey` 對 `len(key) != 1` 直接回 label —— 這一列畫出來是純文字,
  旁邊的 `[C]lose`、`[D]uplicate` 仍然戴著 bracket。**標記是契約**(§4.4):
  沒有 bracket 就是「沒有鍵能按動它」,而那正是事實。
- `hotkeyIndex` 是完全比對加大小寫 fold,而**沒有任何一次按鍵會被拼成
  `"close-all"`**。`c`、`C`、`a`、`l`… 全部打不到它(`c` 會 fold 到 `[C]lose`,
  那是對的那一列)。

理論上的縫:bubbletea 的 `KeyRunes` 可以一次帶多個 rune(**貼上**),所以在
`[1]` 貼上字串 `close-all` 會命中。這跟 sftp 的 host picker 用 `"@" + name`
當 key 是同一個性質的縫,而這裡多一層保護:**它還會問**。

#### 它是 panel op,而且沒有 session 時整個消失

它作用在整張清單,不是游標那一列 —— 所以歸在 `panel operation` 區,跟
`[C]lose`(關游標那一條)分開。tab `[2]` 的大小寫分工說的是同一件事,只是這裡
沒有字母可以分。

沒有 session 的時候,這個 panel 的**每一列**都沒有對象 —— 包含這一列。原本
「沒有 session 就沒有 item 動作」的判斷因此提到迴圈開頭,對 panel op 一併成立。

#### 只停,不收屍

`doCloseAll` 只呼叫 `stopAll()`,剩下的交給 reaper 在下一個 tick 處理 —— 它才
是「一條 session 結束了」的單一真相:記進 app log、帶著遠端自己說的那句話、
把格子從網格上拿掉。在這裡再做一次,等於有兩條路要對「結束」的定義達成共識。

確認框把數字放進問題裡(`Close all 2 sessions?`),因為「close all」在 1 條和
9 條的時候讀起來是兩件事。

7 個 mutation 全數被抓(給它字母、從 menu 拿掉、掛進 item 區、不問就做、
問題裡沒有數字、commit 之後什麼都沒關、沒有 session 時照樣提供)。

---

### 11.27 log 的折行:別人的文字沒有結構可以尊重

使用者回報 `[2] Logs` 的折行「太糟糕」。一則連線失敗的記錄長這樣:

```
12:00:33 ERR  10.20.12.31 · ❯   other-
              service/ (放 other service
              檔案)Last login: Thu Sep  3
              12:00:27 2026 from 10.20.15.
              1 ubuntu in ⊕ pc12031 in ~ ❯
```

每一行都提早斷,右邊留一大片空白 —— panel 越窄,浪費的比例越大,最後看起來
像文字從一條窄縫裡擠出來。

#### 兇手是一條為別的東西寫的規則

`wrapText` 有一條「偏好在分隔符斷」的規則,而它的註解就寫明了服務對象:

```go
// Prefer a separator in the last third, so "db-replica-tokyo-ap-" breaks
// after the dash rather than inside "ap".
```

那是給 `[1]` 的 **host 名**用的:短、結構已知、分隔符稀疏,在破折號後斷確實
比較好讀。

log 是**別人機器上的任意輸出**,而它恰好密集地由那些字元組成 —— 一個 IP 是
三個點,一條路徑是一串斜線。於是每一行都在最後 1/3 找到一個 `.` 或 `/` 就斷,
`10.20.12.31` 被切成 `10.20.12.` 和 `31`,每行丟掉三分之一的寬度。

所以 `wrapText` 拆成兩個:

| | 用在 | 規則 |
|---|---|---|
| `wrapText` | sshu 自己的字(host 名) | 偏好在分隔符斷 |
| `wrapPlain` | **別人的字**(app log) | 填滿每一行 |

而 log 的整個存在理由是「讓人讀到遠端說的每一個字」(§7.1 記整個最終畫面),
那就該用滿每一行。

#### 順手修掉的三個

1. **訊息欄的下限讓行比 panel 寬。** `msgW := max(8, innerW-prefixW-1)` 的
   floor 是在防負數,但防錯了方向:panel 窄於 24 欄時,15 欄的時間戳 gutter
   加 8 欄訊息 = 23,比 panel 還寬。外層的 `fitLines` 於是把尾巴裁掉 ——
   **字不見了,而版面看起來是好的**,那是更難發現的壞法。
2. **gutter 該讓位。** 一個 20 欄的 panel 花 15 欄當縮排、留 4 欄給文字,
   行沒有超寬,但那正是使用者抱怨的樣子。門檻是「**gutter 不該是比較寬的那
   一半**」(`msgW < prefixW`):再窄就不縮排,靠第一行的時間戳標示 entry 的
   起點。寬 panel 仍然縮排 —— 那時它讓一則多行的記錄讀成一個 block。
3. **`wrapAt` 會無限迴圈。** 一個寬度 2 的字放不進寬度 1 的欄位時 `cut` 算出
   0,而 `rs = rs[0:]` 不前進。這是既有的缺陷,只是原本的 floor 8 剛好蓋住了
   它;把 floor 拿掉就掛住。現在讓它硬塞一個字並溢出一欄,由呼叫端 clip ——
   **少一欄勝過整個 app 停住**。

10 個 mutation 全數被抓。其中兩個第一輪存活,而它們指的是同一件事:
**「行沒有超寬」不等於「行是可讀的」**。gutter 不讓位時,行寬剛好合格、內容
每行四個字;msgW 變成 0 時,行寬合格、內容一片空白。兩條測試因此改成量
**gutter 與文字的比例**,而不是量行寬。

---

### 11.28 一列就是一列:`<glyph><user>@<host>:<port> #<N>`

> **這一節的結論後來被 §11.32 換掉了。** 一個 item 現在固定**兩行**、而且沒有
> `#<N>`。留著是因為它推導出來的兩樣東西活了下來:`fitUserHost`(位址縮減而不
> 折行)還在,而「item 高度固定」這個目標也還在 —— 只是常數從 1 變成 2。下面
> 關於 `#<N>` 為什麼不能截、以及左欄為什麼是 30 的推導,請對照 §11.32 讀。

`[1]` 的一列原本會折:位址太長就換行,`:port #N` 的尾巴再擠不下就自己佔第三列。
使用者裁定改成**一列固定說完整件事**,而可以讓步的只有 `<user>@<host>`。

#### 誰讓步,誰不讓步

| | 讓步? | 為什麼 |
|---|---|---|
| glyph | 否 | 它是「這條 session 在不在網格上」的唯一標記(兩種形狀,不是兩種顏色) |
| `:<port>` | **否** | **截一半的 port 是另一個號碼,不是一個短一點的號碼** |
| ` #<N>` | **否** | 同一台 host 的兩條 session,只有它分得出來;半個 ordinal 不是 ordinal |
| `<user>@<host>` | **是** | 剩下的空間全給它,不夠就縮 |

#### 縮的方式:`@` 留著,兩邊各自縮

```go
func fitUserHost(user, host string, w int) string
```

**`@` 一定留著。** 它是讓這串字讀起來「是一個位址」的東西 —— 拿掉之後
`deploy@10.0.3` 和 `deploy10.0.3` 是同一把字元,沒有形狀。

**兩邊分開縮,不是把整串當一個字串截。** 整串截會從尾巴吃掉整個 host,留下一
列只說得出「你是誰」而不說「在哪台機器」—— 而那一列存在的意義正好是後者
(§7.1「列說的是這條連線是什麼」)。

**短的那邊儘量留完整。** user 通常短、host 通常長,把 `deploy` 削掉兩格去換
DNS 名多兩格,對誰都沒有幫助。只有**兩邊都超過一半**的時候才平分:

```
demo@a-very-long-hostname.example.com  (14 欄)  ->  demo@a-very-l…
a-very-long-username-indeed@h          (14 欄)  ->  a-very-long-…@h
averylongusername@averylonghostname    (15 欄)  ->  averyl…@averyl…
```

#### 左欄跟著加寬到 30

一列要說完整件事,就得先有地方說。量了幾個真實形狀之後:

| `sshLeftW` | 位址可用 | 新裝得下的 |
|---|---|---|
| 26(舊) | 12 | — |
| 28 | 14 | `demo@localhost`、`deploy@10.0.3.14` |
| **30** | **16** | 再加 `root@192.168.1.100`、`deploy@prod-web-01` |
| 32 / 34 | 18 / 20 | **什麼都沒多** |

30 停在這裡,是因為下一個值得裝的是長 DNS 名(`app@staging.example.com`,
23 欄),買它要再花六欄,而那六欄是從網格上拿的。文件裡早就寫過「要換的話調
到 30」(§7.1 的代價註),這次是兌現它。

#### 順帶拆掉 §11.24 一半的程式

§11.24 剛加的自動捲動裡,有一整套「不能除、要往回量」的邏輯,存在的唯一理由
是列高不固定。現在一列就是一列:

- `listRows()` 從一個走訪迴圈變回 `innerH`
- `revealCursor()` 從往回累加變回 `scrollTo` —— **跟 sftp 的兩個清單同一個
  函式**,而不是這個 panel 自己的一套

跟隨本身留著,那是 §11.24 真正修好的東西。這一節只是拿掉了它為了應付折行而
背上的重量。

7 個 mutation 全數被抓(丟掉 `@`、兩邊都硬切、改回折行、把尾巴也拿去縮、
`listRows` 回傳常數、`revealCursor` 不再捲、左欄改回 26)。其中三個第一輪
存活,而且都是同一種錯:**測試問的是「有沒有壞掉」,而不是「有沒有做對」**。
「短的優先完整」的測試只檢查了開頭那幾個字(`truncate` 對夠短的字串是 no-op,
所以平分看起來一模一樣);`listRows` 的測試只檢查它跟捲動自洽(回傳常數也
自洽);左欄寬度根本沒有測試釘住,因為「永遠一列」在 26 欄時同樣成立 ——
只是每一列都被縮了。

---

### 11.29 `[V]iew` —— 表格答不出來的那三件事

`[2] Hosts` 一列講得完 host 的五個欄位,卻答不出三個問題:

1. **窄了以後那些欄去哪了。** §1.4 讓表格由右往左收欄,而收掉的通常正是 port 和
   auth —— 你想確認的東西。
2. **auth 欄是一個 glyph。** 鑰匙 / 鎖 / 卡片是型別訊號(§3.2),但「哪一把鑰匙」
   從來不在表上。
3. **password 到底有沒有存。** 它永遠不可能上表格,而「有沒有」跟「是什麼」是兩
   個問題 —— 前者該答,後者不該。

以前唯一的辦法是**開 edit form 去看**,而 form 是一個你可能改到東西的地方。用一
個唯讀浮層回答唯讀的問題,是這一列存在的全部理由。

#### 分兩段:連線 / 認證

```
╭─ 󰛐 staging-api ─────────────────────╮
│ Connection                          │
│  Name           staging-api         │
│  Host           staging.example.com │
│  Port           22                  │
│                                     │
│ Auth                                │
│  Type           credential          │
│  Credential     shared-deploy       │
│  User           deploy              │
│  Identity file  ~/.ssh/id_ed25519   │
╰─ Esc close ─────────────────────────╯
```

**password 顯示固定寬度的遮罩,不是逐字元的。** form 裡的密碼欄是一個 `•` 一個
字元(`form.go` 的 `mask`),那是對的 —— 你正在打它,長度本來就是你自己的。而唯
讀地看別人存的密碼**沒有任何理由公布它有多長**,所以這裡是固定八個點。這一條有
測試釘住:短密碼與 40 字密碼的遮罩必須一模一樣。

**credential host 顯示「它是什麼」加「它解析成什麼」。** `store.Resolve` 的立場是
credential 是**一整包**而不是一組預設值,所以它供的 user 與 secret 排在
`Credential` 那一列底下,而不是混進 Connection 半邊。只印名字會比表格說得還少。

**斷掉的 credential 就寫在那一列上**,用警告色:`retired-key — missing, cannot
connect`。名字和「這個名字指向不存在的東西」是同一件事,拆成兩行會在一欄有標籤的
值裡放一句沒有標籤的話。

`[2] Credentials` 的 `[V]iew` 只有 Auth 那一段 —— credential 就是那一段。名字放
在浮層**標題**上(§3.4:glyph 說型別、文字說身分)。

#### 標籤暗、值亮 —— 跟 help 相反,而且是對的

§4.4 的「亮鍵暗述」講的是**你按的那個鍵**要亮。這裡沒有鍵可按,亮的那半是**答案**
那半。同一條規則的兩種正確結果,不是不一致。版面沿用 form 的讓位規則:窄的時候
label 欄先讓,因為截掉的 label 還讀得出來、擠掉的值讀不出來。

#### 彩蛋讓位

`V` 本來是 u-family 的彩蛋(splash)。它在 `handleKey` 裡的位置**比 panel 的動作表
還早**,所以照抄一列 `[V]iew` 進 `hostActions` 的結果會是:menu 上寫著
`[V]iew`,按下去跳出 logo。

解法不是換一個字母(`V` 就是 view,sftp 的 `[v]iew` 已經用了同一個符號),而是
**讓彩蛋變成全 app 最低的宣告**:

```go
if msg.String() == "V" && !m.typing() && !m.popupOpen() && !m.panelClaimsView() {
```

`panelClaimsView()` **問的是 menu 問的同一張表**(`hostsApplicable` /
`credsApplicable`),不是把條件重寫一次 —— 所以它不可能跟它要讓位的那一列說法不
一致。連空表格都對:沒有東西可看的時候 `[V]iew` 不在表上,`V` 就還是彩蛋的。

8 個 mutation 全數被抓(密碼直接印出來、遮罩長度跟著密碼、credential 不解析、
斷掉的 credential 不說原因、credential 浮層混進 host 欄位、彩蛋永不讓位、彩蛋永遠
讓位、identity path 空白)。其中「彩蛋永遠讓位」與「永不讓位」寫成**同一個測試的
兩半**:會壞的是這對關係散掉,而不是任何一邊單獨錯。

---

### 11.30 `[H]ide` —— 一個 toggle 該用哪一邊命名

`[1]` 那一列本來叫 `Display`,鍵是 `Tab`。兩個問題:

- **`Display` 是名詞**,而 menu 的每一列都是**動詞**(Open / Close / Duplicate)。
  一個名詞混在動詞裡,讀起來像一個狀態欄位而不是一件會發生的事。
- **它從來不是對稱的。** session 一連上就自動在網格上(`connect` 直接 append 進
  `shown`),所以這一列九成九是拿來**拿掉**格子的。用兩邊裡不常走的那一邊命名,
  等於讓常見情況每次都要翻譯一次。

現在是 `[H]ide`,綁 `H`。`Tab` **照樣能按** —— 它是這個 tab 從 v0.2 就有的慣例
(§4.4.1:ssh tab 把 `Tab` 挪用成顯示開關,因為網格會吞掉它)。

**代價說清楚:§4.4「bracket 印的那個大小寫就是唯一按得動的鍵」在這裡有了一個例外。**
menu 寫 `[H]ide`,而 `Tab` 也做同一件事卻沒有出現在那一列上。接受它的理由是
`Tab` 不是**這一列**的鍵,是**這個 panel** 的鍵 —— 它跟 `1`-`9`、`Space`、`?` 同
一層,由 footer 和 §A.2 的 help 揭露,不是由 item operation 那一列揭露。

#### 標籤講常見方向,hint 講真相

**標籤固定寫 `Hide`,不隨狀態改**(使用者裁定)—— 但這樣一來,對一個已經藏起來
的 session,那一列的字就跟它會做的事相反。動態標籤(`Hide` / `Show` 互換)被否
決:同一列在清單上下文裡長得不一樣,掃的時候會慢。

解法是**分工**,而不是在兩個都不完美的標籤之間選一個:

```
[H]ide       hide/show this session's cell
```

- **label 是你用掃的**,所以它講**常見方向**。session 一連上就在網格上,
  九成九是拿來藏的。
- **hint 是你不確定時才讀的**,所以它講**兩個方向都會發生**。

這正是 §4.4「亮鍵暗述」那一組本來就有的分工:亮的那半給掃、暗的那半給讀。一個
雙向動作有一個單向的名字,不必靠標籤自己解決。

2 個 mutation 被抓,而其中一個原本**活著**:把那一列的 key 從 `H` 改回 `tab`,整
套測試照樣全綠。原因是既有的測試按的是 `a.key` 本身 —— 它問的是「這一列跑不跑得
動」,而不是「menu 上印的那個字母按下去做不做事」。補了一個直接按 `H`、再按 `Tab`
的測試才釘住。(這是同一種老問題:斷言問了「有沒有壞掉」而不是「有沒有做對」。)

---

### 11.31 custom 只問一個數字

`[2] layout` 的 custom 本來問 `rows × columns`,而**rows 那一半近乎謊話**:

```go
return c, max(r, (n+c-1)/c)   // 舊的 gridDims
```

rows 只是**下限**。指定 2×3 開十條 session,拿到的是 2 欄 5 列 —— 你說的 3 從來
沒有發生。它唯一真正生效的時候是**格子比 c×r 還少**,而那時它買到的東西是「保留
一列空的」,沒有人要這個。

所以 custom 現在只問 **columns**,一個數字,列數由它跟 session 數推出來。這不是
砍功能,是**把已經是事實的東西講清楚** —— 舊介面問了一個它不打算遵守的問題。

**`"2x3"` 被拒絕,不是從裡面挖一個數字出來。** 那是使用者在回答**舊的**問題;安靜
地取 2 會套用一個他沒有選的形狀。同一條也擋掉 `0`、`40`、空字串。

條紋上寫 `custom 3 columns` 而不是 `custom 3`:radio 列上一個裸數字,是一個沒有
說單位的數字。

3 個 mutation 全數被抓(rows 又變成保留下限、`"2x3"` 被挖出數字、條紋只印數字)。

---

### 11.32 一個 item 兩行:`<glyph> <name>` / `<user>@<host>:<port>`

§11.28 把一個 item 壓成一行,並且加了 `#<N>` 去分辨同一台 host 的兩條 session。
兩件事都退回來了,而且是同一個原因的兩面。

#### `#<N>` 為什麼拿掉:它 key 的東西不是它畫的東西

```go
count[s.host.Name]++          // 判斷用 hosts.yaml 的 NAME
...
fitUserHost(s.host.User, ...) // 畫出來的是 ADDRESS
```

判斷用 name、畫出來是位址,於是**兩個方向都會錯**:

| 情境 | 發生什麼 | 為什麼 |
|---|---|---|
| 兩筆不同 name 指同一台(`prod-a` / `prod-b` 都是 `root@10.0.0.1`) | 兩列**一模一樣、而且沒有 `#N`** | name 不同 → `count` 各自是 1 |
| 同一筆 host 在兩次連線之間被改過位址 | 兩列拿到 `#1`/`#2`,但它們是**兩台不同的機器** | session 存的是連線當下的快照,name 相同 |

前者只要 hosts.yaml 裡有兩筆指同一台就會發生 —— 而那完全合法(name 唯一,位址從
來不唯一)。改成 key `Addr()` 可以修好兩邊,但使用者裁定**不要這個功能**:兩條到同
一台的 session 本來就是兩條,清單用**順序**表示它們是兩個,不需要一個標記。

#### 為什麼是兩行

一行只裝得下 name 和 address 其中一個,而 §11.28 選的是 address。代價一直都在:
**一個 session 清單從來不顯示 hosts 表整個是由它組成的那個東西 —— 名字。**

```
╭[1] sessions────────────╮
│ 󰍹 prod-web-01            │
│   deploy@10.0.3.14:22    │
│ 󰶐 db-replica-tokyo-ap-no…│
│   postgres@db.inter…:2222│
```

第一行是**你叫它什麼**,第二行是 **ssh 實際上會做什麼**。順序就是問句的順序。

**第二行貼齊邊框,不縮排到名字底下。** 縮排先做過,單看一列比較整齊 —— 但它在
**每一列**都拿走位址兩欄,而位址是那個「絕不能含糊」的一半,名字反倒是可以截的。
glyph 欄照樣讀得下來:它是它那一行上唯一在名字前面的東西,而它底下那一行是位址
唯一可能開始的地方。

```
╭[1] sessions──────────╮      ╭[1] sessions────────────╮
│ 󰍹 10.20.12.31          │      │ 󰍹 10.20.12.31            │
│ ubuntu@10.20.12.31:22  │      │   ubuntu@10.20.12.31:22  │
   ↑ 26 欄,位址拿到 23           ↑ 縮排版要 28 欄才拿得到同樣的 23
```

**游標的 bar 蓋兩行。** 只蓋一行的話,游標讀起來像卡在兩個東西中間。

**一個 item 畫完整,不然不畫。** `listBody` 的條件是 `len(out)+sshItemH <= innerH`
—— 最後一行留白,好過一個有名字沒有位址的 session,那看起來是 render 壞掉而不是
清單放不下。

#### 高度仍然是常數,所以捲動仍然是除法

§11.24 為了「列高不固定」寫的那套往回量的邏輯,在 §11.28 被拿掉、換成 `innerH`。
現在是 `innerH / sshItemH`。**變的是常數,不是做法** —— 因為兩行裡的兩半都是
**縮**而不是折:名字用 `truncate`,位址用 `fitUserHost`。

#### 左欄 30 → 28,而下限是 **port 決定的**

一行版要裝 `<glyph><user>@<host>:<port> #<NN>`;`#<NN>` 沒了(省 4 欄)、名字也不
再跟位址搶同一行,所以可以還兩欄回去。

真正定住這個數字的不是常見主機名,是 **non-default port**:

| `sshLeftW` | 位址可用 | 剛好裝不下的 |
|---|---|---|
| 24 | 21 | `ubuntu@ip-10-0-1-23:22`(22)、`deploy@prod-web-01:2222`(23) |
| **26** | **23** | —— 上面兩個都進得來 |
| 28 | 25 | 多出來的兩欄沒有多裝任何一種常見形狀 |

`:22` → `:2222` 差兩欄,而 24 剛好沒有那兩欄。**一個只裝得下預設 port 的欄寬,縮
掉的恰好是那些「port 值得寫出來」的位址** —— sshu 自己的 demo host
(`demo@localhost:2222`)就是其中一個。

> **這個數字改過兩次,兩次都是被抓出來的。** 第一版寫 28,理由是「26 連
> `root@192.168.1.100:22` 都要縮」—— 錯的,那個位址剛好 21 欄。mutation 把
> `sshLeftW` 改回 26 之後測試全綠,因為當時釘住寬度的四個案例**全都是預設 port**;
> 補上 non-default port 的案例才真的釘住 28。接著位址改成貼齊邊框,同樣的 23 欄
> 只要 26 就給得起,於是又還了兩欄回去。
>
> 兩次的共通點:**沒有被測試釘住的常數會停在第一個看起來合理的值。**

9 個 mutation 全數被抓(item 只回一行、名字改印位址、位址改印名字、位址欄改回扣掉
glyph 寬度、位址畫成縮排、cursor bar 只蓋第一行、`listBody` 容許半個 item、
`listRows` 忘記除以 `sshItemH`、`sshLeftW` 降到 24)。

---

---

### 11.33 選取模式 —— 把遠端的字拿出來,而且不動滑鼠

`[3]` 的格子印出來的東西,使用者要複製走。這件事在 sshu 之前只有一條路:
終端機自己的選字。而**那條路正好被 grid 破壞掉**了 —— 原生選字是沿著螢幕的
「實體列」拖的,一拖就順手把邊框和隔壁格子同一列的輸出一起抓進來。格子愈多
愈沒用。

#### 為什麼不是滑鼠

業界的標準解法是開 mouse tracking,自己接管拖曳。sshu 不買:一旦開了,
**整個 app** 的原生選字都要按修飾鍵才回得來(iTerm2 / Ghostty 是 Option、多數
其他終端是 Shift),包括 `[2]` 的清單、`?` help 這些原生選字**仍然好用**的地方。
為了修好一個 panel 的選字而弄壞其他每一個 panel 的,是虧的。§5 當年把 mouse 停在
「還沒想清楚怎麼跟選字共存之前不開」——**這一節就是想清楚的結果:不開。**

鍵盤模式在自己以外不花任何東西。

#### `Alt+v` 進出

進場一定得是和弦:pty 裡每一個裸鍵都是遠端的。選 `v` 是因為進去之後**開始
選取的也是 `v`** —— 進場鍵和場內鍵同一個字母,要記的只有一件事。

> **被否決:`Alt+Shift+Enter`**(使用者提的第一個候選)。傳統終端編碼裡
> `Shift+Enter` 和 `Enter` 是同一個 byte;Bubble Tea v1.3.10 整包沒有 `shift+enter`
> 這個鍵(`KeyEnter = keyCR`,grep `shift+enter` / `13;2u` / `27;2;13` 皆 0 筆)。
> 能區分的終端機(kitty protocol / CSI-u)送的 `\x1b[13;2u` 它又不認得,會變成
> 幾個雜字元**打進遠端**。最好的情況是它等於 `alt+enter` —— 直接撞 zoom。
> 另外被否決的是 `Alt+Space` / `Alt+PgUp` / `Alt+/`:都可用,但都得多背一個字母
> 以外的東西。

出場有兩個,而且兩個都是既有的字彙:再按一次 `Alt+v`,或 `Alt+Esc`。後者不是
新規矩 —— `Alt+Esc` 在這個 tab 一直是**一次剝一層**(§11.25),而選取模式就是
一層。

#### 進去之後,畫面凍結

模式一開,那一格**停止跟著遠端跑**。session 沒有停(它從來不會停,§11.19),
只是 panel 不再畫新的東西。理由很硬:一個會在你選到一半時重排的頁面,是沒有人
選得起來的頁面。

凍結的 buffer 怎麼組出來,是這一節唯一真正難的地方:

- scrollback 存的是**邏輯行**,螢幕上是**折行後的列**;而且 `commitLine` 把
  strip 後全空白的行丟掉了(那是 prompt 重繪)。所以 scrollback 和螢幕**不是**
  同一組東西。
- 直接把兩者接起來會**重複**:scrollback 是在 bytes 進 emulator 的路上存的,
  所以螢幕上還看得到的那幾行,它早就有了。
- 只用 scrollback 則會**少東西**:shell prompt 那半行沒有 `\n`,永遠不會進 ring。

答案是**用螢幕蓋掉 scrollback 的尾巴**(`copySnapshot`):`scrollback[:len-h]` 接上
當下的 grid。這不是新發明的假設 —— `PgUp` 從第一天起就在假設「螢幕就是最後 h 筆」
(`maxScrollOff = len(scrollback) - rows`),這裡只是第一次把它寫下來。

alt screen 裡則只有 grid:全螢幕程式期間本來就不 capture(§11.19),上面沒有歷史
可接,而 vim 自己的視窗正是那個人要複製的東西。

凍結的頁面**不畫遠端的游標**(`gridLines(w,h,false)`)。模式有自己的游標,而遠端
那顆指的是「打字會打在哪」——那正是這裡唯一不會發生的事。

#### 鍵:vim 的字彙,一個都沒有新發明

| 鍵 | 動作 |
|---|---|
| `h` `j` `k` `l` | 游標;撞到上下邊界就把凍結的頁面捲一行 |
| `u` / `d` | 上 / 下**半頁** —— 整頁會讓畫面上沒有任何東西告訴你落在哪(vim 的 `Ctrl+u`/`Ctrl+d` 同理) |
| `v` / `V` | 從游標處開始 char-wise / line-wise 選取;再按一次同一個鍵取消 |
| `y` | 複製並**結束模式** —— 它是當初開這個模式的理由 |
| `Esc` | 兩段,由內而外:先丟掉選取,再離開模式(§4.3 的 Esc 就是這個形狀) |

`h`/`l` 不在使用者原本列的 `jkud` 裡,但**沒有橫向移動的 `v` 是沒有意義的 `v`**,
所以補上。

`v` 和 `V` 之間切換**保留 anchor**:對選取形狀改變主意,不該賠掉起點。

`y` 在**沒有選取時複製游標那一行**。一個按下去不會動的鍵是壞掉的鍵(同 §11.19
對 footer 的判斷),而在這裡「複製這一行」是它唯一可能的意思 —— 順帶讓最常見的
那件事變成三個鍵:`Alt+v`、`V`、`y`。

游標一開就落在**視窗內最後一行有字的**列。shell 剛印完 prompt 會留下一堆空白列,
開在那裡等於開在什麼都沒有的地方。

#### 說出來的三個地方

- **border 變黃**(`toneSelect` / `selectColor` `#f9e2af`)。不是 focus 藍:藍色說的是
  「你的鍵送到這裡」,那句話現在**是真的但沒用** —— 鍵送到的是**別的東西**,而
  frame 是唯一能在按下第一個鍵之前就講清楚的東西。也不是 warn 紅:沒有事情出錯。
- **footer 整列換掉**。不是加幾個鍵,是換掉:模式裡每一個鍵的意思都變了,還留著
  pty 的鍵等於在描述一個不在螢幕上的 panel。

  而**進去的路也在 footer 上** —— pty 的那一列多一個 `alt+v select`,無條件提供
  (`inPty()` 已經保證是活著而且說過話的 session,那正是模式需要的全部)。它必須
  在那裡:pty 裡按 `?` 是遠端的,所以 footer 是這個 panel **唯一**還活著的揭露
  (§11.19 同一個判斷)。位置排在 `alt+方向鍵` **之前**,因為 `keyLegend` 是從
  **尾端**開始丟的 —— 窄 footer 下先犧牲的該是還有別條路可走的那個。
- **選到的用同一個黃**當背景。跟 border 同色,frame 和 mark 講的是同一個字。
  選取範圍內的字**丟掉自己的顏色** —— reverse video 跟遠端可能在用的任何調色盤
  組合起來都不一樣,而「什麼被選到了」必須是一件毫無疑義的事。範圍**外**的顏色
  完全保留(`ansi.Cut` 會把 active style 帶進每一段)。

#### 剪貼簿走本機指令,不走 OSC 52

darwin `pbcopy`;其他平台依序試 `wl-copy` → `xclip` → `xsel`。OSC 52 的好處是連
「sshu 自己被 ssh 進去跑」也能把字送回本機,但它**取決於外層終端機肯不肯**
(Terminal.app 從來不支援、iTerm2 要先允許),而且**失敗時毫無聲音**。一個複製動作
最不能做的事就是安靜地沒發生。

所以結果一定會回報:成功說 `copied N lines`,失敗說 `copy failed: install
wl-clipboard, xclip or xsel` —— 講的是**該怎麼辦**,不只是出事了。

#### 模式活不過它的 panel

`setFocus` 一律結束模式(鍵盤離開那一格就是離開);`setSize` 只要尺寸真的變了就
結束(凍結的頁面是為**一個**尺寸凍的,重排會把字從選取的兩端底下抽走);session
在模式開著時結束或被隱藏,下一個按鍵會發現 `copyAlive()` 為 false 並收掉 ——
否則會留下一個替不存在的格子吞掉每一個鍵的模式。

模式開著時 `Alt+方向鍵` / `Alt+Enter` **不作用**:那是刻意的模態。選到一半被一個
和弦換到別的格子,選取就沒了,而那個手勢沒有人是故意做的。

10 個 mutation 全數被抓(snapshot 改成接在後面而非蓋尾巴、進場鍵移離 `v`、模式
不再接管鍵盤、格子維持 focus 藍、`v` 退化成 `V`、`u`/`d` 改成整頁、切換形狀時
重設 anchor、複製的字保留 padding、凍結頁面保留遠端游標、mark 多畫一格)。

最後一個是**補上去的**:模式自己畫 body 進格子,所以它能用 live path 做不到的
方式把 frame 撕開,而既有的 `TestSSHTabPreservesFrame` 沒有涵蓋模式開著的狀態。
補了同形狀的一份(六個尺寸、選取刻意伸出右緣),`markRow` 多吐一格就會紅。

---

### 11.34 form 的 Enter 只問一個問題

`[2] Hosts` 的 add/edit popup「使用體驗很亂」(使用者原話),而亂的來源是
**Enter 在不同的列上意思不一樣**:在 IdentityFile 空欄上是「開 picker」、在
Credential 空欄上是「開選單」、在其他每一欄上是「送出」。同一個鍵,三種意思,
而使用者按下去之前分辨它們的唯一辦法是記住自己在第幾列。

新規則只有一句:**Enter 問「這張表填完了沒有」。填完了就存,沒填完就跳下一欄
(而且會繞回去)。**

於是按著 Enter 不放,會走完整張表然後把它送出去 —— 一個手勢,從頭到尾。

#### 「填完了」不需要第二份清單

必填的集合**就是 `enabled()` 的集合**,而 `enabled()` 早就知道每一種 Auth 要什麼:
password 要 Password、privatekey 要 IdentityFile、credential 要 Credential 而且
**不再要 User**(credential 是一個包,User 由它提供)。toggle 永遠有值,所以只問
文字欄。

再開一份「必填欄位表」會是第二個真相來源,而它遲早會跟 `enabled()` 分岔 ——
表單上會出現一個亮著卻不算數的欄位,或一個暗著卻擋著你存檔的欄位。

#### 「有值」不是「有效」

Port 打 `0` 是**有值**的。completeness 問的是「這張表填了沒」,validity 問的是
「填的東西行不行」,兩者是不同的問題,所以有**不同的揭露**:

| 問題 | 誰回答 | 長什麼樣 |
|---|---|---|
| 填完了沒 | 底部的 hint | `Enter next` / `Enter save`,隨著填寫即時翻面 |
| 填的行不行 | 錯誤列 + toast | 紅色標在出問題的那一欄,焦點跳過去 |

把兩者合成一個會得到最糟的東西:Enter 拒絕存檔,卻從來不說為什麼。而現在
**hint 在你問出「為什麼 Enter 沒有存」之前就已經答了** —— 最後一欄填上的那一刻,
legend 從 `next` 翻成 `save`。

#### 空的 pick 欄仍然是例外

IdentityFile 和 Credential 在**空著**的時候,Enter 依然開它們的選擇器。理由不是
「保留舊行為」,是**「next」在那裡會跳過唯一一列沒有別的辦法可以填的欄位** ——
開選擇器就是那一欄拿到值的方式。有值之後它們就是普通的列(換掉是 Backspace
整行清除再 Enter)。

#### 代價:agent-only 的 host 不能再用表單建立

`privatekey` 現在**必須**有 IdentityFile。但 `buildSSHCmd` 在 IdentityFile 為空時
是**不加 `-i`** 的 —— 那等於「交給 `~/.ssh/config` 和 agent 決定」,是一種合法而且
常見的設定(sshu 自己的 `sample()` 裡 `bastion-eu-west-1` 就是這樣)。同理
`password` 現在必須有 Password,而空密碼原本會讓 ssh 自己去問。

這是使用者明確要求的規則,照做;但它確實收掉了一個能力。要放回去,是在
`complete()` 裡對這兩欄開一個例外(一行),而不是改回舊的 Enter 語意。手改
`hosts.yaml` 不受影響:`store.Host.Validate` 沒有跟著變嚴,舊檔照樣讀得進來。

#### `[E]dit` 補上熱鍵

`[2] Credentials` 的 Edit 原本只綁在 `Enter` 上。而 §4.4 的括號標記**只給單一字母**
(`bracketHotkey` 對 `len(key) != 1` 回傳裸標籤),所以那一列在 Space menu 裡印的是
`Edit` 而不是 `[E]dit` —— **學會它的唯一辦法是按下 Enter 看看會發生什麼事。**

改綁 `E`,`Enter` 保留為同義鍵(`credsKey` 開頭把 `enter` 映成 `E`)。在 host 列上
`Enter` 是連線、`E` 是編輯;credential 沒有東西可以連,所以「進去」和「編輯」是
同一扇門。hint 仍然寫著 `Enter · change this credential`,兩個鍵都揭露。

7 個 mutation 全數被抓(host Enter 一律送出、completeness 不看任何欄位、
completeness 不看哪些列是亮的、hint 一律說 save、cred Enter 一律送出、Edit 移離
`E`、`Enter` 不再是同義鍵)。

順帶抓到的:**九個既有測試在改完之後仍然全綠 —— 但它們測的已經不是原本那件事了。**
它們全都是「送出一張不完整的表單、期待錯誤」,而新規則下那個 Enter 根本到不了
送出;它們變成在測「next」分支,而斷言剛好還成立。九個全部重寫成新契約:要拿到
拒絕,得先交出一張**填完了但錯了**的表(重複的名字)。

---

## 附錄 — 按鍵全表(v1.4.0)

### Tab 與 panel

| 鍵 | 語意 |
|---|---|
| `M` / `F` / `S`(大寫;小寫屬於各 panel) | 切 tab —— **pty 內無效**,先 `Alt+Esc`;搜尋 / 打字中無效 |
| `1`-`9` | **當前 tab** 的 panel 直達(pty 內數字屬於遠端) |
| `Tab` | 當前 tab 的下一個 panel(**ssh tab 例外:顯示開關**) |

### Core key(跨 surface 不變)

| 鍵 | 語意 |
|---|---|
| `Enter` | 確認 / 連線 / 進入 |
| `Esc` | 關浮層 / 取消(pty 內屬於遠端) |
| `Space` | contextual 入口(Space menu);**在浮層上按 = 關掉它** |
| `?` | non-contextual 入口(help);**再按一次關掉** |

### [M]anage

| Surface | 鍵 | 動作 |
|---|---|---|
| `[1]` nav | `j`/`k` · `Enter` | 選條目(SSH / Events 分類的 header 直接跳過;內容立刻跟著換)/ 鍵盤交給內容 —— 鍵盤在 `[2]` 時整片 dim(§11.13) |
| `[2]` Hosts | `Enter` · `V` · `E` · `D` · `A` · `/` | Connect(確認)/ **View**(唯讀明細,密碼遮罩,§11.29)/ Edit / Delete(確認)/ Add / Search |
| `[2]` Credentials | **`E`**(`Enter` 同義)· `V` · `D` · `A` | **Edit**(§11.34:原本只綁 Enter,而 Enter 印不出括號)/ **View**(只有 auth 那一段)/ Delete(確認,列出引用數)/ Add |
| `[2]` Logs | `j`/`k`/`u`/`d`/`gg`/`G` · `C` | 捲動(viewport,無游標;上畫面即已讀)/ Clear logs(先問;連 applogs.yaml,空 log 時不存在) |
| ~~`[2]` Export / Import~~ | (遮罩中) | Operation 頁已實作但未上架 —— 設計未定案,見 §11.12 追記 |

### Host / credential form

| 鍵 | 動作 |
|---|---|
| `Tab` / `Shift+Tab` / `↑``↓` | 換欄(所有欄位一致) |
| `←` `→`(Auth 欄) | password / privatekey / credential(host form 三選) |
| `Enter`(IdentityFile / Credential 欄,**空**) | 開 picker / credential 選單(唯一的例外:那一欄沒有別的辦法填) |
| `Enter`(其他所有情況) | **表填完了 → 存檔;沒填完 → 跳下一欄(繞回)**。必填集合 = 當下 Auth 之下亮著的欄位(§11.34) |
| `Backspace`(IdentityFile / Credential) | **整行清除** |
| 底部 hint | `Enter next` / `Enter save` —— 隨填寫即時翻面,是這條規則的全部揭露 |
| `Esc` | 取消 |

### [F]ile transfer

| Surface | 鍵 | 動作 |
|---|---|---|
| 全部 | `1` / `2` / `3` / `4` | 左檔案 / 左 marks / 右檔案 / 右 marks |
| 全部 | `Tab` · `h`/`l` | 輪詢四個 panel / 切左右半 |
| 全部 | `H` · `J` · `r` · `x`/`X` · `c`/`C` · `t`/`T` | Host(切這一側)/ Jobs(傳輸清單,可逐條取消)/ Rename / Delete(項/marks)/ Clear(marks 單項/全部)/ 傳輸(項/marks) |
| 檔案 panel | `R` | **Refresh** —— 立刻重讀這個目錄(傳輸中照樣可用) |
| 檔案 panel | `D` | **Disconnect** —— 這一側回到未選 host |
| 全部 | `H` · `D`(**傳輸進行中**) | 兩列在 menu 裡暗掉、按鍵不執行、跳出「先去 `[J]obs` 取消」 |
| 該側**沒有 host** | `Space` | 直接開 host 清單(不繞只有一列的 menu) |
| 檔案 panel(**正在被寫入的列**) | `a` | 拒絕並說明;該列的 mark 欄改顯示 spinner,傳完消失並重列 |
| 檔案 panel | `Enter` · `Esc` · `a` · `/` · `A` · `r` · `v` · `e` | 進目錄(或去到搜尋結果)/ 退搜尋→上層 / append 到 marks(再按取消)/ 搜尋子樹 / Add / Rename / View / Edit |

### [S]SH

| Surface | 鍵 | 動作 |
|---|---|---|
| `[1]` sessions | `j`/`k`/`u`/`d` | 移動 |
| `[1]` | **`H`** / `Tab` | **`[H]ide`** —— 游標 session 的格子上/下網格。menu 寫 `[H]ide`(預設是開著,所以那一列絕大多數在做的是「拿掉」);`Tab` 是這個 panel 從 v0.2 起的慣例,照樣能按(§11.30) |
| `[1]` | `Enter` | 顯示**並把鍵盤交給那一格**(side 欄收起) |
| `[1]` | `C` · `D` | Close(確認)/ Duplicate(確認,**完成後留在清單**;兩條到同一台不再標 `#N`,§11.32) |
| `[1]` | (**只在 Space menu**,無熱鍵) | **Close all sessions** —— 一次關掉全部(確認,問題帶數量;§11.26) |
| `[2]` layout | `j`/`k`(`h`/`l` 也通)· `Enter` | 換排列(立刻生效)/ custom 上問**欄數**(一個數字 1-9,列數自己長,§11.31) |
| 格子(pty) | 所有裸鍵 | 送給遠端 |
| 格子(pty) | **`PgUp`/`PgDown`**(遠端**不在** alt screen) | 捲這一格的歷史(一次一畫面);title 顯示 `󰋚 N` |
| 格子(pty) | `PgUp`/`PgDown`(遠端**在** alt screen) | 送給遠端 —— vim / less 自己翻頁 |
| 格子(pty) | 任何會送到遠端的鍵 | 順手把畫面拉回 live |
| 格子(pty) | 按住 **`Alt`+`←→↑↓`** | 往鄰格移動(邊緣 clamp,pty 內也有效;**zoom 中照走且留在 zoom**) |
| 格子(pty) | **`Alt+Enter`** | **zoom** —— 這一格佔滿整個網格區;只有一格時屬於遠端(§11.25) |
| 格子(pty) | **`Alt+Esc`** | 一次剝一層:**先離開選取模式**,再離開 zoom,再收回鍵盤、回 `[1]` |
| 格子(pty) | **`Alt+v`** | **選取模式** —— 凍結這一格、border 轉黃,再按一次(或 `Alt+Esc`)離開(§11.33) |
| 選取模式 | `h`/`j`/`k`/`l` · `u`/`d` | 游標(撞邊界捲頁)/ 上下半頁 |
| 選取模式 | `v` / `V` · `y` · `Esc` | char / line 選取(再按取消)/ 複製到剪貼簿並結束 / 先丟選取、再離開 |

### 導覽(所有清單共用)

| 鍵 | 動作 |
|---|---|
| `j` / `k` | 上 / 下一列(繞) |
| `u` / `d`(或 `Ctrl+U` / `Ctrl+D`) | 上 / 下半頁(不繞) |
| `gg` / `G` | 第一列 / 最後一列 |

### 全域

| 鍵 | 動作 |
|---|---|
| `?` | help popup |
| `q` | 離開(無浮層時才生效;有活的 session / 傳輸會先問) |
| `Ctrl+C` | 強制離開(子行程一併帶走)—— **pty / 編輯器內除外,那裡它屬於遠端**(§11.20) |
