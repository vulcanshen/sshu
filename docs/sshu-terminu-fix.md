# sshu — terminu fix

sshu 尚未符合 [terminu design principle](https://github.com/vulcanshen/terminu/tree/v0.1.21/principle)（tdp v0.1.21）的地方，逐條待修。
修好一條就刪掉一條，並同步 README（兩份）與 `docs/dev-remarks.md` 裡描述該行為的段落。

盤點日期：2026-09-29（對照 v0.1.21）。同一份清單的第 2 條（環境變數改名 `SSHU__<NAME>`）與第 3 條（選取模式的 `gg/G`）已於
2026-09-29 修完並刪除（設計文件 §11.69）；只剩下面這一條，**等 filu** 修好參考實作再照搬。


## 先看

- 每修一處補 model test、做 mutation；同一個 commit 同步 dev-remarks 的「尚未符合 tdp 的地方」與設計文件（§11.70 起）；
  CHANGELOG 記 `[Unreleased]`。
- 修完刪掉這份清單。不 push、不發版；把這一輪寫進 terminu `.local/family-fix/sshu/README.md`。


## 1. `compositeDisp()` 在 popup 比畫面大時 panic —— D6（v0.1.21，等 filu 先修）

**現況**（`internal/ui/width.go`）：`compositeDisp()` 與 `clampSpan()` 照搬自 filu，popup 比畫面寬或高時起點算成負的，`strings.Repeat` 或
`bgLines[y+i]` panic；調整終端機大小的那一格就會遇到。

**規則**：D6（v0.1.21）—— 疊 popup 時 popup 可能比畫面寬或高（調整終端機大小的那一格還是舊尺寸）：起點取 0、超出畫面的部分切掉，
**不可以 panic**；測試的邊界要含這種情況。

**怎麼改**：等 filu 清單第 1 條修完（參考實作），照搬它的 `compositeDisp()` 與測試（kbu 的 `TestD6_CompositeDisp` 同一組邊界：比畫面寬、
比畫面高、兩者都大；不 panic、每列剛好畫面寬、列數等於畫面高）。sshu 保留自己的函式名（`dispW`、`clipANSI`）即可。
