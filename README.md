# ocli (Ollama Codex-like CLI)

Go で書いた、Codex CLI 風の対話 CLI です。接続先は Ollama API (`/api/chat`) です。

## Build

```bash
go build -o ocli .
```

## Run

```bash
./ocli --host http://host.docker.internal:11434 --model qwen2.5-coder:14b
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

## Environment Variables

- `OLLAMA_HOST`
- `OLLAMA_MODEL`
- `OLLAMA_SYSTEM`

## In-chat Commands

- `/help`
- `/model <name>`
- `/system <text>`
- `/reset`
- `/exit` or `/quit`

## Tool Calling

`--tools=true` の場合、モデルがツール呼び出し JSON を返すとローカルで実行し、結果をモデルへ返送します。

対応ツール:
- `shell_exec` (`command`, `workdir`, `timeout_sec`)
- `list_files` (`path`, `pattern`, `max_results`)
- `read_file` (`path`)
- `write_file` (`path`, `content`)
- `replace_in_file` (`path`, `old`, `new`, `replace_all`)

ツール実行はデフォルトで都度承認です。全自動にする場合:

```bash
./ocli --host http://host.docker.internal:11434 --model qwen2.5-coder:14b --auto-approve
```

## Notes

- ファイル操作は `--workspace` 配下に制限しています。
- `shell_exec` の出力は長すぎる場合に切り詰めます。
