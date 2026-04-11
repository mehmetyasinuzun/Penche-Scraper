import { defineConfig } from 'vite';
import webExtension from 'vite-plugin-web-extension';
import path from 'path';

export default defineConfig(({ mode }) => {
  const isFirefox = mode === 'firefox';
  const manifest = isFirefox ? 'manifest.firefox.json' : 'manifest.chrome.json';
  const outDir = isFirefox ? 'dist/firefox' : 'dist/chrome';

  return {
    resolve: {
      alias: {
        // Provide the polyfill as the global `browser` object.
        browser: path.resolve(__dirname, 'node_modules/webextension-polyfill/dist/browser-polyfill.js'),
      },
    },
    build: {
      outDir,
      emptyOutDir: true,
      sourcemap: mode === 'development' ? 'inline' : false,
      rollupOptions: {
        // Prevent polyfill from being tree-shaken.
        external: [],
      },
    },
    plugins: [
      webExtension({
        manifest,
        // Gather all HTML entry points automatically.
        additionalInputs: [
          'src/popup/popup.html',
          'src/options/options.html',
          'src/background/index.ts',
          'src/content/index.ts',
        ],
        disableAutoLaunch: true,
      }),
    ],
    define: {
      __BROWSER__: JSON.stringify(isFirefox ? 'firefox' : 'chrome'),
    },
  };
});
