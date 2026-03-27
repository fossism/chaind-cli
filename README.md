<h1 align="center">chaind</h1>

<p align="center">
  <strong>The Sovereign Data Layer for Personal AI Agents</strong><br>
  <em>An Operating System for your DMs across WhatsApp, Telegram, and Matrix.</em>
</p>

## Explain It To Me Like I Have Zero Knowledge
Imagine you have a dozen different chat applications on your phone—WhatsApp, Telegram, Matrix, Discord, etc. Every time you want to send a message to a different friend, you have to open a different app, learn a different interface, and trust a different corporate server.

**`chaind` is like a universal, invisible Post Office that runs silently on your own computer.** 

Instead of dealing with 10 different apps, you (or your automated AI assistants) just talk to the `chaind` Post Office using one simple, identical language. `chaind` securely logs everything into a personal vault (a local database) and automatically translates and delivers your message to Telegram, Matrix, or wherever it needs to go. 

## The Problem: Agents in Silos

Cloud-based bot frameworks (like Vercel's Chat SDK) are built for SaaS platforms. They require webhooks, rely on ephemeral 24-hour windows, and treat your AI as a second-class "bot" that has no historical context of your digital life. If you want an AI agent to help you manage your personal communications, it shouldn't live in the cloud, and it shouldn't have to ask permission to access your chats.

## The Solution: `chaind`

`chaind` is a local-first, heavily encrypted daemon built purely in Go. It doesn't use bot APIs. It logs directly into **your** accounts—acting as a linked companion device on WhatsApp (Multi-Device), a native client on Telegram (MTProto), and a standard client on Matrix.

It pulls your messages down into a single, unified, local SQLite database and exposes them to your local AI agents through a secure **Unix Socket IPC**. 

You get the power of comprehensive AI agents running directly against your human chat history—without deploying a cloud server, without opening a port, and without giving a third party your private data.

### ✨ Features for the FOSS Hack

- **Local-First, Cloud-Free:** Agents talk to `unix:///tmp/chaind.sock`. No webhooks. No API gateways.
- **Native User Access:** `gotd/td` for Telegram, `whatsmeow` for WhatsApp, and `mautrix-go` for Matrix. Acts as *you*, not a bot.
- **Unified Canonical Schema:** AI agents don't need to know if a message came from WhatsApp or Matrix. `chaind` normalizes 3 different network protocols into one clean struct.
- **Absolute Privacy:** Written in Pure Go using `modernc.org/sqlite` and `zalando/go-keyring` for OS-native credential security.
- **Single Binary Magic:** Zero dependencies. Drop the `chaind` binary on a Raspberry Pi, a Linux server, or a Mac, and it just works.

## Installation

Download the single statically linked binary from the releases page, or build it yourself (requires Go 1.21+):

```bash
git clone https://github.com/fossism/chaind-cli.git
cd chaind-cli
go build -o chaind
mv chaind /usr/local/bin/
```

## Quick Start

**1. Start the daemon in the background:**
```bash
chaind daemon start
```

**2. Link your accounts:**
```bash
# Scan a QR code to link WhatsApp Multi-Device
chaind auth whatsapp

# Authenticate with MTProto
chaind auth telegram 

# Login to your homeserver
chaind auth matrix
```

**3. Let your agents query the socket:**
Agents can now query the secure local socket to build context.
```bash
curl --unix-socket /tmp/chaind.sock http://localhost/api/v1/messages/recent?network=whatsapp
```

## Why pure Go?

We consciously chose to avoid CGO dependencies (like `mattn/go-sqlite3`). By using `modernc.org/sqlite` (a pure-Go translation of SQLite), `chaind` cross-compiles flawlessly to `arm64`, `amd64`, Windows, macOS, and Linux out of the box. Security, speed, and zero-friction distribution.

## Community & Contributing

`chaind` is being built for the upcoming FOSS Hack. We welcome contributions, especially towards writing formatting engines for Markdown-to-Platform native rendering. 

---
*Your DMs are your data. Own them.*
