# ocli (Ollama Codex-like CLI)

Go で書いた、Codex CLI 風の最小チャット CLI です。接続先は Ollama API (`/api/chat`) です。

## 1. Build

```bash
go build -o ocli .
```

## 2. Run

```bash
./ocli chat --host http://127.0.0.1:11434 --model llama3.2
```

`chat` を省略しても起動できます。

```bash
./ocli --model qwen2.5-coder
```

## 3. Environment variables

- `OLLAMA_HOST` (default: `http://127.0.0.1:11434`)
- `OLLAMA_MODEL` (default: `llama3.2`)
- `OLLAMA_SYSTEM` (default: `You are a coding assistant.`)

## 4. In-chat commands

- `/help`
- `/model <name>`
- `/system <text>` (変更時は履歴をリセット)
- `/reset`
- `/exit` or `/quit`

## 5. Notes

- まず Ollama を起動し、モデルを pull してください。
- 例: `ollama run llama3.2`
