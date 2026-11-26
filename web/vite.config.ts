import { defineConfig } from "vitest/config";

export default defineConfig({
  base: "/",
  build: { target: "es2022", sourcemap: false },
  test: {
    environment: "jsdom",
    setupFiles: ["./test/setup.ts"],
    restoreMocks: true,
  },
});
