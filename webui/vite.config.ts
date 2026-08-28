import tailwindcss from '@tailwindcss/vite';
import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [
		tailwindcss(),
		sveltekit({
			compilerOptions: {
				// Force runes mode for the project, except for libraries. Can be removed in svelte 6.
				runes: ({ filename }) =>
					filename.split(/[/\\]/).includes('node_modules') ? undefined : true
			},
			adapter: adapter({
                pages: '../internal/web/dist/app',
                assets: '../internal/web/dist/app',
                fallback: 'index.html'
            })
		})
	],
	server: {
		proxy: {
			'/api': {
				target: 'http://127.0.0.1:17890',
				changeOrigin: true,
				ws: true
			}
		}
	}
});
