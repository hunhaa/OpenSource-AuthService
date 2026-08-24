const $ = (s) => document.querySelector(s);
const API = "/api/webui";
let currentToken = localStorage.getItem("bunker_token") || "";
let lastServerID = "";

// ---------- utility ----------
function toast(msg, kind=""){
  const host = document.querySelector(".toast") || (() => {
    const d = document.createElement("div"); d.className="toast"; document.body.appendChild(d); return d;
  })();
  const it = document.createElement("div");
  it.className = "it " + kind;
  it.textContent = msg;
  host.appendChild(it);
  setTimeout(()=>{it.style.opacity="0";it.style.transform="translateY(-4px)";it.style.transition=".2s";setTimeout(()=>it.remove(),200)}, 3000);
}
function setEnabled(cardId, enabled){
  const c = document.getElementById(cardId);
  if(!c) return;
  c.classList.toggle("disabled", !enabled);
}
function setMsg(id, text, kind=""){
  const el = document.getElementById(id);
  if(!el) return;
  el.textContent = text || "";
  el.classList.toggle("ok", kind==="ok");
  el.classList.toggle("err", kind==="err");
}
async function call(path, payload){
  const r = await fetch(API + path, {
    method:"POST",
    headers:{"Content-Type":"application/json"},
    body: payload ? JSON.stringify(payload) : undefined,
  });
  let j;
  try{ j = await r.json(); }catch(e){
    const t = await r.text();
    throw new Error("HTTP "+r.status+" "+ (t || r.statusText));
  }
  if(!j.ok) throw new Error(j.msg || ("请求失败 (code "+(j.code||r.status)+")"));
  return j.data;
}
async function getStatus(){
  const r = await fetch(API + "/status");
  return r.json();
}
// ---------- UI renderers ----------
function renderAuthResult(d){
  const box = $("#auth-result");
  box.classList.remove("hidden");
  const shallow = Object.assign({}, d);
  // 结果面板里不重复 fb_token，避免视觉干扰
  delete shallow.fb_token;
  box.textContent = JSON.stringify(shallow, null, 2);
}
function showFBToken(d){
  const fbBox = document.getElementById("fbtoken-box");
  const fbt = document.getElementById("fbtoken");
  const urlInput = document.getElementById("auth-server-url");
  if(d && d.fb_token){
    fbt.value = d.fb_token;
    const proto = location.protocol;
    // 去掉末尾 /api/webui，ToolDelta 要的是验证服务器根地址（会自行拼 /api/new 与 /api/phoenix/login）
    const base = proto + "//" + location.host;
    urlInput.value = base;
    fbBox.classList.remove("hidden");
  } else {
    fbBox.classList.add("hidden");
    fbt.value = "";
    urlInput.value = "";
  }
}
function copyText(el){
  const v = (el.value || el.textContent || "").toString();
  if(!v) return;
  if(navigator.clipboard){
    navigator.clipboard.writeText(v).then(()=>toast("已复制: "+v.slice(0,28)+(v.length>28?"…":""),"ok"));
  } else {
    el.select && el.select();
    document.execCommand("copy");
    toast("已复制","ok");
  }
}
function renderSearchResults(entities){
  const box = $("#search-result");
  box.innerHTML = "";
  if(!entities || entities.length===0){
    box.innerHTML = `<div class="srv-row"><div class="srv-meta"><span class="srv-name">没有找到匹配的服务器</span></div></div>`;
    return;
  }
  entities.forEach(e=>{
    const row = document.createElement("div");
    row.className="srv-row";
    const meta = document.createElement("div");
    meta.className="srv-meta";
    const n = document.createElement("div"); n.className="srv-name"; n.textContent = e.name || "(无名称)";
    const s = document.createElement("div"); s.className="srv-sub"; s.textContent = "ID: " + (e.entity_id || "") + " · 编号: " + (e.server_code || "") + " · 状态: " + (e.server_status ?? "未知");
    meta.appendChild(n); meta.appendChild(s);
    const act = document.createElement("div"); act.className="srv-actions";
    const btn = document.createElement("button"); btn.className="primary"; btn.textContent = "选择并进入";
    btn.addEventListener("click", ()=>{
      $("#enter-id").value = e.entity_id || "";
      lastServerID = e.entity_id || "";
      document.getElementById("card-enter").scrollIntoView({behavior:"smooth", block:"center"});
    });
    act.appendChild(btn);
    row.appendChild(meta); row.appendChild(act);
    box.appendChild(row);
  });
}
function renderEnterResult(d){
  const box = $("#enter-result");
  box.innerHTML = "";
  const kv = document.createElement("div"); kv.className="kv";
  kv.innerHTML = `<div class="k">服务器地址 (MC)</div><div class="v" style="color:#7cf0c6;font-weight:700">${d.ip || "-"}</div>
                  <div class="k">Host</div><div class="v">${d.host||"-"}</div>
                  <div class="k">Port</div><div class="v">${d.port||"-"}</div>
                  <div class="k">名称</div><div class="v">${d.name||"-"}</div>
                  <div class="k">OwnerUID</div><div class="v">${d.owner_uid||"-"}</div>`;
  box.appendChild(kv);
}
function renderAuthV2(d){
  const box = $("#authv2-result");
  box.innerHTML = "";
  const kv = document.createElement("div"); kv.className="kv";
  kv.innerHTML = `<div class="k">ChainInfo 长度</div><div class="v">${d.chain_info_len} bytes</div>
                  <div class="k">HEX (前120)</div><div class="v">${(d.chain_info_hex||"").slice(0,120)}${(d.chain_info_hex||"").length>120?"…":""}</div>
                  <div class="k">Base64</div><div class="v">${d.chain_info_b64||"-"}</div>`;
  box.appendChild(kv);
  toast("AuthV2 成功: "+d.chain_info_len+" bytes", "ok");
}
// ---------- main flow ----------
async function initStatus(){
  try{
    const j = await getStatus();
    const chip = $("#ver-chip");
    const ev = document.getElementById("ev");
    const pv = document.getElementById("pv");
    if(j && j.ok){
      const d = j.data || {};
      ev.textContent = d.engine_version || "-";
      pv.textContent = d.patch_version  || "-";
      chip.innerHTML = `<span class="dot ok"></span> Engine ${d.engine_version||"-"} · 会话 ${d.active_sessions||0}`;
    } else {
      chip.innerHTML = `<span class="dot err"></span> 状态查询失败`;
    }
  }catch(e){
    console.error(e);
    const chip = $("#ver-chip");
    chip.innerHTML = `<span class="dot err"></span> 接口异常`;
  }
}
async function doAuth(){
  const cookie = $("#cookie").value.trim();
  if(!cookie){ toast("请先粘贴 Cookie 内容", "err"); return; }
  setMsg("auth-msg","认证中…","");
  $("#btn-auth").disabled = true;
  try{
    const d = await call("/auth", {cookie});
    currentToken = d.token;
    localStorage.setItem("bunker_token", currentToken);
    setMsg("auth-msg", "认证成功 uid="+d.user_id, "ok");
    renderAuthResult(d);
    showFBToken(d);
    setEnabled("card-link", true);
    setEnabled("card-search", true);
    toast("登录成功","ok");
  }catch(e){
    setMsg("auth-msg", String(e.message||e), "err");
    toast(String(e.message||e), "err");
  }finally{
    $("#btn-auth").disabled = false;
  }
}
async function doLinkStart(){
  setMsg("link-msg","正在连接 Link 并发送 GameStart…","");
  $("#btn-link-start").disabled = true;
  try{
    await call("/link/start", {token: currentToken});
    setMsg("link-msg","✅ Link + GameStart 成功，可以继续 AuthV2", "ok");
    setEnabled("card-enter", true);
    toast("Link 已建立并发送 GameStart","ok");
  }catch(e){
    setMsg("link-msg", String(e.message||e), "err");
    toast(String(e.message||e),"err");
  }finally{
    $("#btn-link-start").disabled = false;
  }
}
async function doLinkStop(){
  try{
    await call("/link/stop", {token: currentToken});
    setMsg("link-msg","Link 已停止");
    toast("Link 已停止");
  }catch(e){
    toast(String(e.message||e),"err");
  }
}
async function doSearch(){
  const q = $("#search-q").value.trim();
  if(!q){ toast("请输入搜索关键字","err"); return; }
  $("#btn-search").disabled = true;
  try{
    const d = await call("/search", {token: currentToken, query: q});
    renderSearchResults(d.entities || []);
  }catch(e){
    toast(String(e.message||e),"err");
  }finally{
    $("#btn-search").disabled = false;
  }
}
async function doEnter(){
  const sid = $("#enter-id").value.trim();
  const pwd = $("#enter-pwd").value;
  if(!sid){ toast("请先填入 ServerID","err"); return; }
  $("#btn-enter").disabled = true;
  try{
    const d = await call("/rental/enter", {token: currentToken, server_id: sid, password: pwd});
    lastServerID = sid;
    renderEnterResult(d);
    toast("EnterRentalServerWorld 成功: "+d.ip, "ok");
  }catch(e){
    toast(String(e.message||e),"err");
  }finally{
    $("#btn-enter").disabled = false;
  }
}
async function doAuthV2(){
  let sid = $("#enter-id").value.trim() || lastServerID;
  if(!sid){ toast("请先进入租赁服再生成 AuthV2","err"); return; }
  setMsg("authv2-msg","请求 AuthV2…", "");
  $("#btn-authv2").disabled = true;
  try{
    const d = await call("/rental/authv2", {token: currentToken, server_id: sid});
    setMsg("authv2-msg","AuthV2成功，共 "+d.chain_info_len+" 字节", "ok");
    renderAuthV2(d);
  }catch(e){
    setMsg("authv2-msg", String(e.message||e), "err");
    toast(String(e.message||e),"err");
  }finally{
    $("#btn-authv2").disabled = false;
  }
}

// ---------- batch flow ----------
let batchTokens = []; // 批量进入成功后的 token 列表

function batchAddRow(cookie="", nick=""){
  const list = $("#batch-list");
  const idx = list.children.length;
  const row = document.createElement("div");
  row.className = "batch-row";
  row.innerHTML = `
    <div class="batch-row-header">
      <span class="batch-idx">#${idx+1}</span>
      <button class="batch-del" title="删除此行">✕</button>
    </div>
    <textarea class="batch-cookie" placeholder='粘贴 SAuth Cookie' rows="3"></textarea>
    <input class="batch-nick" type="text" placeholder="昵称（留空不改）">
    <div class="batch-status"></div>
  `;
  row.querySelector(".batch-cookie").value = cookie;
  row.querySelector(".batch-nick").value = nick;
  row.querySelector(".batch-del").addEventListener("click", ()=>{
    row.remove();
    batchReindex();
  });
  list.appendChild(row);
}
function batchReindex(){
  const rows = document.querySelectorAll("#batch-list .batch-row");
  rows.forEach((r, i)=>{
    r.querySelector(".batch-idx").textContent = "#"+(i+1);
  });
}
function batchCollect(){
  const rows = document.querySelectorAll("#batch-list .batch-row");
  const accounts = [];
  rows.forEach(r=>{
    const cookie = r.querySelector(".batch-cookie").value.trim();
    const nick = r.querySelector(".batch-nick").value.trim();
    if(cookie) accounts.push({cookie, nickname:nick});
  });
  return accounts;
}
function batchSetStatus(row, text, kind=""){
  const el = row.querySelector(".batch-status");
  if(!el) return;
  el.textContent = text;
  el.className = "batch-status " + kind;
}
function renderBatchEnterResult(data){
  const box = $("#batch-enter-result");
  box.innerHTML = "";
  batchTokens = [];
  (data.results||[]).forEach(r=>{
    const row = document.createElement("div");
    row.className = "batch-res-row " + (r.status==="ok"?"ok":"err");
    const meta = document.createElement("div");
    meta.className = "batch-res-meta";
    const name = document.createElement("div");
    name.className = "batch-res-name";
    name.textContent = `#${r.index+1} ${r.nickname||r.user_id||""} ${r.status==="ok"?"✅":"❌"}`;
    const detail = document.createElement("div");
    detail.className = "batch-res-detail";
    if(r.status==="ok"){
      detail.textContent = `uid=${r.user_id} ip=${r.ip||"-"} token=${r.token?r.token.slice(0,12)+"…":"-"}`;
      if(r.token) batchTokens.push(r.token);
    } else {
      detail.textContent = r.error || "未知错误";
    }
    meta.appendChild(name);
    meta.appendChild(detail);
    row.appendChild(meta);
    const actions = document.createElement("div");
    actions.className = "batch-res-actions";
    if(r.status==="ok" && r.token){
      const btn = document.createElement("button");
      btn.className = "ghost small";
      btn.textContent = "📋 复制Token";
      btn.addEventListener("click", ()=>{
        navigator.clipboard.writeText(r.token).then(()=>toast("Token 已复制","ok"));
      });
      actions.appendChild(btn);
    }
    row.appendChild(actions);
    box.appendChild(row);
  });
  // 汇总
  const summary = document.createElement("div");
  summary.className = "batch-res-row";
  summary.innerHTML = `<div class="batch-res-meta"><div class="batch-res-name">汇总</div>
    <div class="batch-res-detail">共 ${data.total} 个 · 成功 ${data.ok_count} · 失败 ${data.fail_count}</div></div>`;
  box.appendChild(summary);
}
function renderBatchAuthV2Result(data){
  const box = $("#batch-authv2-result");
  box.innerHTML = "";
  (data.results||[]).forEach(r=>{
    const row = document.createElement("div");
    row.className = "batch-res-row " + (r.status==="ok"?"ok":"err");
    const meta = document.createElement("div");
    meta.className = "batch-res-meta";
    const name = document.createElement("div");
    name.className = "batch-res-name";
    name.textContent = `#${r.index+1} ${r.nickname||""} ${r.status==="ok"?"✅":"❌"}`;
    const detail = document.createElement("div");
    detail.className = "batch-res-detail";
    if(r.status==="ok"){
      detail.textContent = `${r.variant||""} len=${r.chain_info_len} b64=${(r.chain_info_b64||"").slice(0,60)}…`;
    } else {
      detail.textContent = r.error || "未知错误";
    }
    meta.appendChild(name);
    meta.appendChild(detail);
    row.appendChild(meta);
    const actions = document.createElement("div");
    actions.className = "batch-res-actions";
    if(r.status==="ok" && r.chain_info_b64){
      const btn = document.createElement("button");
      btn.className = "ghost small";
      btn.textContent = "📋 复制B64";
      btn.addEventListener("click", ()=>{
        navigator.clipboard.writeText(r.chain_info_b64).then(()=>toast("ChainInfo B64 已复制","ok"));
      });
      actions.appendChild(btn);
    }
    row.appendChild(actions);
    box.appendChild(row);
  });
  // 汇总
  const summary = document.createElement("div");
  summary.className = "batch-res-row";
  summary.innerHTML = `<div class="batch-res-meta"><div class="batch-res-name">汇总</div>
    <div class="batch-res-detail">共 ${data.total} 个 · 成功 ${data.ok_count} · 失败 ${data.fail_count}</div></div>`;
  box.appendChild(summary);
}
async function doBatchEnter(){
  const accounts = batchCollect();
  if(accounts.length===0){ toast("请至少添加一个有效 Cookie","err"); return; }
  const sid = $("#batch-server-id").value.trim();
  if(!sid){ toast("请填写服务器号","err"); return; }
  const pwd = $("#batch-server-pwd").value;
  setMsg("batch-enter-msg", `正在批量进入 ${accounts.length} 个账号…（每个约 5-10 秒）`, "");
  $("#btn-batch-enter").disabled = true;
  try{
    const d = await call("/batch/enter", {accounts, server_id:sid, password:pwd});
    renderBatchEnterResult(d);
    setMsg("batch-enter-msg", `完成：成功 ${d.ok_count}/${d.total}`, d.ok_count===d.total?"ok":"err");
    toast(`批量进入完成：${d.ok_count}/${d.total}`, d.ok_count===d.total?"ok":"err");
  }catch(e){
    setMsg("batch-enter-msg", String(e.message||e), "err");
    toast(String(e.message||e),"err");
  }finally{
    $("#btn-batch-enter").disabled = false;
  }
}
async function doBatchAuthV2(){
  if(batchTokens.length===0){ toast("请先完成批量进入","err"); return; }
  const sid = $("#batch-server-id").value.trim();
  if(!sid){ toast("请填写服务器号","err"); return; }
  setMsg("batch-authv2-msg", `正在批量生成 AuthV2（${batchTokens.length} 个）…`, "");
  $("#btn-batch-authv2").disabled = true;
  try{
    const d = await call("/batch/authv2", {tokens:batchTokens, server_id:sid});
    renderBatchAuthV2Result(d);
    setMsg("batch-authv2-msg", `完成：成功 ${d.ok_count}/${d.total}`, d.ok_count===d.total?"ok":"err");
    toast(`批量AuthV2完成：${d.ok_count}/${d.total}`, d.ok_count===d.total?"ok":"err");
  }catch(e){
    setMsg("batch-authv2-msg", String(e.message||e), "err");
    toast(String(e.message||e),"err");
  }finally{
    $("#btn-batch-authv2").disabled = false;
  }
}

// ---------- regbot flow (批量注册机) ----------
let regbotLastResults = [];
let regbotAbortCtrl = null;

async function regbotRefreshStats(){
  try{
    const j = await (await fetch(API + "/batch/stats")).json();
    if(j && j.ok){
      const d = j.data || {};
      const sfzEl = document.getElementById("rb-sfz-cnt");
      const prxEl = document.getElementById("rb-proxy-cnt");
      if(sfzEl) sfzEl.textContent = `· SFZ: ${d.sfz_count||0}`;
      if(prxEl) prxEl.textContent = `· Proxy: ${d.proxy_count||0}`;
    }
  }catch(e){}
}
async function regbotUploadSFZ(){
  const f = document.getElementById("rb-sfz-file");
  const msg = document.getElementById("rb-sfz-msg");
  if(!f.files || f.files.length===0){ toast("请先选择 SFZ 文件","err"); return; }
  msg.textContent = "上传中…";
  msg.classList.remove("ok","err");
  const fd = new FormData();
  fd.append("sfz_file", f.files[0]);
  try{
    const r = await fetch(API + "/batch/sfz/upload", {method:"POST", body: fd});
    const j = await r.json();
    if(!j.ok) throw new Error(j.msg || "上传失败");
    msg.textContent = `✅ SFZ 导入成功，共 ${j.data.count||0} 条`;
    msg.classList.add("ok");
    toast(`SFZ 导入 ${j.data.count} 条`,"ok");
    regbotRefreshStats();
  }catch(e){
    msg.textContent = "❌ " + String(e.message||e);
    msg.classList.add("err");
    toast(String(e.message||e),"err");
  }
}
async function regbotUploadProxy(){
  const f = document.getElementById("rb-proxy-file");
  const msg = document.getElementById("rb-proxy-msg");
  if(!f.files || f.files.length===0){ toast("请先选择代理文件","err"); return; }
  msg.textContent = "上传中…";
  msg.classList.remove("ok","err");
  const fd = new FormData();
  fd.append("proxy_file", f.files[0]);
  try{
    const r = await fetch(API + "/batch/proxy/upload", {method:"POST", body: fd});
    const j = await r.json();
    if(!j.ok) throw new Error(j.msg || "上传失败");
    msg.textContent = `✅ 代理导入成功，共 ${j.data.count||0} 条`;
    msg.classList.add("ok");
    toast(`代理导入 ${j.data.count} 条`,"ok");
    regbotRefreshStats();
  }catch(e){
    msg.textContent = "❌ " + String(e.message||e);
    msg.classList.add("err");
    toast(String(e.message||e),"err");
  }
}
function regbotSetExportEnabled(v){
  ["rb-export-csv","rb-export-txt","rb-export-sauth","rb-export-json"].forEach(id=>{
    const el = document.getElementById(id);
    if(el) el.disabled = !v;
  });
}
function regbotRenderSummary(data){
  const box = document.getElementById("rb-summary");
  if(!box) return;
  const stats = (data && data.stats) || {};
  const total = (data && data.total) || 0;
  const succ = (data && data.success) || 0;
  const fail = (data && data.failed) || 0;
  const chips = [
    {label:"总数", v:total, c:""},
    {label:"成功", v:succ, c:"ok"},
    {label:"失败", v:fail, c:"err"},
    {label:"OCR未就绪", v:stats.ocr_missing||0, c:"err"},
    {label:"识别失败", v:stats.captcha_fail||0, c:"warn"},
    {label:"SFZ受限", v:stats.sfz_limit||0, c:"warn"},
    {label:"用户已存在", v:stats.username_exist||0, c:""},
    {label:"风控拦截", v:stats.risk_control||0, c:"warn"},
  ];
  box.innerHTML = chips.map(c=>`<div class="chip ${c.c}"><b>${c.v}</b> ${c.label}</div>`).join("");
}
function regbotStatusClass(s){
  switch(s){
    case "success": return "ok";
    case "ocr_missing": return "err";
    case "sfz_limit":
    case "sfz_freq":
    case "realname_error": return "warn";
    case "captcha_fail":
    case "username_exist":
    case "risk_control": return "warn";
    default: return "err";
  }
}
function regbotRenderResults(results){
  const box = document.getElementById("rb-results");
  if(!box) return;
  box.innerHTML = "";
  if(!results || results.length===0){
    box.innerHTML = `<div class="hint" style="text-align:center;padding:20px 0">暂无结果，点击上方「开始批量注册」启动任务。</div>`;
    return;
  }
  results.forEach(r=>{
    const row = document.createElement("div");
    row.className = "regbot-result-row " + regbotStatusClass(r.status);
    const statusIcon = r.status==="success" ? "✅" : r.status==="ocr_missing" ? "⛔" : r.status==="risk_control" ? "🛡" : r.status==="captcha_fail" ? "🔍" : r.status==="username_exist" ? "🔁" : r.status==="sfz_limit" ? "🪪" : "❌";
    const meta = document.createElement("div");
    meta.className = "regbot-result-meta";
    const header = document.createElement("div");
    header.className = "regbot-result-head";
    header.innerHTML = `<span class="regbot-result-idx">#${r.index+1}</span>
      <span class="regbot-result-user mono">${r.username||""}</span>
      <span class="regbot-result-status status-${r.status||'error'}">${statusIcon} ${r.DisplayMsg || r.msg || r.status || ""}</span>`;
    const detail = document.createElement("div");
    detail.className = "regbot-result-detail";
    detail.innerHTML = `<span>pwd: <span class="mono">${r.password||""}</span></span>
      <span>昵称: ${r.nickname||""}</span>
      <span>实名: ${r.sfz_name||""} ${(r.sfz_number||"").slice(0,6)}****</span>
      ${r.proxy_used?`<span>代理: ${r.proxy_used}</span>`:""}
      ${r.sauth_len?`<span>SAuth: ${r.sauth_len}b</span>`:""}`;
    meta.appendChild(header);
    meta.appendChild(detail);
    const actions = document.createElement("div");
    actions.className = "regbot-result-actions";
    if(r.status==="success" && r.sauth_cookie){
      const b1 = document.createElement("button");
      b1.className = "ghost small";
      b1.textContent = "🎫 SAuth";
      b1.addEventListener("click", ()=>{
        navigator.clipboard.writeText(r.sauth_cookie).then(()=>toast("SAuth 已复制","ok"));
      });
      actions.appendChild(b1);
    }
    if(r.username && r.password){
      const b2 = document.createElement("button");
      b2.className = "ghost small";
      b2.textContent = "📋 账号";
      b2.addEventListener("click", ()=>{
        navigator.clipboard.writeText(r.username+"----"+r.password).then(()=>toast("账号密码已复制","ok"));
      });
      actions.appendChild(b2);
    }
    row.appendChild(meta);
    row.appendChild(actions);
    box.appendChild(row);
  });
}
async function regbotStart(){
  const count = parseInt(document.getElementById("rb-count").value)||0;
  const conc = parseInt(document.getElementById("rb-concurrency").value)||1;
  const uprefix = document.getElementById("rb-uprefix").value;
  const nprefix = document.getElementById("rb-nprefix").value;
  const password = document.getElementById("rb-password").value;
  const delay = parseInt(document.getElementById("rb-delay").value)||0;
  const useDirect = document.getElementById("rb-use-direct").checked;
  const genSauth = document.getElementById("rb-gen-sauth").checked;
  const useProxy = document.getElementById("rb-use-proxy").checked;

  if(count<=0){ toast("注册数量需 ≥1","err"); return; }
  regbotLastResults = [];
  regbotSetExportEnabled(false);

  const progress = document.getElementById("rb-progress");
  progress.classList.remove("hidden");
  document.getElementById("rb-progress-fill").style.width = "2%";
  document.getElementById("rb-progress-text").textContent = `注册中… 0 / ${count}`;
  setMsg("rb-msg", `批量注册中（${count} 个，并发 ${conc}）…`, "");
  document.getElementById("rb-start").disabled = true;
  document.getElementById("rb-stop").disabled = false;

  regbotAbortCtrl = new AbortController();
  try{
    const r = await fetch(API + "/batch/register", {
      method:"POST",
      headers:{"Content-Type":"application/json"},
      body: JSON.stringify({
        count, concurrency:conc,
        username_prefix:uprefix, nickname_prefix:nprefix,
        password, delay_sec:delay,
        use_direct:useDirect, gen_sauth:genSauth,
        use_proxy_per_task:useProxy,
      }),
      signal: regbotAbortCtrl.signal,
    });
    const j = await r.json();
    if(!j.ok) throw new Error(j.msg || ("HTTP "+r.status));
    const d = j.data;
    regbotLastResults = d.results || [];
    regbotRenderSummary(d);
    regbotRenderResults(regbotLastResults);
    document.getElementById("rb-progress-fill").style.width = "100%";
    document.getElementById("rb-progress-text").textContent = `完成 ${d.success||0} / ${d.total||count}`;
    setMsg("rb-msg", `完成：成功 ${d.success||0} / ${d.total||count}`, (d.success==d.total?"ok":"err"));
    regbotSetExportEnabled(true);
    toast(`批量注册完成：${d.success||0}/${d.total||count}`, (d.success==d.total?"ok":"err"));
  }catch(e){
    if(e && e.name === "AbortError"){
      setMsg("rb-msg", "已停止", "err");
      toast("任务已停止");
    } else {
      setMsg("rb-msg", String(e.message||e), "err");
      toast(String(e.message||e),"err");
    }
  }finally{
    document.getElementById("rb-start").disabled = false;
    document.getElementById("rb-stop").disabled = true;
    regbotAbortCtrl = null;
  }
}
function regbotStop(){
  if(regbotAbortCtrl){
    regbotAbortCtrl.abort();
  }
}
function regbotDownloadBlob(blob, filename){
  const a = document.createElement("a");
  const url = URL.createObjectURL(blob);
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  setTimeout(()=>{a.remove(); URL.revokeObjectURL(url);}, 0);
}
async function regbotExport(fmt){
  if(regbotLastResults.length===0){ toast("没有可导出的结果","err"); return; }
  const onlyOk = document.getElementById("rb-success-only").checked;
  const payload = {results: regbotLastResults, format: fmt, success_only: onlyOk};
  const btn = {
    csv: ["funauth_batch_register.csv", "text/csv"],
    txt_accounts: ["funauth_accounts.txt", "text/plain"],
    txt_sauth: ["funauth_sauth.txt", "text/plain"],
    json: ["funauth_results.json", "application/json"],
  }[fmt] || ["export.txt","text/plain"];
  try{
    const r = await fetch(API + "/batch/register/export", {
      method:"POST",
      headers:{"Content-Type":"application/json"},
      body: JSON.stringify(payload),
    });
    if(fmt==="json"){
      const j = await r.json();
      const blob = new Blob([JSON.stringify(j, null, 2)], {type:"application/json"});
      regbotDownloadBlob(blob, btn[0]);
    } else {
      const blob = await r.blob();
      regbotDownloadBlob(blob, btn[0]);
    }
    toast("已导出: "+btn[0],"ok");
  }catch(e){
    toast(String(e.message||e),"err");
  }
}

// ---------- tabs ----------
function switchTab(id){
  document.querySelectorAll(".tab").forEach(t=>t.classList.toggle("active", t.dataset.tab===id));
  document.querySelectorAll(".tab-panel").forEach(p=>p.classList.toggle("active", p.id===id));
  localStorage.setItem("bunker_tab", id);
  if(id === "tab-regbot") regbotRefreshStats();
}
document.addEventListener("click", (e)=>{
  const t = e.target.closest(".tab");
  if(!t) return;
  switchTab(t.dataset.tab);
});

// ---------- register flow ----------
async function regPreset(){
  const r = await fetch(API + "/register/preset").then(r=>r.json());
  if(!r.ok) return;
  $("#reg-user").value = r.data.username || "";
  $("#reg-pass").value = r.data.password || "";
}
async function doRegister(){
  const user = $("#reg-user").value.trim();
  const pass = $("#reg-pass").value.trim();
  const name = $("#reg-name").value.trim();
  const idno = $("#reg-id").value.trim();
  const loginOnly = $("#reg-loginonly").checked;
  if(loginOnly && !user){ toast("仅登录模式下 username 不能为空","err"); return; }
  if(!loginOnly && (!name || !idno)){ toast("请填写姓名和身份证号，或勾选「仅登录」","err"); return; }
  setMsg("reg-msg","执行中（注册+实名+登录，最长 90 秒）…","");
  $("#btn-register").disabled = true;
  try{
    const d = await call("/register/com4399", {
      username:user, password:pass, real_name:name, id_card:idno, login_only:loginOnly
    });
    setMsg("reg-msg","完成，得到 "+(d.cookie_len||0)+" 字节 SAuth","ok");
    const box = $("#reg-result");
    box.classList.remove("hidden");
    box.textContent = JSON.stringify(d, null, 2);
    if(d.cookie){
      $("#reg-actions").classList.remove("hidden");
      $("#reg-actions").dataset.cookie = d.cookie;
      toast("4399 SAuth 生成成功","ok");
    }
  }catch(e){
    setMsg("reg-msg", String(e.message||e), "err");
    toast(String(e.message||e),"err");
  }finally{
    $("#btn-register").disabled = false;
  }
}
function regFill(){
  const cookie = $("#reg-actions").dataset.cookie;
  if(!cookie){ toast("没有可用的 cookie","err"); return; }
  $("#cookie").value = cookie;
  switchTab("tab-auth");
  setMsg("auth-msg","已从注册页回填，点「登录」即可","ok");
  toast("已回填到登录认证页面","ok");
}
document.addEventListener("DOMContentLoaded", ()=>{
  // regbot (批量注册机)
  document.getElementById("rb-upload-sfz")?.addEventListener("click", regbotUploadSFZ);
  document.getElementById("rb-upload-proxy")?.addEventListener("click", regbotUploadProxy);
  document.getElementById("rb-start")?.addEventListener("click", regbotStart);
  document.getElementById("rb-stop")?.addEventListener("click", regbotStop);
  document.getElementById("rb-export-csv")?.addEventListener("click", ()=>regbotExport("csv"));
  document.getElementById("rb-export-txt")?.addEventListener("click", ()=>regbotExport("txt_accounts"));
  document.getElementById("rb-export-sauth")?.addEventListener("click", ()=>regbotExport("txt_sauth"));
  document.getElementById("rb-export-json")?.addEventListener("click", ()=>regbotExport("json"));
  // 初始化结果展示 & 统计
  regbotRenderResults([]);
  regbotRefreshStats();

  // register
  $("#btn-reg-preset").addEventListener("click", regPreset);
  $("#btn-reg-preset2").addEventListener("click", regPreset);
  $("#btn-register").addEventListener("click", doRegister);
  $("#btn-reg-fill").addEventListener("click", regFill);

  // fbtoken box
  if(document.getElementById("btn-copy-fbtoken")){
    document.getElementById("btn-copy-fbtoken").addEventListener("click", ()=>copyText(document.getElementById("fbtoken")));
  }
  if(document.getElementById("btn-copy-authurl")){
    document.getElementById("btn-copy-authurl").addEventListener("click", ()=>copyText(document.getElementById("auth-server-url")));
  }

  $("#btn-auth").addEventListener("click", doAuth);
  $("#btn-link-start").addEventListener("click", doLinkStart);
  $("#btn-link-stop").addEventListener("click", doLinkStop);
  $("#btn-search").addEventListener("click", doSearch);
  $("#btn-enter").addEventListener("click", doEnter);
  $("#btn-authv2").addEventListener("click", doAuthV2);

  // batch
  batchAddRow();
  $("#btn-batch-add").addEventListener("click", ()=>batchAddRow());
  $("#btn-batch-clear").addEventListener("click", ()=>{
    $("#batch-list").innerHTML = "";
    batchAddRow();
  });
  $("#btn-batch-enter").addEventListener("click", doBatchEnter);
  $("#btn-batch-authv2").addEventListener("click", doBatchAuthV2);

  // restore tab & token persistence
  const savedTab = localStorage.getItem("bunker_tab") || "tab-auth";
  switchTab(savedTab);
  if(currentToken){
    setEnabled("card-link", true);
    setEnabled("card-search", true);
    setMsg("auth-msg","已加载上一次 Token (可直接点刷新重新登录)","ok");
  }
  initStatus();
});
