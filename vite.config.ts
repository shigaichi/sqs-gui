import { defineConfig } from 'vite'
import { resolve } from 'path'
import tailwindcss from '@tailwindcss/vite'

// https://vitejs.dev/config/
export default defineConfig({
    plugins: [tailwindcss(),],
    build: {
        // generates .vite/manifest.json in outDir
        manifest: true,
        outDir: resolve(__dirname, 'dist'), // ← これを追加！
        emptyOutDir: true,
        rollupOptions: {
            // overwrite default .html entry and include a secondary
            input: {
                app: resolve(__dirname, 'assets/js/app.ts'),
                queues: resolve(__dirname, 'assets/js/queues.ts'),
                create_queue: resolve(__dirname, 'assets/js/create_queue.ts'),
            },
        },
    },
})