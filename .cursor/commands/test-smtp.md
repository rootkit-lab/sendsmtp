Test one or more SMTP accounts in the SendSMTP SQLite database.

1. List SMTPs via `./bin/sendsmtp-cli stats` or by querying through the app engine / UI bindings.
2. For a given id, run: `./bin/sendsmtp-cli test-smtp <id>`
3. Report success or the exact SMTP error. Do not print passwords.
