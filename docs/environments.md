# Environments

The CLI can point at any Kestrel Portfolio instance. The environment is determined by the API base URL stored in your config.

## Switching Environments

```bash
# Local development
kestrel login --token <TOKEN> --base-url http://localhost:3000/api/v1

# Staging
kestrel login --token <TOKEN> --base-url https://staging.kestrelportfolio.com/api/v1

# Production (default — no --base-url needed)
kestrel login --token <TOKEN>
```

Each `kestrel login` overwrites the saved config. To see what you're currently pointed at:

```bash
cat ~/.config/kestrel/config.json
```

## One-Off Override

Use `--base-url` on any command without changing your saved config:

```bash
kestrel properties list --base-url http://localhost:3000/api/v1
```

## Environment Variables

These override the config file (useful for CI or scripting):

```bash
export KESTREL_TOKEN="your-token"
export KESTREL_BASE_URL="http://localhost:3000/api/v1"
```

## Config Precedence

Highest wins:

1. `--base-url` flag
2. `KESTREL_BASE_URL` env var
3. Local config (`.kestrel/config.json` in current directory)
4. Global config (`~/.config/kestrel/config.json`)
5. Default: `https://kestrelportfolio.com/api/v1`

## Tokens Are Per-Environment

Each Kestrel Portfolio instance has its own users and tokens. A token from your local dev server won't work against production. When you switch environments, you need a valid token for that instance.

## AI Agents

When an AI agent (Claude Code, etc.) uses the CLI, it picks up whatever environment is configured in `~/.config/kestrel/config.json`. If you're developing locally and want the agent to hit your dev server, just `kestrel login` with `--base-url http://localhost:3000/api/v1` first.
