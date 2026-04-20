---
description: Check Kestrel CLI health — installation, auth, API connectivity, and agent integrations.
invocable: true
---

# /kestrel:kestrel-doctor

Run the Kestrel CLI health check and report results.

```bash
kestrel doctor --json
```

Interpret the output:
- **pass**: Working correctly
- **warn**: Non-critical issue
- **fail**: Broken — needs attention
- **skip**: Check not run (e.g., no token configured)

For any failures, follow the `hint` field in the check output. Common fixes:
- CLI not found → `curl -fsSL https://kestrelportfolio.com/install-cli | bash`
- Not authenticated → `kestrel login`
- Token expired → `kestrel login`
- API disabled → Contact organization admin
- Plugin not installed → `kestrel setup claude`

Report results concisely: list failures and warnings with their hints. If everything passes, say so.
