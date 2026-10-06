# sshu — terminu fix

sshu 發版前要修的 bug，逐條待修。修好一條就刪掉一條。

盤點日期：2026-10-06。依據 `main` 的 `033b247`（已對齊 tdp v0.1.23，工作區乾淨）。**這一輪不是 tdp 改版**：是 input 盤點
（2026-09-29，terminu `.local/input-survey/sshu.md`）翻出來、跟之後 tdp components 怎麼定無關的 bug；五個 app 修完就發版。
tdp 版本不變，連結不用改。這份清單由 terminu session 寫好留在工作樹，還沒 commit。


## 先看

- 每條先寫一個會紅的測試，再修；CHANGELOG `[Unreleased]` 的 Fixed 各記一條（合併或拆開由 sshu 定）。
- 第 1 條是五個 app 共通的做法，五份清單的「做法」同一段文字：畫出來的樣子與行為要一樣，程式怎麼寫各 app 自己定。
- 最後「這一輪不修」列的要等 components 的 input 檔定案，不要先動。
- 修完刪掉這份清單。不 push、不打 tag、不發版：發版是下一步，版號由 user 在 terminu session 一個一個定。把這一輪寫進
  terminu `.local/family-fix/sshu/README.md`。


## 1. 單行的值收進換行、Tab 與控制字元 —— 五個 app 共通

**現況**：共用欄位引擎（`editField`／`insertRune`，`form.go:357`、`:396`）、`inputPopup.update`、`askpassPopup.update`、
`filePicker.update`、`hostsModel.filterKey`、`sftpSideModel.filterKey` 各自收字，都不過濾換行：User 欄貼上 `1\n2`，換行
原樣進值，那一列在框內斷開。

**做法**（五個 app 同一段，2026-10-06 user 定案；之後寫進 tdp components 的 input 檔）：

- 範圍：單行的值 —— 一行文字、路徑、密碼／PIN、搜尋與篩選列。多行編輯框不在這條（只有 webu 的 editor，另有一條）。
  只收數字的欄位照舊只收 `0`–`9`。
- 收進值：只看以文字進來的字元（貼上的那一段）。按下去的 `Tab`、`Enter`、`Ctrl-J` 這些鍵照舊做它們原本的事（K2、K3），
  不變成值裡的字元（locku 修的時候補的，2026-10-06）。
  - 換行（`\r\n` 算一個，單獨的 `\n`、`\r` 也各算一個）與 Tab 原樣留在值裡，不換成空白、不刪。`\r\n` 原樣存或存成
    `\n` 都可以（locku 原樣存；webu 存成 `\n`，之後一個 rune 就是一個單位，`Backspace`、遮罩、量寬都不用另外處理），
    畫、數、刪都當一個。
  - 其他控制字元（其餘的 C0、DEL、C1）丟掉。
- 預填的值（原本的名字、從檔案或別的程式讀來的值、提議）打開時就走同一個過濾：換行與 Tab 照樣畫、照樣擋，其他控制字元
  丟掉，不然 ESC 會直接送進終端機（webu 修的時候補的，2026-10-06）。
- 畫：換行畫成 `\n`、Tab 畫成 `\t`，Red `#f38ba8`，佔 2 格，跟手打的 `\`、`n`（一般值的顏色）分得開。量寬、截斷、
  捲動都把它當成一個 2 格寬、不能切開的單位。整列畫成灰色的列（沒在打的篩選列、提議）`\n`、`\t` 也跟著灰，整列一次
  上色（webu 的做法）。
- 遮罩的值：照樣遮罩，一個換行或 Tab 也是一顆遮罩符號，不露出 `\n`；使用者靠錯誤列知道。
- 刪：`Backspace` 一次刪掉整個（它本來就是一個字元）。
- 送出：值會被拿去用的 input（送出、存檔、執行、交給別的程式），值裡有換行或 Tab 時 `Enter` 不送出，錯誤列說出哪一欄
  不能有換行或 Tab（英文；句式、大小寫照該 app 現有的錯誤訊息，例：`Name can't have line breaks or tabs`）。其他照 K3：
  多欄表單 focus 跳到第一個不合格的欄位、label 變 Red；有「第一次送出後每鍵重驗」的照舊。
  - 原本送出不會失敗、所以沒預留錯誤列的 input，現在會失敗了，照 F7 打開時就預留錯誤列。
- 搜尋與篩選列（值只拿來找東西，不存、不執行；`Enter` 選的是清單裡的項目）：只照上面畫，不擋。

**為什麼**：單行的值裡換行沒有意義。原樣畫出來會把框畫壞；偷偷換成空白或刪掉，又改了使用者的值而看不出來（user：
「應該轉成 `\n` 或 `\t` 這種明確顯示」）。只在畫面上轉、值裡留原字元，是為了跟手打的 `\n` 分得開，也不會把沒有意義的
字元送出去。

**sshu 要改的地方**：
- 收字：上面六處。`digits` 欄（Port 等）照舊只收 `0`–`9`。
- 畫：`renderTextValue`（`form.go:614`，含 focus 時的水平捲動）、`inputPopup.view`、`askpassPopup.view`（遮罩，不露出）、
  `filePicker.view`、`filterRow`（`hosts.go:278`）、`searchRow`（`sftpview.go:136`）。
- 擋：四個表單（Host、Credential、Host block、Add known host；Export、Import 頁用同一個引擎，一起改）、Rename、Add、
  Custom grid、Trusted for、askpass。表單在 `missing()`／`checkForm()` 之外加這一條；第一次送出後每鍵重驗照舊。
- askpass 的密碼框現在沒有錯誤列（`docs/dev-remarks.md`「單行輸入框有錯誤列」一段寫明），照 F7 預留一列，那一段跟著改；
  host key 的 yes/no 不收字，不用。
- 不擋：Identity file 選檔器的篩選、Hosts 搜尋、File transfer 子樹搜尋。
- 預填：Edit／Duplicate 的 host 與 credential、Host block 從 `~/.ssh/config` 讀來的值、Trusted for 從 known_hosts 讀來的名稱、
  Rename 的遠端檔名（sftp 上的名字什麼字元都可能有），打開時就走同一個過濾。
- askpass 補錯誤列之後，量高度的舊測試照上面 webu 的經驗換量法。
- **先擋再 trim**：表單存檔時除了 Password 都 trim，`inputPopup` 送出前也要看有沒有 trim。擋要拿原值、在 trim 之前，不然頭尾
  貼進來的換行會被悄悄吃掉、照樣送出（filu 的 `Icon\r` 原樣按 `Enter` 就被改名成 `Icon`）。


## 2. Auth = sshconfig 時，送出仍要求 User 與 Port

**現況**：`missing()` 把 Port、User 當選填（`docs/dev-remarks.md`「表單的 `Enter`」：sshconfig 除了 Host 什麼都不要，
Port 和 User 選填；兩欄的 placeholder 也寫 `ssh decides`），但 `checkForm()`（`app.go:2342`）仍要求 User 非空、Port 在
1–65535。盤點實測：只填 Name、Host 送出得到 `User is required`；補上 User 再送出得到 `Port must be 1-65535`。

**怎麼改**：Auth = sshconfig 時，`checkForm` 不要求 User；Port 空著（0）通過，有填才檢查 1–65535。


## 3. 搜尋中 focus 離開那個 panel，打的字仍進 query

**現況**：Hosts 搜尋中按 `Tab`，panel focus 移到 nav `[1]`，搜尋不結束，打 `x` 仍進 query（盤點實測 `prod` 變 `prodx`）。
File transfer 子樹搜尋按 `Tab` 移到**同一側**的標記 panel 時一樣（移到另一側則恢復一般按鍵）。輸入態只屬於拿著 focus 的
那個 input（K8）。

**怎麼改**：focus 離開搜尋所在的 panel，就離開輸入態，按鍵照新 panel 的一般按鍵處理；query 與篩選結果留著。focus 回到
那個 panel 時要不要直接回到打字由 sshu 定，兩個搜尋要一樣，README 與 docs 寫清楚。


## 4. `docs/sshu-ui-design.md` 的舊寫法

- §6.3 Host form：節首的 v0.2 註記已說選值欄位改成空欄位上的 `Enter`，節內的 ASCII 圖與欄位表仍寫 `tab to browse ~/.ssh`、
  `Tab` 開檔案選擇器。
- §6.3.1 Identity file picker：圖與「為什麼是 `Tab`」一段同樣是舊寫法。
- §11.40：寫「錯誤列換成 spinner 那一行」；程式碼是標題後的 loading icon，錯誤列只放錯誤（`knownform.go:136` 的註解）。

改成現在的行為，或明白標成歷史，由 sshu 定。


## 5. 註解把「CJK icon 字型」當成「icon 佔兩格」

**現況**：tdp D7（v0.1.23）不拿「一定佔兩格」的字型當例子（filu 實測：Maple Mono NF CN 的 icon 看起來兩格，游標只前進一格）。
`internal/ui/width.go:56`（`a Nerd Font icon that a CJK icon font draws` …）與 `internal/ui/tdpicon_test.go:28`（`one cell, or two on
a CJK icon font`）把「CJK icon 字型」當成「icon 佔兩格」的代稱。CHANGELOG `[Unreleased]` 沒有這個問題。webu、filu 都找到同一類
並改了。

**怎麼改**：照 filu 整類 grep：當代稱用的改成「icon 佔兩格的字型」（佔幾格看字型與終端機，啟動時量）。不說死的寫法可以留，
例：`width.go:19`、`tdpicon_test.go:15`、README 兩份第 57 行、`docs/dev-remarks.md:108`（「有些 CJK 用的 Nerd Font」）。
`docs/sshu-ui-design.md` §3.3 標題「CJK icon 寬度」要不要改由 sshu 定。


## 這一輪不修

- 表單的值水平捲動以 rune 計，CJK 對不齊。
- 一行框的長值從尾端截，看不到正在打的地方。


## locku、webu、filu 先修完的經驗（2026-10-06）

- **值裡的 `\t`、`\n` 不能直接交給 lipgloss**：`Render` 會把 Tab 換成空白、在換行處斷成兩列。先換成要畫的 `\n`、`\t`，
  再上色。
- **同一個值有兩條路進來，要用同一個過濾。** locku 的設定畫面不過濾、鎖定畫面只收 `IsPrint`，兩邊各自合理，合起來就設得出
  一個解不開的 PIN。
- **測試要用跟舊行為不同的輸入。** 貼 `12\n34` 時字元數剛好等於單位數，測不出「`\r\n` 算一個」；換成 `12\r\n34` 才分得出來。
- **bracketed paste 是一整個 `KeyRunes`**（`Paste: true`，換行、`\r`、Tab、ESC 都在裡面）；打字進來的 `KeyRunes` 不會有控制
  字元，所以每個 `KeyRunes` 都過濾是安全的，不用看 `msg.Paste`。按下去的 `Tab`、`Enter`、`Ctrl-J` 是別的 `msg.Type`。
- **灰的列要整列一次上色。** 把提議、沒在打的篩選列拆成「灰字＋`\n`」好幾段，webu 原本「整列是一段 Overlay0」的測試就紅了；
  先畫成純文字，再整列上色一次。
- **預填的值不只一個入口。** webu 加書籤的標題提議每一鍵之後重算，那裡也要過濾；每一個入口都要有測試，拿掉其中一處的過濾要紅。
- **「這個框沒有錯誤列」的舊測試要換量法。** 每個框都有錯誤列之後，拿沒有錯誤列的框當基準量高度就失效了；改量「被拒時原因
  寫進框裡、高度跟打開時一樣」。
- **實機確認用 `tmux paste-buffer -p`**：送的是 bracketed paste。tmux 預設把 LF 換成 CR，正好也驗到「單獨的 `\r` 算一個換行」。
- **`[Unreleased]` 與測試註解也要 grep。** webu 的 CHANGELOG `[Unreleased]` 還寫著舊的變數名與「CJK 字型就是兩格」：已發版的段落是
  歷史不動，還沒發的段落會原樣出現在下一版的說明裡。
- **先擋再 trim（filu）。** filu 送出前 `TrimSpace`，頭尾貼進來的換行在檢查之前就被吃掉：`Icon\r` 原樣按 `Enter` 就改名成
  `Icon`。檢查要拿原值、在 trim 之前。
- **同一類的寫法整個 grep（filu）。** 「CJK 字型的 icon 就是兩格」filu 的清單只列四處，grep 出來七八處，連 dev-remarks 一整段的
  標題都是。清單列的位置只是找到的那幾個。

## 待確認

沒有。
