# 初期ソース配布の検証記録

この文書は、コードを作成したセッションで実際に実行した範囲を記録します。CI設定が存在することを、CIが実行済みであることとは扱いません。

## 実行済み

| 検査 | 結果 |
|---|---|
| Goコアテスト | **59件合格、サブテスト込み74件合格**。race detector有効 |
| コアのgo vet | 合格 |
| テスト実行ファイルのWindows amd64クロスコンパイル | update/appパッケージで成功。Windows上で実行はしていない |
| HTML/JavaScriptブラウザースモーク | 描画契約用の固定配置を使い、ナビゲーション、経路強調、未網羅候補、CSPを確認 |
| サンプルアプリ | 入力不備、保存失敗と入力保持、再試行、削除失敗/再試行、削除後検索、閲覧専用権限を確認 |
| JavaScript構文 | Node.jsによる構文確認成功 |
| JSON Schema | Pythonの別実装でスキーマ自体とサンプル6ファイルを検証 |
| 埋込資産 | ドキュメントと初期化テンプレートの同期テスト合格 |

実行環境は **Go 1.23.2 / Linux amd64**、Node.js v22.16.0です。ブラウザーとPlaywrightの実測版、各テスト名は [results.json](verification/results.json) に記録しています。

ブラウザーの管理設定でfile://読み込みが拒否されたため、ブラウザー試験ではHTML文字列を `set_content` で渡しました。WindowsでHTMLファイルをダブルクリックする動作の確認とは区別しています。

## 未実行・未確認

外部Goモジュールを取得するネットワークが利用できませんでした。`go mod download` はproxy.golang.orgの名前解決で失敗し、`go test ./...` は外部モジュールのgo.sumエントリーがない段階で停止しました。

そのため以下は未確認です。

- **本物のelk-go・Go JSON Schema・YAMLを結合した総合テストとCLIビルド**。
- **Windows実機での起動、実行中exeの自己置換、PowerShellスクリプト実行**。
- GitHub Actionsの実行、Releaseからの実ダウンロードと更新。
- 任意の大規模モデル、階層状態、実アプリとの自動適合テスト。

依存のAPIを前提に実装したアダプターとWindows用テストは同梱しています。依存をダウンロードできるWindows環境で `scripts/build.ps1` を実行し、問題があれば修正したうえで配布してください。

**このZIPには、動作確認済みの製品exeも、偽のELK代替版exeも含めていません。** go.sumの値を推測して生成することもしていません。初回のネットワーク利用可能環境で確定し、コミットします。

## 結果の読み方

[サンプル検査結果](verification/sample-report.json) は、実際のコア解析から生成したものです。表示用fixtureで置換した検査結果ではありません。

HTMLの単体試験だけは `testkit.FixtureLayout` に固定座標を与えています。これはGo製ELKの配置品質を検証したことにはなりません。productionのmainは必ずadapters.LayoutJSONを利用し、ELKの失敗時にfixtureへフォールバックしません。

GitHub接続に新規リポジトリ作成操作がないため、**GitHub上のリポジトリ作成・初回pushは未実施**です。`scripts/publish-github.ps1` は利用者の認証済みGitHub CLIでこの操作を行うためのものです。

## 再実行

コアのみ:

```powershell
.\scripts\test-core.ps1
```

実依存・Windowsを含む総合確認:

```powershell
.\scripts\build.ps1
```

パッケージ化は総合確認の成功後に `scripts/package.ps1` を実行します。テストの合格は対象となるケースでの観測であり、検査器自身の数学的な正しさを完全に証明した意味ではありません。
