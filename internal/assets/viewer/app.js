'use strict';
(() => {
 const data=JSON.parse(document.getElementById('spec-data').textContent);
 const {spec,report}=data;
 spec.usecases ||= {}; spec.transitions ||= {}; spec.states ||= {}; spec.events ||= {};
 report.usecases ||= []; report.scenarios ||= []; report.checks ||= []; report.diagnostics ||= [];
 const el=id=>document.getElementById(id);
 const text=(node,value)=>{node.textContent=value==null?'':String(value);return node;};
 const make=(tag,value,cls)=>{const n=document.createElement(tag);if(value!==undefined)text(n,value);if(cls)n.className=cls;return n;};
 const labels={transitions:'遷移',pairs:'遷移ペア',primePaths:'プライムパス',boundedPaths:'境界付き全パス',conditions:'条件',multipleConditions:'複合条件'};
 let currentUC='',zoom=1;
 const detail=(title,obj)=>{el('detail').replaceChildren(make('h2',title),make('pre',typeof obj==='string'?obj:JSON.stringify(obj,null,2)));};
 function draw(id=''){
  currentUC=id;const template=el('diagram-'+(id?'uc-'+id:'system'));el('graph').replaceChildren(template.content.cloneNode(true));
  const svg=el('graph').querySelector('svg');if(!svg)return;const vb=svg.viewBox.baseVal;zoom=Math.min(1,Math.max(.15,(el('graph').clientWidth-22)/vb.width));setZoom();
  svg.querySelectorAll('[data-transition]').forEach(n=>{n.addEventListener('click',()=>{const id=n.dataset.transition;highlight([id]);detail(id,spec.transitions[id]);});});
  svg.querySelectorAll('[data-state]').forEach(n=>n.addEventListener('click',()=>detail(n.dataset.state,spec.states[n.dataset.state])));
 }
 function setZoom(){const svg=el('graph').querySelector('svg');if(!svg)return;const v=svg.viewBox.baseVal;svg.style.width=(v.width*zoom)+'px';svg.style.height=(v.height*zoom)+'px';el('zoom-label').textContent=Math.round(zoom*100)+'%';}
 function highlight(path){const selected=new Set(path);const svg=el('graph').querySelector('svg');if(!svg)return;svg.querySelectorAll('[data-transition]').forEach(n=>{n.classList.toggle('selected',selected.has(n.dataset.transition));n.classList.toggle('muted',selected.size>0&&!selected.has(n.dataset.transition));});}
 function witnessView(title,w,expect,trace){if(!w){detail(title,'実行可能性が未確定、または表示可能な反例がありません。');return;}highlight(w.steps.map(x=>x.transition));const box=el('detail');box.replaceChildren(make('h2',title),make('pre','初期状態: '+JSON.stringify(w.start,null,2)));const table=make('table');const head=make('tr');['#','状態','イベント・入力','システムのアクション','次状態・期待'].forEach(v=>head.append(make('th',v)));table.append(head);w.steps.forEach((s,i)=>{const t=spec.transitions[s.transition];const tr=make('tr');[i+1,t?.from||'',(({'user':'ユーザー','system':'システム','timer':'タイマー'}[spec.events[t?.event]?.source]||'')+'：'+(spec.events[t?.event]?.label||t?.event||s.transition))+(s.input?' '+JSON.stringify(s.input):'')+(s.reject?'（拒否を期待）':''),t?.action||'',(s.reject?t?.from:t?.to)+(s.expect?' / '+JSON.stringify(s.expect):'')].forEach(v=>tr.append(make('td',v)));table.append(tr)});const scroll=make('div',undefined,'scrolltable');scroll.append(table);box.append(scroll);if(expect!==undefined)box.append(make('h3','独立した最終期待結果'),make('pre',JSON.stringify(expect,null,2)));if(trace?.length)box.append(make('h3','モデル上の終端状態'),make('pre',JSON.stringify(trace[trace.length-1],null,2)));}
 function renderUC(id){draw(id);el('view-select').value=id;const u=spec.usecases[id];el('uc-summary').replaceChildren(make('h2',u?u.name:'システム全体'),make('p',u?u.goal:'画面・遷移の正本を統合した表示です。図の配置は検証結果に影響しません。'));if(u)el('uc-summary').append(make('p','利用者: '+u.actor),make('pre','終了条件: '+JSON.stringify(u.end)));
  const list=el('scenario-list');list.replaceChildren();report.scenarios.filter(s=>!id||s.usecase===id).forEach(s=>{const b=make('button',(s.valid?'✓ ':'✕ ')+s.name,'scenario');b.onclick=()=>{witnessView(s.id+' — '+s.name,s.witness,s.expect,s.trace);if(s.error)el('detail').append(make('p',s.error,'notice'));};list.append(b)});
  renderCoverage(id);document.querySelectorAll('.navbutton').forEach(b=>b.classList.toggle('active',b.dataset.uc===id));
 }
 function renderCoverage(id){const box=el('coverage');box.replaceChildren(make('h2','シナリオ定義の網羅性'));const groups=report.usecases.filter(u=>!id||u.id===id);for(const u of groups){box.append(make('h3',spec.usecases[u.id]?.name||u.id));for(const c of u.coverage){const d=make('details');const pct=c.percent===null?'対象なし':c.percent.toFixed(1)+'%';d.append(make('summary',`${labels[c.kind]||c.kind}: ${c.covered}/${c.total} (${pct})${c.complete?'':' — 解析未完了'}`));d.append(make('p',c.note,'small mutedtext'));if(c.percent!==null){const p=make('progress');p.max=100;p.value=c.percent;d.append(p)};for(const r of c.missing){const b=make('button','未網羅: '+r.id,'scenario small');b.onclick=()=>{if(currentUC!==u.id){renderUC(u.id);}if(r.witness)witnessView('未網羅パス候補（未承認）',r.witness);else detail('未網羅項目',r)};d.append(b)};if(c.infeasible?.length){const x=make('details');x.append(make('summary','成立しないため除外した経路: '+c.infeasible.length));x.append(make('pre',c.infeasible.map(x=>x.id).join('\n')));d.append(x)};box.append(d)}}}
 const nav=el('nav');[['','全体'],...Object.entries(spec.usecases).map(([k,u])=>[k,u.name])].forEach(([id,label])=>{const b=make('button',label,'navbutton');b.dataset.uc=id;b.onclick=()=>renderUC(id);nav.append(b);const o=make('option',label);o.value=id;el('view-select').append(o)});
 el('view-select').onchange=e=>renderUC(e.target.value);
 el('zoom-in').onclick=()=>{zoom=Math.min(2,zoom*1.2);setZoom()};el('zoom-out').onclick=()=>{zoom=Math.max(.1,zoom/1.2);setZoom()};el('zoom-fit').onclick=()=>draw(currentUC);
 el('filter').oninput=e=>{const q=e.target.value.toLowerCase();el('scenario-list').querySelectorAll('button').forEach(b=>b.hidden=!b.textContent.toLowerCase().includes(q))};
 const checks=el('checks');for(const c of report.checks){const d=make('details');d.append(make('summary',`${c.status}: ${c.id} (${c.kind})`));d.append(make('p',c.message));if(c.witness){const b=make('button','経路を表示');b.onclick=()=>{renderUC('');witnessView(c.id,c.witness)};d.append(b)};checks.append(d)}
 const diagnostics=el('diagnostics');for(const d of report.diagnostics){const row=make('details');row.append(make('summary',`${d.severity}: ${d.code} — ${d.subject||''}`));row.append(make('p',d.message));if(d.source?.file)row.append(make('code',d.source.file+(d.source.pointer||'')));if(d.witness){const b=make('button','反例の初期値・経路');b.onclick=()=>{renderUC('');witnessView(d.code,d.witness)};row.append(b)};diagnostics.append(row)}
 if(report.impact)el('impact').append(make('h2','変更影響（保守的な依存解析）'),make('pre',JSON.stringify(report.impact,null,2)));
 el('download-json').onclick=()=>{const blob=new Blob([JSON.stringify(report,null,2)],{type:'application/json'});const a=make('a');const url=URL.createObjectURL(blob);a.href=url;a.download='report.json';a.click();setTimeout(()=>URL.revokeObjectURL(url),1000)};
 renderUC('');
})();
