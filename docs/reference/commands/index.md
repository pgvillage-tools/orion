# Command reference

The options of every Orion command and subcommand:

| Command | Binary | Reference |
|---------|--------|-----------|
| keeper | `orion-keeper` | [orion-keeper](orion-keeper.md) |
| sentinel | `orion-sentinel` | [orion-sentinel](orion-sentinel.md) |
| proxy | `orion-proxy` | [orion-proxy](orion-proxy.md) |
| cli | `orion` | [orion](orion.md), with a page for every subcommand |

For how to pass the options, on the command line or as environment variables, see
[Commands invocation](../commands_invocation.md).

!!! note
    The other pages in this section are generated from the code. After changing the options of a command, run
    `scripts/gen_commands_doc.sh` and commit the result. The generator does not remove pages of commands that no longer
    exist, so remove those by hand.
