(function(){
'use strict';

const KNOWLEDGE_TYPES = [
  {id:'raw', label:'Raw', icon:'📥', desc:'原始材料'},
  {id:'domain', label:'Domain', icon:'🗺️', desc:'领域地图'},
  {id:'concept', label:'Concept', icon:'🧠', desc:'概念模型'},
  {id:'entity', label:'Entity', icon:'🧩', desc:'实体档案'},
  {id:'decision', label:'Decision', icon:'⚖️', desc:'决策记录'},
  {id:'synthesis', label:'Synthesis', icon:'🔭', desc:'综合认知'}
];
const KB_INBOX_KEY = 'shoreos:knowledge:inbox:v1';
let kbEntries = [];
let kbType = 'all';

function money(n){
  n = Number(n)||0;
  if(Math.abs(n)>=10000) return '¥'+(n/10000).toFixed(1)+'万';
  return '¥'+Math.round(n).toLocaleString('zh-CN');
}
function pad2(n){ return String(n).padStart(2,'0'); }
function monthIndex(year, month){ return Number(year)*12 + Number(month)-1; }
function projectionMonthIndex(d, monthlyExtra){
  if(!d || d.fire<=0) return null;
  if(d.investableAssets>=d.fire) return monthIndex(new Date().getFullYear(),new Date().getMonth()+1);
  const monthlySave = Math.max((Number(d.eSave)||0)/12 + (Number(monthlyExtra)||0), 0);
  if(monthlySave<=0 && (Number(d.ret)||0)<=0) return null;
  let a = Math.max(Number(d.eAssets)||0,0);
  const r = Math.max(Number(d.ret)||0,0)/12;
  let m = 0;
  while(a < d.fire && m < 2400){ a = a*(1+r)+monthlySave; m++; }
  if(a < d.fire) return null;
  const now = new Date();
  return monthIndex(now.getFullYear(), now.getMonth()+1)+m;
}
function monthIndexLabel(idx){
  if(idx===null || idx===undefined) return '--';
  const y = Math.floor(idx/12), m = idx%12+1;
  return y+'年'+m+'月';
}
function diffLabel(base, next){
  if(base===null || next===null) return '无法估算';
  const diff = base-next;
  if(diff<=0) return '日期基本不变';
  if(diff<12) return '提前约 '+diff+' 个月';
  return '提前约 '+Math.floor(diff/12)+' 年 '+(diff%12)+' 个月';
}
function percent(n){ return Math.max(0,Math.min(Number(n)||0,100)); }

async function fetchLedgerBaseline(){
  try{
    if(typeof ShoreAPI === 'undefined' || typeof cbReady === 'undefined' || !cbReady) return null;
    return await ShoreAPI.request('/api/v1/ledger/spending-baseline?windowDays=30');
  }catch(e){ return null; }
}

function ensureFreedomSpeedCard(){
  if(document.getElementById('freedomSpeedCard')) return;
  const tab = document.getElementById('dashTab0');
  if(!tab) return;
  const card = document.createElement('section');
  card.id = 'freedomSpeedCard';
  card.className = 'freedom-speed-card fade-in';
  card.innerHTML = `
    <div class="freedom-speed-head">
      <div><div class="freedom-speed-kicker">MY FREEDOM SPEED</div><div class="freedom-speed-title">🚀 我的自由速度</div></div>
      <div class="freedom-speed-eta">按当前速度<strong id="fsEta">--</strong></div>
    </div>
    <div class="freedom-speed-grid">
      <div class="freedom-speed-metric"><b id="fsMonthlySave">--</b><span>当前月可投入</span></div>
      <div class="freedom-speed-metric"><b id="fsMonthlyFloor">--</b><span>最低生活成本/月</span></div>
      <div class="freedom-speed-metric"><b id="fsTarget">--</b><span>FIRE 目标</span></div>
      <div class="freedom-speed-metric"><b id="fsGap">--</b><span>距离目标</span></div>
    </div>
    <div class="freedom-speed-track"><div class="freedom-speed-fill" id="fsFill" style="width:0%"></div></div>
    <div class="freedom-speed-caption"><span id="fsProgressText">自由进度 --</span><span id="fsYearsText">--</span></div>
    <div class="freedom-levers">
      <button class="freedom-lever" type="button"><strong>每月多留下 ¥500</strong><small id="fsPlus500">--</small></button>
      <button class="freedom-lever" type="button"><strong>每月多留下 ¥1,000</strong><small id="fsPlus1000">--</small></button>
    </div>
    <div class="freedom-ledger-note" id="fsLedger">账本基线加载后，这里会显示“真实支出速度 → 自由速度”的影响。</div>`;
  tab.insertBefore(card, tab.firstChild);
}

async function renderFreedomSpeed(){
  ensureFreedomSpeedCard();
  if(typeof window.calcAll !== 'function') return;
  const d = window.calcAll();
  if(!d) return;
  const base = projectionMonthIndex(d,0);
  const plus500 = projectionMonthIndex(d,500);
  const plus1000 = projectionMonthIndex(d,1000);
  const nowIdx = monthIndex(new Date().getFullYear(),new Date().getMonth()+1);
  const remaining = base===null ? null : Math.max(base-nowIdx,0);
  const set = (id,text)=>{ const el=document.getElementById(id); if(el) el.textContent=text; };
  set('fsEta', d.gap<=0 ? '已经达成' : monthIndexLabel(base));
  set('fsMonthlySave', money((d.eSave||0)/12));
  set('fsMonthlyFloor', money((d.eQA||0)/12));
  const hidden = typeof assetHide !== 'undefined' && assetHide;
  set('fsTarget', hidden ? '****' : money(d.fire));
  set('fsGap', hidden ? '****' : money(d.gap));
  set('fsProgressText','自由进度 '+percent(d.fi).toFixed(1)+'%');
  set('fsYearsText', remaining===null ? '--' : (remaining<12 ? remaining+'个月' : (remaining/12).toFixed(1)+'年'));
  set('fsPlus500',diffLabel(base,plus500));
  set('fsPlus1000',diffLabel(base,plus1000));
  const fill=document.getElementById('fsFill'); if(fill) fill.style.width=percent(d.fi)+'%';

  const ledger = await fetchLedgerBaseline();
  const note = document.getElementById('fsLedger');
  if(!note) return;
  if(!ledger){
    note.innerHTML = '当前先按 FIRE 档案参数计算；Ledger 有可用滚动基线后，会自动补上真实支出速度。';
    return;
  }
  const targetAnnual=(Number(ledger.targetAnnualExpenseCents)||0)/100;
  const actualAnnual=(Number(ledger.actualPaceAnnualExpenseCents)||0)/100;
  const monthlyTarget=targetAnnual/12, monthlyActual=actualAnnual/12;
  const delta=monthlyActual-monthlyTarget;
  const coverage=String(ledger.dataCoverage||'');
  if(!targetAnnual || !actualAnnual || coverage==='insufficient'){
    note.innerHTML='Ledger 已连接，但当前覆盖不足以校正自由速度；暂时沿用 FIRE 档案参数。';
    return;
  }
  // Ledger 只校正“当前速度”，不改同一 FIRE 情景的长期目标。
  // 实际支出高于预算会降低每月可投入金额，低于预算则提高。
  const ledgerExtra = -delta;
  const adjustedMonthlySave = Math.max((Number(d.eSave)||0)/12 + ledgerExtra, 0);
  const actualBase = projectionMonthIndex(d, ledgerExtra);
  const actualPlus500 = projectionMonthIndex(d, ledgerExtra+500);
  const actualPlus1000 = projectionMonthIndex(d, ledgerExtra+1000);
  const actualRemaining = actualBase===null ? null : Math.max(actualBase-nowIdx,0);
  set('fsEta', d.gap<=0 ? '已经达成' : monthIndexLabel(actualBase));
  set('fsMonthlySave', money(adjustedMonthlySave));
  set('fsYearsText', actualRemaining===null ? '--' : (actualRemaining<12 ? actualRemaining+'个月' : (actualRemaining/12).toFixed(1)+'年'));
  set('fsPlus500',diffLabel(actualBase,actualPlus500));
  set('fsPlus1000',diffLabel(actualBase,actualPlus1000));
  if(delta>0){
    note.innerHTML=`账本校正：最近支出节奏约 <b>${money(monthlyActual)}/月</b>，比目标高 <b>${money(delta)}/月</b>；已把这部分作为“自由减速”计入预计日期。`;
  }else{
    note.innerHTML=`账本校正：最近支出节奏约 <b>${money(monthlyActual)}/月</b>，比目标低 <b>${money(Math.abs(delta))}/月</b>；已把这部分作为“自由加速”计入预计日期。`;
  }
}

function getInbox(){
  try{ const v=JSON.parse(localStorage.getItem(KB_INBOX_KEY)||'[]'); return Array.isArray(v)?v:[]; }catch(e){ return []; }
}
function saveInbox(items){ localStorage.setItem(KB_INBOX_KEY, JSON.stringify(items)); }

function knowledgeSectionHTML(){
  return `<div class="scroll-section" id="sec-knowledge">
    <div class="app-header"><div class="greeting">🧠 知识库</div><div class="subtitle">不是文档仓库，而是 ShoreOS 的长期结构化记忆</div></div>
    <div class="knowledge-shell">
      <div class="knowledge-hero"><h2>Knowledge</h2><p>Raw → 提炼 → Domain / Concept / Entity / Decision → Synthesis。原始事实和推断分开保存，冲突显式记录，不用新结论覆盖旧证据。</p></div>
      <div class="knowledge-search"><input id="knowledgeSearch" type="search" placeholder="搜索标题、摘要、标签…"></div>
      <div class="knowledge-type-grid" id="knowledgeTypes"></div>
      <div class="knowledge-inbox">
        <div class="knowledge-inbox-title"><span>📥 Inbox</span><span class="knowledge-inbox-count" id="knowledgeInboxCount">0 条待整理</span></div>
        <textarea id="knowledgeInboxInput" placeholder="先收集，不判断。想到什么先丢进来。"></textarea>
        <div class="knowledge-inbox-actions"><button type="button" id="knowledgeInboxSave">存入 Raw Inbox</button></div>
      </div>
      <div class="knowledge-list" id="knowledgeList"></div>
    </div>
  </div>`;
}

function ensureKnowledgePage(){
  if(document.getElementById('sec-knowledge')) return;
  const phone=document.getElementById('phone'); if(!phone) return;
  const anchor=document.getElementById('sec-settings') || document.getElementById('sec-profile');
  const holder=document.createElement('div'); holder.innerHTML=knowledgeSectionHTML();
  const sec=holder.firstElementChild;
  phone.insertBefore(sec, anchor || null);

  const menu=document.getElementById('hamburgerMenu');
  const footer=menu && menu.querySelector('.hm-footer');
  if(menu && !menu.querySelector('[data-nav="knowledge"]')){
    const btn=document.createElement('button');
    btn.className='hm-item'; btn.dataset.nav='knowledge'; btn.type='button';
    btn.innerHTML='<span class="hm-icon">🧠</span>知识库';
    btn.onclick=()=>window.navigateTo('knowledge');
    menu.insertBefore(btn,footer||null);
  }
  renderKnowledgeTypes();
  document.getElementById('knowledgeSearch')?.addEventListener('input',renderKnowledgeList);
  document.getElementById('knowledgeInboxSave')?.addEventListener('click',captureKnowledgeInbox);
}

function renderKnowledgeTypes(){
  const root=document.getElementById('knowledgeTypes'); if(!root) return;
  const all=[{id:'all',label:'全部',icon:'🧠',desc:'全部知识'}].concat(KNOWLEDGE_TYPES);
  root.innerHTML=all.map(t=>`<button class="knowledge-type ${kbType===t.id?'active':''}" data-kb-type="${t.id}" type="button"><i>${t.icon}</i><strong>${t.label}</strong><small>${t.desc}</small></button>`).join('');
  root.querySelectorAll('[data-kb-type]').forEach(b=>b.addEventListener('click',()=>{ kbType=b.dataset.kbType; renderKnowledgeTypes(); renderKnowledgeList(); }));
}

function captureKnowledgeInbox(){
  const input=document.getElementById('knowledgeInboxInput');
  const text=(input?.value||'').trim(); if(!text) return;
  const items=getInbox();
  items.unshift({id:'raw-'+Date.now(),type:'raw',title:text.slice(0,36),summary:text,source:'shoreos-inbox',status:'unprocessed',createdAt:new Date().toISOString(),tags:['inbox']});
  saveInbox(items); input.value='';
  renderKnowledgeList();
  if(typeof window.showToast==='function') window.showToast('已放入 Raw Inbox，尚未自动提炼','success');
}

async function loadKnowledgeIndex(){
  try{
    const res=await fetch('/knowledge-index.json?v=1',{cache:'no-store'});
    if(res.ok){ const data=await res.json(); kbEntries=Array.isArray(data.entries)?data.entries:[]; }
  }catch(e){ kbEntries=[]; }
  renderKnowledgeList();
}

function renderKnowledgeList(){
  const root=document.getElementById('knowledgeList'); if(!root) return;
  const q=(document.getElementById('knowledgeSearch')?.value||'').trim().toLowerCase();
  const inbox=getInbox();
  const all=inbox.concat(kbEntries);
  const rows=all.filter(x=>{
    if(kbType!=='all' && x.type!==kbType) return false;
    if(!q) return true;
    return [x.title,x.summary,x.type,(x.tags||[]).join(' ')].join(' ').toLowerCase().includes(q);
  });
  const cnt=document.getElementById('knowledgeInboxCount'); if(cnt) cnt.textContent=inbox.filter(x=>x.status==='unprocessed').length+' 条待整理';
  if(!rows.length){ root.innerHTML='<div class="knowledge-empty">没有匹配的知识。</div>'; return; }
  root.innerHTML=rows.map(x=>`<article class="knowledge-entry">
    <div class="knowledge-entry-top"><span>${(KNOWLEDGE_TYPES.find(t=>t.id===x.type)||{icon:'🧠'}).icon}</span><span class="knowledge-badge">${String(x.type||'unknown').toUpperCase()}</span>${x.status==='unprocessed'?'<span class="knowledge-badge">待整理</span>':''}</div>
    <h3>${escapeHTML(x.title||'未命名')}</h3><p>${escapeHTML(x.summary||'')}</p>
    <div class="knowledge-entry-meta">${escapeHTML((x.tags||[]).join(' · '))}${x.source?' · '+escapeHTML(x.source):''}</div>
  </article>`).join('');
}
function escapeHTML(s){ return String(s||'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c])); }

function installNavigation(){
  if(window.__shoreosKnowledgeNavInstalled) return;
  window.__shoreosKnowledgeNavInstalled=true;
  const oldNavigate=window.navigateTo;
  window.navigateTo=function(module){
    if(module!=='knowledge') return oldNavigate(module);
    ensureKnowledgePage();
    document.querySelectorAll('.scroll-section').forEach(s=>s.classList.remove('active'));
    document.getElementById('sec-knowledge')?.classList.add('active');
    document.querySelectorAll('[data-nav]').forEach(x=>x.classList.toggle('active',x.dataset.nav==='knowledge'));
    const title=document.getElementById('appPageTitle'); if(title) title.textContent='知识库';
    const assetBtn=document.getElementById('assetHideBtn'); if(assetBtn) assetBtn.style.visibility='hidden';
    document.getElementById('hamburgerMenu')?.classList.remove('active');
    document.getElementById('hamburgerOverlay')?.classList.remove('active');
    document.getElementById('phone')?.scrollTo?.({top:0,behavior:'instant'});
    renderKnowledgeList();
  };
}

function hookRecalc(){
  if(window.__shoreosFreedomHooked) return;
  window.__shoreosFreedomHooked=true;
  const old=window.recalc;
  window.recalc=function(){ const r=old.apply(this,arguments); Promise.resolve().then(renderFreedomSpeed); return r; };
}

function boot(){
  if(typeof window.calcAll!=='function' || typeof window.navigateTo!=='function'){
    setTimeout(boot,80); return;
  }
  ensureFreedomSpeedCard();
  ensureKnowledgePage();
  installNavigation();
  hookRecalc();
  loadKnowledgeIndex();
  renderFreedomSpeed();
  const v=document.getElementById('topbarVersion'); if(v) v.textContent='ShoreOS v5.8.0';
  const hv=document.getElementById('hmVersion'); if(hv) hv.textContent='v5.8.0';
}

boot();
})();
