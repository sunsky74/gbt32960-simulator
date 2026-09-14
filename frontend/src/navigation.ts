import { ref, markRaw, type Component } from 'vue'
import { AppstoreOutlined, CarOutlined, SearchOutlined, DesktopOutlined, SettingOutlined } from '@ant-design/icons-vue'
import ClientSimulatorPage from './pages/ClientSimulatorPage.vue'
import PacketParserPage from './pages/PacketParserPage.vue'
import ServerModePage from './pages/ServerModePage.vue'
import ExtensionsPage from './pages/ExtensionsPage.vue'
import SettingsPage from './pages/SettingsPage.vue'

export interface NavItem {
  key: string
  title: string
  icon: Component
  component: Component
}

// 一级功能入口注册表:新增工具模块 = 在此追加一项,SideNav 与页面切换自动生效。
// markRaw 避免组件对象被 reactive 代理拖累渲染性能。
export const navItems: NavItem[] = [
  { key: 'client', title: '客户端模拟', icon: markRaw(CarOutlined), component: markRaw(ClientSimulatorPage) },
  { key: 'parser', title: '报文解析', icon: markRaw(SearchOutlined), component: markRaw(PacketParserPage) },
  { key: 'server', title: '服务端模式', icon: markRaw(DesktopOutlined), component: markRaw(ServerModePage) },
  { key: 'extensions', title: '扩展包', icon: markRaw(AppstoreOutlined), component: markRaw(ExtensionsPage) },
]

export const bottomNavItems: NavItem[] = [
  { key: 'settings', title: '设置', icon: markRaw(SettingOutlined), component: markRaw(SettingsPage) },
]

export const activeNavKey = ref('client')
