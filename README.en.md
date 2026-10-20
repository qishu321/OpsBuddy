# OpsBuddy

**A lightweight AI desktop assistant for everyday office work and software development.**

[简体中文](README.md) | English

[Downloads](https://github.com/qishu321/OpsBuddy/releases) · [Build status](https://github.com/qishu321/OpsBuddy/actions/workflows/release.yml) · [Implementation status](docs/implementation-status.md)

OpsBuddy brings conversations, documents, code projects and previews into one desktop workspace. Connect your own model service, assign multiple roles to a project, and inspect their actual work and source files. Built with **Go, React / TypeScript and Wails**, it uses the system WebView rather than bundling Electron or a browser. No background Python service is required.

Current version: **v0.18.0 — development preview**. Windows x64 and Linux x64 are supported. The native desktop pet is currently Windows-only. The application interface is primarily Chinese; this README does not imply a fully translated English UI.

## Features

- **Projects and conversations:** project-scoped context, grouped sessions, search, archive, deletion and follow-up edits.
- **Attachments:** files, folders, clipboard screenshots and dragged images; knowledge sources, Skills and declarative plugin packages. Image understanding depends on the selected model.
- **Office tasks:** editable DOCX, PPTX and XLSX generation, PDF reading and processing, file organization, Markdown creation and editing.
- **Development:** single-file HTML and multi-file Go / Node.js / Python projects, source trees, editing, start/stop controls, logs and in-app web previews. Executing generated code requires the corresponding local toolchain.
- **Multi-agent orchestration:** real model planning or a manual plan, separate role/model/context per member, dependency-based parallel execution, review, bounded retries and rework, and source integration. Includes a live plan canvas, pause/edit/resume and interrupted-run recovery.
- **Workflows and knowledge:** visual connections, conditions, local document chunking, keyword retrieval and source references.
- **Schedules and memory:** local scheduled tasks, separate run conversations, preferences and project experience. The desktop application must remain running for schedules to execute.
- **Model adapters:** OpenAI-compatible Chat Completions, Anthropic Messages and Gemini GenerateContent, using your own credentials.
- **Windows companion:** a native pixel pet with low-rate animation and task-event reactions, without extra model requests.

## Download and run

Choose your platform from [Releases](https://github.com/qishu321/OpsBuddy/releases). Verify downloads against `SHA256SUMS.txt`.

| Platform | Package and requirements |
| --- | --- |
| Windows 10/11 x64 | `OpsBuddy.exe` or `OpsBuddy-Windows-amd64.zip`; Microsoft Edge WebView2 Runtime |
| Linux x64 | `OpsBuddy-Linux-amd64.tar.gz`; built on Ubuntu 24.04, requires GTK3, WebKit2GTK 4.1, a graphical desktop and `xdg-open` |

On Windows, extract into a writable folder and launch `OpsBuddy.exe`. The desktop application itself does not require Go, Node.js or an external browser. Generated code projects need their respective runtimes. The preview binary is unsigned and may trigger operating-system reputation prompts.

On Ubuntu 24.04 or a compatible system:

```bash
sudo apt install libgtk-3-0t64 libwebkit2gtk-4.1-0 xdg-utils
tar -xzf OpsBuddy-Linux-amd64.tar.gz
./opsbuddy
```

The Linux build is dynamically linked, not a universal static executable. Other distributions need compatible libraries. ARM64, macOS and AppImage packages are not currently provided. After placing the executable on your PATH, the included `opsbuddy.desktop` file can be copied to your personal applications directory.

First launch: open **Settings → Add model**, choose a protocol, enter the Base URL, model ID and API Key, and test the connection. Return to the workspace and send a task. Enter sends; Shift+Enter inserts a newline. Inspect files and applications in the right sidebar. ZIP is used only for explicit project exports and application distribution.

## Multi-agent example

Ask for an office todo board with a frontend, a Go backend and an independent review. Open **More task options → Dispatch subtasks** in the composer. Let the main model plan, or specify worker, researcher, explorer and reviewer responsibilities and models. Roles are part of the collaboration rather than a separate expert library.

The conversation's orchestration canvas shows actual dependencies, tasks and provider receipts. Pause a running collaboration to edit its plan before resuming. Limits: six members, three concurrent tasks globally, one delegation level, one automatic retry per node and two review/rework rounds. Requests consume the selected provider's service quota.

The repository includes [todo example source](demos/multi-agent-todo/project) and `demos/multi-agent-todo/todo-demo.opsbuddy-project.json`. Import the package from **Projects → Import project** to inspect completed demo history. Importing neither calls a model again nor executes code. Click **Run locally** in the sidebar to launch the final project. See the [collaboration guide](docs/multi-agent-guide.md), currently in Chinese.

## Build from source

Requires **Go 1.26+, Node.js 22+ and npm**, with network access for initial dependencies.

```bash
git clone https://github.com/qishu321/OpsBuddy.git
cd OpsBuddy
npm ci
```

Windows PowerShell:

```powershell
./scripts/build-desktop.ps1
# Output: release/OpsBuddy/OpsBuddy.exe
# Optional archive: ./scripts/build-desktop.ps1 -Package
```

Linux / Ubuntu 24.04:

```bash
sudo apt install build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev
bash scripts/build-desktop.sh
# Outputs: release/linux/opsbuddy and release/OpsBuddy-Linux-amd64.tar.gz
```

Build Linux packages on Linux. The build uses CGO, GTK3 and the `webkit2_41` tag, following the [Wails v2 installation documentation](https://v2.wails.io/docs/gettingstarted/installation/). `npm run build` builds embedded frontend assets without starting a web server. `npm run dev` is the Windows desktop development shortcut. There is no separate user-facing web edition.

## Data and credentials

- Windows data defaults to `.opsbuddy/` beside the executable.
- Linux data defaults to `${XDG_CONFIG_HOME:-~/.config}/opsbuddy/`.
- Both accept `--data /absolute/path`. Close the application before backing up the entire directory. Do not run multiple instances against the same database.
- Windows encrypts model credentials with the current user's DPAPI.
- Linux uses AES-256-GCM and a random per-user key at `${XDG_CONFIG_HOME:-~/.config}/opsbuddy-credentials/key`, with a 0700 directory and 0600 key file. This is filesystem-based local encryption, not a hardware-backed vault: access to both the database and key allows decryption. Securely back up the key when migrating Linux data.
- Re-enter API Keys after moving between operating systems. Windows ciphertext cannot be decrypted on Linux. Task snapshots do not contain API Keys. Submitted content and selected attachments are sent to the provider you choose.
- Personal conversations, databases, credentials, caches and runtime output are excluded from source and release packages. The included project package contains demo history and source only.

## Testing and releases

Windows: `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test.ps1`.

Linux: `go test -tags webkit2_41 ./...`, `go vet -tags webkit2_41 ./...`, and `npm run build`. After building, `bash scripts/test-linux-desktop.sh` exercises the native desktop under a virtual display using an isolated data directory and no real model calls; install `xvfb dbus-x11` first.

GitHub Actions tests and builds both platforms. A matching `v*` tag publishes a prerelease with platform packages and SHA-256 checksums only after all jobs pass. See [release notes](docs/releases/v0.18.0.md).

## Current limits

Office support provides basic content generation and content previews, not full Office layout rendering. Knowledge retrieval is keyword-based; vector retrieval and OCR are not implemented. Model adapters are non-streaming. MCP initialization and tool discovery are available, but tool execution is not connected. There is no general tool loop, automatic dependency installation, unlimited delegation, operating-system scheduler that runs after exit, or OS-level code sandbox. Linux does not yet include the native pet.

## Layout

```text
cmd/opsbuddy-desktop/  Windows/Linux entry points and embedded frontend
internal/app/         Go tasks, models, storage, office tools and orchestration
internal/desktop/     Native Wails operations and in-process request bridge
internal/pet/         Windows native companion and shared artwork
src/                  React / TypeScript UI
scripts/              Build, launch and verification scripts
.github/workflows/    Cross-platform checks and tagged releases
docs/                 Requirements, guides and verification records
demos/                Example source and an importable demo project
```

Report issues with your platform, version and redacted reproduction steps in [Issues](https://github.com/qishu321/OpsBuddy/issues). Never upload API Keys, personal databases or sensitive attachments.
