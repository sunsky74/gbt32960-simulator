<script setup lang="ts">
import { ref } from 'vue'
import { UpOutlined, DownOutlined } from '@ant-design/icons-vue'

const props = withDefaults(
  defineProps<{
    title: string
    defaultOpen?: boolean
  }>(),
  { defaultOpen: true },
)

const open = ref(props.defaultOpen)

function toggle() {
  open.value = !open.value
}
</script>

<template>
  <div class="side-card collapsible-card" :class="{ open }">
    <div class="cc-head" role="button" tabindex="0" @click="toggle" @keydown.enter="toggle">
      <span class="cc-title">{{ title }}</span>
      <span class="cc-extra">
        <slot name="extra" />
        <span class="cc-chevron">
          <UpOutlined v-if="open" />
          <DownOutlined v-else />
        </span>
      </span>
    </div>
    <div class="cc-body" :class="{ collapsed: !open }">
      <div class="cc-body-inner">
        <div class="cc-content">
          <slot />
        </div>
      </div>
    </div>
  </div>
</template>
