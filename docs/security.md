# Security Guide

The architecture mandates a three-layer defense: sandboxing, validation, and approvals.

- Sandbox: enforce path allow-lists and resolve symlinks before filesystem access.
- Validator: inspect commands/tools for injection vectors and enforce JSON schema validation.
- Approval queue: route high-risk operations through HITL flows.

Document concrete policies here as the implementation lands.
