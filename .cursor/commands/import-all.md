Import all SendSMTP data files into SQLite using the headless CLI.

1. Build CLI if needed: `go build -o bin/sendsmtp-cli ./cmd/sendsmtp`
2. Ensure files exist at paths from `app.config.yml` (or copy from `data/*.example.*`).
3. Run: `./bin/sendsmtp-cli import all`
4. Then run `./bin/sendsmtp-cli status` and summarize inserted counts / pending emails / active SMTPs.
