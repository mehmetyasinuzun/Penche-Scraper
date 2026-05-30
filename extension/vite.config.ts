import { defineConfig } from 'vite';
import webExtension from 'vite-plugin-web-extension';
import { cpSync, existsSync } from 'node:fs';
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('.', import.meta.url));

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
      {
        name: 'penche-copy-icons',
        closeBundle() {
          const src = resolve(root, 'icons');
          if (existsSync(src)) {
            cpSync(src, resolve(root, outDir, 'icons'), { recursive: true });
          }
        },
      },
    ],
  };
});
