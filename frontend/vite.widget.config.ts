import path from 'path'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import cssInjectedByJs from 'vite-plugin-css-injected-by-js'

export default defineConfig({
  plugins: [react(), cssInjectedByJs()],
  resolve: {
    alias: { '@': path.resolve(__dirname, './src') },
  },
  build: {
    outDir: 'dist-widget',
    emptyOutDir: true,
    lib: {
      entry: path.resolve(__dirname, 'widget/widget-entry.tsx'),
      name: 'AngrosistChat',
      formats: ['iife'],
      fileName: () => 'widget.js',
    },
    rollupOptions: {
      external: [],
    },
  },
})
