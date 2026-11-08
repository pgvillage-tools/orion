# About Orion

Orion is a [PgVillage](https://github.com/pgvillage) project. It is a fork of
[stolon](https://github.com/sorintlab/stolon) by Sorint.lab. Orion keeps stolon's keeper, sentinel and proxy, renames
them to `orion-*`, and adds a REST api and a cli on top of them.

## License

Orion is open source, under the [Apache 2.0 license](https://github.com/pgvillage-tools/orion/blob/main/LICENSE), like
stolon.

## Contributing

Contributions are welcome: open a [pull request](https://github.com/pgvillage-tools/orion/pulls) on GitHub. For bugs
and feature requests, file an [issue](https://github.com/pgvillage-tools/orion/issues/new/choose).

The documentation lives in the `docs` directory of the repository, and is built with
[mkdocs-material](https://squidfunk.github.io/mkdocs-material/). The top level tabs are in `mkdocs.yml`. A section
directory can have a `README.md`: it is the landing page of the section, and the list after `<!--nav-->` in it is the
navigation of the section (see [mkdocs-literate-nav](https://oprypin.github.io/mkdocs-literate-nav/)). Without a
`README.md`, the pages of a directory are listed alphabetically, after its `index.md`. To preview the documentation:

```bash
python3 -m venv ~/venv/orion-docs
~/venv/orion-docs/bin/pip install -r docs-requirements.txt
~/venv/orion-docs/bin/mkdocs serve
```

## More

<!--nav-->
* [FAQ](faq.md)
* [Unofficial packages](unofficial_packages.md)
