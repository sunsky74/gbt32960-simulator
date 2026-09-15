import { ref } from 'vue'

// 跨页跳转目标:设置页需聚焦的分类 key。
// 写入方:openSettingsCategory(toast「发现新版本」点击等);消费方:SettingsPage(消费后置空)。
export const settingsFocusCategory = ref<string | null>(null)

// 请求打开 设置 → 指定分类(实际导航切换由 App.vue 监听该 ref 完成)。
export function openSettingsCategory(key: string) {
  settingsFocusCategory.value = key
}
