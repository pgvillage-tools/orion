## Commands invocation

There are 4 commands provided by orion:

* [orion-keeper](commands/orion-keeper.md)
* [orion-sentinel](commands/orion-sentinel.md)
* [orion-proxy](commands/orion-proxy.md)
* [orion](commands/orion-cli.md)

every command has different options and when called without any option or with `--help` they'll show an help with a description for every subcommand and option. Please take also a look at the various examples to see how the commands' options are used.

An option can be specified in the command line or as an environment variables.

Environment variables are prefixed with a string related to the command and take the name of the options but are UPPERCASE and any dashes are replaced by underscores.

For example for the  `orion-keeper` `--data-dir` command line option the equivalent environment variable is `ORIONKEEPER_DATA_DIR`


| Command            | Environment variable prefix |
|--------------------|-----------------------------|
| orion-keeper      | ORIONKEEPER                    |
| orion-sentinel    | ORIONSENTINEL                  |
| orion-proxy       | ORIONPROXY                     |
| orion          | ORIONCLI                   |
