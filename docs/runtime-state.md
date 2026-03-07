# Runtime State

This document describes the machine-readable runtime surfaces that Codex and other agents can use instead of scraping human-facing text.

## Public CLI Surfaces

- `./simple-agent tools list --json`
- `./simple-agent models list --json`
- `./simple-agent doctor --json`
- `/cancel` in the TUI for deterministic in-band interruption

Runtime guarantees:

- file tools operate relative to the current working directory and reject paths outside that workspace
- `models.json` can define named OpenAI-compatible local/remote endpoints that appear in `models list`

## Private Local State

- `~/.simple-agent/sessions/`
- `~/.simple-agent/harness/<repo-slug>/latest.json`
- `~/.simple-agent/harness/<repo-slug>/runs/**/*.jsonl`
- `~/.simple-agent/harness/<repo-slug>/codex-analysis/`

## Session Run State

Conversation history now stores explicit run state in the session model:

- `running`
- `completed`
- `failed`
- `cancelled`
- `timed_out`
- `interrupted`

## Purpose

The goal is application legibility:

- agents should be able to inspect current tool inventory
- agents should be able to inspect known model inventory
- agents should be able to inspect local harness state
- agents should be able to reason about whether a previous run finished, failed, or was interrupted
