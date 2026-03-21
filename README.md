# ChainD - Developer Workflow Bridge (Golang Edition)

ChainD is a powerful CLI tool built for FOSS Hack 2026. It integrates your open-source tasks directly into your local development environment. By syncing your assigned issues and pull requests, it eliminates context-switching and speeds up your workflow. 

## The Problem
Developers live in the terminal, but project management and issue tracking occur in clunky web dashboards, which breaks focus and flow.

## The Solution
An open-source integration engine that automatically brews lists of tasks and PRs, syncing them effortlessly. ChainD is a fast, terminal-friendly TUI/CLI designed to pull down GitHub tasks into a local-first interface, bridging the web out of your local dev experience.

## Installation

```bash
go mod tidy
go build -o chaind

# Optional: Add it to your PATH
sudo mv chaind /usr/local/bin/
```

## Usage

1. **Initialize the local database and Setup Authentication**
   ```bash
   ./chaind auth
   ```

2. **Sync Assigned Tasks**
   ```bash
   ./chaind sync
   ```

3. **List Your Tasks**
   ```bash
   ./chaind ls
   ```

4. **Start Working on an Issue**
   ```bash
   ./chaind start <repo> <issue_number>
   ```

5. **Mark as Done**
   ```bash
   ./chaind done <repo> <issue_number>
   ```

6. **Open in Browser**
   ```bash
   ./chaind open <repo> <issue_number>
   ```

## Stack
- Go 1.20+
- [Cobra](https://cobra.dev/) - Modern CLI framework
- [Tablewriter](https://github.com/olekukonko/tablewriter) - Simple markdown-style terminal tables
- [GORM](https://gorm.io/) - Elegant ORM

## Future Roadmap: Bridging Matrix, WhatsApp, and Telegram

One of the largest hurdles in FOSS communities is scattered communication. Users report bugs in WhatsApp groups, Telegram chats, and Matrix rooms—and maintainers lose track.

**How ChainD integrates this:**
Using **Matrix** as the primary backbone, along with bridges like `mautrix-whatsapp` and `mautrix-telegram`, ChainD can act as a local federated client overlay:
- **Notification Aggregation:** Fetch "@mentions" and critical bug reports across all synchronized federated apps right into your terminal alongside your GitHub issues.
- **Bi-directional Status Pushing:** Running `./chaind done <repo> <issue>` can automatically dispatch a completed notification back to the Matrix room that sparked the issue, which simultaneously bridges out to the original reporter on WhatsApp/Telegram!
- **Zero-Switching:** You never have to touch a web browser or a chat GUI. You manage the issue, resolve the code, and notify the community entirely via terminal commands.
