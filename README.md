# Loadlore

Loadlore is an open-source, terminal-first lab for system design. You describe a system in a YAML design file, run it on real Docker containers under realistic load and faults, and get a report that explains where and why it broke.

The CLI is `loadl`.

> **Status:** early development. Nothing here is usable yet.

- **Spec:** [Loadlore — Product Spec](https://claude.ai/artifact/TEkM1GHG5mpmKXTcH3jJ6m) is the source of truth for scope and behavior.
- **AI skill:** [`skills/loadlore-designer/`](skills/loadlore-designer/) turns plain-language descriptions into validated design files.

## Development

Requires Go (the version in `go.mod`; the toolchain downloads itself) and, for end-to-end tests, Docker with Compose v2.

```sh
make build   # bin/loadl
make test    # unit tests
make lint    # golangci-lint v2
make e2e     # end-to-end tests against Docker
```

## License

[Apache-2.0](LICENSE)
