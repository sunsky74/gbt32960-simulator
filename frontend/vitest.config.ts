import {defineConfig} from 'vitest/config'
import vue from '@vitejs/plugin-vue'

// 独立于 vite.config.ts 的测试配置:应用构建配置保持不变
export default defineConfig({
  plugins: [vue()],
  test: {
    environment: 'jsdom',
    include: ['src/**/*.test.ts'],
  },
})
