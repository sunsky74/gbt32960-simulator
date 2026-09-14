// ESLint 9 扁平配置(flat config,ESM)。
// 生成产物(dist)与 Wails 生成绑定(wailsjs)不参与 lint,也不应被格式化。
import js from '@eslint/js'
import globals from 'globals'
import pluginVue from 'eslint-plugin-vue'
import tseslint from 'typescript-eslint'
import configPrettier from 'eslint-config-prettier/flat'

export default tseslint.config(
  { ignores: ['dist', 'node_modules', 'wailsjs'] },

  js.configs.recommended,
  ...tseslint.configs.recommended,
  // 用 recommended(非 strongly-recommended):只保留正确性规则,规避纯格式规则的噪声。
  ...pluginVue.configs['flat/recommended'],

  {
    // .vue 内嵌 TS:外层用 vue-eslint-parser,<script lang="ts"> 交给 TS 解析器。
    files: ['**/*.vue'],
    languageOptions: {
      parserOptions: { parser: tseslint.parser },
    },
    rules: {
      // eslint-recommended 的 files 仅覆盖 .ts/.tsx,这里为 .vue 手动关闭核心重复规则:
      // no-undef 由 TS 类型检查负责,no-unused-vars 由 @typescript-eslint/no-unused-vars 负责。
      'no-undef': 'off',
      'no-unused-vars': 'off',
    },
  },

  {
    // 配置类文件(eslint/vite/vitest)运行在 Node 环境。
    files: ['*.config.js', '*.config.ts'],
    languageOptions: {
      globals: { ...globals.node },
    },
  },

  // 必须最后:关闭与 Prettier 冲突的格式类规则。
  configPrettier,
)
