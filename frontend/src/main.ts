import {createApp} from 'vue'
import Antd from 'ant-design-vue'
import 'ant-design-vue/dist/reset.css'
import App from './App.vue'
import './style.css'
import { initTheme } from './theme/useTheme'

// 挂载前初始化主题(localStorage → <html data-theme> + 同步模块单例),首帧即正确配色,无闪烁。
initTheme()

createApp(App).use(Antd).mount('#app')
