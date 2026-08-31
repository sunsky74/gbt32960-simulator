import { reactive, watch } from 'vue'
import { bridge } from '../../wailsjs/go/models'
import { store } from '../state'

// 当前编辑中的连接配置(右侧多张卡片共享:连接卡片/车辆卡片)
export const cfg = reactive(new bridge.ConnectionConfig())

export function initConnConfig() {
  if (!cfg.tls) {
    cfg.tls = { enabled: false, ca: '', clientCert: '', clientKey: '', serverName: '', insecure: false }
  }
  if (!cfg.version) cfg.version = '2016'
  if (!cfg.port) cfg.port = 32960
  if (!cfg.name) cfg.name = '默认连接'
  if (cfg.heartbeatSec === undefined || cfg.heartbeatSec === null) cfg.heartbeatSec = 30
  if (!cfg.subsystemCodes) cfg.subsystemCodes = []

  if (store.config) {
    Object.assign(cfg, store.config)
    if (store.config.tls) Object.assign(cfg.tls, store.config.tls)
  }
}

// 激活档案切换后,同步表单
watch(
  () => store.config,
  (v) => {
    if (v) {
      Object.assign(cfg, v)
      if (v.tls) Object.assign(cfg.tls, v.tls)
    }
  },
)

// 新建档案:重置为空白默认值(保存后才入档案列表)
export function resetToNewProfile(n: number) {
  Object.assign(cfg, {
    name: `新连接-${n}`,
    host: '127.0.0.1',
    port: 32960,
    version: '2016',
    vin: '',
    iccid: '',
    subsystemCodes: [],
    heartbeatSec: 30,
    autoClockSync: true,
    autoReconnect: false,
    reportInterval: 10,
    reissueOffsetSec: 60,
    tls: { enabled: false, ca: '', clientCert: '', clientKey: '', serverName: '', insecure: false },
  })
}
