# TODO

`codew` を Codex CLI に近づけるための改善候補。

## High Priority

- [x] `apply_patch` 相当の安全編集機能を追加する
  - 直接上書きではなく差分適用ベースにして破壊的変更を減らす

- [x] 編集前プレビュー + 承認フローを追加する
  - `diff` を表示し、適用前に `y/n` 確認できるようにする

- [x] セッション永続化 / 再開機能を追加する
  - 会話履歴・実行ログを保存し、`resume` できるようにする

- [x] トークン管理（履歴圧縮）を追加する
  - 長時間セッションで文脈維持しやすくする

## Medium Priority

- [x] ツール権限の細分化を追加する
  - 例: `read-only` / `workspace-write` / `network`

- [x] 長時間コマンドの PTY 対応を追加する
  - `tail -f` や対話系コマンドを扱えるようにする

- [ ] 実行結果の構造化表示を追加する
  - `stdout/stderr/exit_code/duration` を見やすく表示する

- [ ] 編集後のテスト・lint 自動実行モードを追加する
  - 例: `go test` / `golangci-lint` の自動実行

## Optional

- [ ] Web 検索ツールを統合する
  - 最新情報が必要な質問に対応しやすくする

- [ ] 失敗時の再試行戦略を追加する
  - `retry/backoff/model fallback` を実装する
