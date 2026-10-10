# 開発・テスト

Windows向け開発にはGo 1.23以上とPowerShellを使用します。初回はネットワークで依存を復元します。

```powershell
go mod tidy
go test ./...
go vet ./...
.\scripts\build.ps1
```

`go.sum` は初回復元で作成した後にコミットします。このソース作成時は外部Goモジュール取得ができなかったため、依存を偽装したロックを作成していません。

## テストの区分

- model/engine/compiler/analysis: 式、同時代入、ID整合、状態探索、網羅率、独立期待、上限到達、変更影響。
- project: dry-run、衝突時の無変更、再実行、既存ファイル保護。
- update: バージョン、HTTPSリダイレクト、トークン、破損、危険なZIP、バックアップ、版固定。
- report/app: テスト専用配置座標による描画契約、エスケープ、決定論、CLI終了コード。
- adapters: 本物のYAML/JSON Schema/elk-goによる総合テスト。外部依存取得が必要。
- `replace_windows_test.go`: 実行中のWindowsテストexeを自分で置き換える。Windowsでのみ動作。

ネットワークなしでコアだけ確認する場合:

```powershell
.\scripts\test-core.ps1
```

コアテストに通ったことを総合テスト・実機試験完了と表現しないでください。`testkit.FixtureLayout` は製品フォールバックではありません。

## 変更時の約束

仕様の意味を変更するときは互換性とschemaVersionを検討します。探索上限で打ち切った場合を成功にしません。網羅率の分母を登録シナリオから生成しません。回帰テストで期待値を変更する場合は、業務的な理由も記録します。

閲覧JSの変更は `node --check internal/assets/viewer/app.js` でも確認できます。ただしNode.jsは製品実行・ビルドの必須依存ではありません。

`docs/authoring.md` と `docs/user-manual.md` は `internal/assets/authoring.md` / `manual.md` に同じ内容を置きます。buildスクリプトはこの同期を確認します。サンプル仕様とinitテンプレートも同期しておきます。

テスト用モデルの追加は小さく、結果を人が手計算できる形を優先します。形式的にもっともらしい名前だけで新しい検証を追加しないでください。

## 任意のブラウザースモークテスト

PythonとPlaywright/Chromiumがある開発環境では、実CLIで生成したHTMLに対して以下を実行できます。これはGoビルドや通常利用の必須依存ではありません。

```powershell
python scripts/browser-smoke.py --report examples/customer-crud/artifacts/usecase-spec/index.html
```

既にあるChromiumを使う場合は `--chromium <実行ファイル>` を指定します。シナリオ選択、経路強調、反例候補とサンプルCRUD画面を検査します。この試験はHTML文字列をブラウザーへ渡すもので、Windowsのfile://表示やELKの配置品質をそれ自体で保証するものではありません。結果はvalidation/browserに出力します。
