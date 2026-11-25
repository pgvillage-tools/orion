---
title: Command reference
summary: The options of orion-api, orion-cli, orion-keeper, orion-proxy and orion-sentinel, and how to set them with environment variables
authors:
  - Sebas Mannem
date: 2026-11-25
---

# Command reference

| Command | What it runs | Environment variable prefix |
|---------|--------------|-----------------------------|
| [orion-api](orion-api.md) | The api that the cli talks to | `ORIONAPI` |
| [orion-cli](orion-cli.md) | The command line interface for the api | `ORIONCLI` |
| [orion-keeper](orion-keeper.md) | Manages one PostgreSQL instance | `ORIONKEEPER` |
| [orion-sentinel](orion-sentinel.md) | Decides the state of the cluster, e.g. which instance is the primary | `ORIONSENTINEL` |
| [orion-proxy](orion-proxy.md) | Forwards client connections to the primary | `ORIONPROXY` |

Every command shows its options with `--help`.

## Environment variables

An option can also be set with an environment variable: the prefix of the command, an underscore, and the name of the
option in upper case with underscores instead of dashes. E.g. `--data-dir` of `orion-keeper` is
`ORIONKEEPER_DATA_DIR`. An option on the command line wins over its environment variable.

This works for the options of the command itself, not for the options of the subcommands of `orion-cli`: for
`orion-cli` it covers the options under _Options inherited from parent commands_ (like `--host` as `ORIONCLI_HOST`),
not e.g. `--api-san` of `orion-cli bootstrap`.

Some secrets are only read from the environment (or a file), never from an option on the command line:

| Variable | Used by | Alternative |
|----------|---------|-------------|
| `ORIONAPI_BOOTSTRAP_SECRET` | `orion-api` in bootstrap mode | |
| `ORIONCLI_BOOTSTRAP_SECRET` | `orion-cli bootstrap` | `--bootstrap-secret-file`, or the terminal |
| `ORIONCLI_CA_PASSPHRASE` | `orion-cli pki init` and `orion-cli bootstrap` | `--ca-passphrase-file`, or the terminal |

See [Bootstrap design](../../api/bootstrap-design.md) for the bootstrap secret and the passphrase.

## The pages

The other pages in this section are generated from the commands with `scripts/gen_commands_doc.sh`. Don't edit them:
change the help of the command and run the script. CI fails when they are not up to date.
