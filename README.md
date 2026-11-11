## CC-Router

CC-Router is a lightweight, local API gateway designed to keep Claude Code running smoothly by managing and rotating Anthropic-compatible API keys. It is built for developers who need a zero-dependency, strictly local solution for seamless key management.


## Why I built this instead of using LiteLLM

While LiteLLM is a massive, production-grade gateway, it is overkill for a single developer using Claude Code (My opiniion). I wanted a zero-dependency, strictly local, highly opinionated tool that solves *one* problem perfectly: keeping Claude Code running by seamlessly swapping Anthropic-compatible keys.


## Install

### Linux / macOS

Download the latest release binary directly via curl:

```sh
curl -fsSL https://github.com/haidaralif-24/cc-router/releases/latest/download/claude-router_linux_amd64.tar.gz | tar -xz
sudo mv claude-router /usr/local/bin/claude-router
sudo chmod +x /usr/local/bin/claude-router
claude-router
```

Or use the install script:

```sh
curl -fsSL https://raw.githubusercontent.com/haidaralif-24/cc-router/main/install.sh | sh
```

### Windows

Download the latest release zip via curl and extract:

```powershell
curl -L -o claude-router.zip https://github.com/haidaralif-24/cc-router/releases/latest/download/claude-router_windows_amd64.zip
tar -xf claude-router.zip
.\claude-router.exe
```

Or use the installer:

```powershell
irm https://raw.githubusercontent.com/haidaralif-24/cc-router/main/install.ps1 | iex
```

> Releases are built automatically by GitHub Actions whenever a `v*` tag is pushed.

After installing, provide a `config.jsonc` with your provider keys and router token, then start the router. The router reads the token from `router_token` in `config.jsonc` (or the `ROUTER_TOKEN` environment variable).
