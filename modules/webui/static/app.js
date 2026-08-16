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
  box.textContent = JSON.stringify(d, null, 2);
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

// ---------- tabs ----------
function switchTab(id){
  document.querySelectorAll(".tab").forEach(t=>t.classList.toggle("active", t.dataset.tab===id));
  document.querySelectorAll(".tab-panel").forEach(p=>p.classList.toggle("active", p.id===id));
  localStorage.setItem("bunker_tab", id);
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
  // register
  $("#btn-reg-preset").addEventListener("click", regPreset);
  $("#btn-reg-preset2").addEventListener("click", regPreset);
  $("#btn-register").addEventListener("click", doRegister);
  $("#btn-reg-fill").addEventListener("click", regFill);

  $("#btn-auth").addEventListener("click", doAuth);
  $("#btn-link-start").addEventListener("click", doLinkStart);
  $("#btn-link-stop").addEventListener("click", doLinkStop);
  $("#btn-search").addEventListener("click", doSearch);
  $("#btn-enter").addEventListener("click", doEnter);
  $("#btn-authv2").addEventListener("click", doAuthV2);

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
