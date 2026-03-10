# Contributing

**Pull requests are welcome and encouraged.** Whether it's a bug fix, new adapter, additional redaction patterns, or documentation improvement — we'd love to have it.

## Quick start

```bash
git clone https://github.com/sageox/frictionax.git
cd frictionax
make check   # fmt + lint + test
```

**Prerequisites:** Go 1.22+, [golangci-lint](https://golangci-lint.run), [gotestsum](https://github.com/gotestyourself/gotestsum)

## Submitting a PR

1. Fork and branch from `main`
2. Write tests for new functionality
3. Run `make check` before submitting
4. Open a pull request — we review promptly

Good areas for contribution:

- New CLI framework adapters (beyond Cobra, Kong, urfave/cli)
- Additional secret redaction patterns
- New actor detectors for specific agent environments
- Bug fixes and test coverage improvements

## Reporting issues

File a [GitHub issue](https://github.com/sageox/frictionax/issues) with steps to reproduce.

