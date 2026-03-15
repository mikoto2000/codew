# TODO

`codew` を Codex CLI にさらに近づけるための改善候補。

## Review Follow-ups

`codew-review-pr-plan.md` をもとにした着手リスト。優先順。

- [ ] `cmd/chat.go` を分割し、会話制御を薄くする
  - `internal/chatloop/runner.go`
  - `internal/chatloop/approval.go`
  - `internal/chatloop/recovery.go`
  - `internal/chatloop/render.go`

- [x] 承認ロジックを 1 箇所に寄せる
  - `DecisionEngine` 相当を導入し、`allowed` / `denied` / `needs-user-approval` / `needs-network-escalation` を統一判定する

- [ ] 会話ターン単位の統合テストを追加する
  - tool 実行
  - 拒否時の tool result 永続化
  - checkpoint 作成
  - post-edit validation
  - JSON tool call 非表示

- [ ] `internal/tools/executor.go` を責務ごとに分割する
  - definitions / policy_eval / builtin_file / builtin_shell / builtin_web / mcp_bridge

- [x] `toolparse` に診断情報を返す
  - unknown tool
  - invalid arguments
  - malformed json
  - rejected by allowlist

- [ ] README に「最初の 3 パターン」と用途別起動例を追加する

- [ ] `chat` / `run` / `review` の共通実行基盤を作る

- [ ] ログを構造化して失敗解析しやすくする
  - `turn_started`
  - `model_response_received`
  - `tool_call_parsed`
  - `tool_call_denied`
  - `tool_call_executed`
  - `checkpoint_created`
  - `post_validate_finished`

- [ ] 「安全なデフォルト」をもう一段強くする
  - mutating tool の preview 必須表示
  - `shell_exec` allowlist
  - `--auto-approve` 警告強化
  - `web_search` の URL 出力方針統一

- [ ] `go.mod` のモジュール名とリポジトリ名の整合を検討する

## Existing Progress

- [x] サンドボックス権限の本格運用
  - ツールごとに read/write/network/exec を明示し、実行前に一貫した権限判定を行う

- [x] ネットワーク昇格承認フロー
  - 通常拒否される外部アクセスを、都度承認またはルール承認で許可できるようにする

- [x] 高精度なパッチ適用エンジン
  - `git apply` 失敗時に文脈再解決や小分け適用でリカバリする

- [x] レビュー特化モード
  - 変更を重大度順に整理して報告する `review` ワークフローを実装する

- [x] 会話計画UI（Plan mode 相当）
  - ステップの進行状態を管理し、ユーザーに明示する

- [x] マルチツール並列オーケストレーションの高度化
  - 依存関係つきで複数ツールを並列実行できる計画エンジンを実装する

- [x] Web 情報の厳密ソース管理
  - 鮮度確認、出典追跡、引用ルールを組み込んだ検索応答にする

- [x] エラーテレメトリ/実行トレース
  - ターン単位の実行トレース・失敗分析ログを可視化する
