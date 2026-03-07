# codew (Ollama Codex-like CLI)

Go で書いた、Codex CLI 風の対話 CLI です。接続先は Ollama API (`/api/chat`) です。

## Build

```bash
go build -o codew .
```

## Commands

- `codew` / `codew chat`: interactive chat
- `codew doctor`: environment diagnostics

## Run

```bash
./codew --host http://host.docker.internal:11434 --model qwen2.5-coder:14b
```

## Flags

- `--host` (default: `http://127.0.0.1:11434`)
- `--model` (default: `llama3.2`)
- `--system`
- `--timeout`
- `--tools` (default: `true`)
- `--auto-approve` (default: `false`)
- `--workspace` (default: `.`)
- `--max-tool-steps` (default: `8`)
- `--session-file` (default: `.codew/session.json`)
- `--resume` (default: `false`)
- `--auto-save` (default: `true`)
- `--max-context-chars` (default: `24000`)
- `--tool-profile` (default: `workspace-write`)
- `--auto-validate` (default: `false`)
- `--post-edit-cmd` (default: `go test ./...`, 複数指定可)
- `--retries` (default: `2`)
- `--retry-backoff` (default: `2s`)
- `--fallback-model` (default: empty)
- `--auto-context` (default: `true`)
- `--auto-context-files` (default: `4`)
- `--auto-context-chars` (default: `8000`)
- `--dry-run` (default: `false`)
- `--auto-checkpoint` (default: `true`)
- `--tool-log` (default: `true`)
- `--tool-log-file` (default: `.codew/tool_logs.jsonl`)
- `--model-profile` (default: empty, `coding-fast` | `coding-safe` | `research`)

## Environment Variables

- `OLLAMA_HOST`
- `OLLAMA_MODEL`
- `OLLAMA_SYSTEM`

## In-chat Commands

- `/help`
- `/model <name>`
- `/system <text>`
- `/reset`
- `/save`
- `/load`
- `/checkpoint`
- `/undo`
- `/exit` or `/quit`

入力履歴ナビゲーション:
- `↑` / `↓`
- `Ctrl+P` / `Ctrl+N`

履歴は `.codew/history.txt` に保存されます。

`/checkpoint` は現在状態のスナップショットを `.codew/checkpoints` に作成し、`/undo` で最新チェックポイントへ戻せます。  
`--auto-checkpoint=true` の場合、編集系ツールの実行前に自動チェックポイントを作成します。

## Tool Calling

`--tools=true` の場合、モデルがツール呼び出し JSON を返すとローカルで実行し、結果をモデルへ返送します。

対応ツール:
- `shell_exec` (`command`, `workdir`, `timeout_sec`, `pty`)
- `list_files` (`path`, `pattern`, `max_results`)
- `read_file` (`path`)
- `write_file` (`path`, `content`)
- `replace_in_file` (`path`, `old`, `new`, `replace_all`)
- `apply_patch` (`patch`, `check_only`)
- `web_search` (`query`, `max_results`)

### Safe Edit (`apply_patch`)

`apply_patch` は unified diff を受け取り、先に `git apply --check` で検証してから適用します。  
`check_only=true` を指定すると検証のみ実行します。

例:

```diff
diff --git a/README.md b/README.md
index 1111111..2222222 100644
--- a/README.md
+++ b/README.md
@@ -1,1 +1,1 @@
-# old
+# new
```

ツール実行はデフォルトで都度承認です。全自動にする場合:

```bash
./codew --host http://host.docker.internal:11434 --model qwen2.5-coder:14b --auto-approve
```

編集系ツール（`write_file`, `replace_in_file`, `apply_patch`）は、承認プロンプト前に変更内容プレビューを表示します。

実行後は `[tool:<name>] ...` 形式で構造化サマリ（`ok`, `replaced`, `files`, `applied` など）を表示します。

## Session Persistence

- `--auto-save=true` の場合、各ターン後に `--session-file` へ履歴を保存します。
- `--resume` を指定すると起動時に `--session-file` を読み込みます。
- チャット中でも `/save` と `/load` で明示的に保存・復元できます。

## Context Compression

- `--max-context-chars` を超える履歴は、古いメッセージを要約して圧縮してからモデルに送信します。
- 直近メッセージを優先し、古い履歴は summary メッセージに畳み込みます。

## Tool Permission Profiles

- `read-only`: `list_files`, `read_file`
- `workspace-write`: 上記 + `write_file`, `replace_in_file`, `apply_patch`
- `full`: すべてのツール（`shell_exec`, `web_search` など）

## PTY Execution

- `shell_exec` は `pty=true` を指定すると擬似TTYでコマンド実行します。
- 対話系ツールやTTY前提のコマンドで利用できます。

## Post-edit Validation

- `--auto-validate` を有効化すると、編集系ツール成功後に検証コマンドを自動実行します。
- `--post-edit-cmd` を複数指定して test/lint を連続実行できます。

## Web Search Tool

- `web_search` は DuckDuckGo Instant Answer API を使って検索結果を返します。
- 外部ネットワークにアクセスできる環境で利用してください。

## Retry Strategy

- API 失敗時は `--retries` 回まで指数バックオフで再試行します。
- すべて失敗した場合、`--fallback-model` が指定されていればモデルを切り替えて再試行します。

## Auto Context From Project Files

- `--auto-context=true` の場合、ユーザー入力ごとに関連しそうなファイルをプロジェクト内から自動抽出します。
- 抽出したファイル内容は一時的な system 文脈として注入されます（履歴には永続化しません）。
- 件数と文字数は `--auto-context-files` / `--auto-context-chars` で制御できます。

## Dry Run

- `--dry-run` を有効化すると、編集系ツールは実適用せず実行計画のみ返します。
- `write_file` / `replace_in_file` / `apply_patch` は `dry_run=true` の結果を返します。

## Tool Execution Logs

- `--tool-log=true` の場合、各ツール呼び出しを JSONL で記録します。
- ログには時刻、入力、ツール名、引数、結果、承認可否が含まれます。

## Parallel Tool Execution

- 同一レスポンスで複数の読み取り系ツール（`read_file`, `list_files`, `web_search`）が要求された場合は並列実行します。
- 結果は元の呼び出し順で会話履歴へ反映します。

## Diff Preview

- 承認プロンプト時の unified diff は ANSI 色付きで表示します。
- `+` は緑、`-` は赤、`@@` は黄、`---/+++` ヘッダはシアン表示です。

## Model Profiles

- `coding-fast`: 速度重視のコーディング設定
- `coding-safe`: 読み取り中心で慎重な設定
- `research`: `web_search` 利用を想定した調査向け設定

`--model-profile` を指定すると、未明示の `--model` / `--system` / `--tool-profile` / `--retries` をプリセットで補完します。

## Doctor

`codew doctor` は以下をチェックします。
- Ollama 接続 (`/api/tags`)
- 指定モデルの存在
- ワークスペース書き込み可否
- git 状態（clean/dirty）

## Notes

- ファイル操作は `--workspace` 配下に制限しています。
- `shell_exec` の出力は長すぎる場合に切り詰めます。
