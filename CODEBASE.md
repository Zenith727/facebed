# Facebed Go Codebase Directory

This file documents the structure, components, and default configuration values for the Go implementation of the Facebed Facebook embed provider.

## Workspace Overview

The Facebed project has been fully migrated to Go 1.25. It uses the standard library `net/http` router, `goquery` for HTML parsing, and `imroc/req/v3` for Chrome TLS fingerprint impersonation.

| File | Lines | Purpose |
| --- | --- | --- |
| [main.go](file:///c:/Users/nigger%20pc/Desktop/New%20folder%20%282%29/facebed/main.go) | 297 | Entry point, server setup, routing, `/dump` diagnostic, `/text` routing, bot user-agent redirect logic. |
| [extractors.go](file:///c:/Users/nigger%20pc/Desktop/New%20folder%20%282%29/facebed/extractors.go) | 1412 | Facebook HTML scraping extractors (`JsonParser`, `SinglePhoto`, `Photocom`, `Reels`, `Watch`). |
| [parser.go](file:///c:/Users/nigger%20pc/Desktop/New%20folder%20%282%29/facebed/parser.go) | 432 | `ParsedPost`, `Story` structures, Jq JSON selectors emulation, share-link redirect resolver (supporting `next` query extract). |
| [embed.go](file:///c:/Users/nigger%20pc/Desktop/New%20folder%20%282%29/facebed/embed.go) | 151 | HTML meta embeds formatting (supporting textOnly). |
| [crawler.go](file:///c:/Users/nigger%20pc/Desktop/New%20folder%20%282%29/facebed/crawler.go) | 48 | Crawler user-agent pattern compiler (loaded from `crawler-user-agents.json`). |
| [cookies.go](file:///c:/Users/nigger%20pc/Desktop/New%20folder%20%282%29/facebed/cookies.go) | 92 | Cookie manager (loads `cookies.json`, checks expiration, warns via Discord). |
| [config.go](file:///c:/Users/nigger%20pc/Desktop/New%20folder%20%282%29/facebed/config.go) | 58 | Configuration loader and mapping. |
| [utils.go](file:///c:/Users/nigger%20pc/Desktop/New%20folder%20%282%29/facebed/utils.go) | 181 | Shared helper functions (timestamp, human format, Discord multipart forms). |
| [facebed_test.go](file:///c:/Users/nigger%20pc/Desktop/New%20folder%20%282%29/facebed/facebed_test.go) | 322 | Unit and integration tests for configurations, crawler detection, Jq helpers, URL patterns, embed formatting, and handler routing. |
| [COOKIES.md](file:///c:/Users/nigger%20pc/Desktop/New%20folder%20%282%29/facebed/COOKIES.md) | 115 | Cookie requirements guide and extraction instructions for AI agents. |


---

## Configuration Variables

Default configuration values set in `config.go`:

| Variable | Default Value | Description |
| --- | --- | --- |
| `host` | `0.0.0.0` | Listen host IP |
| `port` | `9812` | Server listen port |
| `timezone` | `7` | GMT Offset for embed timestamp strings |
| `banned_users` | `[]` | List of user IDs to block embeds for |
| `notifier_webhook` | `""` | Discord webhook for error dumps/warnings |

---

## Standalone Deployment

- **[Dockerfile](file:///c:/Users/nigger%20pc/Desktop/New%20folder%20%282%29/facebed/Dockerfile)**: Multi-stage Docker builder compiling static Linux binary and copying assets.
- **[docker-compose.yml](file:///c:/Users/nigger%20pc/Desktop/New%20folder%20%282%29/facebed/docker-compose.yml)**: Instant run configuration.
- **[.dockerignore](file:///c:/Users/nigger%20pc/Desktop/New%20folder%20%282%29/facebed/.dockerignore)**: Ignores dev scripts, python test scripts, and compiled binaries during container builds.
