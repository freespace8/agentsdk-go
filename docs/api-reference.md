# API Reference

This file intentionally stays lightweight until concrete interfaces stabilize.

- `pkg/agent`: Core runtime and lifecycle hooks.
- `pkg/tool`: Tool registry, schema handling, builtin tools, and MCP adapters.
- `pkg/model`: Backend adapters for Anthropic, OpenAI, and generic factories.
- `pkg/session`: Session persistence, checkpointing, WAL, summarization.
- `pkg/event`: Progress/control/monitor event channels.
- `pkg/workflow`: State graph, middleware, routing, looping, branching.
- `pkg/evals`: Local evaluation helpers.
- `pkg/security`: Sandboxing, validators, approval queues, symlink resolution.

Expand each section with request/response structs and error semantics once implemented.
