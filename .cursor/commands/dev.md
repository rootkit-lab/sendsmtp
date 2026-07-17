Run the SendSMTP desktop app in development mode with hot reload.

1. Ensure Go 1.25+ toolchain is available (`GOTOOLCHAIN=go1.25.0` if needed).
2. From the project root, run: `wails3 dev` (or `task dev`).
3. If frontend deps are missing, run `cd frontend && npm install` first.
4. Report any startup errors and fix them until the window opens.
