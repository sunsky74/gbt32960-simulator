<script setup lang="ts">
import { computed } from 'vue'
import { ApiOutlined, MenuFoldOutlined, MenuUnfoldOutlined } from '@ant-design/icons-vue'
import { activeNavKey, bottomNavItems, navItems } from '../../navigation'

// IDE 工具栏式侧边导航宽度:折叠仅图标 / 展开图标 + 文字。
// 通过 CSS 变量 --nav-w 注入,宽度切换仍由 .sidenav 的 width transition 平滑过渡。
const EXPANDED_W = 168
const COLLAPSED_W = 56

const props = defineProps<{ collapsed: boolean }>()
const emit = defineEmits<{ (e: 'update:collapsed', v: boolean): void }>()

const navWidth = computed(() => (props.collapsed ? COLLAPSED_W : EXPANDED_W))

function onSelect(info: { key: string | number }) {
  activeNavKey.value = String(info.key)
}
</script>

<template>
  <aside
    class="sidenav"
    :class="{ collapsed }"
    :style="{ '--nav-w': navWidth + 'px' }"
  >
    <div class="sidenav-brand" @click="emit('update:collapsed', !collapsed)">
      <ApiOutlined class="brand-icon" />
      <span v-if="!collapsed" class="brand-text">32960 工具</span>
      <component
        :is="collapsed ? MenuUnfoldOutlined : MenuFoldOutlined"
        class="collapse-trigger"
      />
    </div>

    <a-menu
      mode="inline"
      :inline-collapsed="collapsed"
      :selected-keys="[activeNavKey]"
      @click="onSelect"
    >
      <a-menu-item v-for="item in navItems" :key="item.key">
        <component :is="item.icon" />
        <span>{{ item.title }}</span>
      </a-menu-item>
    </a-menu>

    <!-- 底部固定区:系统级入口(设置)下沉至此,与主菜单同为 a-menu 以继承统一样式/选中态/折叠 Tooltip -->
    <div class="sidenav-bottom">
      <a-menu
        mode="inline"
        :inline-collapsed="collapsed"
        :selected-keys="[activeNavKey]"
        @click="onSelect"
      >
        <a-menu-item v-for="item in bottomNavItems" :key="item.key">
          <component :is="item.icon" />
          <span>{{ item.title }}</span>
        </a-menu-item>
      </a-menu>
    </div>
  </aside>
</template>

<style scoped>
.sidenav {
  width: var(--nav-w, 168px);
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  background: var(--bg-panel);
  border-right: 1px solid var(--border-subtle);
  transition: width 0.2s ease;
  overflow: hidden;
}

.sidenav-brand {
  height: 44px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 14px;
  cursor: pointer;
  font-weight: 600;
  font-size: 14px;
  color: var(--text-primary);
  border-bottom: 1px solid var(--border-subtle);
  flex-shrink: 0;
  white-space: nowrap;
}

.brand-icon {
  font-size: 16px;
  color: var(--primary);
}

.collapse-trigger {
  margin-left: auto;
  color: var(--text-tertiary);
  font-size: 13px;
}

.collapsed .sidenav-brand {
  padding: 0;
  justify-content: center;
}

.collapsed .collapse-trigger {
  display: none;
}

/* 底部系统入口区:菜单项 padding 自带,容器只负责贴底(margin-top:auto 由 flex 列布局收尾)与分隔线 */
.sidenav-bottom {
  margin-top: auto;
  display: flex;
  flex-direction: column;
  padding-bottom: 12px;
  border-top: 1px solid var(--border-subtle);
  flex-shrink: 0;
}

.sidenav :deep(.ant-menu) {
  background: transparent;
  border-inline-end: none !important;
  padding-top: 4px;
}

/* ---- 折叠态(56px):antd 内置 Tooltip(placement=right)负责功能名提示;此处只管图标居中 ---- */
/* antd 折叠菜单默认宽 80px,在 56px 窄栏下需收满,否则图标不居中且溢出被裁 */
.sidenav :deep(.ant-menu-inline-collapsed) {
  width: 100%;
}

/* 覆盖 antd 的 calc(50% - 12px) 内边距;折叠时菜单 mode 切为 vertical(li 回退为
   display:block),需显式恢复 flex 布局才能水平精确居中图标 */
.sidenav :deep(.ant-menu-inline-collapsed > .ant-menu-item) {
  display: flex;
  align-items: center;
  justify-content: center;
  padding-inline: 0;
}

.sidenav :deep(.ant-menu-inline-collapsed > .ant-menu-item > .ant-menu-title-content) {
  flex: none;
  min-width: 0;
  overflow: visible;
}

/* 折叠时隐藏文字 span(antd 默认仅 opacity:0 仍占位,会推歪图标)。
   选择器带 .sidenav 前缀,不会命中渲染在 body 的 antd 内置 Tooltip 弹层。 */
.sidenav :deep(.ant-menu-inline-collapsed .ant-menu-title-content > span:not(.anticon)) {
  display: none;
}

/* ---- 选中态:保留 antd primary 色文字/底色高亮,叠加左侧指示条微调加深(IDE 风格) ---- */
.sidenav :deep(.ant-menu-item-selected::after) {
  content: '';
  position: absolute;
  left: 0;
  top: 50%;
  transform: translateY(-50%);
  width: 2px;
  height: 14px;
  border-radius: 0 2px 2px 0;
  background: var(--primary);
}

.sidenav :deep(.ant-menu-item-selected .ant-menu-title-content) {
  color: var(--primary);
}
</style>
