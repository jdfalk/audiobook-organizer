<!-- file: .github/instructions/protobuf.instructions.md -->
<!-- version: 4.0.0 -->
<!-- guid: 7d6c5b4a-3c2d-1e0f-9a8b-7c6d5e4f3a2b -->
<!-- last-edited: 2026-01-31 -->

<!-- prettier-ignore-start -->
<!-- markdownlint-disable -->
---
applyTo: "**/*.proto"
description: |
  Protobuf rules. This repo does not currently use protobuf. If added, use Edition 2023 and 1-1-1 pattern.
---
<!-- markdownlint-enable -->
<!-- prettier-ignore-end -->

# Protobuf Coding Instructions

This repo does not currently use Protocol Buffers. If proto files are added:

- Use **Edition 2023** (`edition = "2023";`). Not proto2 or proto3.
- Follow the **1-1-1 pattern**: one top-level entity (message, enum, or service) per file.
- Use module prefixes on message names to avoid conflicts: `AuthUserInfo`, `MetricsStatus`.
- Field names: `snake_case`. Enum values: `UPPER_SNAKE_CASE`. First enum value must be `_UNSPECIFIED = 0`.
- Use `features.field_presence = EXPLICIT` instead of `optional` keyword.
- Reference: [Google Protobuf Style Guide](https://protobuf.dev/programming-guides/style/)

## Required File Header

```protobuf
// file: path/to/file.proto
// version: 1.0.0
// guid: 123e4567-e89b-12d3-a456-426614174000
```
