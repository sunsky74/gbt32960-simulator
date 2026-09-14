/// <reference types="vite/client" />

/* eslint-disable @typescript-eslint/no-explicit-any, @typescript-eslint/no-empty-object-type -- Vue SFC 模块垫片沿用 Vite 官方模板写法:{} 与 any 是"未指定 props/实例"的占位语义 */
declare module '*.vue' {
    import type {DefineComponent} from 'vue'
    const component: DefineComponent<{}, {}, any>
    export default component
}
