## 🧠 Opscure Log Recommendation Go Agent

A high-performance Go service that runs as a sidecar agent for the Opscure VS Code extension.
It continuously ingests logs, builds intelligent correlation bundles, sends them to an AI analysis service, and ***can apply automated fixes*** on the user’s codebase with live execution streaming and rollback support.

---

## 📌 What this agent does

- Acts as a local runtime companion (sidecar) to the VS Code extension

- Ingests logs from applications in real time

- Builds structured log bundles (windowed, severity-aware, service-aware)

- Calls AI analyze service to generate root cause + recommendations

- Applies AI-generated fixes safely using git workflows

- Streams fix execution logs back to the extension UI

- Supports automatic rollback if a fix fails

---

## ✨ Core Features

- 🔄 Continuous log ingestion & batching

- 📦 Intelligent bundle creation (patterns, services, metrics)

- 🤖 AI preprocessing + analyze pipeline

- 🛠️ Automated fix execution (git, sed, docker, kubectl, etc.)

- 📡 Live SSE streams for:

  - Log bundles

  - Fix execution output

- 🔁 Auto-rollback support

- 🔌 Auto-port binding + extension discovery

- ⚙️ Config-driven log sources

---

## 🧩 How it runs (Sidecar Mode)

When started, the agent:

1. Binds to 127.0.0.1:8080 (or auto-selects a free port if busy)

2. Writes the selected port into a local file called:

```
agent.port
```

3. The VS Code extension reads this file to discover the running agent.

This allows the agent to run **automatically alongside the extension without manual port configuration**.

---

## 🧰 Prerequisites

Install the following before running the agent.

#### 🟦 1. Go (Required)

Version: Go 1.20+

Download:
https://go.dev/dl/

Verify:

```go
go version
```

---

#### 🧩 2. Git (Required – for auto-fix system)

Download:
https://git-scm.com/downloads

Verify:

```css
git --version
```


Git is mandatory because the agent:

Detects default branch

- Runs checkout / pull / push

- Applies fixes

- Performs rollback

---

#### 🟢 3. Node.js (Optional – Extension side)

Required only if running the VS Code extension.

Version: Node.js 18+

Verify:

```nginx
node -v
npm -v
```

---

## Project Structure

```
.
├── main.go                  # HTTP server, APIs, sidecar logic
├── stream_manager.go        # Streaming, buffering, bundling
├── log_preprocessor.go      # Pattern mining & correlation logic
├── go.mod
├── go.sum
├── config.yaml              # Local config (DO NOT COMMIT)
├── config.example.yaml      # Sample config
└── .gitignore
```

---

## ⚙️ Configuration

The agent supports optional static log sources via config.yaml.

Example:

```yaml
server:
  default_lines: 100
  max_lines: 1000

apps:
  banking:
    logs:
      app:
        type: file
        path: E:/replit_prj/banking/logs.log
        service: FileService

      api-errors:
        type: api
        url: https://internal/api/logs
        service: PaymentAPI
```

Supported types

- file → local log files

- api → HTTP log endpoint

---

## 🚀 Running the agent
**▶️ Run locally (development mode)**

From the project root:

```arduino
go run .
```


or with config:

```arduino
go run . -config="E:\replit_prj\log-agent\config.yaml"
```

Linux / macOS:

```arduino
go run . -config="/home/user/log-agent/config.yaml"
```

On startup you will see:

```csharp
[OPSCURE] Agent running on 127.0.0.1:PORT
```

The selected port is written to:

```
agent.port
```

---

## 🔌 HTTP APIs
**Log ingestion**

```bash
POST /stream/ingest
```

Used by applications / extension to push logs.

---

## Live bundle stream (SSE)

```bash
GET /stream/live
```

Extension subscribes here to receive correlation bundles.

---

## Preprocess + AI analyze

```bash
POST /logs/preprocess
```

- Builds correlation bundle

- Injects git config (if present)

- Calls AI analyze service

- Returns combined response

---

## Apply AI fix

```bash
POST /fix/apply
```

- Validates AI recommendation

- Executes commands

- Streams output

- Supports dry-run mode

---

## Fix execution stream

```bash
GET /fix/stream
```

Live execution logs (SSE).

---

## Rollback

```bash
POST /fix/rollback
```

Replays last stored rollback commands.

---

## 🔄 Internal Flow

```bash
Application / Extension
        ↓
/stream/ingest
        ↓
Stream Manager
        ↓
Bundle Flush
        ↓
/logs/preprocess
        ↓
AI Analyze Service
        ↓
Recommendations
        ↓
/fix/apply → /fix/stream
        ↓
Git workflow + live output
```

## ✅ Best Practices

- Do not commit config.yaml

- Always use absolute paths

- Ensure GitHub authentication is working before applying fixes

- Keep the agent running as a background sidecar for the extension

- Review AI fixes before disabling dry-run

---

## 🔮 Roadmap Ideas

- Dockerized agent

- Secure auth between extension & agent

- Pluggable log adapters

- Policy-based fix restrictions

- Visual metrics dashboard

---

## 📜 License

MIT License

---
