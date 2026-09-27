# sshu — terminu fix

sshu 尚未符合 [terminu design principle](https://github.com/vulcanshen/terminu/tree/v0.1.1/principle)（tdp v0.1.1）的地方，逐條待修。
修好一條就刪掉一條，並同步 README（兩份）、`docs/sshu-ui-design.md` 與 `?` menu 的 key reference（`helppopup.go` 的 `keyReference`）裡描述該行為的段落。有意不修的，改寫成
`dev-remarks.md`「偏離 tdp」的一條並附理由。

盤點日期：2026-09-26。行號以當天的 `main` 為準（檔案都在 `internal/ui/`）。

**2026-09-27 裁定**：原本的第 1 條（SSH tab 的 `Tab`）、第 2 條（表單的 `Enter`）與「待確認」（格子上只有熱鍵的動作）保留 sshu 現在的做法，改寫成 `dev-remarks.md`「偏離 tdp」；條號不重排。同日對照 tdp v0.1.1 補上第 15、16 條，並修正第 7、13 條。

**2026-09-27 修好（路由核心）**：第 3、9、10、11、13、16 條，見 `sshu-ui-design.md` §11.54。
**2026-09-27 修好（menu）**：第 4–8 條，見 `sshu-ui-design.md` §11.55。
**2026-09-27 裁定（試行）**：`?` 只拿來讀（key reference），global operation 改成 Space menu 最後一列打開的 popup；SSH tab 的 `Tab` 不作用。都寫成 `dev-remarks.md`「偏離 tdp」，見 `sshu-ui-design.md` §11.56。


## 先看：locku、webu 修完的經驗（2026-09-27 更新）

locku（v0.1.2、v0.1.3）與 webu（v0.4.0）已照 tdp 修完，修的時候發現這些，這裡的各條也適用：

- **每修一條補一個 model test**，測試名稱或註解寫明 tdp 條目（locku `internal/ui/app_test.go`）。
- **`?` menu 的 global 列也適用 F4**：選了會開下一個框（confirm、輸入、選項）的列，`?` menu 留在底下，
  取消回到它，完成才清掉整疊。浮層因此最多三層（menu、它開出的框、框上的 `?` help），D2 的層色要夠用。
- **K9 的離開流程**：離開的 confirm 開著時，`Ctrl-C` 直接離開；`q` 在離開 confirm 上不再疊一個。
- **S3**：splash 的判斷放在 `Ctrl-C` 之前，任何鍵都只關 splash。
- **F3**：判斷「popup 還在不在」用開啟中或已開（`owns()` 一類），不用含關閉中的 `isActive()`；toast 也一樣。
- **刻意保留的行為寫成偏離**：locku 的全螢幕預覽「任何鍵就回來」、欄位字典取代 `?` menu，都寫進
  `dev-remarks.md`「偏離 tdp」並附理由，而不是留在 fix 檔。
- **文件裡的 tdp 連結釘在 tdp 的版本 tag（目前 `v0.1.1`）**（本檔與 README、dev-remarks 已改好）。
- **修完一批就更新 CHANGELOG、發一版**，README 與 `ux.md` 等描述行為的段落同一個 commit 改。
- **（webu）動手前先把 fix 清單互相對一遍。** 清單裡的條目可能互相衝突（webu 同時要「global 列標 `[?]`」與
  「popup 上的 `?` 顯示該 popup 的 help」—— Space menu 也是 popup，只能成立一條）。
- **（webu）行號很快就過期。** 前幾條一改，後面引用的行號全部位移；換註解時用內容比對，不要照行號。
- **（webu）F4 可以統一在按鍵路由處理**：記下按鍵前最上層的等級（menu、options、動作開出的框），按鍵後若不是
  `Esc`、最上層掉了一級以上，就關掉底下的 menu。執行 menu 的一列時，在 **dispatch 回傳的 model** 上判斷有沒有
  開出新框，不要在舊 model 上關（會關在沒人回傳的副本上；locku 也踩過 value receiver 的同類陷阱）。
- **（webu）改完一條就回頭檢查其他畫面的提示。** `Space` 不再關 popup 之後，下框的 `Space close` 要跟著改。
- **（webu）`bracketHotkey` 會重複加括號**：label 已寫出鍵的列（`[/] Search`）變成 `[/] [/] Search`。
- **（webu）每個 panel 的 Space menu 都接上 global 區之後，原本只有一區的 menu 要補上區塊標題**（tdp v0.1.1 M2）。
- **（locku）打 tag 前先 `git branch --show-current`**：locku 0.1.2 的 tag 打在分支上，推上去的 `main` 沒有變動。
- **tdp v0.1.1 新增 K11「模式」**：模式裡 `Space` 列出模式的鍵（可執行）、`?` 是模式的 help、`Esc` 離開、`q` /
  `Ctrl-C` 照 K9、`Tab` 可暫停但要有回應。D3、D4 也補了路由（help 在最上層）與 `key reference` 不可選的做法。
- 完整紀錄：terminu repo 的 `.local/family-fix/locku/`、`.local/family-fix/webu/`（本機）。

---

## 12. README 的 `[M]anage` 按鍵表過時 —— S2、文件對齊

- **現況**：`README.md` 與 `README-zh_TW.md` 的 `[M]anage` 表有一列「`V`：View what this row holds /
  唯讀檢視這一列」；但 `internal/ui/app.go`（約 695 行）的註解寫「nothing claims the letter any more」，
  `V` 在 pty 與輸入框以外都是 splash，檢視已經改成 `Enter`（`credkeys.go`、`sshcfgkeys.go`、
  `knownkeys.go` 的 `View` 都綁 `enter`）。同一張表 `Enter` 的說明（Credentials: edit）也可能對不上。
- **規則**：splash 不寫進 README（tdp S2）；README 照 app 現在的行為寫。
- **怎麼改**：對照程式碼核對整張 `[M]anage` 表（兩份語言）：拿掉 `V` 那一列，修正 `Enter` 在各區段的說明。

## 14. 程式碼註解仍引用 VTP 的 § 編號 —— 文件對齊

- **現況**：`internal/ui` 的註解用 VTP 的 § 編號、「u-family」與「the principle」（不影響行為）。只換引用 VTP 的；
  指 `sshu-ui-design.md` 自己章節的不動：所有 `§11.x`、`§7.3.2`、`theme.go:5` 寫明的 `sshu-ui-design.md §2.1 / §B`，
  以及 `theme.go:107`、`:131`（§3.4、§3）、`viewer.go:34`（§4.2 導覽）、`askpass.go:292`（§6.3 host form）、
  `hosts.go:225`、`:296`（§1.1、§1.5）、`chrome.go:134`（§1.1）、`app.go:1611`、`sshkeys.go:287`（§6.2 Space menu）。
- 路由核心（§11.54）與 menu（§11.55）兩批改寫過的註解已換成 tdp 編號，表裡不再列；其餘行號是盤點當天的，照內容比對。
- **怎麼改**：照 [terminu `vtp/README.md` 的對照表](https://github.com/vulcanshen/terminu/blob/v0.1.1/vtp/README.md) 換成 tdp 編號：

| 檔案:行 | 現在 | 換成 |
|---|---|---|
| `app.go:38` | `§1.3` | `tdp L3` |
| `app.go:115`、`:118` | `§A.0.K` | `tdp K1` |
| `app.go:117` | `the principle capped` | `VTP capped`（歷史敘述，保留原意）或刪掉這句 |
| `app.go:265` | `§6.4` | `tdp F4` |
| `app.go:607` | `§4.3` | `tdp K4` |
| `app.go:621` | `§4.5` | `tdp K8` |
| `app.go:680` | `§4.3` | `tdp K4` |
| `app.go:682` | `§4.5` | `tdp K8` |
| `app.go:695` | `u-family easter egg` | `terminu family easter egg` |
| `app.go:718`、`:727` | `§4.5` | `tdp K8` |
| `app.go:810`、`:1630`、`:1830` | `§6.4` | `tdp F4` |
| `app.go:860`、`:1758` | `§7.1` | `tdp T1` |
| `app.go:947`、`:1234`、`:1265`、`:1461` | `§4.2` | `tdp M3` |
| `app.go:1145` | `§4.5` | `tdp K8` |
| `app.go:1496` | `§A.1` | `tdp K5` |
| `askpass.go:225` | `§4.3` | `tdp K4` |
| `bundlepage.go:17`、`:79`、`:126` | `§4.5` | `tdp K8` |
| `bundlepage.go:68` | `§6.7` | `tdp F5` |
| `chrome.go:119` | `§1.1 — narrow must stay usable` | `tdp L1` |
| `chrome.go:136` | `§1.3` | `tdp L3` |
| `chrome.go:141` | `§7.2` | `tdp T2` |
| `chrome.go:176` | `§A.1 / §A.2` | `tdp M1` |
| `chrome.go:199`、`:263` | `§2.1` | `tdp D2` |
| `confirm.go:32` | `§6.1` | `tdp F1` |
| `confirm.go:71` | `§4.3` | `tdp K4` |
| `copymode.go:127` | `§4.3` | `tdp K4` |
| `credform.go:219` | `§4.5` | `tdp K8` |
| `credkeys.go:11` | `§4.2` | `tdp M3` |
| `credkeys.go:107` | `§6.4` | `tdp F4` |
| `credkeys.go:232` | `§6.7` | `tdp F5` |
| `detail.go:24` | `the §6.1 VIEWPORT class` | `the viewport class (tdp F1)` |
| `detail.go:29` | `§1.4`（已退役的 `sshu-implementation.md` §1.4 欄位收縮） | `sshu-ui-design.md §1.2` |
| `detail.go:41` | `§4.4's "bright key, dim description"` | `tdp M5 / D1 的「亮鍵暗述」` |
| `detail.go:110` | `§4.3` | `tdp K4` |
| `filepicker.go:23` | `§4.5` | `tdp K8` |
| `filepicker.go:273` | `§2.4 override colour` | `tdp D2` |
| `form.go:15` | `the hybrid float §6.1 forbids` | `the hybrid float tdp F1 forbids` |
| `form.go:18` | `the §A.1 entry key` | `the Space menu entry key (tdp K5)` |
| `form.go:19`、`:471`、`:474` | `§4.5` | `tdp K8` |
| `form.go:431` | `§6.7` | `tdp F5` |
| `highlight.go:12` | `the point of the u-family` | `the point of the terminu family` |
| `hosts.go:83`、`:171` | `§4.5` | `tdp K8` |
| `hosts.go:255` | `§1.3` | `tdp L3` |
| `inputpopup.go:24` | `§6.1` | `tdp F1` |
| `inputpopup.go:94` | `§4.3` | `tdp K4` |
| `inputpopup.go:123` | `§B` | `tdp P4` |
| `knownkeys.go:140` | `§6.1` | `tdp F1` |
| `knownkeys.go:166` | `§6.4` | `tdp F4` |
| `popup.go:12` | `§2.2 / §6.3` | `tdp D2` |
| `popup.go:14` | `§B` | `tdp P4` |
| `popup.go:31` | `§6.2 … inside the 100-200ms band` | `tdp F2 … the family default (tdp D3)` |
| `popup.go:71` | `§6.2` | `tdp F2` |
| `popup.go:151` | `a hole in the principle (§4.5)` | `a hole in tdp K8` |
| `popup.go:208`、`:214`、`:262` | `§4.4` | `tdp M5` |
| `preftab.go:108` | `§1.2` | `tdp L2` |
| `sftpkeys.go:77` | `§4.4` | `tdp M5` |
| `sftpkeys.go:142`、`:161` | `§4.2` | `tdp M3` |
| `sftpsearch.go:191` | `§4.5` | `tdp K8` |
| `sftptab.go:369` | `§4.5` | `tdp K8` |
| `sftpview.go:11` | `§4.4` | `tdp M5` |
| `spacemenu.go:12` | `§4.2` | `tdp M3` |
| `spacemenu.go:150` | `§4.4` | `tdp M5` |
| `splash.go:19` | `The u-family mark` | `The terminu family mark` |
| `sshcfgkeys.go:13`、`sshkeys.go:13` | `§4.2` | `tdp M3` |
| `sshtab.go:50` | `§1.2` | `tdp L2` |
| `sshtab.go:372` | `§A.1` | `tdp M5` |
| `sshtab.go:1035` | `§1.2` | `tdp L4` |
| `theme.go:10` | `§4.4` | `tdp M5` |
| `theme.go:21`、`:25`、`:39`、`:61`、`:80`、`:152`、`:162` | `§B` | `tdp P4` |
| `theme.go:35` | `§2.4` | `tdp D2` |
| `toast.go:26` | `§6.5` | `tdp F3` |
| `transfer.go:196` | `§7.2` | `tdp T2` |
| `view.go:13`、`:30` | `§1.3` | `tdp L3` |
| `view.go:188`–`189` | `§A.1 / §A.2` | `tdp M1` |
| `view.go:193`–`194` | `the principle's disclosure score` | `VTP's disclosure score`（歷史敘述） |
| `view.go:264` | `§4.4` | `tdp M5` |
| `width.go:11` | `§1.2` | `tdp L2` |

## 15. 選取模式（`Alt+v`）的 core key 都被吞掉 —— K11、M1

- **現況**：`app.go` `copyModeKey()` 只認 `Alt+Esc` / `Alt+v`（離開）與 `copyState.key()` 的移動、選取、`y`、
  `Esc`；`Space`、`?`、`q`、`Ctrl-C`、`Tab` 全部靜默吞掉。footer（`view.go` `copyLegendPairs()`）列的是模式的鍵，
  沒有 `space` 與 `?`。
- **規則**：模式裡 `Space` 開關**模式的按鍵清單**（每一列可直接執行，執行後關掉，不分區）；`?` 是模式的 help
  （唯讀）；`Esc` 離開模式；`q`、`Ctrl-C` 照 K9 進離開流程；`Tab` 可以暫停但要有回應（toast 說先 `Esc`）。
  footer 照樣顯示 `Space` 與 `?`。
- **怎麼改**：`copyModeKey()` 在交給 `copy.key()` 之前處理這五個鍵；按鍵清單的列就是 `copyLegendPairs()` 那幾組
  （一份來源，footer 與清單共用）。`Esc` 維持兩段（先取消選取，再離開模式），符合 K4 的「一次一層」。
  footer 前面固定 `space` 與 `?`，其餘照舊從尾端捨棄。README 兩份的選取模式小表、`sshu-ui-design.md` §11.33 一起改。
