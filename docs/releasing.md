# 初回登録とリリース

## 前提

この納品時点でGitHubリポジトリ/Releaseを作成した事実はありません。用意しているのは登録用ソースとスクリプトです。初回作成にはWindows、Git、Go、GitHub CLIとログイン済みの作成権限が必要です。

GitHubの接続先が異なる場合はRepositoryを変更します。更新元はbuild時のldflagsで実行ファイルに埋め込みます。ソース上のGoモジュールパスは固定の内部importに使うため、ホスティング先を変更するだけなら一括置換は不要です。

## 最初のpush

```powershell
gh auth login
.\scripts\publish-github.ps1 -Repository nuitsjp/ucspec
```

既定はprivateです。公開する場合だけ `-Visibility public` を指定します。既存リポジトリ、既存originには書き込みません。認証トークンはソースに保存しません。

スクリプトは実依存を復元し、総合テストとWindowsビルドを行います。成功して作られたgo.sumを含めて初回コミットします。テストで問題が発生した場合は、修正して再実行します。CIが緑になるまでReleaseを発行しません。

## バージョンを公開する

```powershell
.\scripts\build.ps1 -Version 0.1.0 -Repository nuitsjp/ucspec
git add go.mod go.sum
# 変更がある場合は理由と共にコミットする
git tag v0.1.0
git push origin v0.1.0
```

release.ymlがWindowsで依存復元、go test、go vet、実exe生成、サンプルの実ELK生成を行い、梱包してReleaseを作成します。go.mod/go.sumがタグ時点から変わっていた場合は公開を拒否します。

生成物の名前はupdaterと一致させます。

```text
ucspec_0.1.0_windows_amd64.zip
checksums.txt
```

ZIPのルートにucspec.exeを置きます。README、docs、LICENSE、依存ライセンス、SOURCES.txt、BUILD-INFO.txtも含みます。同じバージョンのバイナリを後から差し替えず、新しいバージョンを発行してください。

## リリース前チェック

Windows上での `go test ./...`、実行中イメージ置換テスト、init→build、サンプルHTML表示、署名なし配布の組織方針、依存ライセンス/ソース提供先、版固定更新と旧版復帰を確認してください。

現在の実装はSHA-256を照合しますがAuthenticode署名を行いません。公開元の侵害に対する署名ベースの信頼を保証するものではありません。将来署名を追加する場合は、署名鍵をGitHub Secrets等で管理し、リポジトリへコミットしないでください。

GitHub Actionsの権限やRunnerのポリシー、依存取得先への通信が利用環境で許可されているかも確認します。CLIの通常check/buildにネットワークは不要です。
