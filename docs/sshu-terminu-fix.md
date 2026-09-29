# sshu — terminu fix

sshu 尚未符合 [terminu design principle](https://github.com/vulcanshen/terminu/tree/v0.1.21/principle)（tdp v0.1.21）的地方，逐條待修。
修好一條就刪掉一條，並同步 README（兩份）與 `docs/dev-remarks.md` 裡描述該行為的段落。

盤點日期：2026-09-29。依據 `main` 的 `da0858d`（已對齊 v0.1.20，工作區乾淨）。**這一輪只對 v0.1.20 → v0.1.21 的改動**
（`git -C ~/Documents/sideproj/terminu diff v0.1.20 v0.1.21 -- principle/`）：

- D5：選取文字的模式照 vim 移動 —— `h/j/k/l`、`w/b/e`、`0/$`、`gg/G`、`u/d`（user 要求寫回 tdp，filu 轉達）。
- D6：環境變數命名 `<APP>__<NAME>`（app 名後兩個底線、變數名全大寫單底線分隔），共用名 `<APP>__CONFIG` / `__STATE` / `__DATA`
  / `__CACHE`（都指向**目錄**）/ `__ICON_WIDTH`；給別的程式讀的變數例外；**改名不留舊名**（user 2026-09-29 裁定）。
- D6：疊 popup 時 popup 比畫面寬或高（調整終端機大小的那一格）：起點取 0、超出的部分切掉，不可以 panic（kbu、locku 照搬時抓到，
  filu 的參考實作有這個 bug）。

tdp 連結（README 兩份、docs、`.claude/rules`）已由 terminu session 從 v0.1.20 改成 v0.1.21，只改網址，跟這份清單一起留在工作樹，
還沒 commit。


## 先看

- 清單與改釘的連結先一起 commit，再動程式；commit 只加自己改的路徑。
- 每修一處補 model test、做 mutation；同一個 commit 同步 README 兩份與 dev-remarks；CHANGELOG 記 `[Unreleased]`。
- **環境變數改名是破壞性改動**：CHANGELOG `[Unreleased]` 要寫一條「改名，舊名不再讀」並列出新舊對照；CHANGELOG 裡已發版的舊段落不改。
  `.checkpoints/` 是本機筆記，不用改。改完 `grep -rn '<舊名>'` 除了 CHANGELOG 舊段落應該是零。
- **修完拿 v0.1.21 全文再逐條對一次**，修完刪掉這份清單。不 push、不發版；把這一輪寫進 terminu `.local/family-fix/sshu/README.md`。


## 1. `compositeDisp()` 在 popup 比畫面大時 panic —— D6（v0.1.21，等 filu 先修）

**現況**（`internal/ui/width.go`）：`compositeDisp()` 與 `clampSpan()` 照搬自 filu，popup 比畫面寬或高時起點算成負的，`strings.Repeat` 或
`bgLines[y+i]` panic；調整終端機大小的那一格就會遇到。

**規則**：D6（v0.1.21）—— 疊 popup 時 popup 可能比畫面寬或高（調整終端機大小的那一格還是舊尺寸）：起點取 0、超出畫面的部分切掉，
**不可以 panic**；測試的邊界要含這種情況。

**怎麼改**：等 filu 清單第 1 條修完（參考實作），照搬它的 `compositeDisp()` 與測試（kbu 的 `TestD6_CompositeDisp` 同一組邊界：比畫面寬、
比畫面高、兩者都大；不 panic、每列剛好畫面寬、列數等於畫面高）。sshu 保留自己的函式名（`dispW`、`clipANSI`）即可。


## 2. 環境變數沒照家族命名 —— D6（v0.1.21）

| 現在 | 意思 | 改成 |
|---|---|---|
| `SSHU_CONFIG`（`internal/store/store.go`） | 設定目錄 | `SSHU__CONFIG` |
| `SSHU_ICON_WIDTH`（`internal/ui/iconwidth_unix.go`、`width.go`） | icon 寬度覆寫 | `SSHU__ICON_WIDTH` |
| `SSHU_KEY_FILE`（`internal/store/crypt.go`） | 加密金鑰檔 | `SSHU__KEY_FILE` |
| `SSHU_ASKPASS_HOST`、`SSHU_ASKPASS_SOCK`（`session.go`、`askpass.go`） | 傳給自己的 askpass 子程序 | `SSHU__ASKPASS_HOST`、`SSHU__ASKPASS_SOCK` |
| `SSHU_KEEP_ME`（`edit_test.go`）、`SSHU_TEST_SFTP_SERVER`（`internal/remote/pipe_test.go`） | 測試用 | `SSHU__KEEP_ME`、`SSHU__TEST_SFTP_SERVER` |
| `LC_SSHU_COLORTERM`（`session.go`） | 經 ssh 帶到遠端給 PTY 的 | **不改**（例外：OpenSSH 預設只轉送 `LC_*`，user 裁定該例外就例外） |

**規則**：D6（v0.1.21）—— `<大寫 app 名>__<變數名>`，變數名全大寫、單字之間一個底線；app 自己讀的變數（含測試用、傳給自己子程序的）
都照這個寫。共用名：`<APP>__CONFIG`（設定目錄）、`<APP>__STATE`（狀態目錄）、`<APP>__DATA`（資料目錄）、`<APP>__CACHE`（快取目錄）、
`<APP>__ICON_WIDTH`。給別的程式讀的變數例外。**改名不留舊名**（user 裁定）。

**怎麼改**：照上表改名。askpass 子程序是同一個 binary，兩邊一起改即可。一起改的地方：`Makefile`、`.local/demos/` 設 `SSHU_CONFIG` 的
tape 與腳本（`setup.sh`、`drive-nest.sh` 等）、README 兩份（`SSHU_KEY_FILE`、`SSHU_ICON_WIDTH`）、`docs/dev-remarks.md`、
`docs/sshu-ui-design.md`（設計文件裡的舊名就地改，或照它的慣例記一節新決定）、測試。巢狀告知 icon 寬度的 OSC 通道不受影響。改完
`grep -rn 'SSHU_[A-Z]' .` 除了 CHANGELOG 舊段落與 `LC_SSHU_COLORTERM` 是零。


## 3. 選取模式沒有 `gg/G` —— D5（v0.1.21）

**現況**（`internal/ui/copymode.go`）：選取模式有 `h/j/k/l`、`w/b/e`、`0/$`、`u/d`，沒有 `gg`（頂）、`G`（底）。

**規則**：D5（v0.1.21，家族預設）—— 選取文字的模式照 vim 移動：`h/j/k/l`、`w/b/e`、`0/$`、`gg/G`、`u/d`。

**怎麼改**：`gg` 到凍結畫面（含 scrollback）的第一列、`G` 到最後一列；`g` 的第一下等第二下（照 sshu 其他 `gg` 的做法）。補進選取模式的
`?`（`copyModeHelp()`）與 footer / legend 的鍵表。測試：`gg`、`G` 的落點；選取中延伸。


## 待確認

沒有。
