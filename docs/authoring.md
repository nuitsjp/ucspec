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
  "repository": "nuitsjp/ucspec",
  "files": ["model/*.json", "usecases/*.json", "checks/*.json"],
  "output": "artifacts/ucspec",
  "initial": [{"state":"search", "values":{"role":"editor"}}],
  "limits": {"states":5000,"steps":8,"paths":100000,"combinations":256}
}
```

initialはシステムとして許す具体的な初期構成の集合です。省略された変数には変数定義のinitialが入ります。有限ドメインのすべての値を暗黙に初期状態にするわけではありません。

`coverage` のキーは `states`, `compositeStates`, `transitions`, `pairs`, `primePaths`, `boundedPaths`, `conditions`, `multipleConditions` です。指定した指標のみ閾値をCI判定に使います。未指定指標も計算・表示します。解析未完了は指標の閾値指定にかかわらず不合格です。

## 共通モデル

モデル定義のトップレベルは `variables`, `screens`, `states`, `choices`, `events`, `conditions`, `transitions`。複数ファイルは一つのモデルに統合されます。`states`と`choices`は共通のID名前空間を使うため、同じIDを再利用できません。

- 画面 `screens`: label、任意description。
- 状態 `states`: label、screenへの参照、受け付けるevents。部分状態ならparent、複合状態ならinitial。末端の終端状態ならterminal:true。描画順を指定する場合は任意の非負整数`layoutLayer`を置き、同じ親の中で小さい値を上の配置帯にします。
- イベント `events`: label、source(user/system/timer)、任意parameters、任意when。
- 変数 `variables`: 有限domain、initial、任意description。
- 選択疑似状態 `choices`: label、任意の既存複合状態へのparent。通常状態や画面ではなく、イベントを持ちません。
- 遷移 `transitions`: from/to、任意`kind`（`external`/`internal`）、任意event、任意guard、任意guardLabel、action、任意set、任意`defaultVisible`。
- 共通ルール `commonRules`: Documentのルートに置く、IDから`{name,description}`へのmap。有限性質ではなく、複数のユースケースに共通する文章を記述します。

イベントの`label`と`action`は、主語やsource名を付けず、簡潔な語句で記述します。イベントはユーザー操作・時間経過・外部処理の応答などの契機、アクションは選択した遷移で行うシステムの振る舞いです。`source`（`user`/`system`/`timer`）は定義データとして保持し、`user`はユーザー操作、`system`は外部処理の応答、`timer`は時間経過を表します。`action`キー自体は必須ですが、イベントを受け付けるだけでシステム動作を記述しない遷移は空文字を指定できます。

### 外部遷移と内部遷移

`transitions.kind`を省略するか`external`にすると外部遷移、`internal`にすると内部遷移です。省略時は従来どおり外部遷移として扱います。内部遷移は`from`と`to`に同じ通常stateを指定し、choiceを端点にできません。イベント、ガード、アクション、`set`は通常の遷移と同じように実行しますが、現在のアクティブな末端stateを保持します。親stateを指定した場合も現在の子stateを維持し、退場・再入場やinitialへの入り直しは起こりません。外部遷移の自己遷移は対象へ入り直し、複合stateならinitialから開始します。

```json
{
  "transitions": {
    "refreshSearch": {
      "kind": "internal",
      "from": "search", "to": "search", "event": "search",
      "guard": {"condition": "exists"},
      "action": "一覧を表示",
      "set": {"searched": true}
    }
  }
}
```

内部遷移も通常の遷移IDです。シナリオのstep、`scope`、遷移・連続経路・条件系の網羅率に含めます。

同一画面内の編集中/確認中などは別状態として定義できます。同じ論理画面をユースケースごとに複製しません。ソフトウェア部品の共有は論理画面の同一性とは別です。

### 共通ルールと抽象化方針

DB、通信、実メール送信などのシステムエラー処理は、業務状態の分岐を増やさず、Documentのルートに共通ルールとして記述します。`commonRules`はIDから`name`と`description`へのmapです。HTMLでは「共通ルール」へ表示されます。

```json
{
  "schemaVersion": "1",
  "commonRules": {
    "systemError": {
      "name": "システムエラー",
      "description": "システムエラー時は共通メッセージを表示し、編集入力を保持する。更新・削除の確定は原子的に行い、失敗時は直前の確定データを維持する。"
    },
    "progress": {
      "name": "進捗表示",
      "description": "更新・削除の進捗表示は操作に付随するUI表現とし状態網羅率の対象としない"
    }
  }
}
```

`commonRules`は有限性質を検査する`properties`とは別の、実装方針を共有する文章です。モデル検査は、システムエラー時の共通メッセージ、入力保持、原子的な確定などの実装を保証しません。実装に必要な有限の業務上の結果だけを状態・イベント・変数へ抽象化し、DBの内部状態や進捗表示を状態として追加しないでください。

顧客管理サンプルの検索は、画面`search`で`search`/`open`イベントを受け付ける単一の末端状態です。検索条件と結果を同じ検索画面に表示します。`searchFound`（顧客あり）と`searchEmpty`（0件）は、`search`から`search`へのガード付き内部遷移として検索結果を更新します。`searched`というbool変数は検索実行済みだけを記録します。`open.when`は`searched`かつ`exists`（一覧に閲覧対象がある）で、`open`イベントは閲覧時点の存在確認結果を表す有限入力`exists`（`true`/`false`）を持ちます。`openCustomer`は入力値を現在の`exists`へ反映してから`customerAvailability`という選択疑似状態へ入り、`openView`は顧客ありなら`view`へ、`openGone`は顧客なしなら`search`へ戻って再検索とアラート表示を行います。この入力はユーザーが任意に選ぶ値ではなく、実DBを使わないサンプルで閲覧時の再確認を抽象化したものです。0件のアラートは`searchAlert`で検査します。検索前・0件・N件を子状態に分けず、状態が変わっても画面が変わるとは限りません。

顧客管理サンプルは社内担当者が使うシステムとして、顧客検索を初期画面にします。参考アプリの左メニューは「顧客管理」「設定」への機能移動、仕様書の左メニューはパッケージ→ユースケース→シナリオの分類です。未更新の入力がある機能切り替え時の破棄確認と取消時の入力保持は横断回帰シナリオで確認します。削除確認中にメニューイベントが受け付けられないことは、独立した判定表で検査します。通信、実メール送信、永続化はモデル化しません。メニューイベントの`discard`は、未更新の入力を破棄して移動する場合を`true`、確認を取り消して入力を保持する場合を`false`として抽象化しています。未更新の入力がない場合は確認なしに移動できるため、`true`に対応します。

設定は画面と状態が同名の`notificationSettings`という単一の末端状態です。下書きの編集を`editNotification`、更新を`updateNotification`、取消を`cancelNotification`で表し、いずれも同じ状態への自己遷移にします。通知の成功・失敗結果、進捗、再試行はモデル化しません。取消時は下書きを確定済みの値へ戻します。

### 複合状態と部分状態

statesはIDで管理し、parentで親状態を指定します。子を持つ状態は複合状態で、そのinitialには直接の子状態を一つ指定します。複合状態へ入るとinitialをたどって末端状態に入ります。子を直接遷移先に指定することもできます。

```json
{
  "states": {
    "view": {"label":"項目閲覧", "screen":"view", "events":[], "initial":"viewing"},
    "viewing": {"label":"通常表示", "screen":"view", "parent":"view", "events":["requestDelete"]},
    "confirming": {"label":"削除確認", "screen":"view", "parent":"view", "events":["confirmDelete","cancelDelete"]}
  }
}
```

これは抜粋です。受け付けるイベントと遷移、参照する画面の定義は別途必要です。子が削除確認なら、確認中と親の項目閲覧の両方が有効です。実行構成のstateと`$state`は末端状態のIDを持ちます。親を含む有効状態の検査には`{"inState":"view"}`を使います。project.initial、usecase.starts、scenario.start、decision.stateに複合状態を指定した場合も、initialをたどった末端状態から評価します。

parentの循環は許しません。子を持つ状態のinitialは必須で、末端状態にはinitialを置けません。並行領域、履歴状態、入場・退場アクションは扱いません。複合状態へ戻る遷移は、前回の子状態ではなくinitialへ入ります。

### 選択疑似状態

`choices` はUMLの選択疑似状態を表します。通常の`states`とは別の構成要素で、画面・イベント・終端状態ではありません。`parent`を指定する場合は既存の複合状態だけを指定します。choiceを`project.initial`、`usecase.starts`、`scenario.start`の起点にはできません。`states`と`choices`のIDは重複させません。

choiceへ入る遷移は既存の`event`/`guard`/`action`/`set`をそのまま持てます。選択疑似状態へ入る遷移の`set`を適用してからchoiceのガードを評価します。choiceから出る分岐は`from`にchoice ID、`to`に通常状態または別のchoice IDを指定し、`event`を省略して`guard`を必須にします。分岐に外部イベントは入りません。choiceからchoiceへの鎖は許しますが、choiceの循環は入力検証で拒否します。

同じchoiceの分岐は、到達した各構成でガードがちょうど一つ成立しなければなりません。重複成立は競合、どれも成立しない場合は抜けとして有限モデル検査に失敗します。暗黙のelse分岐や、イベントのない通常状態遷移を追加しません。choiceを含む内部の実行グラフでは中間ノードを保持しますが、choiceで安定状態になったとは扱いません。終了条件と状態プロパティは通常状態へ戻った安定状態の構成で評価します。

図では遷移を複数行の「イベント [ガード] / アクション」で表示し、位置でイベントとアクションを区別します。`guardLabel`を指定した場合は日本語などの表示用文言として使いますが、成立判定の正本は`guard`式です。`source`に対応する「ユーザー：」「システム：」「タイマー：」の接頭辞は表示しません。`action`はシステムの振る舞いの説明、`set`は有限変数へ実際に反映する更新です。

`defaultVisible`は外部・内部を問わず、検査の入力ではなく図の表示集合を決める属性です。`true`なら通常図のレイアウトへ含め、未指定または`false`なら通常図のレイアウトから除外します。シナリオ、未網羅候補、反例で選択した経路は属性に関係なく通常表示の集合へ加わり、「全遷移を表示」では全遷移を配置します。表示集合が変わるとelk-goが状態の位置、複合stateの枠高さ、図幅を再計算します。非表示の内部遷移行は通常図の枠高さに含めません。この属性は実行エンジン、共通モデル、`scope`、網羅率を変えません。通常図へ含めたい遷移は`true`を明示してください。

全体図、ユースケース別図、複合stateの枠内は、共通のELK設定で上から下（`DOWN`）へ自動配置し、`NETWORK_SIMPLEX`をノード配置方式に使います。この設定は図全体へ共通に適用し、個別状態の配置属性は追加しません。状態の任意属性`layoutLayer`は同じ親の中での配置順だけを指定し、非負整数の小さい層を上に、同じ層を同じ配置帯にします。異なる層の順序は保証され、同じ層の帯内の上下・横配置は自動です。同じY座標や横一列は保証しません。未指定の状態とchoiceは自動配置に任せます。全体図とユースケース別図の外側開始を示す黒丸は各図の最上段・左右中央に固定し、複数の開始状態がある場合は開始記号の列全体を中央に並べます（個々の記号は重ねません）。その他の最上位の状態・ディシジョンを下へ配置します。複合stateの枠内にあるinitial、横位置、間隔、矢印は自動です。`layoutLayer`はピクセル位置や層の高さを指定するものではありません。

```json
{
  "choices": {
    "customerAvailability": {"label":"顧客の有無"}
  },
  "transitions": {
    "searchFound": {
      "defaultVisible": true,
      "kind":"internal",
      "from":"search", "to":"search", "event":"search",
      "guard":{"condition":"exists"}, "guardLabel":"1件以上",
      "action":"一覧を表示",
      "set":{"outcome":"none", "searched":true, "searchAlert":false}
    },
    "searchEmpty": {
      "defaultVisible": true,
      "kind":"internal",
      "from":"search", "to":"search", "event":"search",
      "guard":{"not":{"condition":"exists"}}, "guardLabel":"0件",
      "action":"アラートを表示",
      "set":{"outcome":"empty", "searched":true, "searchAlert":true}
    },
    "openCustomer": {
      "defaultVisible": true,
      "from":"search", "to":"customerAvailability", "event":"open",
      "action":"",
      "set":{"exists":{"param":"exists"}}
    },
    "openView": {
      "defaultVisible": true,
      "from":"customerAvailability", "to":"view",
      "guard":{"condition":"exists"}, "guardLabel":"顧客あり",
      "action":"顧客を表示", "set":{"outcome":"viewed"}
    },
    "openGone": {
      "defaultVisible": true,
      "from":"customerAvailability", "to":"search",
      "guard":{"not":{"condition":"exists"}}, "guardLabel":"顧客なし",
      "action":"再検索してアラートを表示",
      "set":{"outcome":"empty", "searched":true, "searchAlert":true}
    }
  }
}
```

これは抜粋です。`exists`は`conditions`に定義します。`searchFound`と`searchEmpty`は同じ`search`状態に留まる内部遷移であり、通常の遷移IDとしてシナリオと網羅率へ含めます。`openCustomer`だけがイベントを持ってchoiceへ入り、`openView`または`openGone`の分岐を、内部の実行ステップとして別々に明示します。分岐側は入力を持たず、選んだガードに合わない構成ならシナリオは失敗します。

### 条件と式

式はコードではなく、値または一つの演算子を持つオブジェクトです。

| 式 | 意味 |
|---|---|
| `true`, `"updated"`, `1` | スカラー値 |
| `{"var":"role"}` | 現在の変数 |
| `{"var":"searched"}` | 検索を実行済みか |
| `{"var":"exists"}` | 既存の顧客抽象が存在するか |
| `{"var":"$state"}` | 現在の末端状態ID |
| `{"inState":"view"}` | 指定した状態が現在の末端状態またはその親として有効か |
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
    "canEdit": {"eq":[{"var":"role"},"editor"]}
  },
  "transitions": {
    "updateCustomer": {
      "defaultVisible": true,
      "from":"edit", "to":"view", "event":"update",
      "guard":{"condition":"canEdit"},
      "action":"入力を検証して更新する",
      "set":{"outcome":"updated", "revision":"updated"}
    }
  }
}
```

これは抜粋です。独立した入力ファイルにはschemaVersionと参照先の定義が必要です。イベント`update`はbool入力`valid`を持ち、`when: {"param":"valid"}`で有効な入力だけを受け付けます。入力不備は更新前に拒否し、失敗結果のイベントや描画用の失敗分岐は追加しません。名前付き原子条件の内部にはand/or/conditionを置きません。論理結合はガード側で記述します。入力パラメーターは、それを持つイベントの評価時のみ参照できます。

`action` は人が読む説明です。検証に使う効果は `set` に明示します。全代入は遷移前の値を読み、同時に反映します。ドメイン外への更新はエラーです。

### イベントと分岐

状態のeventsにあるイベントは、そのstate/data/inputでevent.whenが真の場合に受け付けます。子状態は親のeventsも継承します。受け付けるイベントに対する遷移は末端状態から親へ順に調べ、有効な遷移がある最も内側の状態を採用します。その状態でガードが成立する遷移がちょうど一つ必要です。同じ状態に2本以上あれば競合、すべての有効状態に0本なら抜けです。子に有効な候補があれば親の候補は実行せず、ガードも条件網羅の評価対象にしません。`kind`が`internal`の候補は同じstateに留まり、`set`やアクションを実行しても現在の子stateを維持します。入力不備など受理前に拒否する条件はevent.whenで表し、更新失敗のためだけの状態・イベント・分岐は描画しません。

操作自体を利用不能にする場合はstate.eventsまたはevent.whenで表します。未定義だから無視する、という暗黙ルールはありません。

サンプルの `system` イベントは外部処理の応答を有限値で表すものです。実API呼び出しやプロセス実行は行いません。

## ユースケースとシナリオ

`usecases` の各項目に、name/goal/actor/starts/end/scope/scenariosを定義します。任意の`package`で所属するユースケースパッケージのIDを指定します。

パッケージは`usecasePackages`で定義し、`name`と任意の`parent`を持ちます。親を省略するとルートになります。同じパッケージに子パッケージとユースケースを置けます。親の不存在、自己参照、循環、存在しないパッケージへの所属はエラーです。`package`未指定のユースケースはルートに表示します。

ネストの一般例として、次のようにパッケージを定義できます。

```json
{
  "schemaVersion": "1",
  "usecasePackages": {
    "operations": {"name": "運用設定"},
    "notifications": {"name": "通知設定", "parent": "operations"}
  }
}
```

一般例では、別ファイルのユースケースに`"package":"notifications"`を指定すると、その配下に表示します。パッケージは整理単位です。所属やネストによって状態遷移、`scope`、開始・終了条件、網羅率の分母は変わりません。パッケージ名や親の変更は、所属するユースケースへの変更影響に記録します。

顧客管理サンプルでは、`customerManagement`（顧客管理）に`searchCustomer`（顧客検索）と顧客の閲覧・編集・削除を、`settings`（設定）に`updateNotifications`（顧客情報の更新通知設定）を所属させます。いずれもルートのパッケージです。深いネストは自然な業務分類としてサンプルへ追加せず、ネスト機能自体は一般例と検証用仕様で確認します。

各シナリオ（regressionsを含む）には `name`、`goal`、`expectedOutcome` を空でない文章で記述します。`goal` は利用者が何のために実施するか、`expectedOutcome` はその目的に対して何が成立して終わるかです。完了画面の表示だけを期待結果にせず、保存された内容、取消時に維持される情報、権限によって保護される情報などを書きます。失敗・中止・拒否も、その場合の結果を明示します。

人が読む `expectedOutcome` と機械検査する `expect` は別の項目です。前者は顧客との仕様確認や実装・E2Eテストの根拠、後者は有限モデル上の期待式です。文章の正しさ、実データの永続化、モデルと実装の一致はこの検査だけでは保証しません。HTMLでは目的と期待結果を操作列より先に表示します。

`scope` はその目的の検査範囲を遷移IDで定義する項目です。シナリオ列から自動生成しません。これにより登録シナリオの書き漏れを分母から消しません。ただし、人がscopeから本来必要な遷移を除いた誤りまで完全に推測できるわけではありません。どのユースケースにも属さない遷移は警告されます。意図的ならtransition.unownedReasonで理由を残します。

`starts` は具体的な起点構成です。システム全体で到達できるか、各起点からendを満たす経路があるかを検査します。endは画面だけでなくoutcome等を用いた結果条件にします。成功、中止、拒否を終点として含めることもできます。

```json
{
  "normal": {
    "name":"顧客を編集して更新する",
    "goal":"顧客情報を最新の内容へ編集して更新する",
    "expectedOutcome":"有効な入力で顧客情報が更新され、閲覧画面で更新結果を確認できる",
    "start":{"state":"search","values":{"role":"editor"}},
    "steps":[
      {"transition":"searchFound"},
      {"transition":"openCustomer","input":{"exists":true}},
      {"transition":"openView"},
      {"transition":"openEdit"},
      {"transition":"updateCustomer","input":{"valid":true}}
    ],
    "expect":{"and":[
      {"eq":[{"var":"outcome"},"updated"]},
      {"eq":[{"var":"revision"},"updated"]}
    ]}
  }
}
```

これはscenarios内の抜粋です。各ステップにはexpectとnoteを追加できます。期待式は遷移後の状態を見ます。名前付きusecaseのend成立後に続けるシナリオは拒否します。ユースケース横断の継続操作はregressionsに記述します。

不成立シナリオは網羅済みに数えません。choice分岐を含むシナリオも、選択疑似状態へ入る遷移と選択分岐の順序を明示します。別入力による再実行を同一とみなすパターンの包含検査は初版にありません。具体的な遷移列を明示します。

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
        {"ne":[{"var":"$state"},"edit"]},
        {"var":"exists"}
      ]}
    }
  }
}
```

`decisions` はstate/event/casesを持ち、各ケースのvalues/inputに対して有効な遷移ID集合enabledを独立に記述します。テーブル行はモデルの初期集合から到達しない入力も単体チェックとして指定できます。全テーブル行の期待一致を確認しますが、表の行自体が業務上の全組み合わせを列挙したかは別の判断です。

図の`choices`と、独立した期待を記す`decisions`は別の概念です。`decisions`は`state`にchoice IDを指定し、`event`を省略して分岐も検査できます。choiceの分岐網羅は通常の遷移と同じ定義IDを使い、シナリオで選択した分岐遷移を確認します。

`regressions` は通常シナリオと同形式で、usecaseのscope/endに束縛されない横断経路です。`{"transition":"openEdit","reject":true}` は、現在状態からのその遷移が選択できないことを確認し、状態を変えません。これは否定テストであり、実装のイベントを実行した意味ではありません。regressionsは通常シナリオの定義網羅率へ加算しません。

## 変更レビュー

### 未網羅項目の評価記録

ユースケースの任意の `coverageReviews` に、未網羅項目を評価した記録を保存できます。一つの記録の `requirements` に複数の項目を指定して、同じ業務ルール・設計要求としてまとめられます。対象は同じ指標内に限ります。

`kind` は `states`、`compositeStates`、`transitions`、`pairs`、`primePaths`、`boundedPaths`、`conditions`、`multipleConditions` の8指標から選びます。末端状態と複合状態は別に評価し、親への到達だけで未通過の子を確認済みとしません。choiceは状態網羅の対象には加えず、その分岐遷移を遷移・経路・条件の指標で評価します。

```json
{
  "coverageReviews": {
    "repeatedNotificationEdit": {
      "kind": "boundedPaths",
      "requirements": ["report.jsonのmissingにあるIDを転記"],
      "status": "represented",
      "reason": "未更新の通知入力を複数回変更した後の有効化は、最終入力で更新するenableシナリオで代表させる",
      "scenario": "enable",
      "architectureRequirements": [
        "更新前の入力変更では現在の通知設定を変更しない",
        "入力変更の回数によらず、更新時の最終入力を通知設定へ反映する"
      ],
      "basisHash": "report.jsonの対象usecaseのreviewBasisHashを転記"
    }
  }
}
```

これは通知設定ユースケースに対する形式の説明であり、代表判断を承認済みとするものではありません。IDとハッシュのプレースホルダーをそのまま入力しないでください。`check` または `build` 後のreport.jsonで、対象ユースケースの `coverage[].missing[].id` と `reviewBasisHash` を取得します。HTMLでも「評価記録を編集するための情報」を確認できます。

| status | 意味 |
|---|---|
| `addScenario` | 追加の目的・期待結果があり、シナリオ追加が必要 |
| `represented` | 指定した既存シナリオで確認観点を代表させる。`scenario` に同じユースケース内のシナリオIDを必須指定 |
| `forbidden` | 起きてはいけない振る舞いとして、モデル・設計の修正が必要 |

`kind`、一つ以上の `requirements`、`status`、空でない `reason`、`basisHash` は必須です。`architectureRequirements` は参照IDではなく設計への要求を記述する文章の配列です。同じ指標・項目を複数の記録へ重複登録しません。現在の評価がない項目は「未評価」として表示され、未決のものを無理に分類する必要はありません。

`represented` は経路を実際に網羅した意味ではありません。評価記録は生の網羅率、閾値、検査の合否を変更しません。設計要求の記録も、それが実装・テストで満たされたという保証ではありません。禁止する判断を記録しただけではモデルから経路は消えません。

`reviewBasisHash` はプロジェクト設定、共有モデル・検査定義、対象ユースケース（シナリオを含む）、ツール版を元に計算し、評価記録自身は除きます。仕様・設定・版が変わった記録、対象が現在の未網羅項目にない記録、代表シナリオが成立しない記録、解析が未完了の記録は `stale`（再評価が必要）になります。出力先などの設定変更でも保守的に再評価を求める場合があります。古い記録は隠さず表示します。ハッシュだけを機械的に更新せず、経路・期待結果・設計要求を見直してから記録を更新または整理してください。

### 変更影響の限界

変更影響は再確認の候補です。定義の差分、依存関係、モデル上のシナリオ成立性は自動で調べられますが、目的や期待結果の文章の意味が変わったか、顧客への再確認が必要かは開発者が判断します。顧客との合意状態の管理やE2Eテスト結果との連携は対象外です。

scopeを狭める、条件ドメインを減らす、閾値を下げる、性質を弱めるだけで検査を通さないでください。候補の理由・期待結果と共に人の判断を残します。共通変数/条件の変更は、取りこぼしを避けるため全ユースケースへの保守的な影響として表示されます。

この形式の機械検査はモデル内の整合性を強めるものです。記述されていない要求、抽象化で省いた実データの不具合、実装との不一致を完全に検出するものではありません。
