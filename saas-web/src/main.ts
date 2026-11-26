import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import './style.css'
import App from './App.vue'
import i18n from './i18n'
import router from './router'

createApp(App).use(createPinia()).use(router).use(i18n).use(ElementPlus).mount('#app')
