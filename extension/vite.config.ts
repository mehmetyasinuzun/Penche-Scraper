import { defineConfig } from 'vite';
import webExtension from 'vite-plugin-web-extension';

export default defineConfig(({ mode }) => {
  const isFirefox = mode === 'firefox';
  const manifest = isFirefox ? 'manifest.firefox.json' : 'manifest.chrome.json';
  const outDir = isFirefox ? 'dist/firefox' : 'dist/chrome';

  return {
    build: {
      outDir,
      emptyOutDir: true,
      sourcemap: mode === 'development' ? 'inline' : false,
    },
    plugins: [
      webExtension({
        manifest,
        disableAutoLaunch: true,
      }),
    ],
  };
});
