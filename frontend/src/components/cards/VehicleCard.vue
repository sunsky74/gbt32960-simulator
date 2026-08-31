<script setup lang="ts">
import CollapsibleCard from '../layout/CollapsibleCard.vue'
import { cfg } from '../../composables/useConnConfig'
</script>

<template>
  <CollapsibleCard title="车辆配置">
    <a-form layout="vertical" class="mini-form">
      <a-form-item label="VIN(车辆识别码)">
        <a-input v-model:value="cfg.vin" :maxlength="17" placeholder="17 位车架号" />
      </a-form-item>
      <a-form-item label="ICCID">
        <a-input v-model:value="cfg.iccid" :maxlength="20" placeholder="20 位 SIM 卡号" />
      </a-form-item>
    </a-form>
  </CollapsibleCard>
</template>
