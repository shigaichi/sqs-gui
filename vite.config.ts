import { resolve } from "node:path";
import tailwindcss from "@tailwindcss/vite";
import { defineConfig } from "vite";

// https://vitejs.dev/config/
export default defineConfig({
	plugins: [tailwindcss()],
	build: {
		// generates .vite/manifest.json in outDir
		manifest: true,
		outDir: resolve(__dirname, "dist"), // ensure bundles land in the embedded dist directory
		emptyOutDir: true,
		rollupOptions: {
			// overwrite default .html entry and include a secondary
			input: {
				app: resolve(__dirname, "assets/js/app.ts"),
				queues: resolve(__dirname, "assets/js/queues.ts"),
				queue: resolve(__dirname, "assets/js/queue.ts"),
				create_queue: resolve(__dirname, "assets/js/create_queue.ts"),
				send_receive: resolve(__dirname, "assets/js/send_receive.ts"),
			},
		},
	},
});
