import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import wails from "@wailsio/runtime/plugins/vite";
import tailwindcss from "@tailwindcss/vite";
import path from "path";

export default defineConfig({
  plugins: [react(), wails("./bindings"), tailwindcss()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  // Wails asset proxy dials 127.0.0.1; binding only to ::1 leaves the window blank.
  server: {
    host: "127.0.0.1",
    port: 9245,
    strictPort: true,
  },
  clearScreen: false,
});
