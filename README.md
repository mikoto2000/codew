# codew (Ollama Codex-like CLI)

Go で書いた、Codex CLI 風の対話 CLI です。接続先は Ollama API (`/api/chat`) です。

## Build

```bash
go build -o codew .
```

## Commands

- `codew` / `codew chat`: interactive chat
- `codew run "<prompt>"`: non-interactive single-shot
- `codew doctor`: environment diagnostics
- `codew review`: severity-ordered review of current changes

## Project Type Detection

- 起動時にプロジェクトルートのファイル一覧から種別（例: `go`, `nodejs`, `python` など）を判定します。
- 判定結果は起動時表示とシステムプロンプト補助ヒントに使われます。

## Run

```bash
./codew --host http://host.docker.internal:11434 --model qwen2.5-coder:14b
```

## Flags

| Flag | 概要 | Default |
| --- | --- | --- |
| `--host` | Ollama API の接続先 URL を指定します。 | `http://127.0.0.1:11434` |
| `--model` | 利用する既定モデル名を指定します。 | `llama3.2` |
| `--system` | システムプロンプト文を上書きします。 | (none) |
| `--timeout` | API リクエストのタイムアウト時間です。 | `120s` |
| `--tools` | ツール呼び出し機能の有効/無効を切り替えます。 | `true` |
| `--auto-approve` | ツール実行時の承認確認を省略します。 | `false` |
| `--workspace` | ツール操作の基準となるワークスペースルートです。 | `.` |
| `--max-tool-steps` | 1ターン内のツール呼び出しラウンド上限です。 | `8` |
| `--session-file` | 会話履歴の保存/復元先ファイルです。 | `.codew/session.json` |
| `--resume` | 起動時に `--session-file` からセッションを復元します。 | `false` |
| `--auto-save` | 各ターン後にセッションを自動保存します。 | `true` |
| `--max-context-chars` | モデル送信前の会話文脈の最大文字数目安です。 | `24000` |
| `--tool-profile` | 利用可能ツール範囲のプリセット権限です。 | `workspace-write` |
| `--auto-validate` | 編集系ツール成功後に検証コマンドを自動実行します。 | `false` |
| `--post-edit-cmd` | 編集後に実行する検証コマンド（複数指定可）です。 | `go test ./...` |
| `--retries` | API 失敗時の再試行回数です。 | `2` |
| `--retry-backoff` | 再試行間隔のベースバックオフ時間です。 | `2s` |
| `--fallback-model` | 再試行失敗時に切り替える予備モデルです。 | empty |
| `--auto-context` | 入力ごとに関連ファイルを自動で文脈注入します。 | `true` |
| `--auto-context-files` | 自動注入するファイル数の上限です。 | `4` |
| `--auto-context-chars` | 自動注入する合計文字数の上限です。 | `8000` |
| `--dry-run` | 編集ツールを実適用せず計画のみ返します。 | `false` |
| `--auto-checkpoint` | 編集前に自動チェックポイントを作成します。 | `true` |
| `--tool-log` | ツール実行ログ（JSONL）出力を有効化します。 | `true` |
| `--tool-log-file` | ツール実行ログの出力先ファイルです。 | `.codew/tool_logs.jsonl` |
| `--model-profile` | モデル/システム/権限などの推奨設定を一括適用します。 | empty (`coding-fast` \| `coding-safe` \| `research`) |
| `--mcp` | MCP クライアント連携ツールを有効化します。 | `false` |
| `--mcp-config` | MCP サーバー設定 JSON のパスです。 | `.codew/mcp.json` |
| `--sandbox-mode` | ツール実行時のサンドボックス権限レベルです。 | `workspace-write` (`read-only` \| `workspace-write` \| `full`) |
| `--network-allow` | セッション全体でネットワーク昇格を許可します。 | `false` |
| `--network-allow-tool` | 指定ツール名に限定してネットワーク昇格を許可します。 | (none) |
| `--trace-log` | ターン単位トレースログ（JSONL）を出力します。 | `true` |
| `--trace-log-file` | ターン単位トレースログの出力先ファイルです。 | `.codew/turn_traces.jsonl` |

## Environment Variables

| Variable | 概要 | Default |
| --- | --- | --- |
| `OLLAMA_HOST` | Ollama API 接続先を `--host` の初期値として設定します。 | `http://127.0.0.1:11434` |
| `OLLAMA_MODEL` | 既定モデル名を `--model` の初期値として設定します。 | `llama3.2` |
| `OLLAMA_SYSTEM` | システムプロンプトを `--system` の初期値として設定します。 | `You are a coding assistant.` |

## In-chat Commands

| Command | 概要 | Default |
| --- | --- | --- |
| `/help` | 利用可能なコマンド一覧を表示します。 | `-` |
| `/model <name>` | 現在セッションで使うモデルを切り替えます。 | `-` |
| `/models` | 利用可能なモデル一覧を表示します（現在モデルは `*`）。 | `-` |
| `/system <text>` | 現在セッションのシステムプロンプトを更新します。 | `-` |
| `/reset` | 会話コンテキストをリセットします。 | `-` |
| `/save` | 現在のセッションを `--session-file` に保存します。 | `-` |
| `/load` | `--session-file` からセッションを読み込みます。 | `-` |
| `/checkpoint` | 現在状態のチェックポイントを作成します。 | `-` |
| `/undo` | 最新のチェックポイント状態へ戻します。 | `-` |
| `/plan <step>` | 計画にステップを追加します。 | `-` |
| `/plan-list` | 計画ステップ一覧を表示します。 | `-` |
| `/plan-doing <index>` | 指定ステップを進行中に更新します。 | `-` |
| `/plan-done <index>` | 指定ステップを完了に更新します。 | `-` |
| `/exit` or `/quit` | セッションを終了します。 | `-` |

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
- 各結果には `source_url` と `retrieved_at` を付与し、出典追跡できる形式で返します。

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
- `apply_patch` は失敗時に `--3way` とファイル単位分割適用へフォールバックします。

## Tool Execution Logs

- `--tool-log=true` の場合、各ツール呼び出しを JSONL で記録します。
- ログには時刻、入力、ツール名、引数、結果、承認可否が含まれます。

## Turn Trace Logs

- `--trace-log=true` の場合、ターン単位の実行トレースを JSONL で記録します。
- 記録項目: mode(chat/run), input, duration_ms, tool_calls, error。

## Parallel Tool Execution

- 同一レスポンスで複数の読み取り系ツール（`read_file`, `list_files`, `web_search`）が要求された場合は並列実行します。
- 結果は元の呼び出し順で会話履歴へ反映します。
- 各 tool call 引数に `_depends_on: ["tool_name", ...]` を指定すると、依存解決後に並列実行する簡易DAGオーケストレーションが有効になります。

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

## Review Mode

- `codew review` は現在の `git diff` を対象に変更ファイルを `high/medium/low` で並べて表示します。
- あわせて `*_test.go` の変更不足も簡易検出します。

## Non-interactive Mode

```bash
codew run "このリポジトリで TODO の進め方を提案して"
```

- CI やスクリプト向けに 1 プロンプト実行して終了します。
- 通常のツール呼び出しループ（`max-tool-steps` まで）も利用します。

## MCP Client (stdio)

- `--mcp` を有効にすると `.codew/mcp.json` の MCP サーバーを起動し、`tools/list` の結果を `mcp.<server>.<tool>` として公開します。
- MCP ツール実行は `tools/call` へ中継されます。
- 安全のため MCP ツールは `--tool-profile full` でのみ実行可能です。

## Sandbox Permissions

`--sandbox-mode` で実行時権限を制御します。

- `read-only`: `read` のみ
- `workspace-write`: `read` + `write`
- `full`: `read` + `write` + `network` + `exec`

各ツールには必要権限が割り当てられ、実行前に一貫した判定を行います。

## Network Escalation

- `sandbox-mode` で `network` が許可されていない場合、ネットワーク系ツールは拒否されます。
- 対話モードでは都度承認（allow once / allow always / deny）できます。
- 非対話や事前許可には `--network-allow` または `--network-allow-tool` を使用します。

設定例 (`.codew/mcp.json`):

```json
{
  "servers": [
    {
      "name": "example",
      "command": "node",
      "args": ["./mcp-server.js"],
      "env": {},
      "cwd": "."
    }
  ]
}
```

## Notes

- ファイル操作は `--workspace` 配下に制限しています。
- `shell_exec` の出力は長すぎる場合に切り詰めます。
