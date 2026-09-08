import { createApp } from 'vue';
import { createPinia } from 'pinia';
import router from './router';
import App from './App.vue';
import './assets/main.css';

// Element Plus 暗色变量（按需引入模式下单独挂全量变量表，体积小）：
// html.dark + 此表 = 暗色主题的组件侧基座
import 'element-plus/theme-chalk/dark/css-vars.css';

const app = createApp(App);

app.use(createPinia());
app.use(router);

app.mount('#app');
