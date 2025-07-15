<!-- file: .github/instructions/protobuf.instructions.md -->
<!-- version: 1.0.0 -->
<!-- guid: 7d6c5b4a-3c2d-1e0f-9a8b-7c6d5e4f3a2b -->

applyTo: "\*\*/\*.proto"ile: .github/instructions/protobuf.instructions.md -->
<!-- version: 1.0.0 -->

## <!-- guid: 7d6c5b4a-3c2d-1e0f-9a8b-7c6d5e4f3a2b -->

applyTo: "\*_/_.proto"
description: |
Protocol Buffers (protobuf) style and documentation rules for Copilot/AI agents and VS Code Copilot customization. These rules extend the general instructions in `general-coding.instructions.md` and merge all unique content from the Google Protobuf Style Guide.

---

# Protobuf Coding Instructions

- Follow the [general coding instructions](general-coding.instructions.md).
- Follow the [Google Protobuf Style Guide](https://protobuf.dev/programming-guides/style/) for additional best practices.
- All protobuf files must begin with the required file header (see general instructions for details and Protobuf example).

## Edition 2023 Features

- All proto files MUST use Edition 2023: `edition = "2023";`
- Enhanced features, better defaults, future-proofing, hybrid APIs, improved validation

## File Structure

- Use `pkg/` structure for organization
- Use standard imports for common types
- Use package documentation and go_package option

## Naming Conventions

- Use `gcommon.v1.service_name` format for packages
- Use PascalCase for messages, SCREAMING_SNAKE_CASE for enum values
- Use snake_case for fields
- Use descriptive names, avoid abbreviations

## Message and Service Design

- Use standard request/response patterns
- Use common types from shared proto
- Use REST and gRPC annotations
- Use validation rules for fields
- Document all messages, fields, and services with clear comments

## Required File Header

All protobuf files must begin with a standard header as described in the [general coding instructions](general-coding.instructions.md). Example for Protobuf:

```protobuf
// file: path/to/file.proto
// version: 1.0.0
// guid: 123e4567-e89b-12d3-a456-426614174000
```
