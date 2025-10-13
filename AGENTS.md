# Repository Guidelines

## Project Structure & Module Organization
- `cmd/main.go` is the entrypoint that wires the SQS repository, service, handlers, and HTTP server on :8080.
- `internal/` holds the layered logic: `handler.go`, `sqs_service.go`, `sqs_repository.go`, and `route.go`.
- Go HTML templates live in `templates/` (layout, partials, pages); add a page `.gohtml` before routing.
- Frontend source lives in `assets/js` and `assets/css`; Vite builds to `dist/`, embedded through `dist.go`.
- Tooling roots: `Makefile` for Go build/lint, `package.json` + `biome.json`, `.golangci.yml`.

## Build, Test, and Development Commands
- `pnpm run dev` starts the Vite dev server on localhost:5173.
- `export DEV_MODE=true && go run cmd/main.go` launches the Go server against dev assets.
- `make build` emits the production `./sqs-gui` binary with embedded assets.
- `pnpm run build` transpiles TypeScript/Tailwind into `dist/`.
- `make dry-lint` or `make lint` run `golangci-lint`; `pnpm lint`, `pnpm check`, `pnpm format` invoke Biome.

## Coding Style & Naming Conventions
- Keep Go code gofmt/goimports clean; respect the handler -> service -> repository layering and inject via interfaces.
- Exported Go types stay PascalCase, functions lowerCamelCase, files lower_snake (e.g., `sqs_repository.go`).
- TypeScript follows Biome defaults: tab indentation, double quotes, ES modules. Name entry files after their pages (`create_queue.ts`).
- Tailwind utilities stay in `assets/css/app.css`; templates should remain thin, driven by handler-provided data.

## Testing Guidelines
- テストはtestify/mock、testify/assertを使って実装します
  - suiteは使わなくて良いです
- 必要ならばテーブル駆動テストにしてください
- nameなどは英語にしてください

## Commit & Pull Request Guidelines
- Follow the concise, imperative commit style already in history (`Prepare the framework`); keep each commit focused.
- PRs must summarize intent, list affected routes/assets, and record manual checks (build, lint, UI smoke test).
- Link issues, attach screenshots or GIFs for UI changes, and call out required env vars (`DEV_MODE`, AWS credentials).

## Architecture & Environment Notes
- Preserve the handler -> service -> repository flow; limit SQS SDK calls to `SqsRepository`.
- Adding a page requires a template in `templates/pages`, a TS entry in `assets/js`, and a new input in `vite.config.ts`.
- Backend reads SQS endpoints from environment variables; local dev skips auth but keep AWS-ready settings in mind.
