import { execFileSync } from "node:child_process";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

// The frontend calls the BFF built from the same commit at /bff/<sha>/, so
// every build needs VITE_BUILD_SHA: the first 12 characters of the commit,
//   VITE_BUILD_SHA=$(git rev-parse HEAD | cut -c1-12) pnpm build
// The dev server falls back to the checkout's HEAD.
const SHA = /^[0-9a-f]{12}$/;

function headSHA(): string {
  try {
    return execFileSync("git", ["rev-parse", "HEAD"], { encoding: "utf8" }).trim().slice(0, 12);
  } catch {
    return "";
  }
}

export default defineConfig(({ command }) => {
  let sha = process.env.VITE_BUILD_SHA ?? "";
  if (sha === "" && command === "serve") {
    sha = headSHA();
    process.env.VITE_BUILD_SHA = sha;
  }
  if (!SHA.test(sha)) {
    throw new Error(
      `VITE_BUILD_SHA must be the 12 lowercase hex characters of the commit (git rev-parse HEAD | cut -c1-12), got ${JSON.stringify(sha)}`,
    );
  }
  return {
    plugins: [react()],
    server: {
      proxy: {
        // The BFF (bff/main.go), and the Todo API for commands.
        "/bff": "http://localhost:8081",
        "/commerce/todo/v1": "http://localhost:8080",
      },
    },
  };
});
