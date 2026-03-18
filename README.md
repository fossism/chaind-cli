# ChainD - Developer Workflow Bridge

ChainD is a powerful CLI tool built for FOSS Hack 2026. It integrates your open-source tasks directly into your local development environment. By syncing your assigned issues and pull requests, it eliminates context-switching and speeds up your workflow.

## The Problem
Developers live in the terminal, but project management and issue tracking occur in clunky web dashboards, which breaks focus and flow.

## The Solution
An open-source integration engine that automatically brews lists of tasks and PRs, syncing them effortlessly. ChainD is a fast, terminal-friendly TUI/CLI designed to pull down GitHub tasks into a local-first interface, bridging the web out of your local dev experience.

## Installation

You can install it locally to test:
```bash
python3 -m venv venv
source venv/bin/activate
pip install -e .
```

## Usage

1. **Initialize the local database and Setup Authentication**
   ```bash
   chaind auth
   ```
   *Expects your GitHub Username and Personal Access Token to be stored securely in `~/.config/chaind/config.json`.*

2. **Sync Assigned Tasks**
   ```bash
   chaind sync
   ```

3. **List Your Tasks**
   ```bash
   chaind ls
   ```

4. **Start Working on an Issue**
   ```bash
   chaind start <repo> <issue_number>
   ```
   *This automatically marks the issue as in-progress and checks out a new branch (`issue-<number>`) if you are in the repository folder.*

5. **Mark as Done**
   ```bash
   chaind done <repo> <issue_number>
   ```
   *Sets the local status to "done" and suggests the correct closing commit format!*

6. **Open in Browser**
   ```bash
   chaind open <repo> <issue_number>
   ```

## Stack
- Python 3.10+
- [Typer](https://typer.tiangolo.com/) - Beautiful CLI
- [Rich](https://rich.readthedocs.io/en/stable/) - Gorgeous Terminal Interface
- [SQLModel](https://sqlmodel.tiangolo.com/) - Elegant interactions with SQLite

Made for FOSS Hack 2026!
