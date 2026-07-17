Start a SendSMTP campaign via the headless CLI and report progress.

1. Confirm SMTPs, emails, subjects, links, and HTML are loaded (`./bin/sendsmtp-cli stats`).
2. Run: `./bin/sendsmtp-cli send`
3. When finished, print final stats (sent/failed/pending/top errors).
4. Do not invent SMTP credentials; use data already in the project DB/files.
