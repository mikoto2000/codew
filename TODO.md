# TODO

`codew` をさらに実用的にするための追加候補。

- [x] `--dry-run`
  - 編集ツールを実適用せず、差分と実行計画のみ表示する

- [x] `undo` / `checkpoint`
  - 自動で git stash/commit 的な巻き戻しポイントを作る

- [x] ツール実行ログの JSONL 出力
  - 後から監査・再現しやすくする

- [x] 並列ツール実行
  - `read_file` 複数件の取得を同時実行して速度改善する

- [x] ファイル差分ビュー強化
  - 承認時に unified diff を色付きで表示する

- [x] モデルごとのプロファイル
  - 既定 `system prompt` / `tool-profile` / `retries` をプリセット化する

- [x] `codew doctor` コマンド
  - Ollama接続、モデル存在、権限、git状態を一発診断する

- [x] `non-interactive` モード
  - `codew run "..."` でCIやスクリプトから使えるようにする
