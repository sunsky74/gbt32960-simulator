import {defineConfig} from 'vite'
import vue from '@vitejs/plugin-vue'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [vue()],
  build: {
    // antd-vendor 单块约 1.44MB:桌面端经本地文件加载,无首屏网络传输成本,1445 为可清除 500kB 告警的最小阈值
    chunkSizeWarningLimit: 1445,
    rollupOptions: {
      output: {
        // 手动分包:框架与组件库各自成块,避免单文件 1.6MB 巨包
        manualChunks: {
          'vue-vendor': ['vue'],
          'antd-vendor': ['ant-design-vue', '@ant-design/icons-vue'],
        },
      },
    },
  },
})
