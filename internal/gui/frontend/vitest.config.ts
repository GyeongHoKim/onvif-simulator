import { defineConfig } from "vitest/config"
import react from "@vitejs/plugin-react"
import path from "node:path"

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/test/setup.ts"],
    coverage: {
      provider: "v8",
      reporter: ["text", "html", "json-summary"],
      include: ["src/**/*.{ts,tsx}"],
      exclude: [
        "src/lib/wails/**",
        "src/main.tsx",
        "src/App.tsx",
        "src/vite-env.d.ts",
        "**/*.d.ts",
        "src/components/ui/**",
        "src/components/theme-provider.tsx",
        "src/hooks/use-mobile.ts",
        "src/test/**",
        "src/store/README.md",
      ],
      thresholds: {
        lines: 70,
        functions: 70,
        branches: 70,
        statements: 70,
      },
    },
  },
})
