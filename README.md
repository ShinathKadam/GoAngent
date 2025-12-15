# 🧠 Log Recommendation Go Agent
## 📌 Description

This project acts as an intermediary service that fetches logs based on the file name specified in the configuration file. It sends the collected logs to a recommendation engine for analysis and attempts to automatically fix issues when the log file is located on a remote server.

---

## ✨ Features

### 🔄 Supports multiple applications

### 📂 Fetches logs from:

Local or remote files

HTTP API endpoints

### 🤖 Sends logs for AI-based analysis and recommendations

### 🛠️ Attempts auto-fix for remote log sources (if supported)

### ⚙️ Config-driven and easily extensible

---

## 🧰 Prerequisites

Before running this agent, install the following software.

### 🟦 1. Go (Golang)

**Version:** Go 1.20 or later

**🔗 Download:** https://go.dev/dl/

Verify installation:

```bash
go version
```

---

## 🧩 2. Git

***🔗 Download:** https://git-scm.com/downloads

Verify installation:

```bash
git --version
```

---

## 🟢 3. Node.js (Optional)

Required only if the agent is integrated with a VS Code extension or AI service.

**Version:** Node.js 18 or later

**🔗 Download:** https://nodejs.org/

Verify installation:

```bash
node -v
npm -v
```

---

## 🗂️ Project Structure
```lua
.
├── main.go
├── go.mod
├── go.sum
├── config.yaml            # Local config (do not commit if it contains secrets)
├── config.example.yaml    # Sample config for reference
└── .gitignore
```

---

## ⚙️ Configuration
## 📄 Configuration File (config.yaml)

The agent reads log sources from config.yaml.
You can define multiple applications, and each application can have multiple log sources.

### 🧪 Sample config.yaml
```yaml
apps:
  banking:
    logs:
      app:
        type: file
        path: E:/replit_prj/banking/logs.log

      app-log:
        type: file
        path: E:/replit_prj/banking/app.log

      api-errors:
        type: api
        url: https://logs.internal/payments/errors
```

---

## 🔍 Configuration Explanation

**apps →** Root section containing all applications

**banking →** Application name (can be any identifier)

**logs →** All log sources for the application

**type**

📄 file → Reads logs from a file

🌐 api → Fetches logs from an HTTP endpoint

**path →** Absolute file path (for file type)

**url →** API endpoint (for api type)

---

## 🚀 Setup Instructions
### 📥 Clone the Repository
```bash
git clone <your-repository-url>
cd <your-project-folder>
```

---

## 📝 Create Configuration File

Create a local config.yaml file and update values as required.

⚠️ Do not commit config.yaml if it contains secrets or credentials.
✅ Use config.example.yaml for version control.

---

## ▶️ Run the Go Agent
### 💻 Command
```bash
go run main.go -config="your_file_path\config.yaml"
```

### 🪟 Example (Windows)
```bash
go run main.go -config="E:\replit_prj\log-agent\config.yaml"
```

### 🐧 Example (Linux / macOS)
```bash
go run main.go -config="/home/user/log-agent/config.yaml"
```

---

## 🔄 How It Works

📥 Loads application and log source details from config.yaml

📊 Fetches logs from file paths or API endpoints

🤖 Sends logs to the recommendation engine for analysis

📬 Receives recommendations or fixes

🛠️ Attempts to auto-fix issues when logs are from remote sources

---

## ✅ Best Practices

🔐 Keep secrets out of Git

📍 Use absolute paths for log files

🌐 Validate API endpoints before running the agent

📄 Commit only config.example.yaml

---

## 🔮 Future Enhancements

🔑 Authentication support for API-based logs

⏱️ Configurable polling intervals

🐳 Docker support

📊 Web dashboard for insights

---

## 📜 License

This project is licensed under the MIT License.
