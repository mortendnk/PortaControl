// vite empties dist/ on every build; go:embed in web.go needs dist/.keep to
// exist in a fresh checkout, so it is written back after each build.
import { writeFileSync } from "node:fs";

writeFileSync(new URL("../dist/.keep", import.meta.url), "Placeholder so go:embed finds the directory before the web UI is built.\n");
