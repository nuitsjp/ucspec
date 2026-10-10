# 構造化仕様の記述ガイド

## 正本とファイル

`spec/project.json` と、その `files` から読み込むファイルが正本です。入力はJSONまたはYAMLです。テンプレートは、人とAIの差分確認や曖昧な型変換を避けるためJSONを使用します。YAMLの複数ドキュメント、アンカー/エイリアス、マージキー、非文字列キーは拒否します。JSON/YAMLとも同じキーの重複、未知の構造フィールド、同じIDの再定義を拒否します。

各ファイルには `"schemaVersion":"1"` を付けます。`$schema` フィールドは入力形式に含めません。エディターの設定から同梱の `spec/ucspec.schema.json` を関連付けてください。ファイルパターンはspec相対の通常globです。`**` による再帰globはありません。必要なパターンを明示します。

IDは英字で始まる英数字・`_`・`.`・`-`です。日本語はlabel/name/description/actionなどの値に使います。診断の位置は相対ファイル名とJSON Pointerです。YAMLパースエラーの一部は行番号も示しますが、全診断を行番号へ対応付ける機能ではありません。

## プロジェクト設定

```json
{
  "schemaVersion": "1",
  "name": "顧客管理",
  "repository": "nuitsjp/usecase-spec",
  "files": ["model/*.json", "usecases/*.json", "checks/*.json"],
  "output": "artifacts/usecase-spec",
  "initial": [{"state":"search", "values":{"role":"editor"}}],
  "limits": {"states":5000,"steps":8,"paths":100000,"combinations":256},
  "coverage": {"transitions":100,"conditions":100,"multipleConditions":100}
}
```

initialはシステムとして許す具体的な初期構成の集合です。省略された変数には変数定義のinitialが入ります。有限ドメインのすべての値を暗黙に初期状態にするわけではありません。

`coverage` のキーは `transitions`, `pairs`, `primePaths`, `boundedPaths`, `conditions`, `multipleConditions` です。指定した指標のみ閾値をCI判定に使います。未指定指標も計算・表示します。解析未完了は指標の閾値指定にかかわらず不合格です。

## 共通モデル

モデル定義のトップレベルは `variables`, `screens`, `states`, `events`, `conditions`, `transitions`。複数ファイルは一つのモデルに統合されます。

- 画面 `screens`: label、任意description。
- 状態 `states`: label、screenへの参照、受け付けるevents。終端ならterminal:true。
- イベント `events`: label、source(user/system/timer)、任意parameters、任意when。
- 変数 `variables`: 有限domain、initial、任意description。
- 遷移 `transitions`: from/to/event、任意guard、action、任意set。

同一画面内の編集中/保存中などは別状態として定義できます。同じ論理画面をユースケースごとに複製しません。ソフトウェア部品の共有は論理画面の同一性とは別です。

### 条件と式

式はコードではなく、値または一つの演算子を持つオブジェクトです。

| 式 | 意味 |
|---|---|
| `true`, `"saved"`, `1` | スカラー値 |
| `{"var":"role"}` | 現在の変数 |
| `{"var":"$state"}` | 現在の制御状態ID |
| `{"param":"valid"}` | 今回のイベント入力 |
| `{"condition":"canEdit"}` | 名前付き条件 |
| `{"eq":[式,式]}` / `ne` | 一致/不一致 |
| `lt`, `le`, `gt`, `ge` | 数値比較、二つの式 |
| `{"not":式}` | 否定 |
| `{"and":[式,式]}` / `or` | 論理結合。空配列不可 |

null、任意のGo/JavaScript、ファイル/APIアクセス、算術で無制限に増える値はありません。数値はJSON数値として扱い、巨大な整数の厳密演算には使いません。

ガードはbool、名前付き条件、and/or/notだけで構成します。原子的な条件は `conditions` に定義します。

```json
{
  "conditions": {
    "canEdit": {"eq":[{"var":"role"},"editor"]},
    "inputValid": {"param":"valid"}
  },
  "transitions": {
    "saveValid": {
      "from":"editing", "to":"saving", "event":"save",
      "guard":{"and":[{"condition":"canEdit"},{"condition":"inputValid"}]},
      "action":"入力を確認して保存を開始する",
      "set":{"outcome":"none"}
    }
  }
}
```

これは抜粋です。独立した入力ファイルにはschemaVersionと参照先の定義が必要です。名前付き原子条件の内部にはand/or/conditionを置きません。論理結合はガード側で記述します。入力パラメーターは、それを持つイベントの評価時のみ参照できます。

`action` は人が読む説明です。検証に使う効果は `set` に明示します。全代入は遷移前の値を読み、同時に反映します。ドメイン外への更新はエラーです。

### イベントと分岐

状態のeventsにあるイベントは、そのstate/data/inputでevent.whenが真の場合に受け付けます。受け付けるイベントは、ガードが成立する遷移がちょうど一つ必要です。0本は抜け、2本以上は競合です。

拒否も振る舞いとして記述するなら、相補的なガードを持つ「拒否通知を表示する」遷移を定義します。操作自体を利用不能にする場合はstate.eventsまたはevent.whenで表します。未定義だから無視する、という暗黙ルールはありません。

サンプルの `system` イベントは外部処理の結果を有限値で表すものです。実API呼び出しやプロセス実行は行いません。

## ユースケースとシナリオ

`usecases` の各項目に、name/goal/actor/starts/end/scope/scenariosを定義します。

`scope` はその目的の検査範囲を遷移IDで定義する項目です。シナリオ列から自動生成しません。これにより登録シナリオの書き漏れを分母から消しません。ただし、人がscopeから本来必要な遷移を除いた誤りまで完全に推測できるわけではありません。どのユースケースにも属さない遷移は警告されます。意図的ならtransition.unownedReasonで理由を残します。

`starts` は具体的な起点構成です。システム全体で到達できるか、各起点からendを満たす経路があるかを検査します。endは画面だけでなくoutcome等を用いた結果条件にします。成功、中止、拒否を終点として含めることもできます。

```json
{
  "normal": {
    "name":"顧客を変更して保存する",
    "start":{"state":"search","values":{"role":"editor"}},
    "steps":[
      {"transition":"searchFound"},
      {"transition":"openView"},
      {"transition":"openEdit"},
      {"transition":"saveValid","input":{"valid":true}},
      {"transition":"saveSucceeded","input":{"success":true}}
    ],
    "expect":{"eq":[{"var":"outcome"},"saved"]}
  }
}
```

これはscenarios内の抜粋です。各ステップにはexpectとnoteを追加できます。期待式は遷移後の状態を見ます。名前付きusecaseのend成立後に続けるシナリオは拒否します。ユースケース横断の継続操作はregressionsに記述します。

不成立シナリオは網羅済みに数えません。再試行1回と2回を同一とみなすパターンの包含検査は初版にありません。具体的な遷移列を明示します。

## 検証ルール

`properties` には次のkindを定義します。

- `invariant`: 全到達構成でassertが真。違反はそこへ至る最短BFS反例を示す。
- `reachable`: 初期集合の少なくとも一つからassertを満たす構成に到達できる（EF）。
- `recoverable`: すべての到達構成からassertを満たす構成へ進む経路が存在する（AG EF）。

recoverableは「必ずいつか完了する」（AF）ではありません。無限に再試行し続ける経路があっても、回復経路自体が存在すればこの検査には合格します。

```json
{
  "schemaVersion":"1",
  "properties":{
    "editRequiresTarget":{
      "kind":"invariant",
      "description":"編集中は対象が存在する",
      "assert":{"or":[
        {"ne":[{"var":"$state"},"editing"]},
        {"var":"exists"}
      ]}
    }
  }
}
```

`decisions` はstate/event/casesを持ち、各ケースのvalues/inputに対して有効な遷移ID集合enabledを独立に記述します。テーブル行はモデルの初期集合から到達しない入力も単体チェックとして指定できます。全テーブル行の期待一致を確認しますが、表の行自体が業務上の全組み合わせを列挙したかは別の判断です。

`regressions` は通常シナリオと同形式で、usecaseのscope/endに束縛されない横断経路です。`{"transition":"openEdit","reject":true}` は、現在状態からのその遷移が選択できないことを確認し、状態を変えません。これは否定テストであり、実装のイベントを実行した意味ではありません。regressionsは通常シナリオの定義網羅率へ加算しません。

## 変更レビュー

scopeを狭める、条件ドメインを減らす、閾値を下げる、性質を弱めるだけで検査を通さないでください。候補の理由・期待結果と共に人の判断を残します。共通変数/条件の変更は、取りこぼしを避けるため全ユースケースへの保守的な影響として表示されます。

この形式の機械検査はモデル内の整合性を強めるものです。記述されていない要求、抽象化で省いた実データの不具合、実装との不一致を完全に検出するものではありません。
