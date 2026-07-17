Build the SendSMTP production desktop app.

1. From project root run `wails3 build` or `task build`.
2. If bindings are stale, regenerate with:
   `GOTOOLCHAIN=go1.25.0 wails3 generate bindings -ts -d frontend/bindings ./...`
3. Ensure `frontend` builds (`cd frontend && npm run build`).
4. Confirm the binary appears under `bin/` and report the path.
