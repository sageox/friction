# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2026-03-10

### Added

- **Error detection**: Identify unknown commands, flags, typos, and missing arguments from CLI errors
- **Smart suggestions**: Catalog remaps, token fixes, and Levenshtein distance fallback for typo correction
- **Auto-execute**: Automatically run corrected commands for high-confidence fixes
- **Agent-aware formatting**: Detect AI coworker context and format suggestions accordingly
- **Secret redaction**: Built-in patterns for 25+ secret types (AWS, GitHub, Slack, Stripe, JWT, etc.)
- **Telemetry collection**: Background event buffering with rate limiting and periodic flush
- **Pluggable adapters**: CLI framework adapters for Cobra, Kong, and urfave/cli
- **Catalog system**: Thread-safe catalog with literal and regex command/token mappings
- **Catalog builder**: Aggregation pipeline for building friction catalogs from event data
- **Example CLI**: Sample `frictionx` CLI demonstrating library usage
- **Example server**: Sample `frictionx-server` with SQLite storage for learning friction patterns
