import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// `npm run dev` proxies the API to a local portacontrold (default :8080).
const target = process.env.PORTACONTROL_DEV_API ?? "http://127.0.0.1:8080";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: { outDir: "dist", emptyOutDir: true, assetsDir: "assets", chunkSizeWarningLimit: 700 },
  server: { proxy: { "/api": { target, changeOrigin: false }, "/health": target } },
});
