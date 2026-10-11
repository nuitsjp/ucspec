# UCSPEC

**利用者の目的を中心に、仕様・シナリオ・網羅性を一緒に育てるためのGo製CLIです。**

プロダクト内で一元管理した状態遷移モデルを、複数のユースケースが参照します。人とAIが構造化した仕様を編集し、決定論的な検査器が整合性・シナリオの定義網羅率・有限モデルの性質を検査します。結果を、図とシナリオを行き来できる自己完結型HTMLへ出力します。

**検証状況:** 2026-10-11にWindows amd64でコア・総合テスト、ELK・YAML・JSON Schema連携、実行中exeの置換、配布ビルド、実CLIの初期化とHTML生成、Edgeでのブラウザースモークを確認しました。GitHub Actionsの実行と実Releaseからの更新は未確認です。実行環境と確認範囲は [検証記録](docs/verification.md) を参照してください。

## 目的と設計原則

- 開発はユースケースの目的から始める。画面遷移図を先に完成させて分解する方式ではありません。
- 同じ画面・遷移・条件をユースケースごとにコピーしない。共有する論理画面は一度だけ定義します。
- 図、シナリオ表、網羅率は正本データから生成します。生成HTMLを直接編集しません。
- 意図した振る舞い・禁止事項・期待結果はモデルと独立に記述します。AIが候補を自動承認する方式ではありません。
- AIは編集を支援しますが、検査・生成の実行にAIサービスは必要ありません。

## 機能

| 分類 | 実装 |
|---|---|
| プロダクト導入 | `init`、dry-run、既存ファイル保護、ローカル版固定、PowerShellランチャー |
| 仕様の正本 | JSON / YAML、JSON Schema、参照解決、条件・有限データ更新 |
| シナリオ検査 | 経路・入力・ガード・終了条件・各ステップと最終期待結果 |
| 網羅率 | 遷移、連続2遷移、プライムパス、上限付き完了パス、条件、複合条件 |
| 有限モデル検査 | 不変条件、到達可能性、回復経路、イベント分岐の競合・抜け |
| 回帰・変更影響 | デシジョンテーブル、禁止操作、過去のreport.jsonとの差分 |
| 仕様書 | elk-goで配置したSVG、全体/ユースケース別図、シナリオ・反例・未網羅経路の強調 |
| 更新 | GitHub Releases、明示更新、SHA-256、Windows PE/版確認、旧版保持、プロダクトの横並び更新 |

`verified` は「モデルと設定された検査に合格」の意味です。すべての網羅率100%や実装のテスト完了を意味しません。

## 最初に動かす

### このソースZIPから

Windows amd64、PowerShell、Go 1.23以上、Goモジュールを取得できるネットワークが必要です。配布済み実行ファイルを使う側にはGo・Node.jsは不要です。

```powershell
# リポジトリのルートで実行。依存復元→総合テスト→Windowsビルド
.\scripts\build.ps1

# 同梱サンプルの検査・HTML生成
.\dist\ucspec.exe build --project examples/customer-crud/spec/project.json
Start-Process .\examples\customer-crud\artifacts\usecase-spec\index.html

# サンプルの実際のUI（サーバー不要の参考アプリ）
Start-Process .\examples\customer-crud\app\index.html
```

サンプルは「検索 → 閲覧 → 変更・削除」です。基本シナリオだけでは取り上げていない経路を意図的に残し、指標の差を確認できるようにしています。

### 自分のプロダクトへ導入

```powershell
.\dist\ucspec.exe init --dir C:\work\product-a --name Product-A --dry-run
.\dist\ucspec.exe init --dir C:\work\product-a --name Product-A
Set-Location C:\work\product-a
.\scripts\ucspec.ps1 build
```

`init` はモデル・ユースケース・検査ルール・記述ガイド・利用ガイド・スキーマ・版固定・ランチャーを生成し、実行中のCLIを `.tools/ucspec/<version>/` にコピーします。既存の異なる内容を上書きしません。

## 使い始める順序

1. [オンボーディング](docs/onboarding.md)でサンプルを確認する。
2. [利用者マニュアル](docs/user-manual.md)で初期化・生成・更新の操作を確認する。
3. [記述ガイド](docs/authoring.md)を参照して、最初のユースケースからモデルを置き換える。
4. 検査結果を読み、[網羅率と保証範囲](docs/coverage.md)を理解してからCIの閾値を決める。

設計と開発: [アーキテクチャ](docs/architecture.md) / [貢献・テスト](CONTRIBUTING.md) / [リリース](docs/releasing.md) / [セキュリティ](SECURITY.md)。

## GitHubへの初回登録

```powershell
# Git、認証済みGitHub CLI、Goが必要。初期値はprivate。既存repoは変更しません。
.\scripts\publish-github.ps1 -Repository nuitsjp/usecase-spec
```

このスクリプトはソースを検査し、依存ロックを作成してから新規リポジトリを作成・初回pushします。公開したい場合だけ `-Visibility public` を指定します。タグとReleaseは別の明示操作です。

## 対象外

実アプリのE2Eテスト、無限データの証明、必然的終了の証明、階層/並行状態機械、画面編集GUI、DB、サーバー、定期的な自動更新は初版の対象外です。ブラウザーに表示する図は検査結果のビューであり、グラフ配置を検証に逆利用しません。

UCSPEC本体はMIT。依存ライブラリーには別のライセンスが適用されます。[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) を参照してください。
