---
title: "Start"
description: "Install DeepORG, set it up in your browser, and ask your first question."
nav_group: "start"
weight: 1
---

Install DeepORG and run it. A setup page opens in your browser, and you finish there: create your account, connect a database, and choose a model. These pages take you through it and then show the query language underneath.

| Page | Use it for |
| --- | --- |
| [Install](/start/install/) | Install the command and start DeepORG. |
| [Quick Start](/start/quick-start/) | Go from nothing to a first answer from your own data in about five minutes. |
| [Set Up And Deploy](/start/deploy/) | See each setup screen, then put DeepORG on a server for your team. |
| [Demos](/start/demos/) | Try a made-up company with its data already filled in. |
| [First Query](/start/first-query/) | Understand root fields, primary-key lookups, aliases, variables, and explicit ordering. |
| [Saved Queries](/start/saved-queries/) | Run DeepORG as an API service that serves only the queries you saved. |

Most query examples use the test schema from `tests/postgres.sql`: `users`, `products`, `purchases`, `comments`, categories, and a few JSON/geo fixtures. The same GraphQL shape is what the Go example tests execute across dialects.
