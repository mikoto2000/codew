# codew (Ollama Codex-like CLI)

Go で書いた、Codex CLI 風の対話 CLI です。接続先は Ollama API (`/api/chat`) です。

## Build

```bash
go build -o codew .
```

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
- `/exit` or `/quit`

## Tool Calling

`--tools=true` の場合、モデルがツール呼び出し JSON を返すとローカルで実行し、結果をモデルへ返送します。

対応ツール:
- `shell_exec` (`command`, `workdir`, `timeout_sec`)
- `list_files` (`path`, `pattern`, `max_results`)
- `read_file` (`path`)
- `write_file` (`path`, `content`)
- `replace_in_file` (`path`, `old`, `new`, `replace_all`)
- `apply_patch` (`patch`, `check_only`)

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

## Session Persistence

- `--auto-save=true` の場合、各ターン後に `--session-file` へ履歴を保存します。
- `--resume` を指定すると起動時に `--session-file` を読み込みます。
- チャット中でも `/save` と `/load` で明示的に保存・復元できます。

## Context Compression

- `--max-context-chars` を超える履歴は、古いメッセージを要約して圧縮してからモデルに送信します。
- 直近メッセージを優先し、古い履歴は summary メッセージに畳み込みます。

## Notes

- ファイル操作は `--workspace` 配下に制限しています。
- `shell_exec` の出力は長すぎる場合に切り詰めます。
