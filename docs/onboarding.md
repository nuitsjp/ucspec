# オンボーディング

## 1. 役割を理解する

UCSPECは「画面を描くツール」ではなく、構造化されたユースケース仕様を検査して文書化するツールです。

人: 目的、期待結果、禁止事項、未網羅ケースを追加する意味を判断します。
AI: 合意した目的に沿って共有モデル・参照・シナリオを編集し、CLIを実行します。
CLI: 同じ入力から同じ検査結果を算出します。意味のあるシナリオかどうかは自動承認しません。

## 2. サンプルを読む

`examples/customer-crud/app/index.html` を開くと、検索・閲覧・変更・削除を試せます。閲覧専用/編集可能の切替、保存/削除の失敗、削除後の再検索を試してください。これは参考アプリであり、CLIの検査がアプリとの適合を保証するものではありません。

ソースビルドはREADMEの `scripts/build.ps1` を使用します。成功後に次を実行します。

```powershell
.\dist\ucspec.exe build --project examples/customer-crud/spec/project.json
```

`artifacts/usecase-spec/index.html` はサンプルプロダクトのディレクトリに作成されます。全体図、ユースケース、シナリオ、未網羅経路の順に見てください。プライムパスと遷移網羅の値が異なることを確認します。

## 3. 自分のプロダクトを初期化する

```powershell
C:\Tools\ucspec\ucspec.exe init --dir C:\work\my-product --name MyProduct --dry-run
C:\Tools\ucspec\ucspec.exe init --dir C:\work\my-product --name MyProduct
```

`create` / `unchanged` / `conflict` の一覧を確認します。conflictがある場合、既存ファイルを手で確認してから構成を調整してください。強制上書きはありません。

初期化時は顧客CRUDの説明用仕様が入ります。プロダクト固有の仕様を推測して生成するわけではありません。最初の目的を決め、サンプルを自分の仕様へ置き換えます。

## 4. 最初のユースケースを作る

最初から全画面を洗い出す必要はありません。目的・成功/中止条件・役割を定め、必要な状態・イベント・遷移を共通モデルへ追加します。ユースケースの `scope` と `starts/end` は、シナリオを書く前に独立に定義します。

`spec/AGENTS.md` にプロダクト固有の約束を追記します。たとえば「変更/削除は閲覧を経由する」「削除前に確認する」「権限不足の場合は入力データを変更しない」です。

AIに渡す作業例:

> 顧客を変更する目的の仕様を追加してください。既存の検索・閲覧の遷移IDを再利用します。保存成功・入力不備・保存失敗・キャンセルを定義し、独立した期待結果を記述してください。検査閾値やルールを勝手に緩めず、check/buildと変更影響を提示してください。

## 5. レビューとCIへつなぐ

```powershell
.\scripts\ucspec.ps1 build
Copy-Item .\artifacts\usecase-spec\report.json .\baseline.json
# 仕様を編集
.\scripts\ucspec.ps1 build --baseline baseline.json
```

確認するものは「モデル差分」「影響するユースケース」「無効になったシナリオ」「増えた未網羅項目」「検証ルール/閾値の変更」です。生成図の見た目だけではレビューしません。

新しい開発環境では `scripts/setup-ucspec.ps1` で指定版だけを取得し、`scripts/ucspec.ps1 check` をCIで実行します。リリースがまだない初期開発では、ソースから作った同版CLIを `init` でコピーしてください。
