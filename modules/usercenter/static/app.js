/* ===== FunAuth 用户中心 SPA (Vue3 + Element Plus CDN) ===== */
(function () {
  const { createApp, ref, reactive, computed, onMounted, watch, nextTick, h } = Vue;
  const { ElMessage, ElMessageBox } = ElementPlus;

  /* ---------- 工具 ---------- */
  const API_PREFIX = '/api/usercenter';

  async function api(method, path, body, opts) {
    opts = opts || {};
    const token = localStorage.getItem('token') || '';
    const headers = { 'Content-Type': 'application/json' };
    if (token) headers['Authorization'] = 'Bearer ' + token;
    const fetchOpts = { method, headers };
    if (body !== undefined && body !== null) fetchOpts.body = JSON.stringify(body);
    let res;
    try {
      res = await fetch(API_PREFIX + path, fetchOpts);
    } catch (e) {
      ElMessage.error('网络请求失败：' + (e && e.message ? e.message : e));
      throw e;
    }
    if (res.status === 401) {
      localStorage.removeItem('token');
      store.user = null;
      if (location.hash !== '#/login') location.hash = '#/login';
      const j = await safeJson(res);
      ElMessage.error((j && j.msg) || '登录已失效，请重新登录');
      throw new Error('unauthorized');
    }
    let data = await safeJson(res);
    if (!res.ok) {
      const msg = (data && data.msg) || ('HTTP ' + res.status);
      ElMessage.error(msg);
      throw new Error(msg);
    }
    if (data && data.ok === false) {
      ElMessage.error(data.msg || '操作失败');
      throw new Error(data.msg || 'failed');
    }
    return data && data.data !== undefined ? data.data : data;
  }
  async function safeJson(res) {
    const ct = res.headers.get('content-type') || '';
    if (ct.includes('application/json')) {
      try { return await res.json(); } catch (e) { return null; }
    }
    return null;
  }

  function fmtMoney(fen) {
    const n = Number(fen || 0);
    return '\u00a5' + Math.floor(n / 100) + '.' + String(Math.abs(n % 100)).padStart(2, '0');
  }
  function fmtDate(s) {
    if (!s) return '\u2014';
    try { return new Date(s).toLocaleString('zh-CN'); } catch (e) { return String(s); }
  }
  function fmtDateOnly(s) {
    if (!s) return '\u2014';
    try { return new Date(s).toLocaleDateString('zh-CN'); } catch (e) { return String(s); }
  }
  function go(p) { if (location.hash !== '#' + p) location.hash = '#' + p; }
  function isExpiringSoon(endTime, days) {
    if (!endTime) return false;
    const t = new Date(endTime).getTime();
    if (!t) return false;
    const diff = t - Date.now();
    return diff < (days || 7) * 86400000;
  }
  function isExpired(endTime) {
    if (!endTime) return false;
    const t = new Date(endTime).getTime();
    if (!t) return false;
    return t < Date.now();
  }
  function md(text) {
    if (!text) return '';
    try {
      if (window.marked) return window.marked.parse(text);
      return text;
    } catch (e) { return text; }
  }
  function copyText(t) {
    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(t).then(() => ElMessage.success('已复制'), () => ElMessage.warning('复制失败'));
    } else {
      const ta = document.createElement('textarea');
      ta.value = t; document.body.appendChild(ta); ta.select();
      try { document.execCommand('copy'); ElMessage.success('已复制'); } catch (e) { ElMessage.warning('复制失败'); }
      document.body.removeChild(ta);
    }
  }

  /* ---------- 全局 store ---------- */
  const store = reactive({
    user: null,
    sidebarOpen: false,
    get isAdmin() {
      const u = this.user;
      if (!u || !u.groups) return false;
      for (const g of u.groups) {
        if (g && g.permissions && g.permissions.indexOf('*') >= 0) return true;
      }
      return false;
    },
    get loggedIn() { return !!localStorage.getItem('token'); }
  });

  /* ---------- 主题 ---------- */
  function applyTheme(name) {
    const html = document.documentElement;
    html.setAttribute('data-theme', name);
    if (name === 'dark') html.classList.add('dark'); else html.classList.remove('dark');
    localStorage.setItem('theme', name);
  }
  function toggleTheme() {
    const cur = localStorage.getItem('theme') || 'light';
    applyTheme(cur === 'dark' ? 'light' : 'dark');
  }
  function initTheme() {
    const t = localStorage.getItem('theme') || 'light';
    applyTheme(t);
  }

  /* ---------- 路由 ---------- */
  const route = reactive({ path: '/login', sub: '' });
  function parseHash() {
    let h = location.hash.replace(/^#/, '');
    if (!h) h = '/login';
    const parts = h.split('/').filter(Boolean);
    if (parts.length === 0) { route.path = '/login'; route.sub = ''; return; }
    if (parts[0] === 'admin') {
      route.path = '/admin';
      route.sub = parts[1] || 'users';
    } else {
      route.path = '/' + parts[0];
      route.sub = '';
    }
  }
  function requireAuth() {
    if (route.path !== '/login' && !localStorage.getItem('token')) {
      location.hash = '#/login';
      return false;
    }
    return true;
  }

  /* ============ 登录页 ============ */
  const LoginPage = {
    template: `
    <div class="login-wrap">
      <button class="theme-fab" @click="toggleTheme" :title="'切换主题'">
        {{ theme === 'dark' ? '\u2600\ufe0f' : '\ud83c\udf19' }}
      </button>
      <div class="login-card">
        <h2>FunAuth 用户中心</h2>
        <div class="sub">登录以管理你的卡槽、钱包与额度</div>
        <el-form :model="form" label-position="top" @submit.prevent="onLogin">
          <el-form-item label="用户名">
            <el-input v-model="form.username" placeholder="请输入用户名" autocomplete="username" clearable></el-input>
          </el-form-item>
          <el-form-item label="密码">
            <el-input v-model="form.password" type="password" placeholder="请输入密码" show-password @keyup.enter="onLogin"></el-input>
          </el-form-item>
          <el-button type="primary" :loading="loading" @click="onLogin" style="width:100%;margin-top:6px">登录</el-button>
          <div style="margin-top:12px;display:flex;justify-content:space-between;align-items:center">
            <span class="muted" style="font-size:12px">还没有账号？</span>
            <el-button text type="primary" @click="openRegister">立即注册 \u2192</el-button>
          </div>
        </el-form>
      </div>

      <el-dialog v-model="reg.visible" title="注册新账号" width="380px">
        <el-form :model="reg.form" label-position="top">
          <el-form-item label="用户名（3-50 字符）">
            <el-input v-model="reg.form.username" placeholder="用户名"></el-input>
          </el-form-item>
          <el-form-item label="密码（6-64 字符）">
            <el-input v-model="reg.form.password" type="password" show-password placeholder="密码"></el-input>
          </el-form-item>
        </el-form>
        <template #footer>
          <el-button @click="reg.visible=false">取消</el-button>
          <el-button type="primary" :loading="reg.loading" @click="onRegister">注册并登录</el-button>
        </template>
      </el-dialog>
    </div>`,
    setup() {
      const form = reactive({ username: '', password: '' });
      const reg = reactive({ visible: false, loading: false, form: { username: '', password: '' } });
      const loading = ref(false);
      const theme = computed(() => localStorage.getItem('theme') || 'light');

      async function onLogin() {
        if (!form.username || !form.password) { ElMessage.warning('请输入用户名和密码'); return; }
        loading.value = true;
        try {
          const data = await api('POST', '/login', { username: form.username, password: form.password });
          if (data && data.token) {
            localStorage.setItem('token', data.token);
            store.user = data.user;
            ElMessage.success('登录成功');
            location.hash = '#/dashboard';
          }
        } catch (e) { /* msg already shown */ }
        finally { loading.value = false; }
      }
      function openRegister() { reg.visible = true; reg.form = { username: '', password: '' }; }
      async function onRegister() {
        if (!reg.form.username || !reg.form.password) { ElMessage.warning('请填写用户名和密码'); return; }
        if (reg.form.password.length < 6) { ElMessage.warning('密码至少 6 位'); return; }
        reg.loading = true;
        try {
          const data = await api('POST', '/register', { username: reg.form.username, password: reg.form.password });
          if (data && data.token) {
            localStorage.setItem('token', data.token);
            store.user = data.user;
            reg.visible = false;
            ElMessage.success('注册成功，已自动登录');
            location.hash = '#/dashboard';
          }
        } catch (e) {}
        finally { reg.loading = false; }
      }
      return { form, reg, loading, theme, onLogin, openRegister, onRegister, toggleTheme };
    }
  };

  /* ============ 仪表盘 ============ */
  const DashboardPage = {
    template: `
    <div v-loading="loading">
      <h1 class="page-title">仪表盘</h1>
      <p class="page-desc">查看你的账户概览与最近动态</p>

      <div class="card">
        <div class="user-card">
          <el-avatar :size="56" :src="user && user.avatar_url">{{ avatarText }}</el-avatar>
          <div class="info">
            <div class="name">{{ user && (user.nickname || user.username) }}</div>
            <div class="uuid">@{{ user && user.username }} \u00b7 <span class="mono">{{ user && user.uuid }}</span></div>
            <div class="tags">
              <el-tag v-for="g in (user && user.groups) || []" :key="g.id" size="small" effect="plain">{{ g.name }}</el-tag>
              <span v-if="!user || !user.groups || !user.groups.length" class="muted" style="font-size:12px">无身份组</span>
            </div>
            <div class="expire">
              到期时间：
              <span v-if="!user || !user.end_time" class="muted">永久有效</span>
              <span v-else-if="expired" class="tag-warn">{{ fmtDate(user.end_time) }}（已过期）</span>
              <span v-else-if="expiringSoon" class="tag-warn">{{ fmtDate(user.end_time) }}（即将到期）</span>
              <span v-else>{{ fmtDate(user.end_time) }}</span>
            </div>
          </div>
          <div>
            <el-button type="danger" plain @click="onLogout">退出登录</el-button>
          </div>
        </div>
      </div>

      <div class="stat-grid">
        <div class="stat-card primary">
          <div class="label">\ud83d\udcb0 钱包余额</div>
          <div class="value">{{ fmtMoney(profile.balance) }}</div>
          <div class="sub">可消费余额</div>
        </div>
        <div class="stat-card">
          <div class="label">\ud83c\udfaf 卡槽数</div>
          <div class="value">{{ profile.bound_slots }} / {{ profile.total_slots }}</div>
          <div class="sub">已绑 / 总数</div>
        </div>
        <div class="stat-card">
          <div class="label">\u26a1 额度</div>
          <div class="value">{{ profile.quota }}</div>
          <div class="sub">剩余额度</div>
        </div>
        <div class="stat-card">
          <div class="label">\u231b 次数</div>
          <div class="value">{{ profile.times }}</div>
          <div class="sub">剩余次数</div>
        </div>
      </div>

      <div class="card">
        <div class="card-title">
          <span>最近钱包流水</span>
          <div class="actions">
            <el-button size="small" @click="go('/records')">查看全部</el-button>
          </div>
        </div>
        <el-table :data="recentTxs" size="small" empty-text="暂无流水">
          <el-table-column prop="amount" label="金额" width="110">
            <template #default="{ row }"><span :class="row.amount >= 0 ? '' : 'danger'">{{ fmtMoney(row.amount) }}</span></template>
          </el-table-column>
          <el-table-column prop="type" label="类型" width="140"></el-table-column>
          <el-table-column prop="remark" label="备注" show-overflow-tooltip></el-table-column>
          <el-table-column label="时间" width="180">
            <template #default="{ row }">{{ fmtDate(row.created_at) }}</template>
          </el-table-column>
        </el-table>
      </div>

      <div class="card">
        <div class="card-title"><span>快捷操作</span></div>
        <div class="row wrap">
          <el-button type="primary" @click="go('/shop')">\ud83d\udecd\ufe0f 前往商店</el-button>
          <el-button @click="go('/redeem')">\ud83c\udfa3 使用兑换码</el-button>
          <el-button @click="go('/slots')">\ud83c\udfaf 我的卡槽</el-button>
          <el-button @click="go('/records')">\ud83d\udccd 流水记录</el-button>
          <el-button @click="go('/announcements')">\ud83d\udce3 查看公告</el-button>
        </div>
      </div>
    </div>`,
    setup() {
      const loading = ref(true);
      const profile = reactive({ balance: 0, total_slots: 0, bound_slots: 0, quota: 0, times: 0 });
      const recentTxs = ref([]);
      const user = computed(() => store.user);
      const avatarText = computed(() => {
        const u = store.user;
        if (!u) return '?';
        const s = u.nickname || u.username || '';
        return s ? s.charAt(0).toUpperCase() : '?';
      });
      const expired = computed(() => store.user && isExpired(store.user.end_time));
      const expiringSoon = computed(() => store.user && !expired.value && isExpiringSoon(store.user.end_time, 7));

      async function load() {
        loading.value = true;
        try {
          const [p, w] = await Promise.all([api('GET', '/profile'), api('GET', '/profile/wallet')]);
          Object.assign(profile, {
            balance: p.balance || 0,
            total_slots: p.total_slots || 0,
            bound_slots: p.bound_slots || 0,
            quota: p.quota || 0,
            times: p.times || 0,
          });
          if (p.user) store.user = p.user;
          const arr = (w && w.transactions_last_10) || [];
          recentTxs.value = arr.slice(0, 5);
        } catch (e) {}
        finally { loading.value = false; }
      }
      async function onLogout() {
        try {
          await ElMessageBox.confirm('确定退出登录吗？', '提示', { type: 'warning' });
        } catch (e) { return; }
        try { await api('POST', '/logout', {}); } catch (e) {}
        localStorage.removeItem('token');
        store.user = null;
        ElMessage.success('已退出登录');
        location.hash = '#/login';
      }
      onMounted(load);
      return { loading, profile, recentTxs, user, avatarText, expired, expiringSoon, fmtMoney, fmtDate, go, onLogout };
    }
  };

  /* ============ 个人资料 ============ */
  const ProfilePage = {
    template: `
    <div v-loading="loading">
      <h1 class="page-title">个人资料</h1>
      <p class="page-desc">编辑你的昵称、简介与头像</p>
      <div class="card">
        <div class="user-card" style="margin-bottom:18px">
          <el-avatar :size="72" :src="form.avatar_url">{{ avatarText }}</el-avatar>
          <div class="info">
            <div class="name">@{{ user && user.username }}</div>
            <div class="uuid mono">{{ user && user.uuid }}</div>
            <div class="tags">
              <el-tag v-for="g in (user && user.groups) || []" :key="g.id" size="small" effect="plain">{{ g.name }}</el-tag>
            </div>
          </div>
        </div>
        <el-form :model="form" label-position="top">
          <el-row :gutter="16">
            <el-col :span="12">
              <el-form-item label="昵称"><el-input v-model="form.nickname" maxlength="64" show-word-limit></el-input></el-form-item>
            </el-col>
            <el-col :span="12">
              <el-form-item label="头像 URL"><el-input v-model="form.avatar_url" placeholder="https://..."></el-input></el-form-item>
            </el-col>
          </el-row>
          <el-form-item label="简介"><el-input v-model="form.bio" type="textarea" :rows="3" maxlength="1024" show-word-limit></el-input></el-form-item>
          <el-button type="primary" :loading="saving" @click="save">保存修改</el-button>
          <el-button @click="openPwd">修改密码</el-button>
        </el-form>
      </div>

      <div class="card">
        <div class="card-title"><span>账户信息</span></div>
        <div class="kv"><span class="k">剩余额度</span><span class="v">{{ user && user.quota }}</span></div>
        <div class="kv"><span class="k">剩余次数</span><span class="v">{{ user && user.times }}</span></div>
        <div class="kv"><span class="k">到期时间</span><span class="v">{{ user && user.end_time ? fmtDate(user.end_time) : '永久' }}</span></div>
        <div class="kv"><span class="k">注册时间</span><span class="v">{{ user && user.created_at ? fmtDate(user.created_at) : '—' }}</span></div>
      </div>

      <el-dialog v-model="pwd.visible" title="修改密码" width="420px">
        <el-form :model="pwd.form" label-position="top">
          <el-form-item label="旧密码"><el-input v-model="pwd.form.old_password" type="password" show-password></el-input></el-form-item>
          <el-form-item label="新密码（6-64 位）"><el-input v-model="pwd.form.new_password" type="password" show-password></el-input></el-form-item>
          <el-form-item label="确认新密码"><el-input v-model="pwd.form.confirm" type="password" show-password></el-input></el-form-item>
        </el-form>
        <template #footer>
          <el-button @click="pwd.visible=false">取消</el-button>
          <el-button type="primary" :loading="pwd.loading" @click="savePwd">确认修改</el-button>
        </template>
      </el-dialog>
    </div>`,
    setup() {
      const loading = ref(true);
      const saving = ref(false);
      const form = reactive({ nickname: '', avatar_url: '', bio: '' });
      const user = computed(() => store.user);
      const avatarText = computed(() => {
        const u = store.user; if (!u) return '?';
        const s = u.nickname || u.username || ''; return s ? s.charAt(0).toUpperCase() : '?';
      });
      const pwd = reactive({ visible: false, loading: false, form: { old_password: '', new_password: '', confirm: '' } });

      async function load() {
        loading.value = true;
        try {
          const p = await api('GET', '/profile');
          if (p.user) {
            store.user = p.user;
            form.nickname = p.user.nickname || '';
            form.avatar_url = p.user.avatar_url || '';
            form.bio = p.user.bio || '';
          }
        } catch (e) {}
        finally { loading.value = false; }
      }
      async function save() {
        saving.value = true;
        try {
          const data = await api('PUT', '/profile', { nickname: form.nickname, avatar_url: form.avatar_url, bio: form.bio });
          if (data && data.user) store.user = data.user;
          ElMessage.success('已保存');
        } catch (e) {}
        finally { saving.value = false; }
      }
      function openPwd() { pwd.visible = true; pwd.form = { old_password: '', new_password: '', confirm: '' }; }
      async function savePwd() {
        const f = pwd.form;
        if (!f.old_password || !f.new_password) { ElMessage.warning('请填写完整'); return; }
        if (f.new_password.length < 6 || f.new_password.length > 64) { ElMessage.warning('新密码需 6-64 位'); return; }
        if (f.new_password !== f.confirm) { ElMessage.warning('两次密码不一致'); return; }
        pwd.loading = true;
        try {
          await api('POST', '/profile/password', { old_password: f.old_password, new_password: f.new_password });
          ElMessage.success('密码已更新');
          pwd.visible = false;
        } catch (e) {}
        finally { pwd.loading = false; }
      }
      onMounted(load);
      return { loading, saving, form, user, avatarText, pwd, save, openPwd, savePwd, fmtDate };
    }
  };

  /* ============ 我的卡槽 ============ */
  const SlotsPage = {
    template: `
    <div v-loading="loading">
      <div class="row" style="justify-content:space-between">
        <div>
          <h1 class="page-title">我的卡槽</h1>
          <p class="page-desc">共 {{ total }} 个卡槽，已绑 {{ boundCount }} 个</p>
        </div>
        <el-button @click="load">刷新</el-button>
      </div>
      <div class="card">
        <el-table :data="list" empty-text="暂无卡槽">
          <el-table-column label="ID" width="320">
            <template #default="{ row }"><span class="mono">{{ row.id }}</span></template>
          </el-table-column>
          <el-table-column label="服务器号" width="160">
            <template #default="{ row }">
              <el-tag v-if="row.server_code" type="success" size="small">{{ row.server_code }}</el-tag>
              <span v-else class="muted">未绑定</span>
            </template>
          </el-table-column>
          <el-table-column label="到期时间" width="200">
            <template #default="{ row }">
              <span v-if="!row.end_time" class="muted">永久</span>
              <span v-else>{{ fmtDate(row.end_time) }}</span>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="220">
            <template #default="{ row }">
              <el-button size="small" type="primary" plain v-if="!row.server_code" @click="openBind(row)">绑定</el-button>
              <el-button size="small" type="warning" plain v-else @click="openUnbind(row)">解绑</el-button>
            </template>
          </el-table-column>
        </el-table>
      </div>

      <el-dialog v-model="bindDialog.visible" title="绑定服务器" width="420px">
        <p class="muted" style="margin-top:0">卡槽：<span class="mono">{{ bindDialog.row && bindDialog.row.id }}</span></p>
        <el-form label-position="top">
          <el-form-item label="服务器号"><el-input v-model="bindDialog.server_code" placeholder="如 52258662"></el-input></el-form-item>
        </el-form>
        <template #footer>
          <el-button @click="bindDialog.visible=false">取消</el-button>
          <el-button type="primary" :loading="bindDialog.loading" @click="doBind">确认绑定</el-button>
        </template>
      </el-dialog>
    </div>`,
    setup() {
      const loading = ref(true);
      const list = ref([]);
      const total = ref(0);
      const bindDialog = reactive({ visible: false, loading: false, row: null, server_code: '' });
      const boundCount = computed(() => list.value.filter(x => x.server_code).length);

      async function load() {
        loading.value = true;
        try {
          const data = await api('GET', '/slots/mine');
          list.value = (data && data.list) || [];
          total.value = (data && data.total) || list.value.length;
        } catch (e) {}
        finally { loading.value = false; }
      }
      function openBind(row) { bindDialog.row = row; bindDialog.server_code = row.server_code || ''; bindDialog.visible = true; }
      async function doBind() {
        if (!bindDialog.server_code) { ElMessage.warning('请输入服务器号'); return; }
        bindDialog.loading = true;
        try {
          const data = await api('POST', '/slots/' + encodeURIComponent(bindDialog.row.id) + '/bind', { server_code: bindDialog.server_code });
          ElMessage.success('绑定成功');
          bindDialog.visible = false;
          await load();
        } catch (e) {}
        finally { bindDialog.loading = false; }
      }
      async function openUnbind(row) {
        try {
          await ElMessageBox.confirm('确定要解绑该卡槽吗？服务器号：' + row.server_code, '二次确认', { type: 'warning' });
        } catch (e) { return; }
        try {
          await api('POST', '/slots/' + encodeURIComponent(row.id) + '/unbind', {});
          ElMessage.success('已解绑');
          await load();
        } catch (e) {}
      }
      onMounted(load);
      return { loading, list, total, boundCount, bindDialog, load, openBind, doBind, openUnbind, fmtDate };
    }
  };

  /* ============ 商店 ============ */
  const ShopPage = {
    template: `
    <div>
      <div class="row" style="justify-content:space-between;margin-bottom:14px">
        <div>
          <h1 class="page-title">商店</h1>
          <p class="page-desc">购买额度、次数、卡槽等</p>
        </div>
      </div>
      <el-tabs v-model="activeTab">
        <el-tab-pane label="商品浏览" name="shop">
          <div class="shop-layout" v-loading="loading">
            <div class="shop-cats">
              <div class="cat-item" :class="{active: !categoryId}" @click="categoryId=''">全部</div>
              <div v-for="c in cats" :key="c.id" class="cat-item" :class="{active: categoryId == c.id}" @click="categoryId=c.id">{{ c.name }}</div>
            </div>
            <div class="shop-grid">
              <div v-for="p in products" :key="p.id" class="product-card">
                <div class="img">
                  <img v-if="p.image_url" :src="p.image_url" alt="">
                  <span v-else>\ud83d\udcdd</span>
                </div>
                <div class="body">
                  <div class="name">{{ p.name }}</div>
                  <div class="desc">{{ p.description || '\u6682\u65e0\u63cf\u8ff0' }}</div>
                  <div class="price">{{ fmtMoney(p.price) }}</div>
                  <div class="meta">
                    <span>库存：{{ p.stock < 0 ? '\u5145\u8db3' : p.stock }}</span>
                    <span>{{ p.category }}</span>
                  </div>
                  <div class="actions">
                    <el-button size="small" type="primary" @click="openBuy(p)">购买</el-button>
                  </div>
                </div>
              </div>
              <div v-if="!loading && !products.length" class="empty-tip">该分类下暂无商品</div>
            </div>
          </div>
        </el-tab-pane>
        <el-tab-pane label="我的订单" name="orders">
          <div class="card" v-loading="ordersLoading">
            <el-table :data="orders" empty-text="暂无订单">
              <el-table-column prop="order_no" label="订单号" min-width="200" show-overflow-tooltip></el-table-column>
              <el-table-column label="商品" min-width="140"><template #default="{ row }">{{ row.product_name || row.product_id }}</template></el-table-column>
              <el-table-column prop="quantity" label="数量" width="80"></el-table-column>
              <el-table-column label="实付" width="100"><template #default="{ row }">{{ fmtMoney(row.final_price) }}</template></el-table-column>
              <el-table-column label="状态" width="100"><template #default="{ row }"><el-tag :type="statusType(row.status)" size="small">{{ row.status }}</el-tag></template></el-table-column>
              <el-table-column label="时间" width="180"><template #default="{ row }">{{ fmtDate(row.created_at) }}</template></el-table-column>
            </el-table>
            <div class="pager">
              <el-pagination background layout="prev, pager, next, total" :total="ordersTotal" :current-page="ordersPage" :page-size="10" @current-change="onOrdersPage"></el-pagination>
            </div>
          </div>
        </el-tab-pane>
      </el-tabs>

      <el-dialog v-model="buyDialog.visible" :title="'购买：' + (buyDialog.product && buyDialog.product.name || '')" width="400px">
        <div v-if="buyDialog.product" class="row" style="margin-bottom:12px">
          <span class="muted">单价：</span><b>{{ fmtMoney(buyDialog.product.price) }}</b>
        </div>
        <el-form label-position="top">
          <el-form-item label="数量">
            <el-input-number v-model="buyDialog.quantity" :min="1" :max="999"></el-input-number>
          </el-form-item>
          <div class="muted" style="font-size:13px">合计：<b style="color:var(--primary)">{{ fmtMoney((buyDialog.product ? buyDialog.product.price : 0) * buyDialog.quantity) }}</b></div>
        </el-form>
        <template #footer>
          <el-button @click="buyDialog.visible=false">取消</el-button>
          <el-button type="primary" :loading="buyDialog.loading" @click="doBuy">确认购买</el-button>
        </template>
      </el-dialog>
    </div>`,
    setup() {
      const activeTab = ref('shop');
      const loading = ref(true);
      const cats = ref([]);
      const categoryId = ref('');
      const products = ref([]);

      const ordersLoading = ref(false);
      const orders = ref([]);
      const ordersTotal = ref(0);
      const ordersPage = ref(1);

      const buyDialog = reactive({ visible: false, loading: false, product: null, quantity: 1 });

      async function loadCats() {
        try { cats.value = (await api('GET', '/shop/categories')) || []; } catch (e) {}
      }
      async function loadProducts() {
        loading.value = true;
        try {
          const q = categoryId.value ? ('?category_id=' + encodeURIComponent(categoryId.value) + '&size=100') : '?size=100';
          const data = await api('GET', '/shop/products' + q);
          products.value = (data && data.list) || [];
        } catch (e) {}
        finally { loading.value = false; }
      }
      async function loadOrders() {
        ordersLoading.value = true;
        try {
          const data = await api('GET', '/shop/orders/mine?page=' + ordersPage.value + '&size=10');
          orders.value = (data && data.list) || [];
          ordersTotal.value = (data && data.total) || 0;
        } catch (e) {}
        finally { ordersLoading.value = false; }
      }
      function onOrdersPage(p) { ordersPage.value = p; loadOrders(); }
      function openBuy(p) { buyDialog.product = p; buyDialog.quantity = 1; buyDialog.visible = true; }
      async function doBuy() {
        const p = buyDialog.product; if (!p) return;
        buyDialog.loading = true;
        try {
          const data = await api('POST', '/shop/orders', { product_id: p.id, quantity: buyDialog.quantity });
          ElMessage.success('购买成功：' + (data && data.order_no || ''));
          buyDialog.visible = false;
          // 刷新钱包展示
          if (activeTab.value === 'orders') loadOrders();
        } catch (e) {}
        finally { buyDialog.loading = false; }
      }
      function statusType(s) {
        if (s === 'paid') return 'success';
        if (s === 'cancelled') return 'info';
        if (s === 'refunded') return 'warning';
        return 'primary';
      }
      watch(categoryId, loadProducts);
      onMounted(async () => { await Promise.all([loadCats(), loadProducts(), loadOrders()]); });
      return { activeTab, loading, cats, categoryId, products, ordersLoading, orders, ordersTotal, ordersPage, buyDialog, loadProducts, onOrdersPage, openBuy, doBuy, statusType, fmtMoney, fmtDate };
    }
  };

  /* ============ 兑换码 ============ */
  const RedeemPage = {
    template: `
    <div>
      <h1 class="page-title">兑换码</h1>
      <p class="page-desc">输入兑换码以获取余额、额度、次数或卡槽</p>
      <div class="redeem-hero">
        <div class="icon">\ud83c\udfa3</div>
        <h2>使用兑换码</h2>
        <div class="sub">兑换码由管理员发放，每个码仅可使用一次</div>
        <div class="redeem-input">
          <el-input v-model="code" placeholder="输入兑换码（如 ABCD-1234-EFGH-5678）" size="large" @keyup.enter="doRedeem" clearable></el-input>
          <el-button type="primary" size="large" :loading="loading" @click="doRedeem">使用</el-button>
        </div>
      </div>
      <div class="card" v-if="lastGranted">
        <div class="card-title"><span>上次到账明细</span></div>
        <div class="kv" v-if="lastGranted.balance_added != null"><span class="k">余额到账</span><span class="v">{{ fmtMoney(lastGranted.balance_added) }}</span></div>
        <div class="kv" v-if="lastGranted.quota_added != null"><span class="k">额度到账</span><span class="v">{{ lastGranted.quota_added }}</span></div>
        <div class="kv" v-if="lastGranted.times_added != null"><span class="k">次数到账</span><span class="v">{{ lastGranted.times_added }}</span></div>
        <div class="kv" v-if="lastGranted.slots_added != null"><span class="k">卡槽到账</span><span class="v">{{ lastGranted.slots_added }} 个</span></div>
      </div>
    </div>`,
    setup() {
      const code = ref('');
      const loading = ref(false);
      const lastGranted = ref(null);
      async function doRedeem() {
        if (!code.value.trim()) { ElMessage.warning('请输入兑换码'); return; }
        loading.value = true;
        try {
          const data = await api('POST', '/redeem', { code: code.value.trim() });
          const granted = (data && data.granted) || {};
          lastGranted.value = granted;
          const parts = [];
          if (granted.balance_added != null) parts.push('余额 +' + fmtMoney(granted.balance_added));
          if (granted.quota_added != null) parts.push('额度 +' + granted.quota_added);
          if (granted.times_added != null) parts.push('次数 +' + granted.times_added);
          if (granted.slots_added != null) parts.push('卡槽 +' + granted.slots_added);
          ElMessage.success('兑换成功' + (parts.length ? '：' + parts.join('，') : ''));
          code.value = '';
        } catch (e) {}
        finally { loading.value = false; }
      }
      return { code, loading, lastGranted, doRedeem, fmtMoney };
    }
  };

  /* ============ 流水记录 ============ */
  const RecordsPage = {
    template: `
    <div>
      <h1 class="page-title">流水记录</h1>
      <p class="page-desc">查看钱包、额度、次数与订单历史</p>
      <div class="card">
        <el-tabs v-model="activeTab" @tab-change="onTabChange">
          <el-tab-pane label="钱包" name="wallet"></el-tab-pane>
          <el-tab-pane label="额度" name="quota"></el-tab-pane>
          <el-tab-pane label="次数" name="times"></el-tab-pane>
          <el-tab-pane label="订单" name="orders"></el-tab-pane>
        </el-tabs>
        <div class="toolbar" v-if="activeTab !== 'orders'">
          <el-date-picker v-model="dateRange" type="daterange" value-format="YYYY-MM-DD" range-separator="\u81f3" start-placeholder="开始日期" end-placeholder="结束日期" style="width:280px"></el-date-picker>
          <el-button @click="load(1)">查询</el-button>
          <el-button @click="resetDate">重置</el-button>
          <span class="spacer"></span>
        </div>
        <el-table :data="list" v-loading="loading" empty-text="暂无数据" size="small">
          <el-table-column prop="id" label="ID" width="80"></el-table-column>
          <el-table-column label="金额/数量" width="120">
            <template #default="{ row }"><span :class="row.amount >= 0 ? '' : 'danger'">{{ activeTab==='wallet' ? fmtMoney(row.amount) : row.amount }}</span></template>
          </el-table-column>
          <el-table-column prop="type" label="类型" width="140"></el-table-column>
          <el-table-column prop="remark" label="备注" show-overflow-tooltip></el-table-column>
          <el-table-column label="时间" width="180"><template #default="{ row }">{{ fmtDate(row.created_at) }}</template></el-table-column>
        </el-table>
        <div v-if="activeTab === 'orders'">
          <el-table :data="ordersList" v-loading="ordersLoading" empty-text="暂无订单" size="small">
            <el-table-column prop="order_no" label="订单号" min-width="200" show-overflow-tooltip></el-table-column>
            <el-table-column label="商品" min-width="120"><template #default="{ row }">{{ row.product_name || row.product_id }}</template></el-table-column>
            <el-table-column prop="quantity" label="数量" width="80"></el-table-column>
            <el-table-column label="实付" width="100"><template #default="{ row }">{{ fmtMoney(row.final_price) }}</template></el-table-column>
            <el-table-column label="状态" width="100"><template #default="{ row }"><el-tag :type="statusType(row.status)" size="small">{{ row.status }}</el-tag></template></el-table-column>
            <el-table-column label="时间" width="180"><template #default="{ row }">{{ fmtDate(row.created_at) }}</template></el-table-column>
          </el-table>
        </div>
        <div class="pager" v-if="activeTab !== 'orders'">
          <el-pagination background layout="prev, pager, next, total" :total="total" :current-page="page" :page-size="pageSize" @current-change="onPage"></el-pagination>
        </div>
        <div class="pager" v-else>
          <el-pagination background layout="prev, pager, next, total" :total="ordersTotal" :current-page="ordersPage" :page-size="ordersPageSize" @current-change="onOrdersPage"></el-pagination>
        </div>
      </div>
    </div>`,
    setup() {
      const activeTab = ref('wallet');
      const loading = ref(false);
      const list = ref([]);
      const total = ref(0);
      const page = ref(1);
      const pageSize = ref(10);
      const dateRange = ref(null);

      const ordersLoading = ref(false);
      const ordersList = ref([]);
      const ordersTotal = ref(0);
      const ordersPage = ref(1);
      const ordersPageSize = ref(10);

      async function load(p) {
        if (p) page.value = p;
        loading.value = true;
        try {
          const params = { type: activeTab.value, page: page.value, size: pageSize.value };
          if (dateRange.value && dateRange.value.length === 2) {
            params.start_date = dateRange.value[0];
            params.end_date = dateRange.value[1];
          }
          const qs = '?' + new URLSearchParams(params).toString();
          const data = await api('GET', '/transactions' + qs);
          list.value = (data && data.list) || [];
          total.value = (data && data.total) || 0;
        } catch (e) {}
        finally { loading.value = false; }
      }
      async function loadOrders() {
        ordersLoading.value = true;
        try {
          const data = await api('GET', '/shop/orders/mine?page=' + ordersPage.value + '&size=' + ordersPageSize.value);
          ordersList.value = (data && data.list) || [];
          ordersTotal.value = (data && data.total) || 0;
        } catch (e) {}
        finally { ordersLoading.value = false; }
      }
      function onPage(p) { load(p); }
      function onOrdersPage(p) { ordersPage.value = p; loadOrders(); }
      function onTabChange(name) {
        if (name === 'orders') loadOrders();
        else { page.value = 1; load(1); }
      }
      function resetDate() { dateRange.value = null; load(1); }
      function statusType(s) {
        if (s === 'paid') return 'success';
        if (s === 'cancelled') return 'info';
        if (s === 'refunded') return 'warning';
        return 'primary';
      }
      onMounted(() => load(1));
      return { activeTab, loading, list, total, page, pageSize, dateRange, ordersLoading, ordersList, ordersTotal, ordersPage, ordersPageSize, load, onPage, onOrdersPage, onTabChange, resetDate, statusType, fmtMoney, fmtDate };
    }
  };

  /* ============ 公告 ============ */
  const AnnouncementsPage = {
    template: `
    <div v-loading="loading">
      <h1 class="page-title">公告</h1>
      <p class="page-desc">站点重要通知与更新</p>
      <div v-if="!loading && !list.length" class="empty-tip">暂无公告</div>
      <div v-for="a in list" :key="a.id" class="ann-card" @click="toggle(a.id)">
        <div class="head">
          <div class="title">{{ a.title }}</div>
          <div class="meta">{{ a.author }} \u00b7 {{ fmtDate(a.date) }}</div>
        </div>
        <div class="body" v-if="opened === a.id" v-html="md(a.content)"></div>
      </div>
      <div class="pager" v-if="total > size">
        <el-pagination background layout="prev, pager, next, total" :total="total" :current-page="page" :page-size="size" @current-change="onPage"></el-pagination>
      </div>
    </div>`,
    setup() {
      const loading = ref(false);
      const list = ref([]);
      const total = ref(0);
      const page = ref(1);
      const size = ref(10);
      const opened = ref(null);
      async function load() {
        loading.value = true;
        try {
          const data = await api('GET', '/announcements?page=' + page.value + '&size=' + size.value);
          list.value = (data && data.list) || [];
          total.value = (data && data.total) || 0;
        } catch (e) {}
        finally { loading.value = false; }
      }
      function onPage(p) { page.value = p; load(); }
      function toggle(id) { opened.value = opened.value === id ? null : id; }
      onMounted(load);
      return { loading, list, total, page, size, opened, load, onPage, toggle, fmtDate, md };
    }
  };

  /* ============ 管理后台 - 用户管理 ============ */
  const AdminUsersPage = {
    template: `
    <div v-loading="loading">
      <div class="toolbar">
        <el-input v-model="filters.keyword" placeholder="用户名/昵称/UUID" clearable style="width:220px" @keyup.enter="search"></el-input>
        <el-select v-model="filters.banned" placeholder="状态" clearable style="width:120px">
          <el-option label="正常" value="0"></el-option>
          <el-option label="封禁" value="1"></el-option>
        </el-select>
        <el-button @click="search">搜索</el-button>
      </div>
      <el-table :data="list" empty-text="暂无用户" size="small">
        <el-table-column prop="username" label="用户名" width="140"></el-table-column>
        <el-table-column label="昵称" width="140"><template #default="{ row }">{{ row.nickname || '\u2014' }}</template></el-table-column>
        <el-table-column label="UUID" min-width="180"><template #default="{ row }"><span class="mono">{{ row.uuid }}</span></template></el-table-column>
        <el-table-column label="余额" width="100"><template #default="{ row }">{{ fmtMoney(row.balance) }}</template></el-table-column>
        <el-table-column label="额度" width="80"><template #default="{ row }">{{ row.quota }}</template></el-table-column>
        <el-table-column label="次数" width="80"><template #default="{ row }">{{ row.times }}</template></el-table-column>
        <el-table-column label="身份组" min-width="160"><template #default="{ row }"><el-tag v-for="g in row.groups||[]" :key="g.id" size="small" style="margin:2px">{{ g.name }}</el-tag></template></el-table-column>
        <el-table-column label="状态" width="80"><template #default="{ row }"><el-tag :type="row.is_banned ? 'danger' : 'success'" size="small">{{ row.is_banned ? '封禁' : '正常' }}</el-tag></template></el-table-column>
        <el-table-column label="到期" width="160"><template #default="{ row }">{{ row.end_time ? fmtDate(row.end_time) : '永久' }}</template></el-table-column>
        <el-table-column label="操作" width="320" fixed="right">
          <template #default="{ row }">
            <el-button size="small" @click="openEdit(row)">编辑</el-button>
            <el-button size="small" type="primary" @click="openAdjust(row,'balance')">余额</el-button>
            <el-button size="small" type="success" @click="openAdjust(row,'quota')">额度</el-button>
            <el-button size="small" type="warning" @click="openAdjust(row,'times')">次数</el-button>
            <el-button size="small" @click="openGroups(row)">身份组</el-button>
          </template>
        </el-table-column>
      </el-table>
      <div class="pager"><el-pagination background layout="prev, pager, next, total" :total="total" :current-page="page" :page-size="size" @current-change="onPage"></el-pagination></div>

      <el-dialog v-model="edit.visible" title="编辑用户" width="540px">
        <el-form :model="edit.form" label-position="top">
          <el-row :gutter="12">
            <el-col :span="12"><el-form-item label="昵称"><el-input v-model="edit.form.nickname"></el-input></el-form-item></el-col>
            <el-col :span="12"><el-form-item label="昵称前缀"><el-input v-model="edit.form.nickname_prefix"></el-input></el-form-item></el-col>
            <el-col :span="12"><el-form-item label="头像URL"><el-input v-model="edit.form.avatar_url"></el-input></el-form-item></el-col>
            <el-col :span="12"><el-form-item label="到期时间"><el-date-picker v-model="edit.form.end_time" type="datetime" value-format="YYYY-MM-DDTHH:mm:ss" style="width:100%"></el-date-picker></el-form-item></el-col>
            <el-col :span="24"><el-form-item label="简介"><el-input v-model="edit.form.bio" type="textarea" :rows="2"></el-input></el-form-item></el-col>
            <el-col :span="12"><el-form-item label="封禁"><el-switch v-model="edit.form.is_banned"></el-switch></el-form-item></el-col>
          </el-row>
        </el-form>
        <template #footer>
          <el-button @click="edit.visible=false">取消</el-button>
          <el-button type="primary" :loading="edit.loading" @click="saveEdit">保存</el-button>
        </template>
      </el-dialog>

      <el-dialog v-model="adjust.visible" :title="'调整' + (adjust.field===''?'':'') + adjustLabel" width="440px">
        <div class="muted" style="margin-bottom:8px">用户：{{ adjust.row && adjust.row.username }} \u00b7 当前 {{ currentAdjustValue }}</div>
        <el-form :model="adjust.form" label-position="top">
          <el-form-item label="操作类型">
            <el-radio-group v-model="adjust.form.type">
              <el-radio label="admin_adjust">增减（±）</el-radio>
              <el-radio label="admin_set">直接设置</el-radio>
            </el-radio-group>
          </el-form-item>
          <el-form-item label="数值">
            <el-input-number v-model="adjust.form.amount" :step="100" style="width:100%"></el-input-number>
            <div class="muted" style="font-size:12px;margin-top:4px">{{ adjust.field === 'balance' ? '单位：分' : '正负均可' }}</div>
          </el-form-item>
          <el-form-item label="备注"><el-input v-model="adjust.form.remark" placeholder="可选"></el-input></el-form-item>
        </el-form>
        <template #footer>
          <el-button @click="adjust.visible=false">取消</el-button>
          <el-button type="primary" :loading="adjust.loading" @click="saveAdjust">确认</el-button>
        </template>
      </el-dialog>

      <el-dialog v-model="groupsDlg.visible" title="设置身份组" width="440px">
        <div class="muted" style="margin-bottom:8px">用户：{{ groupsDlg.row && groupsDlg.row.username }}</div>
        <el-form label-position="top">
          <el-form-item label="身份组">
            <el-select v-model="groupsDlg.group_ids" multiple filterable style="width:100%">
              <el-option v-for="g in allGroups" :key="g.id" :label="g.name" :value="g.id"></el-option>
            </el-select>
          </el-form-item>
        </el-form>
        <template #footer>
          <el-button @click="groupsDlg.visible=false">取消</el-button>
          <el-button type="primary" :loading="groupsDlg.loading" @click="saveGroups">保存</el-button>
        </template>
      </el-dialog>
    </div>`,
    setup() {
      const loading = ref(false);
      const list = ref([]);
      const total = ref(0);
      const page = ref(1);
      const size = ref(10);
      const filters = reactive({ keyword: '', banned: '' });
      const allGroups = ref([]);

      const edit = reactive({ visible: false, loading: false, row: null, form: { nickname: '', nickname_prefix: '', avatar_url: '', end_time: null, bio: '', is_banned: false } });
      const adjust = reactive({ visible: false, loading: false, row: null, field: 'balance', form: { amount: 0, type: 'admin_adjust', remark: '' } });
      const groupsDlg = reactive({ visible: false, loading: false, row: null, group_ids: [] });

      const adjustLabel = computed(() => ({ balance: '余额', quota: '额度', times: '次数' }[adjust.field] || ''));
      const currentAdjustValue = computed(() => {
        if (!adjust.row) return '\u2014';
        if (adjust.field === 'balance') return fmtMoney(adjust.row.balance);
        return adjust.row[adjust.field];
      });

      async function load() {
        loading.value = true;
        try {
          const params = { page: page.value, size: size.value };
          if (filters.keyword) params.keyword = filters.keyword;
          if (filters.banned !== '') params.banned = filters.banned;
          const data = await api('GET', '/admin/users?' + new URLSearchParams(params).toString());
          list.value = (data && data.list) || [];
          total.value = (data && data.total) || 0;
        } catch (e) {}
        finally { loading.value = false; }
      }
      async function loadGroups() {
        try { allGroups.value = (await api('GET', '/admin/groups')) || []; } catch (e) {}
      }
      function search() { page.value = 1; load(); }
      function onPage(p) { page.value = p; load(); }
      function openEdit(row) {
        edit.row = row;
        edit.form = {
          nickname: row.nickname || '', nickname_prefix: row.nickname_prefix || '',
          avatar_url: row.avatar_url || '', end_time: row.end_time ? row.end_time.replace('Z', '').slice(0, 19) : null,
          bio: row.bio || '', is_banned: !!row.is_banned
        };
        edit.visible = true;
      }
      async function saveEdit() {
        const f = JSON.parse(JSON.stringify(edit.form));
        if (f.end_time === '') f.end_time = null;
        edit.loading = true;
        try {
          await api('PUT', '/admin/users/' + encodeURIComponent(edit.row.uuid), f);
          ElMessage.success('已保存');
          edit.visible = false;
          await load();
        } catch (e) {}
        finally { edit.loading = false; }
      }
      function openAdjust(row, field) {
        adjust.row = row; adjust.field = field;
        adjust.form = { amount: 0, type: 'admin_adjust', remark: '' };
        adjust.visible = true;
      }
      async function saveAdjust() {
        adjust.loading = true;
        try {
          await api('POST', '/admin/users/' + encodeURIComponent(adjust.row.uuid) + '/adjust-' + adjust.field, adjust.form);
          ElMessage.success('已调整');
          adjust.visible = false;
          await load();
        } catch (e) {}
        finally { adjust.loading = false; }
      }
      function openGroups(row) {
        groupsDlg.row = row;
        groupsDlg.group_ids = (row.groups || []).map(g => g.id);
        groupsDlg.visible = true;
        if (!allGroups.value.length) loadGroups();
      }
      async function saveGroups() {
        groupsDlg.loading = true;
        try {
          await api('POST', '/admin/users/' + encodeURIComponent(groupsDlg.row.uuid) + '/groups', { group_ids: groupsDlg.group_ids });
          ElMessage.success('已设置');
          groupsDlg.visible = false;
          await load();
        } catch (e) {}
        finally { groupsDlg.loading = false; }
      }
      onMounted(async () => { await Promise.all([load(), loadGroups()]); });
      return { loading, list, total, page, size, filters, allGroups, edit, adjust, groupsDlg, adjustLabel, currentAdjustValue, search, onPage, openEdit, saveEdit, openAdjust, saveAdjust, openGroups, saveGroups, fmtMoney, fmtDate };
    }
  };

  /* ============ 管理后台 - 身份组 ============ */
  const AdminGroupsPage = {
    template: `
    <div v-loading="loading">
      <div class="toolbar">
        <el-button type="primary" @click="openEdit(null)">新建身份组</el-button>
      </div>
      <el-table :data="list" empty-text="暂无数据" size="small">
        <el-table-column prop="id" label="ID" width="80"></el-table-column>
        <el-table-column prop="name" label="名称" width="160"></el-table-column>
        <el-table-column label="权限" min-width="240"><template #default="{ row }"><el-tag v-for="p in row.permissions||[]" :key="p" size="small" style="margin:2px">{{ p }}</el-tag></template></el-table-column>
        <el-table-column prop="description" label="描述" min-width="180"></el-table-column>
        <el-table-column label="默认组" width="90"><template #default="{ row }">{{ row.is_default ? '\u662f' : '\u5426' }}</template></el-table-column>
        <el-table-column prop="sort_order" label="排序" width="80"></el-table-column>
        <el-table-column prop="member_count" label="成员数" width="90"></el-table-column>
        <el-table-column label="操作" width="180" fixed="right">
          <template #default="{ row }">
            <el-button size="small" @click="openEdit(row)">编辑</el-button>
            <el-button size="small" type="danger" @click="del(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-dialog v-model="dlg.visible" :title="dlg.row ? '编辑身份组' : '新建身份组'" width="480px">
        <el-form :model="dlg.form" label-position="top">
          <el-form-item label="名称"><el-input v-model="dlg.form.name"></el-input></el-form-item>
          <el-form-item label="描述"><el-input v-model="dlg.form.description"></el-input></el-form-item>
          <el-form-item label="权限（每行一个，如 * / user.read）">
            <el-input v-model="permText" type="textarea" :rows="3" placeholder="*"></el-input>
          </el-form-item>
          <el-row :gutter="12">
            <el-col :span="12"><el-form-item label="默认组"><el-switch v-model="dlg.form.is_default"></el-switch></el-form-item></el-col>
            <el-col :span="12"><el-form-item label="排序"><el-input-number v-model="dlg.form.sort_order" :min="0" style="width:100%"></el-input-number></el-form-item></el-col>
          </el-row>
        </el-form>
        <template #footer>
          <el-button @click="dlg.visible=false">取消</el-button>
          <el-button type="primary" :loading="dlg.loading" @click="save">保存</el-button>
        </template>
      </el-dialog>
    </div>`,
    setup() {
      const loading = ref(false);
      const list = ref([]);
      const dlg = reactive({ visible: false, loading: false, row: null, form: { name: '', description: '', is_default: false, sort_order: 0 } });
      const permText = ref('');
      async function load() {
        loading.value = true;
        try { list.value = (await api('GET', '/admin/groups')) || []; }
        finally { loading.value = false; }
      }
      function openEdit(row) {
        dlg.row = row;
        if (row) {
          dlg.form = { name: row.name, description: row.description, is_default: !!row.is_default, sort_order: row.sort_order || 0 };
          permText.value = (row.permissions || []).join('\n');
        } else {
          dlg.form = { name: '', description: '', is_default: false, sort_order: 0 };
          permText.value = '';
        }
        dlg.visible = true;
      }
      async function save() {
        if (!dlg.form.name) { ElMessage.warning('请填写名称'); return; }
        const perms = permText.value.split('\n').map(s => s.trim()).filter(Boolean);
        const body = Object.assign({}, dlg.form, { permissions: perms });
        dlg.loading = true;
        try {
          if (dlg.row) await api('PUT', '/admin/groups/' + dlg.row.id, body);
          else await api('POST', '/admin/groups', body);
          ElMessage.success('已保存');
          dlg.visible = false;
          await load();
        } catch (e) {}
        finally { dlg.loading = false; }
      }
      async function del(row) {
        try { await ElMessageBox.confirm('确定删除身份组「' + row.name + '」？', '提示', { type: 'warning' }); } catch (e) { return; }
        try { await api('DELETE', '/admin/groups/' + row.id); ElMessage.success('已删除'); await load(); } catch (e) {}
      }
      onMounted(load);
      return { loading, list, dlg, permText, openEdit, save, del };
    }
  };

  /* ============ 管理后台 - 商品管理 ============ */
  const AdminProductsPage = {
    template: `
    <div v-loading="loading">
      <div class="toolbar">
        <el-input v-model="keyword" placeholder="名称过滤（仅前端）" clearable style="width:200px"></el-input>
        <el-select v-model="activeFilter" placeholder="状态" clearable style="width:120px" @change="search">
          <el-option label="上架" value="1"></el-option>
          <el-option label="下架" value="0"></el-option>
        </el-select>
        <el-button type="primary" @click="openEdit(null)">新建商品</el-button>
      </div>
      <el-table :data="list" empty-text="暂无商品" size="small">
        <el-table-column prop="id" label="ID" width="80"></el-table-column>
        <el-table-column prop="name" label="名称" min-width="160"></el-table-column>
        <el-table-column prop="category" label="分类" width="120"></el-table-column>
        <el-table-column label="价格" width="100"><template #default="{ row }">{{ fmtMoney(row.price) }}</template></el-table-column>
        <el-table-column label="库存" width="90"><template #default="{ row }">{{ row.stock < 0 ? '\u5145\u8db3' : row.stock }}</template></el-table-column>
        <el-table-column label="状态" width="90"><template #default="{ row }"><el-tag :type="row.is_active ? 'success' : 'info'" size="small">{{ row.is_active ? '\u4e0a\u67b6' : '\u4e0b\u67b6' }}</el-tag></template></el-table-column>
        <el-table-column prop="sort_order" label="排序" width="80"></el-table-column>
        <el-table-column label="操作" width="180" fixed="right">
          <template #default="{ row }">
            <el-button size="small" @click="openEdit(row)">编辑</el-button>
            <el-button size="small" type="danger" @click="del(row)">下架</el-button>
          </template>
        </el-table-column>
      </el-table>
      <div class="pager"><el-pagination background layout="prev, pager, next, total" :total="total" :current-page="page" :page-size="size" @current-change="onPage"></el-pagination></div>

      <el-dialog v-model="dlg.visible" :title="dlg.row ? '编辑商品' : '新建商品'" width="640px">
        <el-form :model="dlg.form" label-position="top">
          <el-row :gutter="12">
            <el-col :span="12"><el-form-item label="名称"><el-input v-model="dlg.form.name"></el-input></el-form-item></el-col>
            <el-col :span="6"><el-form-item label="价格（元）"><el-input-number v-model="priceYuan" :min="0" :precision="2" :step="1" style="width:100%"></el-input-number></el-form-item></el-col>
            <el-col :span="6"><el-form-item label="库存（-1=无限）"><el-input-number v-model="dlg.form.stock" :min="-1" style="width:100%"></el-input-number></el-form-item></el-col>
            <el-col :span="12"><el-form-item label="分类"><el-input v-model="dlg.form.category" placeholder="分类名称"></el-input></el-form-item></el-col>
            <el-col :span="6"><el-form-item label="排序"><el-input-number v-model="dlg.form.sort_order" :min="0" style="width:100%"></el-input-number></el-form-item></el-col>
            <el-col :span="6"><el-form-item label="上架"><el-switch v-model="dlg.form.is_active"></el-switch></el-form-item></el-col>
            <el-col :span="24"><el-form-item label="图片URL"><el-input v-model="dlg.form.image_url"></el-input></el-form-item></el-col>
            <el-col :span="24"><el-form-item label="描述"><el-input v-model="dlg.form.description" type="textarea" :rows="2"></el-input></el-form-item></el-col>
            <el-col :span="24"><el-form-item label="扩展配置（JSON：grant_type/grant_value/visible_group_ids）"><el-input v-model="extraText" type="textarea" :rows="3" placeholder='{"grant_type":"balance","grant_value":1000}'></el-input></el-form-item></el-col>
          </el-row>
        </el-form>
        <template #footer>
          <el-button @click="dlg.visible=false">取消</el-button>
          <el-button type="primary" :loading="dlg.loading" @click="save">保存</el-button>
        </template>
      </el-dialog>
    </div>`,
    setup() {
      const loading = ref(false);
      const list = ref([]);
      const total = ref(0);
      const page = ref(1);
      const size = ref(10);
      const keyword = ref('');
      const activeFilter = ref('');
      const dlg = reactive({ visible: false, loading: false, row: null, form: { name: '', description: '', price: 0, category: '', image_url: '', is_active: true, sort_order: 0, stock: -1, extra_config: {} } });
      const priceYuan = ref(0);
      const extraText = ref('{}');

      async function load() {
        loading.value = true;
        try {
          const params = { page: page.value, size: size.value };
          if (activeFilter.value) params.active = activeFilter.value;
          const data = await api('GET', '/admin/products?' + new URLSearchParams(params).toString());
          list.value = (data && data.list) || [];
          total.value = (data && data.total) || 0;
        } catch (e) {}
        finally { loading.value = false; }
      }
      function search() { page.value = 1; load(); }
      function onPage(p) { page.value = p; load(); }
      function openEdit(row) {
        dlg.row = row;
        if (row) {
          dlg.form = { name: row.name, description: row.description || '', price: row.price, category: row.category || '', image_url: row.image_url || '', is_active: !!row.is_active, sort_order: row.sort_order || 0, stock: row.stock, extra_config: row.extra_config || {} };
          priceYuan.value = (row.price || 0) / 100;
          extraText.value = JSON.stringify(row.extra_config || {}, null, 2);
        } else {
          dlg.form = { name: '', description: '', price: 0, category: '', image_url: '', is_active: true, sort_order: 0, stock: -1, extra_config: {} };
          priceYuan.value = 0;
          extraText.value = '{}';
        }
        dlg.visible = true;
      }
      async function save() {
        if (!dlg.form.name) { ElMessage.warning('请填写名称'); return; }
        let extra = {};
        try { extra = JSON.parse(extraText.value || '{}'); } catch (e) { ElMessage.warning('扩展配置 JSON 格式错误'); return; }
        const body = Object.assign({}, dlg.form, { price: Math.round((priceYuan.value || 0) * 100), extra_config: extra });
        dlg.loading = true;
        try {
          if (dlg.row) await api('PUT', '/admin/products/' + dlg.row.id, body);
          else await api('POST', '/admin/products', body);
          ElMessage.success('已保存');
          dlg.visible = false;
          await load();
        } catch (e) {}
        finally { dlg.loading = false; }
      }
      async function del(row) {
        try { await ElMessageBox.confirm('确定下架该商品？（软删除）', '提示', { type: 'warning' }); } catch (e) { return; }
        try { await api('DELETE', '/admin/products/' + row.id); ElMessage.success('已下架'); await load(); } catch (e) {}
      }
      onMounted(load);
      return { loading, list, total, page, size, keyword, activeFilter, dlg, priceYuan, extraText, search, onPage, openEdit, save, del, fmtMoney };
    }
  };

  /* ============ 管理后台 - 分类管理 ============ */
  const AdminCategoriesPage = {
    template: `
    <div v-loading="loading">
      <div class="toolbar"><el-button type="primary" @click="openEdit(null)">新建分类</el-button></div>
      <el-table :data="list" empty-text="暂无分类" size="small">
        <el-table-column prop="id" label="ID" width="80"></el-table-column>
        <el-table-column prop="name" label="名称" width="180"></el-table-column>
        <el-table-column prop="sort_order" label="排序" width="80"></el-table-column>
        <el-table-column label="启用" width="80"><template #default="{ row }">{{ row.is_active ? '\u662f' : '\u5426' }}</template></el-table-column>
        <el-table-column label="可见身份组" min-width="200"><template #default="{ row }"><el-tag v-for="g in row.visible_group_ids||[]" :key="g" size="small" style="margin:2px">{{ g }}</el-tag></template></el-table-column>
        <el-table-column label="操作" width="180" fixed="right">
          <template #default="{ row }">
            <el-button size="small" @click="openEdit(row)">编辑</el-button>
            <el-button size="small" type="danger" @click="del(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-dialog v-model="dlg.visible" :title="dlg.row ? '编辑分类' : '新建分类'" width="460px">
        <el-form :model="dlg.form" label-position="top">
          <el-form-item label="名称"><el-input v-model="dlg.form.name"></el-input></el-form-item>
          <el-row :gutter="12">
            <el-col :span="12"><el-form-item label="排序"><el-input-number v-model="dlg.form.sort_order" :min="0" style="width:100%"></el-input-number></el-form-item></el-col>
            <el-col :span="12"><el-form-item label="启用"><el-switch v-model="dlg.form.is_active"></el-switch></el-form-item></el-col>
          </el-row>
          <el-form-item label="可见身份组 ID">
            <el-select v-model="visIds" multiple filterable allow-create style="width:100%" placeholder="留空则所有人可见"></el-select>
          </el-form-item>
        </el-form>
        <template #footer>
          <el-button @click="dlg.visible=false">取消</el-button>
          <el-button type="primary" :loading="dlg.loading" @click="save">保存</el-button>
        </template>
      </el-dialog>
    </div>`,
    setup() {
      const loading = ref(false);
      const list = ref([]);
      const dlg = reactive({ visible: false, loading: false, row: null, form: { name: '', sort_order: 0, is_active: true, visible_group_ids: [] } });
      const visIds = ref([]);
      async function load() {
        loading.value = true;
        try { list.value = (await api('GET', '/admin/categories')) || []; }
        finally { loading.value = false; }
      }
      function openEdit(row) {
        dlg.row = row;
        if (row) {
          dlg.form = { name: row.name, sort_order: row.sort_order || 0, is_active: !!row.is_active, visible_group_ids: row.visible_group_ids || [] };
          visIds.value = (row.visible_group_ids || []).map(String);
        } else {
          dlg.form = { name: '', sort_order: 0, is_active: true, visible_group_ids: [] };
          visIds.value = [];
        }
        dlg.visible = true;
      }
      async function save() {
        if (!dlg.form.name) { ElMessage.warning('请填写名称'); return; }
        const ids = visIds.value.map(v => parseInt(v, 10)).filter(n => !isNaN(n) && n > 0);
        const body = Object.assign({}, dlg.form, { visible_group_ids: ids });
        dlg.loading = true;
        try {
          if (dlg.row) await api('PUT', '/admin/categories/' + dlg.row.id, body);
          else await api('POST', '/admin/categories', body);
          ElMessage.success('已保存');
          dlg.visible = false;
          await load();
        } catch (e) {}
        finally { dlg.loading = false; }
      }
      async function del(row) {
        try { await ElMessageBox.confirm('确定删除分类？', '提示', { type: 'warning' }); } catch (e) { return; }
        try { await api('DELETE', '/admin/categories/' + row.id); ElMessage.success('已删除'); await load(); } catch (e) {}
      }
      onMounted(load);
      return { loading, list, dlg, visIds, openEdit, save, del };
    }
  };

  /* ============ 管理后台 - 订单管理 ============ */
  const AdminOrdersPage = {
    template: `
    <div v-loading="loading">
      <div class="toolbar">
        <el-input v-model="filters.order_no" placeholder="订单号" clearable style="width:180px"></el-input>
        <el-input v-model="filters.user_uuid" placeholder="用户UUID" clearable style="width:200px"></el-input>
        <el-select v-model="filters.status" placeholder="状态" clearable style="width:120px">
          <el-option label="已支付" value="paid"></el-option>
          <el-option label="已取消" value="cancelled"></el-option>
          <el-option label="已退款" value="refunded"></el-option>
        </el-select>
        <el-button @click="search">搜索</el-button>
      </div>
      <el-table :data="list" empty-text="暂无订单" size="small">
        <el-table-column prop="id" label="ID" width="80"></el-table-column>
        <el-table-column prop="order_no" label="订单号" min-width="200" show-overflow-tooltip></el-table-column>
        <el-table-column label="用户UUID" min-width="180"><template #default="{ row }"><span class="mono">{{ row.user_uuid }}</span></template></el-table-column>
        <el-table-column prop="product_id" label="商品ID" width="90"></el-table-column>
        <el-table-column prop="quantity" label="数量" width="80"></el-table-column>
        <el-table-column label="原价" width="100"><template #default="{ row }">{{ fmtMoney(row.original_price) }}</template></el-table-column>
        <el-table-column label="实付" width="100"><template #default="{ row }">{{ fmtMoney(row.final_price) }}</template></el-table-column>
        <el-table-column label="状态" width="90"><template #default="{ row }"><el-tag :type="statusType(row.status)" size="small">{{ row.status }}</el-tag></template></el-table-column>
        <el-table-column label="时间" width="180"><template #default="{ row }">{{ fmtDate(row.created_at) }}</template></el-table-column>
        <el-table-column label="操作" width="140" fixed="right">
          <template #default="{ row }">
            <el-button size="small" @click="openStatus(row)">改状态</el-button>
          </template>
        </el-table-column>
      </el-table>
      <div class="pager"><el-pagination background layout="prev, pager, next, total" :total="total" :current-page="page" :page-size="size" @current-change="onPage"></el-pagination></div>

      <el-dialog v-model="st.visible" title="修改订单状态" width="400px">
        <div class="muted" style="margin-bottom:8px">订单：{{ st.row && st.row.order_no }}</div>
        <el-form label-position="top">
          <el-form-item label="新状态">
            <el-select v-model="st.form.status" style="width:100%">
              <el-option label="已支付 paid" value="paid"></el-option>
              <el-option label="已取消 cancelled" value="cancelled"></el-option>
              <el-option label="已退款 refunded" value="refunded"></el-option>
            </el-select>
          </el-form-item>
        </el-form>
        <template #footer>
          <el-button @click="st.visible=false">取消</el-button>
          <el-button type="primary" :loading="st.loading" @click="saveStatus">保存</el-button>
        </template>
      </el-dialog>
    </div>`,
    setup() {
      const loading = ref(false);
      const list = ref([]);
      const total = ref(0);
      const page = ref(1);
      const size = ref(10);
      const filters = reactive({ order_no: '', user_uuid: '', status: '' });
      const st = reactive({ visible: false, loading: false, row: null, form: { status: 'cancelled' } });
      async function load() {
        loading.value = true;
        try {
          const params = { page: page.value, size: size.value };
          if (filters.order_no) params.order_no = filters.order_no;
          if (filters.user_uuid) params.user_uuid = filters.user_uuid;
          if (filters.status) params.status = filters.status;
          const data = await api('GET', '/admin/orders?' + new URLSearchParams(params).toString());
          list.value = (data && data.list) || [];
          total.value = (data && data.total) || 0;
        } catch (e) {}
        finally { loading.value = false; }
      }
      function search() { page.value = 1; load(); }
      function onPage(p) { page.value = p; load(); }
      function openStatus(row) { st.row = row; st.form.status = row.status; st.visible = true; }
      async function saveStatus() {
        st.loading = true;
        try {
          await api('PUT', '/admin/orders/' + st.row.id + '/status', { status: st.form.status });
          ElMessage.success('已更新');
          st.visible = false;
          await load();
        } catch (e) {}
        finally { st.loading = false; }
      }
      function statusType(s) { if (s === 'paid') return 'success'; if (s === 'cancelled') return 'info'; if (s === 'refunded') return 'warning'; return 'primary'; }
      onMounted(load);
      return { loading, list, total, page, size, filters, st, search, onPage, openStatus, saveStatus, statusType, fmtMoney, fmtDate };
    }
  };

  /* ============ 管理后台 - 兑换码 ============ */
  const AdminRedeemCodesPage = {
    template: `
    <div v-loading="loading">
      <div class="toolbar">
        <el-button type="primary" @click="openGen">批量生成</el-button>
        <el-input v-model="filters.code" placeholder="兑换码" clearable style="width:220px"></el-input>
        <el-select v-model="filters.used" placeholder="状态" clearable style="width:120px">
          <el-option label="未使用" value="0"></el-option>
          <el-option label="已使用" value="1"></el-option>
        </el-select>
        <el-button @click="search">搜索</el-button>
      </div>
      <el-table :data="list" empty-text="暂无兑换码" size="small">
        <el-table-column prop="id" label="ID" width="80"></el-table-column>
        <el-table-column label="兑换码" min-width="200"><template #default="{ row }"><span class="mono">{{ row.code }}</span></template></el-table-column>
        <el-table-column prop="grant_type" label="类型" width="90"></el-table-column>
        <el-table-column label="数量" width="100"><template #default="{ row }">{{ row.grant_type === 'balance' ? fmtMoney(row.grant_amount) : row.grant_amount }}</template></el-table-column>
        <el-table-column label="状态" width="90"><template #default="{ row }"><el-tag :type="row.used ? 'info' : 'success'" size="small">{{ row.used ? '\u5df2\u4f7f\u7528' : '\u672a\u4f7f\u7528' }}</el-tag></template></el-table-column>
        <el-table-column label="使用者" min-width="160"><template #default="{ row }"><span class="mono">{{ row.used_by || '\u2014' }}</span></template></el-table-column>
        <el-table-column label="使用时间" width="180"><template #default="{ row }">{{ row.used_at ? fmtDate(row.used_at) : '\u2014' }}</template></el-table-column>
        <el-table-column label="创建时间" width="180"><template #default="{ row }">{{ fmtDate(row.created_at) }}</template></el-table-column>
      </el-table>
      <div class="pager"><el-pagination background layout="prev, pager, next, total" :total="total" :current-page="page" :page-size="size" @current-change="onPage"></el-pagination></div>

      <el-dialog v-model="gen.visible" title="生成兑换码" width="460px">
        <el-form :model="gen.form" label-position="top">
          <el-row :gutter="12">
            <el-col :span="12"><el-form-item label="数量"><el-input-number v-model="gen.form.count" :min="1" :max="10000" style="width:100%"></el-input-number></el-form-item></el-col>
            <el-col :span="12"><el-form-item label="发放类型">
              <el-select v-model="gen.form.grant_type" style="width:100%">
                <el-option label="余额" value="balance"></el-option>
                <el-option label="额度" value="quota"></el-option>
                <el-option label="次数" value="times"></el-option>
                <el-option label="卡槽" value="slot"></el-option>
              </el-select>
            </el-form-item></el-col>
            <el-col :span="12"><el-form-item label="数量/金额（分）"><el-input-number v-model="gen.form.grant_amount" :min="1" style="width:100%"></el-input-number></el-form-item></el-col>
            <el-col :span="12"><el-form-item label="前缀（可选）"><el-input v-model="gen.form.prefix" placeholder="如 VIP"></el-input></el-form-item></el-col>
            <el-col :span="24"><el-form-item label="备注"><el-input v-model="gen.form.remark"></el-input></el-form-item></el-col>
          </el-row>
        </el-form>
        <template #footer>
          <el-button @click="gen.visible=false">取消</el-button>
          <el-button type="primary" :loading="gen.loading" @click="doGen">生成</el-button>
        </template>
      </el-dialog>

      <el-dialog v-model="result.visible" title="生成结果" width="540px">
        <div class="muted" style="margin-bottom:8px">共生成 {{ result.codes.length }} 个：</div>
        <el-input type="textarea" :rows="10" :model-value="result.codes.join('\n')" readonly></el-input>
        <template #footer>
          <el-button @click="copyAll">复制全部</el-button>
          <el-button type="primary" @click="result.visible=false">关闭</el-button>
        </template>
      </el-dialog>
    </div>`,
    setup() {
      const loading = ref(false);
      const list = ref([]);
      const total = ref(0);
      const page = ref(1);
      const size = ref(10);
      const filters = reactive({ code: '', used: '' });
      const gen = reactive({ visible: false, loading: false, form: { count: 10, grant_type: 'balance', grant_amount: 1000, remark: '', prefix: '' } });
      const result = reactive({ visible: false, codes: [] });
      async function load() {
        loading.value = true;
        try {
          const params = { page: page.value, size: size.value };
          if (filters.code) params.code = filters.code;
          if (filters.used) params.used = filters.used;
          const data = await api('GET', '/admin/redeem-codes?' + new URLSearchParams(params).toString());
          list.value = (data && data.list) || [];
          total.value = (data && data.total) || 0;
        } catch (e) {}
        finally { loading.value = false; }
      }
      function search() { page.value = 1; load(); }
      function onPage(p) { page.value = p; load(); }
      function openGen() { gen.form = { count: 10, grant_type: 'balance', grant_amount: 1000, remark: '', prefix: '' }; gen.visible = true; }
      async function doGen() {
        if (gen.form.grant_amount < 1) { ElMessage.warning('数量/金额必须 >=1'); return; }
        gen.loading = true;
        try {
          const data = await api('POST', '/admin/redeem-codes/generate', gen.form);
          result.codes = (data && data.codes) || [];
          result.visible = true;
          gen.visible = false;
          ElMessage.success('已生成 ' + result.codes.length + ' 个');
          await load();
        } catch (e) {}
        finally { gen.loading = false; }
      }
      function copyAll() { copyText(result.codes.join('\n')); }
      onMounted(load);
      return { loading, list, total, page, size, filters, gen, result, search, onPage, openGen, doGen, copyAll, fmtMoney, fmtDate };
    }
  };

  /* ============ 管理后台 - 卡槽管理 ============ */
  const AdminSlotsPage = {
    template: `
    <div v-loading="loading">
      <div class="toolbar">
        <el-input v-model="filters.user_uuid" placeholder="用户UUID" clearable style="width:220px"></el-input>
        <el-input v-model="filters.server_code" placeholder="服务器号" clearable style="width:160px"></el-input>
        <el-button @click="search">搜索</el-button>
        <el-button type="primary" @click="grant.visible=true">发放卡槽</el-button>
      </div>
      <el-table :data="list" empty-text="暂无卡槽" size="small">
        <el-table-column label="ID" min-width="280"><template #default="{ row }"><span class="mono">{{ row.id }}</span></template></el-table-column>
        <el-table-column label="用户UUID" min-width="180"><template #default="{ row }"><span class="mono">{{ row.user_uuid }}</span></template></el-table-column>
        <el-table-column label="服务器号" width="140"><template #default="{ row }">{{ row.server_code || '\u2014' }}</template></el-table-column>
        <el-table-column label="到期时间" width="180"><template #default="{ row }">{{ row.end_time ? fmtDate(row.end_time) : '\u6c38\u4e45' }}</template></el-table-column>
        <el-table-column label="操作" width="120" fixed="right">
          <template #default="{ row }"><el-button size="small" type="danger" @click="del(row)">删除</el-button></template>
        </el-table-column>
      </el-table>
      <div class="pager"><el-pagination background layout="prev, pager, next, total" :total="total" :current-page="page" :page-size="size" @current-change="onPage"></el-pagination></div>

      <el-dialog v-model="grant.visible" title="发放卡槽" width="440px">
        <el-form :model="grant.form" label-position="top">
          <el-form-item label="用户UUID"><el-input v-model="grant.form.user_uuid"></el-input></el-form-item>
          <el-form-item label="数量"><el-input-number v-model="grant.form.count" :min="1" :max="1000" style="width:100%"></el-input-number></el-form-item>
          <el-form-item label="有效天数（可选，留空=永久）"><el-input-number v-model="grant.form.duration_days" :min="0" style="width:100%"></el-input-number></el-form-item>
        </el-form>
        <template #footer>
          <el-button @click="grant.visible=false">取消</el-button>
          <el-button type="primary" :loading="grant.loading" @click="doGrant">发放</el-button>
        </template>
      </el-dialog>
    </div>`,
    setup() {
      const loading = ref(false);
      const list = ref([]);
      const total = ref(0);
      const page = ref(1);
      const size = ref(10);
      const filters = reactive({ user_uuid: '', server_code: '' });
      const grant = reactive({ visible: false, loading: false, form: { user_uuid: '', count: 1, duration_days: 0 } });
      async function load() {
        loading.value = true;
        try {
          const params = { page: page.value, size: size.value };
          if (filters.user_uuid) params.user_uuid = filters.user_uuid;
          if (filters.server_code) params.server_code = filters.server_code;
          const data = await api('GET', '/admin/slots?' + new URLSearchParams(params).toString());
          list.value = (data && data.list) || [];
          total.value = (data && data.total) || 0;
        } catch (e) {}
        finally { loading.value = false; }
      }
      function search() { page.value = 1; load(); }
      function onPage(p) { page.value = p; load(); }
      async function doGrant() {
        if (!grant.form.user_uuid) { ElMessage.warning('请填写用户UUID'); return; }
        const body = { user_uuid: grant.form.user_uuid, count: grant.form.count };
        if (grant.form.duration_days && grant.form.duration_days > 0) body.duration_days = grant.form.duration_days;
        grant.loading = true;
        try {
          await api('POST', '/admin/slots/grant', body);
          ElMessage.success('已发放');
          grant.visible = false;
          await load();
        } catch (e) {}
        finally { grant.loading = false; }
      }
      async function del(row) {
        try { await ElMessageBox.confirm('确定删除该卡槽？', '提示', { type: 'warning' }); } catch (e) { return; }
        try { await api('DELETE', '/admin/slots/' + encodeURIComponent(row.id)); ElMessage.success('已删除'); await load(); } catch (e) {}
      }
      onMounted(load);
      return { loading, list, total, page, size, filters, grant, search, onPage, doGrant, del, fmtDate };
    }
  };

  /* ============ 管理后台 - 公告管理 ============ */
  const AdminAnnouncementsPage = {
    template: `
    <div v-loading="loading">
      <div class="toolbar"><el-button type="primary" @click="openEdit(null)">新建公告</el-button></div>
      <el-table :data="list" empty-text="暂无公告" size="small">
        <el-table-column prop="id" label="ID" width="80"></el-table-column>
        <el-table-column prop="title" label="标题" min-width="200" show-overflow-tooltip></el-table-column>
        <el-table-column prop="author" label="作者" width="140"></el-table-column>
        <el-table-column label="日期" width="180"><template #default="{ row }">{{ fmtDate(row.date) }}</template></el-table-column>
        <el-table-column label="内容" min-width="220" show-overflow-tooltip><template #default="{ row }">{{ (row.content||'').slice(0,60) }}</template></el-table-column>
        <el-table-column label="操作" width="180" fixed="right">
          <template #default="{ row }">
            <el-button size="small" @click="openEdit(row)">编辑</el-button>
            <el-button size="small" type="danger" @click="del(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
      <div class="pager"><el-pagination background layout="prev, pager, next, total" :total="total" :current-page="page" :page-size="size" @current-change="onPage"></el-pagination></div>

      <el-dialog v-model="dlg.visible" :title="dlg.row ? '编辑公告' : '新建公告'" width="640px">
        <el-form :model="dlg.form" label-position="top">
          <el-form-item label="标题"><el-input v-model="dlg.form.title"></el-input></el-form-item>
          <el-form-item label="作者（可选）"><el-input v-model="dlg.form.author" placeholder="留空则使用当前用户名"></el-input></el-form-item>
          <el-form-item label="内容（支持 Markdown）"><el-input v-model="dlg.form.content" type="textarea" :rows="8"></el-input></el-form-item>
        </el-form>
        <template #footer>
          <el-button @click="dlg.visible=false">取消</el-button>
          <el-button type="primary" :loading="dlg.loading" @click="save">保存</el-button>
        </template>
      </el-dialog>
    </div>`,
    setup() {
      const loading = ref(false);
      const list = ref([]);
      const total = ref(0);
      const page = ref(1);
      const size = ref(10);
      const dlg = reactive({ visible: false, loading: false, row: null, form: { title: '', author: '', content: '' } });
      async function load() {
        loading.value = true;
        try {
          const data = await api('GET', '/admin/announcements?page=' + page.value + '&size=' + size.value);
          list.value = (data && data.list) || [];
          total.value = (data && data.total) || 0;
        } catch (e) {}
        finally { loading.value = false; }
      }
      function onPage(p) { page.value = p; load(); }
      function openEdit(row) {
        dlg.row = row;
        if (row) dlg.form = { title: row.title, author: row.author, content: row.content };
        else dlg.form = { title: '', author: '', content: '' };
        dlg.visible = true;
      }
      async function save() {
        if (!dlg.form.title || !dlg.form.content) { ElMessage.warning('请填写标题和内容'); return; }
        dlg.loading = true;
        try {
          if (dlg.row) await api('PUT', '/admin/announcements/' + dlg.row.id, dlg.form);
          else await api('POST', '/admin/announcements', dlg.form);
          ElMessage.success('已保存');
          dlg.visible = false;
          await load();
        } catch (e) {}
        finally { dlg.loading = false; }
      }
      async function del(row) {
        try { await ElMessageBox.confirm('确定删除该公告？', '提示', { type: 'warning' }); } catch (e) { return; }
        try { await api('DELETE', '/admin/announcements/' + row.id); ElMessage.success('已删除'); await load(); } catch (e) {}
      }
      onMounted(load);
      return { loading, list, total, page, size, dlg, onPage, openEdit, save, del, fmtDate };
    }
  };

  /* ============ 管理后台 - 系统设置 ============ */
  const AdminSystemSettingsPage = {
    template: `
    <div v-loading="loading">
      <div class="card">
        <div class="card-title"><span>登录与注册</span></div>
        <div class="kv"><span class="k">允许登录</span><el-switch v-model="form.login_enabled"></el-switch></div>
        <div class="kv"><span class="k">允许注册</span><el-switch v-model="form.register_enabled"></el-switch></div>
      </div>
      <div class="card">
        <div class="card-title"><span>限流</span></div>
        <el-form label-position="top">
          <el-form-item label="每分钟限流（0=不限制）"><el-input-number v-model="form.rate_limit_per_min" :min="0" style="width:100%"></el-input-number></el-form-item>
        </el-form>
      </div>
      <div class="card">
        <div class="card-title"><span>IP 黑白名单</span></div>
        <el-form label-position="top">
          <el-form-item label="IP 白名单（每行一个，留空=不限）"><el-input v-model="form.ip_whitelist" type="textarea" :rows="3" placeholder="1.2.3.4"></el-input></el-form-item>
          <el-form-item label="IP 黑名单（每行一个）"><el-input v-model="form.ip_blacklist" type="textarea" :rows="3" placeholder="5.6.7.8"></el-input></el-form-item>
          <el-form-item label="自动黑名单"><el-switch v-model="form.auto_blacklist_enabled"></el-switch></el-form-item>
        </el-form>
      </div>
      <div class="card">
        <div class="card-title"><span>维护模式</span></div>
        <div class="kv"><span class="k">维护中</span><el-switch v-model="form.maintenance_enabled"></el-switch></div>
        <el-form label-position="top" style="margin-top:10px">
          <el-form-item label="维护提示语"><el-input v-model="form.maintenance_message" type="textarea" :rows="2"></el-input></el-form-item>
        </el-form>
      </div>
      <el-button type="primary" :loading="saving" @click="save">保存设置</el-button>
    </div>`,
    setup() {
      const loading = ref(false);
      const saving = ref(false);
      const form = reactive({
        login_enabled: true, register_enabled: true, rate_limit_per_min: 0,
        ip_whitelist: '', ip_blacklist: '', auto_blacklist_enabled: false,
        maintenance_enabled: false, maintenance_message: '',
        login_allowed_group_ids: '', maintenance_whitelist_ips: '', extra_config: ''
      });
      async function load() {
        loading.value = true;
        try {
          const data = await api('GET', '/admin/system-settings');
          if (data) Object.assign(form, data);
        } catch (e) {}
        finally { loading.value = false; }
      }
      async function save() {
        saving.value = true;
        try {
          await api('PUT', '/admin/system-settings', form);
          ElMessage.success('已保存');
          await load();
        } catch (e) {}
        finally { saving.value = false; }
      }
      onMounted(load);
      return { loading, saving, form, load, save };
    }
  };

  /* ============ 管理后台 - 全部流水 ============ */
  const AdminTransactionsPage = {
    template: `
    <div v-loading="loading">
      <div class="toolbar">
        <el-select v-model="type" placeholder="类型" style="width:120px" @change="search">
          <el-option label="钱包" value="wallet"></el-option>
          <el-option label="额度" value="quota"></el-option>
          <el-option label="次数" value="times"></el-option>
        </el-select>
        <el-input v-model="userUuid" placeholder="用户UUID（可选）" clearable style="width:220px"></el-input>
        <el-date-picker v-model="dateRange" type="daterange" value-format="YYYY-MM-DD" range-separator="\u81f3" start-placeholder="开始" end-placeholder="结束" style="width:280px"></el-date-picker>
        <el-button @click="search">查询</el-button>
      </div>
      <el-table :data="list" empty-text="暂无数据" size="small">
        <el-table-column prop="id" label="ID" width="80"></el-table-column>
        <el-table-column label="用户UUID" min-width="180"><template #default="{ row }"><span class="mono">{{ row.user_uuid }}</span></template></el-table-column>
        <el-table-column label="金额/数量" width="130"><template #default="{ row }"><span :class="row.amount >= 0 ? '' : 'danger'">{{ type==='wallet' ? fmtMoney(row.amount) : row.amount }}</span></template></el-table-column>
        <el-table-column prop="type" label="类型" width="140"></el-table-column>
        <el-table-column prop="remark" label="备注" show-overflow-tooltip></el-table-column>
        <el-table-column label="时间" width="180"><template #default="{ row }">{{ fmtDate(row.created_at) }}</template></el-table-column>
      </el-table>
      <div class="pager"><el-pagination background layout="prev, pager, next, total" :total="total" :current-page="page" :page-size="size" @current-change="onPage"></el-pagination></div>
    </div>`,
    setup() {
      const loading = ref(false);
      const list = ref([]);
      const total = ref(0);
      const page = ref(1);
      const size = ref(10);
      const type = ref('wallet');
      const userUuid = ref('');
      const dateRange = ref(null);
      async function load() {
        loading.value = true;
        try {
          const params = { type: type.value, page: page.value, size: size.value };
          if (userUuid.value) params.user_uuid = userUuid.value;
          if (dateRange.value && dateRange.value.length === 2) { params.start_date = dateRange.value[0]; params.end_date = dateRange.value[1]; }
          const data = await api('GET', '/admin/transactions?' + new URLSearchParams(params).toString());
          list.value = (data && data.list) || [];
          total.value = (data && data.total) || 0;
        } catch (e) {}
        finally { loading.value = false; }
      }
      function search() { page.value = 1; load(); }
      function onPage(p) { page.value = p; load(); }
      onMounted(load);
      return { loading, list, total, page, size, type, userUuid, dateRange, search, onPage, fmtMoney, fmtDate };
    }
  };

  /* ============ 管理后台包装 ============ */
  const ADMIN_SECTIONS = [
    { key: 'users', label: '用户管理' },
    { key: 'groups', label: '身份组' },
    { key: 'products', label: '商品管理' },
    { key: 'categories', label: '分类管理' },
    { key: 'orders', label: '订单管理' },
    { key: 'redeem-codes', label: '兑换码' },
    { key: 'slots', label: '卡槽管理' },
    { key: 'announcements', label: '公告管理' },
    { key: 'system-settings', label: '系统设置' },
    { key: 'transactions', label: '全部流水' }
  ];
  const AdminPage = {
    components: {
      AdminUsersPage, AdminGroupsPage, AdminProductsPage, AdminCategoriesPage,
      AdminOrdersPage, AdminRedeemCodesPage, AdminSlotsPage, AdminAnnouncementsPage,
      AdminSystemSettingsPage, AdminTransactionsPage
    },
    template: `
    <div>
      <h1 class="page-title">管理后台</h1>
      <p class="page-desc">站点运维与管理</p>
      <div v-if="!isAdmin">
        <el-result icon="warning" title="403" sub-title="无权限：仅管理员可访问该页面">
          <template #extra>
            <el-button type="primary" @click="go('/dashboard')">返回仪表盘</el-button>
          </template>
        </el-result>
      </div>
      <div v-else class="admin-layout">
        <div class="admin-sider">
          <button v-for="s in sections" :key="s.key" class="sub-item" :class="{active: active===s.key}" @click="goSub(s.key)">{{ s.label }}</button>
        </div>
        <div class="admin-body">
          <AdminUsersPage v-if="active==='users'" />
          <AdminGroupsPage v-else-if="active==='groups'" />
          <AdminProductsPage v-else-if="active==='products'" />
          <AdminCategoriesPage v-else-if="active==='categories'" />
          <AdminOrdersPage v-else-if="active==='orders'" />
          <AdminRedeemCodesPage v-else-if="active==='redeem-codes'" />
          <AdminSlotsPage v-else-if="active==='slots'" />
          <AdminAnnouncementsPage v-else-if="active==='announcements'" />
          <AdminSystemSettingsPage v-else-if="active==='system-settings'" />
          <AdminTransactionsPage v-else-if="active==='transactions'" />
        </div>
      </div>
    </div>`,
    setup() {
      const isAdmin = computed(() => store.isAdmin);
      const active = computed(() => route.sub || 'users');
      function goSub(k) { location.hash = '#/admin/' + k; }
      return { isAdmin, active, sections: ADMIN_SECTIONS, goSub, go };
    }
  };

  /* ============ 根 App ============ */
  const NAV_ITEMS = [
    { path: '/dashboard', label: '仪表盘', icon: '\ud83d\udcca' },
    { path: '/profile', label: '个人资料', icon: '\ud83d\udc64' },
    { path: '/slots', label: '我的卡槽', icon: '\ud83c\udfaf' },
    { path: '/shop', label: '商店', icon: '\ud83d\udecd\ufe0f' },
    { path: '/redeem', label: '兑换码', icon: '\ud83c\udfa3' },
    { path: '/records', label: '流水记录', icon: '\ud83d\udccd' },
    { path: '/announcements', label: '公告', icon: '\ud83d\udce3' },
    { path: '/admin', label: '管理后台', icon: '\u2699\ufe0f' }
  ];
  const App = {
    components: { LoginPage, DashboardPage, ProfilePage, SlotsPage, ShopPage, RedeemPage, RecordsPage, AnnouncementsPage, AdminPage },
    template: `
    <div class="app-shell" v-if="route.path !== '/login'">
      <div class="sidebar-mask" :class="{show: sidebarOpen}" @click="sidebarOpen=false"></div>
      <aside class="app-sidebar" :class="{open: sidebarOpen}">
        <div class="brand">
          <div class="logo">FA</div>
          <div class="title">FunAuth<small>\u7528\u6237\u4e2d\u5fc3</small></div>
        </div>
        <nav class="nav-list">
          <div v-for="n in navItems" :key="n.path" class="nav-item"
            :class="{active: isActive(n.path)}" @click="onNav(n.path)">
            <span class="ico">{{ n.icon }}</span><span>{{ n.label }}</span>
          </div>
        </nav>
        <div class="nav-footer">FunAuth User Center \u00b7 v1</div>
      </aside>
      <div class="app-main">
        <header class="app-topbar">
          <div class="row">
            <el-button class="mobile-bar" text @click="sidebarOpen=!sidebarOpen" style="margin-right:6px">\u2630</el-button>
            <span style="font-weight:600">{{ currentTitle }}</span>
          </div>
          <div class="row">
            <el-button text @click="toggleTheme" :title="'\u5207\u6362\u4e3b\u9898'">
              <span>{{ themeIcon }}</span>
            </el-button>
            <el-button text type="danger" @click="onLogout">\u9000\u51fa\u767b\u5f55</el-button>
          </div>
        </header>
        <main class="app-content">
          <DashboardPage v-if="route.path==='/dashboard'" />
          <ProfilePage v-else-if="route.path==='/profile'" />
          <SlotsPage v-else-if="route.path==='/slots'" />
          <ShopPage v-else-if="route.path==='/shop'" />
          <RedeemPage v-else-if="route.path==='/redeem'" />
          <RecordsPage v-else-if="route.path==='/records'" />
          <AnnouncementsPage v-else-if="route.path==='/announcements'" />
          <AdminPage v-else-if="route.path==='/admin'" />
          <div v-else class="empty-tip">
            <p>\u672a\u627e\u5230\u9875\u9762\uff1a{{ route.path }}</p>
            <el-button @click="go('/dashboard')">\u8fd4\u56de\u4eea\u8868\u76d8</el-button>
          </div>
        </main>
      </div>
    </div>
    <LoginPage v-else-if="ready" />`,
    setup() {
      const ready = ref(false);
      const sidebarOpen = ref(false);
      const themeIcon = computed(() => (localStorage.getItem('theme') || 'light') === 'dark' ? '\u2600\ufe0f' : '\ud83c\udf19');
      const currentTitle = computed(() => {
        const m = { '/dashboard': '\u4eea\u8868\u76d8', '/profile': '\u4e2a\u4eba\u8d44\u6599', '/slots': '\u6211\u7684\u5361\u69fd',
          '/shop': '\u5546\u5e97', '/redeem': '\u5151\u6362\u7801', '/records': '\u6d41\u6c34\u8bb0\u5f55',
          '/announcements': '\u516c\u544a', '/admin': '\u7ba1\u7406\u540e\u53f0' };
        return m[route.path] || 'FunAuth';
      });

      function isActive(p) {
        if (p === '/admin') return route.path === '/admin';
        return route.path === p;
      }
      function onNav(p) {
        if (p === '/admin') location.hash = '#/admin/users';
        else location.hash = '#' + p;
        sidebarOpen.value = false;
      }
      async function fetchProfile() {
        if (!localStorage.getItem('token')) { ready.value = true; return; }
        try {
          const p = await api('GET', '/profile');
          if (p && p.user) store.user = p.user;
        } catch (e) { /* 401 已自动跳转 */ }
        finally { ready.value = true; }
      }
      async function onLogout() {
        try { await ElMessageBox.confirm('\u786e\u5b9a\u9000\u51fa\u767b\u5f55\uff1f', '\u63d0\u793a', { type: 'warning' }); }
        catch (e) { return; }
        try { await api('POST', '/logout', {}); } catch (e) {}
        localStorage.removeItem('token');
        store.user = null;
        ElMessage.success('\u5df2\u9000\u51fa\u767b\u5f55');
        location.hash = '#/login';
      }

      onMounted(async () => {
        initTheme();
        parseHash();
        window.addEventListener('hashchange', () => {
          parseHash();
          requireAuth();
          if (route.path !== '/login') sidebarOpen.value = false;
        });
        // 已登录访问 #/login 则跳到 dashboard
        if (route.path === '/login' && localStorage.getItem('token')) {
          location.hash = '#/dashboard';
        }
        requireAuth();
        await fetchProfile();
      });

      return { route, navItems: NAV_ITEMS, sidebarOpen, themeIcon, currentTitle, ready, isActive, onNav, onLogout, toggleTheme, go };
    }
  };

  /* ---------- 启动 ---------- */
  initTheme();
  const app = createApp(App);
  app.use(ElementPlus, { locale: window.ElementPlusLocaleZhCn });
  app.mount('#app');
})();
