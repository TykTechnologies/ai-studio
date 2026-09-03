# test-secrets/

Untracked credentials for the **live vendor conformance suite** — the tests that
call real LLM vendors to prove our request/response translation is schema-correct
across every ingress surface.

Everything in this directory is gitignored except this README and `*.example`.
Never commit a filled-in `.env` file. Never paste real keys into a PR, an issue,
or a test fixture.

## Setup

```bash
cp test-secrets/vendors.env.example test-secrets/vendors.env
$EDITOR test-secrets/vendors.env      # fill in the vendors you can reach
make test-vendors
```

You do **not** need to `source` the file. The harness loads it directly with
`godotenv`, so it works identically under bash, zsh and fish. Real process
environment variables always win over file values, which is how CI injects
secrets without a file on disk.

Point at a different file with `VENDOR_TESTS_ENV_FILE=/path/to/other.env`.

## Behaviour

- The suite is behind the `vendorlive` build tag **and** `VENDOR_TESTS_ENABLED=true`.
  It can never be picked up by `go test ./...`, `make test-all` or CI by accident.
- A vendor with blank required variables is **skipped**, not failed. Fill in only
  what you have; the run summary lists every skip and why.
- These tests spend real money. `VENDOR_TESTS_MAX_TOKENS` caps every completion.

See `features/VendorConformance.md` for the full design.
