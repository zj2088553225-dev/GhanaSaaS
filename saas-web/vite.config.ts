import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [vue(), tailwindcss()],
  build: {
    rolldownOptions: {
      output: {
        codeSplitting: {
          minSize: 20 * 1024,
          groups: [
            {
              name: 'vue-vendor',
              test: /node_modules[\\/](?:vue|vue-router|vue-i18n|pinia|@vue)[\\/]/,
              priority: 40,
            },
            {
              name: 'element-plus',
              test: /node_modules[\\/](?:element-plus|@element-plus)[\\/]/,
              priority: 30,
            },
            {
              name: 'echarts',
              test: /node_modules[\\/](?:echarts|zrender)[\\/]/,
              priority: 30,
            },
            {
              name: 'exceljs',
              test: /node_modules[\\/](?:exceljs|archiver|fast-csv|jszip|unzipper|readable-stream)[\\/]/,
              priority: 30,
            },
          ],
        },
      },
    },
  },
  server: {
    port: 5174,
    proxy: {
      '/api': {
        target: 'http://localhost:8081',
        changeOrigin: true,
      },
      '/uploads': {
        target: 'http://localhost:8081',
        changeOrigin: true,
      },
    },
  },
})
