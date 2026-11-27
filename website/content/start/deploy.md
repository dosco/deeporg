---
title: "Set Up And Deploy"
description: "Set DeepORG up on your laptop with a picture of each screen, then put it on a server your team can reach."
nav_group: "start"
doc_kind: "guide"
weight: 22
---

This page shows every screen you see when you set DeepORG up. Part 1 is your own computer and takes about five minutes. Part 2 puts it on a server so your team can use it. Do Part 1 first even if a server is where you are heading, because the steps on a server are the same ones.

## Part 1: Set it up on your laptop

### 1. Install and start

Run these two commands in a terminal:

```bash
curl -fsSL https://deeporg.io/install.sh | bash
deeporg
```

DeepORG starts, prints a link, and opens that link in your browser.

```text
Finish setup in your browser:

  http://127.0.0.1:8080/setup?token=...
```

DeepORG keeps its own files in `~/.deeporg/`. Nothing is written to the folder you ran it in. If port 8080 is already in use, it uses the next free port and tells you which.

### 2. Create your admin account

Enter an email and a password of at least 10 characters. This is the account you sign in with, and the one that can change settings later.

{{< shot src="setup-account.webp" alt="Setup step 1 of 3, Create your admin account, with an email field, a password field and a Create account button." />}}

### 3. Connect your data

Paste your database's connection URL, such as `postgres://user:password@host:5432/database`. If you would rather type the host, user and database name one by one, choose **Enter the details separately**.

If you have no database to hand, choose **Use sample data**. It connects a small made-up company with accounts, subscriptions, invoices and support tickets. The pictures on this page use it.

{{< shot src="setup-database.webp" alt="Setup step 2 of 3, Connect your data, with a connection URL field, a Connect button and a Use sample data button." />}}

DeepORG tests the connection before it saves it, then tells you how many tables it found. If the connection fails, the page says why and what to change.

{{< shot src="setup-database-connected.webp" alt="Setup step 2 of 3 after connecting: Your data is connected. We found 7 tables." />}}

### 4. Choose a model

The built-in agent uses a model from a provider you have an account with: OpenAI, Anthropic or Google Gemini. Pick the provider and paste an API key. DeepORG checks the key with one small request and keeps it encrypted.

You can choose **Do this later**. Until you choose a model, the built-in agent is off and the rest of the console works.

{{< shot src="setup-model.webp" height="960" alt="Setup step 3 of 3, Choose a model, with a provider menu, an API key field and a Check and save button." />}}

### 5. Ask your first question

Setup ends on the agent page. It suggests a few questions made from your own tables. Pick one, or type your own.

{{< shot src="setup-agent-start.webp" alt="The agent page after setup, titled Ask about your data, with three suggested questions and a box to type a question." />}}

The agent writes a query, your database runs it, and the answer comes back. **Data** opens the figures the answer came from, and **Evidence** opens what the agent looked at to get them.

{{< shot src="setup-agent-answer.webp" alt="The agent page after asking How many accounts are there. The answer reads: There are 8 accounts in the connected data." />}}

### 6. Connect Claude Code

This step is optional. Open **Connect** in the left menu and copy the command it shows.

{{< shot src="setup-connect.webp" alt="The Connect page, with a claude mcp add command to copy and three numbered steps for Claude Code." />}}

Run the command in a terminal. Then start Claude Code, type `/mcp`, choose `deeporg`, and choose **Authenticate**. Your browser opens on DeepORG and asks whether to allow it. Choose **Allow**, and Claude Code is signed in as you. There is no token to copy.

Claude Code gets its answers by asking DeepORG's built-in agent, so finish step 4 first.

That is the whole setup. A laptop install listens on your own computer only, so nobody else can reach it. To share it with your team, put it on a server.

## Part 2: Put it on a server

You need a server with a disk that keeps its files across restarts. If people will reach it over the internet, you also need a domain name for it.

### 1. Start DeepORG on the server

There are two ways to do this. Pick one.

**With the install script.** Setting `DEEPORG_LISTEN` makes DeepORG reachable from other computers.

```bash
curl -fsSL https://deeporg.io/install.sh | bash
DEEPORG_LISTEN=0.0.0.0:8080 deeporg
```

Run it with whatever your server uses to keep a program running, such as systemd, so that it starts again after a reboot. Its files are in `~/.deeporg/` of the user that runs it.

**With Docker.**

```bash
docker run -d --name deeporg \
  -p 8080:8080 \
  -v deeporg:/home/nonroot \
  dosco/deeporg
```

Inside the image DeepORG listens on port 8080 for other computers without being told to, so publishing the port is enough.

The volume is where DeepORG keeps its files, so they are still there after the container restarts. Mount it at `/home/nonroot` as shown. The image does not run as root, and a volume mounted anywhere else belongs to root. If DeepORG cannot write to its volume, it stops and says which folder to mount. Run `docker logs deeporg` to see the setup link.

### 2. Put HTTPS in front of it

DeepORG set up this way serves plain HTTP. Put a web server in front of it that handles HTTPS and passes requests on. With [Caddy](https://caddyserver.com/), this is the whole configuration:

```text
deeporg.example.com {
    reverse_proxy localhost:8080
}
```

DeepORG reads the `X-Forwarded-Proto` and `X-Forwarded-Host` headers that the web server in front of it sends. It uses them to mark its sign-in cookie as secure, and to put your `https` address in the command on the Connect page. Caddy sends both headers without being asked. With nginx you set them yourself.

Do this before you create the admin account, so your password is never sent over plain HTTP.

### 3. Finish setup from your own computer

The setup link DeepORG prints is written for the server itself:

```text
http://localhost:8080/setup?token=...
```

Replace the first part with your server's address and open it on your own computer:

```text
https://deeporg.example.com/setup?token=...
```

Only someone who has this link can create the admin account, and it stops working once the account exists. The three steps are the ones in Part 1.

When you connect your production database, give DeepORG a database user that can only read. Nobody can write to your data through DeepORG unless an admin allows it later, and a read-only user makes sure of it.

### 4. Invite your team

Open **Admin**, then **Setup**. Enter a teammate's email and choose **Make an invite link**. DeepORG makes a link for you to send. It does not send the email for you. The link works once, for one week, and your teammate chooses their own password.

{{< shot src="setup-admin.webp" alt="The Admin Setup page, listing the connected database, the model, the people who can sign in, and a form to invite a teammate." />}}

Everyone who signs in can read every table in the databases you connected, and nobody can write to them. If some tables are not for everyone, hide them before you invite people.

On the same page, under **Your data**, choose **Change** next to a database. Each table can be for everyone, for admins only, or for nobody. Saving restarts DeepORG, which takes a few seconds.

- **Admins only** means an admin can still query the table, for example in the Workbench. Nobody else can.
- **Nobody** closes the table to admins too.
- The agent only uses tables that are open to everyone. It is not told that the others exist, whoever asks.

This works a table at a time. There is no screen yet for hiding some rows of a table and not others.

Each person can connect their own Claude Code from the **Connect** page. On a server, the command there holds the server's address.

### 5. Look after the accounts

The same page lists everyone who has an account. Next to each person you can do three things.

- **Change what they can do.** Choose between "can use DeepORG" and "is an admin". It takes effect at once.
- **Give them a new password link.** DeepORG makes a link for you to send, as it does for an invite. It works once, for 24 hours. When they choose the new password, the old one stops working and they are signed out everywhere else, including in any Claude Code they connected.
- **Remove them.** They are signed out at once and can no longer sign in. You can invite them again later.

You cannot remove your own account or take your own admin rights away, and the last admin cannot be removed by anyone. This keeps a server from being left with nobody in charge.

If you are the only admin and you lose your password, make the link yourself on the server:

```bash
deeporg reset-password you@example.com
```

It prints a link written for the server itself, like the setup link. Replace the first part with your server's address. With Docker, run it inside the container: `docker exec deeporg /ko-app/v3 reset-password you@example.com`.

### What to know before you rely on it

- **Back up DeepORG's folder.** That is `~/.deeporg/`, or the Docker volume. The file `deeporg.yml` in it holds a secret key, and `store.db` holds the accounts and settings. Database passwords and the model key are encrypted with that key. If you lose it, everyone is signed out and the stored passwords cannot be read.
- **Run one copy.** DeepORG keeps its own data in a local file for now, so two servers cannot share it.
- **The health address is `/health`.** It answers as soon as DeepORG is running, before setup is finished too. If your hosting restarts a program that fails its health check, you can turn that check on from the start.
- **To update,** read the [release notes](/releases/), install the new version, and start it again.

The [release notes](/releases/#what-deeporg-does-not-do-yet) list what this setup cannot do yet. For example, a database can be added or replaced from the console, but not disconnected.

## If you want the API service instead

This page sets up the agent server. DeepORG can also run as an API over your database that serves saved queries, with no agent and no setup page. That path uses config files. Start with [saved queries](/start/saved-queries/) and [Production Deploy](/configure/environment-production/).
