<script setup lang="ts">
import CollapsibleCard from '../layout/CollapsibleCard.vue'
import { cfg } from '../../composables/useConnConfig'

// 2025 车端签名(表8):0=不带;1=SM2;2=RSA;3=ECC。R/S 由外部签名工具生成。
const signatureOptions = [
  { value: 0, label: '不带签名' },
  { value: 1, label: 'SM2 (0x01)' },
  { value: 2, label: 'RSA (0x02)' },
  { value: 3, label: 'ECC (0x03)' },
]
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
      <template v-if="cfg.version === '2025'">
        <a-form-item label="车端签名(表8)">
          <a-select v-model:value="cfg.signatureType" :options="signatureOptions" />
        </a-form-item>
        <template v-if="cfg.signatureType > 0">
          <a-form-item label="签名 R 值 (HEX)">
            <a-input v-model:value="cfg.signatureR" placeholder="外部签名工具生成的 R 值" />
          </a-form-item>
          <a-form-item label="签名 S 值 (HEX)">
            <a-input v-model:value="cfg.signatureS" placeholder="外部签名工具生成的 S 值" />
          </a-form-item>
        </template>
      </template>
    </a-form>
  </CollapsibleCard>
</template>
