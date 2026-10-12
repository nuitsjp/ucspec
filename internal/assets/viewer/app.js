'use strict';
// Use-case specification viewer. Pages read as documents; use-case and
// scenario pages put the state diagram on the left and the text on the right.
(() => {
 const data=JSON.parse(document.getElementById('spec-data').textContent);
 const {spec,report,diagramVariants}=data;
 spec.usecasePackages ||= {}; spec.usecases ||= {}; spec.transitions ||= {}; spec.states ||= {}; spec.decisions ||= {}; spec.actors ||= {}; spec.commonRules ||= {};
 report.usecases ||= []; report.scenarios ||= []; report.checks ||= []; report.diagnostics ||= [];
 const el=id=>document.getElementById(id);
 const props=new Set(['value','checked','open','hidden','readOnly','max','type']);
 // Builds elements; strings always become text nodes, never HTML.
 const h=(tag,attrs,...children)=>{
  const n=document.createElement(tag);
  for(const [k,v] of Object.entries(attrs||{})){
   if(v===undefined||v===null||v===false)continue;
   if(k==='class')n.className=v;
   else if(k.startsWith('on'))n.addEventListener(k.slice(2),v);
   else if(props.has(k))n[k]=v;
   else n.setAttribute(k,v===true?'':v);
  }
  for(const c of children.flat(Infinity))if(c!==undefined&&c!==null&&c!==false&&c!=='')n.append(c instanceof Node?c:document.createTextNode(String(c)));
  return n;
 };
 const labels={states:'末端状態',compositeStates:'複合状態',transitions:'遷移',pairs:'遷移ペア',primePaths:'プライムパス',boundedPaths:'上限付き完了パス'};
 const reviewLabels={addScenario:'シナリオ追加が必要',represented:'既存シナリオで代表',forbidden:'禁止すべき'};
 const reviewGuidance={
  states:'到達すべき末端状態と、その状態で利用者が確認できる結果を確認します。',
  compositeStates:'複合状態に含まれる業務のまとまりと、親状態に共通するイベントを確認します。',
  transitions:'各イベント・分岐の目的と結果を確認し、必要な振る舞いをシナリオに追加します。',
  pairs:'連続する2つの遷移で、利用者が得る結果が変わらないか確認します。',
  primePaths:'分岐と循環を含む経路から、業務上異なる目的や結果を見落としていないか確認します。',
  boundedPaths:'繰り返し回数による結果の違いを確認します。既存シナリオで代表させる場合は、その根拠を残します。'
 };

 // ---- names -------------------------------------------------------------
 const isDecision=id=>!!spec.decisions[id];
 const statePath=id=>{if(isDecision(id))id=spec.decisions[id].parent;const path=[];while(id&&spec.states[id]){path.unshift(id);id=spec.states[id].parent||'';}return path;};
 const vertexText=id=>isDecision(id)?'ディシジョン「'+(spec.decisions[id].label||'')+'」':(statePath(id).map(x=>spec.states[x].name).join(' / ')||'ルート');
 const enteredState=id=>{while(spec.states[id]?.children?.length){const start=spec.transitions[spec.states[id].start];if(!start)break;id=start.to;}return id;};
 const transitionLabel=t=>{if(!t)return '';if(t.kind==='event')return t.name;if(t.kind==='branch')return (spec.decisions[t.from]?.label?spec.decisions[t.from].label+' ':'')+'['+t.name+']';return '開始';};
 const describeTransition=id=>{
  const t=spec.transitions[id];if(!t)return id;
  const root=spec.transitions[t.root];const where=root?vertexText(root.from):'';
  if(t.kind==='event')return `「${where}」の「${t.name}」`;
  if(t.kind==='branch')return `「${where}」の「${root?.kind==='event'?root.name:'開始'}」で${spec.decisions[t.from]?.label||'判定'}が「${t.name}」`;
  return `「${where}」の開始`;
 };
 const refLabel=id=>{const t=spec.transitions[id];if(!t)return id;const root=spec.transitions[t.root];const state=root?vertexText(root.from):'';return (state?state+' › ':'')+(t.kind==='branch'?(root?.name||'開始')+' › '+t.name:t.name);};
 const scenarioByID=id=>report.scenarios.find(s=>s.id===id);
 const usecaseResult=id=>report.usecases.find(u=>u.id===id);
 const scenariosOf=uc=>report.scenarios.filter(s=>(s.usecase||'')===uc);
 const detailText=sel=>{
  if(sel.transition){const t=spec.transitions[sel.transition];const root=spec.transitions[t?.root];return describeTransition(sel.transition)+(root?.actors?.length?'。アクター: '+root.actors.join('・'):'')+(t?.action?'。システムは'+t.action:'')+'。（'+sel.transition+'）';}
  if(sel.decision){const d=spec.decisions[sel.decision];return vertexText(sel.decision)+': '+d.branches.map(b=>spec.transitions[b]?.name||b).join(' / ');}
  const s=spec.states[sel.state];if(!s)return '';
  const events=Object.values(spec.transitions).filter(t=>t.kind==='event'&&t.from===sel.state).map(t=>t.name);
  return '状態「'+vertexText(sel.state)+'」'+(s.screen&&s.screen!==s.name?'。画面: '+s.screen:'')+(events.length?'。イベント: '+events.join('、'):'');
 };

 // ---- diagram -------------------------------------------------------------
 // Mirrors report.DiagramTransitions: scope events, their branches and start branches.
 const scopeTransitions=owner=>{
  const scope=owner?new Set(spec.usecases[owner].scope):null;
  return Object.entries(spec.transitions).filter(([,t])=>{if(t.kind==='start')return false;const root=spec.transitions[t.root];if(t.kind==='branch'&&root?.kind==='start')return true;return !scope||scope.has(t.root);}).map(([id])=>id);
 };
 const empty={path:[],states:[],decisions:[]};
 // One laid-out diagram per page: the generated SVG for the owner (system or a
 // use case) and the visible set, with wheel zoom, drag pan and selection.
 class Diagram{
  constructor({owner='',base=empty,toolbar=[]}={}){
   this.owner=owner;this.base=base;this.sel=base;this.actor='';this.k=1;this.x=0;this.y=0;this.zoomed=false;
   this.canvas=h('div',{class:'dg-canvas'});
   this.zoomLabel=h('span',{id:'zoom-label',class:'dg-zoom'});
   this.viewport=h('div',{id:'graph',class:'diagram'},this.canvas,
    h('div',{class:'dg-tools'},h('button',{type:'button',id:'zoom-out','aria-label':'縮小',onclick:()=>this.zoomBy(1/1.2)},'−'),this.zoomLabel,h('button',{type:'button',id:'zoom-in','aria-label':'拡大',onclick:()=>this.zoomBy(1.2)},'＋'),h('button',{type:'button',id:'zoom-fit',onclick:()=>this.fit()},'全体'),h('button',{type:'button',id:'zoom-actual',onclick:()=>this.zoomTo(1)},'100%')),
    h('div',{class:'dg-hint'},'ホイールで拡大縮小・ドラッグで移動・クリックで詳細'));
   this.showAll=h('input',{type:'checkbox',id:'show-all-transitions',onchange:()=>this.render()});
   this.hiddenCount=h('span',{id:'transition-visibility',class:'mutedtext small',role:'status'});
   this.selection=h('p',{id:'selection',class:'selection'});
   this.el=h('div',{class:'figure'},h('div',{class:'toolbar'},toolbar,h('label',{class:'transition-toggle'},this.showAll,'全遷移を表示'),this.hiddenCount),this.viewport,this.selection);
   this.viewport.addEventListener('wheel',e=>{e.preventDefault();const r=this.viewport.getBoundingClientRect();this.zoomAt(Math.exp(-e.deltaY*0.0015),e.clientX-r.left,e.clientY-r.top);},{passive:false});
   let drag=null;
   this.viewport.addEventListener('pointerdown',e=>{if(e.button!==0||e.target.closest('.dg-tools'))return;drag={x:e.clientX,y:e.clientY,ox:this.x,oy:this.y,moved:false};this.viewport.setPointerCapture(e.pointerId);this.viewport.classList.add('dragging');});
   this.viewport.addEventListener('pointermove',e=>{if(!drag)return;const dx=e.clientX-drag.x,dy=e.clientY-drag.y;if(Math.abs(dx)+Math.abs(dy)>3)drag.moved=true;if(!drag.moved)return;this.x=drag.ox+dx;this.y=drag.oy+dy;this.zoomed=true;this.apply();});
   this.viewport.addEventListener('pointerup',e=>{const d=drag;drag=null;this.viewport.classList.remove('dragging');if(d&&!d.moved)this.click(e);});
   new ResizeObserver(()=>{if(!this.zoomed)this.fit();}).observe(this.viewport);
   this.render();
  }
  variant(){
   const variants=diagramVariants[this.owner]||diagramVariants[''];
   if(this.showAll.checked)return variants['*'];
   const scope=new Set(scopeTransitions(this.owner));
   const hidden=[...new Set(this.sel.path.filter(id=>scope.has(id)&&!spec.transitions[id]?.defaultVisible))].sort();
   return variants[JSON.stringify(hidden)]||variants['[]'];
  }
  render(){
   const id=this.variant();
   if(this.viewport.dataset.diagram!==id){
    const template=el('diagram-'+id);this.canvas.replaceChildren(template.content.cloneNode(true));this.viewport.dataset.diagram=id;
    const svg=this.svg();if(svg){const vb=svg.viewBox.baseVal;this.w=vb.width;this.h=vb.height;svg.setAttribute('width',this.w);svg.setAttribute('height',this.h);}
    if(!this.zoomed)this.fit();
   }
   this.highlight();
  }
  svg(){return this.canvas.querySelector('svg');}
  set(changes){Object.assign(this,changes);this.render();}
  select(sel,text=''){this.sel={...empty,...sel};this.selection.textContent=text;this.render();}
  reset(){this.select(this.base);}
  highlight(){
   const svg=this.svg();if(!svg)return;
   const {path,states,decisions}=this.sel;
   const selected=new Set(path),active=new Set(states.flatMap(statePath)),activeDecisions=new Set(decisions);
   path.forEach(id=>{const t=spec.transitions[id];if(!t)return;if(isDecision(t.from))activeDecisions.add(t.from);if(isDecision(t.to))activeDecisions.add(t.to);});
   const actor=this.actor&&!selected.size?this.actor:'';
   const byActor=id=>{const t=spec.transitions[id];const root=spec.transitions[t?.root];return root?.kind!=='event'||root.actors.includes(actor);};
   svg.querySelectorAll('[data-transition]').forEach(n=>{const id=n.dataset.transition;n.classList.toggle('selected',selected.has(id));n.classList.toggle('muted',selected.size>0?!selected.has(id):!!actor&&!byActor(id));});
   svg.querySelectorAll('[data-state]').forEach(n=>n.classList.toggle('selected',active.has(n.dataset.state)));
   svg.querySelectorAll('[data-decision]').forEach(n=>n.classList.toggle('selected',activeDecisions.has(n.dataset.decision)));
   const showAll=this.showAll.checked;
   const hidden=scopeTransitions(this.owner).filter(id=>!showAll&&!spec.transitions[id].defaultVisible&&!selected.has(id)).length;
   svg.querySelectorAll('[data-transition]').forEach(n=>{
    if(n.classList.contains('initial-edge'))return;
    const visible=showAll||n.dataset.defaultVisible==='true'||selected.has(n.dataset.transition);
    n.classList.toggle('transition-hidden',!visible);n.setAttribute('aria-hidden',String(!visible));
   });
   this.hiddenCount.textContent='非表示の遷移: '+hidden;
  }
  apply(){this.canvas.style.transform=`translate(${this.x}px,${this.y}px) scale(${this.k})`;this.zoomLabel.textContent=Math.round(this.k*100)+'%';}
  zoomAt(f,cx,cy){const k=Math.min(4,Math.max(.1,this.k*f));this.x=cx-(cx-this.x)*k/this.k;this.y=cy-(cy-this.y)*k/this.k;this.k=k;this.zoomed=true;this.apply();}
  zoomBy(f){this.zoomAt(f,this.viewport.clientWidth/2,this.viewport.clientHeight/2);}
  zoomTo(k){this.zoomBy(k/this.k);}
  fit(){const W=this.viewport.clientWidth,H=this.viewport.clientHeight;if(!W||!H||!this.w)return;this.k=Math.min((W-16)/this.w,(H-16)/this.h,1);this.x=(W-this.w*this.k)/2;this.y=(H-this.h*this.k)/2;this.zoomed=false;this.apply();}
  click(e){
   const hit=document.elementsFromPoint(e.clientX,e.clientY).map(n=>n.closest?.('[data-transition],[data-decision],[data-state]')).find(n=>n&&this.viewport.contains(n));
   if(!hit){this.reset();this.selection.textContent='';return;}
   if(hit.dataset.transition)this.select({path:[hit.dataset.transition]},detailText({transition:hit.dataset.transition}));
   else if(hit.dataset.decision)this.select({decisions:[hit.dataset.decision]},detailText({decision:hit.dataset.decision}));
   else this.select({states:[hit.dataset.state]},detailText({state:hit.dataset.state}));
  }
 }

 // ---- narrative -------------------------------------------------------------
 // Replays a witness over the structure: an event leaves the current leaf, a
 // branch leaves its decision; no destination returns to the leaf of the event.
 function walk(w){
  const rows=[];let current=w.start,origin=w.start;
  for(const id of w.transitions){
   const t=spec.transitions[id];if(!t){rows.push({id,from:current,to:current});continue;}
   if(t.kind==='event')origin=current;
   const next=t.to?(isDecision(t.to)?t.to:enteredState(t.to)):origin;
   rows.push({id,t,from:current,to:next});current=next;
  }
  return rows;
 }
 const witnessTable=w=>h('div',{class:'scrolltable'},h('table',{},h('tr',{},['#','遷移元','イベント・分岐','アクション','遷移先'].map(v=>h('th',{},v))),walk(w).map((r,i)=>h('tr',{},h('td',{},i+1),h('td',{},vertexText(r.from)),h('td',{},transitionLabel(r.t)),h('td',{},r.t?.action||''),h('td',{},vertexText(r.to))))));
 // Turns a scenario result into numbered steps written as sentences.
 function narrate(r){
  const steps=[];let k=0;const ids=r.witness?.transitions||[];
  (r.steps||[]).forEach((step,i)=>{
   const from=r.trace?.[i]?.state,to=r.trace?.[i+1]?.state;
   if(step.reject){steps.push({no:i+1,step,reject:true,transitions:[],to:from,sentences:[`${r.actor}は「${step.event}」を行えない（拒否されることを確認する）。`]});return;}
   const taken=ids.slice(k,k+1+(step.branches?.length||0));k+=taken.length;
   const sentences=[];
   for(const id of taken){const t=spec.transitions[id];if(!t)continue;
    sentences.push(t.kind==='event'?`${r.actor}が「${t.name}」を行う。`:`${spec.decisions[t.from]?.label||'判定'}は「${t.name}」。`);
    if(t.action)sentences.push(`システムは${t.action}。`);}
   if(to)sentences.push(to===from?`「${vertexText(to)}」に留まる。`:`「${vertexText(to)}」へ移る。`);
   steps.push({no:i+1,step,transitions:taken,to,sentences});
  });
  return steps;
 }
 const pill=valid=>h('span',{class:'status '+(valid?'verified':'failed')},valid?'成立':'不成立');
 const section=(title,...body)=>h('section',{class:'doc-section'},h('h2',{},title),body);
 const list=items=>h('ul',{class:'prose-list'},items.map(x=>h('li',{},x)));
 const link=(page,label,attrs={})=>h('a',{href:'#'+page,'data-page':page,...attrs},label);
 const facts=rows=>h('dl',{class:'facts'},rows.filter(Boolean).map(([k,v])=>[h('dt',{},k),h('dd',{},v)]));

 // ---- table of contents --------------------------------------------------------
 const nav=el('nav');
 const scenarioNodes=[];const navDetails=[];
 const packageEntries=Object.entries(spec.usecasePackages);
 const scenarioLink=s=>{const a=link('sc/'+s.id,s.name,{class:'nav-link nav-scenario'+(s.valid?'':' bad'),'data-scenario':s.id,'data-uc':s.usecase||''});scenarioNodes.push(a);return h('li',{},a);};
 const group=(cls,summary,children,attrs={})=>{const d=h('details',{class:'nav-group '+cls,open:true,...attrs},h('summary',{},summary),h('ul',{},children));navDetails.push(d);return h('li',{},d);};
 const usecaseNode=id=>group('nav-usecase',link('uc/'+id,spec.usecases[id].name,{class:'nav-link nav-usecase-summary nav-usecase-button','data-uc':id}),scenariosOf(id).map(scenarioLink),{'data-uc':id});
 const packageNode=id=>group('nav-package',h('span',{class:'nav-label'},spec.usecasePackages[id].name),[
  ...packageEntries.filter(([,p])=>p.parent===id).map(([c])=>packageNode(c)),
  ...Object.entries(spec.usecases).filter(([,u])=>u.package===id).map(([u])=>usecaseNode(u))],{'data-package':id});
 const page1=(page,label)=>h('li',{},link(page,label,{class:'nav-link'}));
 const regressions=scenariosOf('');
 nav.replaceChildren(h('ul',{class:'tree'},
  page1('overview','概要'),page1('diagram','画面・状態遷移図'),
  group('nav-usecases',h('span',{class:'nav-label'},'ユースケース'),[...packageEntries.filter(([,p])=>!p.parent).map(([id])=>packageNode(id)),...Object.entries(spec.usecases).filter(([,u])=>!u.package).map(([id])=>usecaseNode(id))]),
  regressions.length?group('nav-regressions',h('span',{class:'nav-label'},'回帰シナリオ'),regressions.map(scenarioLink)):'',
  page1('actors','アクター'),Object.keys(spec.commonRules).length?page1('rules','共通ルール'):'',page1('checks','検査結果')));
 let savedOpen=null;
 el('filter').addEventListener('input',e=>{
  const q=e.target.value.trim().toLocaleLowerCase();
  if(!q){scenarioNodes.forEach(n=>n.parentElement.hidden=false);navDetails.forEach(n=>n.parentElement.hidden=false);if(savedOpen){savedOpen.forEach((open,n)=>n.open=open);savedOpen=null;}return;}
  if(!savedOpen)savedOpen=new Map(navDetails.map(n=>[n,n.open]));
  scenarioNodes.forEach(n=>n.parentElement.hidden=!n.textContent.toLocaleLowerCase().includes(q));
  navDetails.forEach(d=>{const match=[...d.querySelectorAll('.nav-scenario')].some(n=>!n.parentElement.hidden);d.parentElement.hidden=!match;if(match)d.open=true;});
 });
 el('toc-button').addEventListener('click',()=>{const open=document.body.classList.toggle('toc-open');el('toc-button').setAttribute('aria-expanded',String(open));});
 el('toc-pin').addEventListener('click',()=>{const pinned=document.body.classList.toggle('toc-pinned');document.body.classList.remove('toc-open');el('toc-pin').textContent=pinned?'固定を解除':'左に固定';});

 // ---- pages ---------------------------------------------------------------------
 let pending=null; // a witness to show on the diagram page (checks, diagnostics)
 const coverageSection=id=>{
  const box=h('section',{class:'doc-section',id:'coverage'},h('h2',{},'シナリオ定義の網羅性'),h('p',{class:'notice'},'網羅率は登録シナリオを状態の構造の上で再生した経路から計算した値です。実装のテスト実行結果ではありません。人の評価を記録しても、未網羅項目を網羅済みには数えません。'));
  const u=usecaseResult(id);if(!u)return box;
  const reviews=u.coverageReviews||[];
  const record=result=>{
   const review=result.review;
   const node=h('div',{class:'coverage-review','data-review-status':result.status},
    h('p',{},result.status==='stale'?'再評価が必要 — 以前の評価: '+reviewLabels[review.status]:'評価: '+reviewLabels[review.status]),
    result.error?h('p',{class:'notice'},result.error):'',h('p',{},'理由: '+review.reason));
   if(review.scenario){
    const s=report.scenarios.find(s=>s.usecase===u.id&&s.id===u.id+'/'+review.scenario);
    node.append(s?h('button',{type:'button',class:'review-scenario',onclick:()=>go('sc/'+s.id)},'代表シナリオ: '+s.name+' ('+review.scenario+')'):h('p',{},'代表シナリオ: '+review.scenario+'（現在の定義にありません）'));
   }
   if(review.architectureRequirements?.length)node.append(h('h4',{},'必要な設計要求'),h('p',{class:'small mutedtext'},'これらは評価の前提となる要求です。実装・検証の完了を示すものではありません。'),list(review.architectureRequirements));
   return node;
  };
  for(const c of u.coverage){
   const stale=reviews.filter(r=>r.status==='stale'&&r.review.kind===c.kind);
   const pct=c.percent===null?'対象なし':c.percent.toFixed(1)+'%';
   const d=h('details',{'data-coverage-kind':c.kind},h('summary',{},`${labels[c.kind]||c.kind}: ${c.covered}/${c.total} (${pct})${c.complete?'':' — 解析未完了'}${stale.length?' — 再評価が必要: '+stale.length+'件':''}`),
    h('p',{class:'review-guidance'},reviewGuidance[c.kind]),h('p',{class:'small mutedtext'},c.note),c.percent!==null?h('progress',{max:100,value:c.percent}):'');
   for(const requirement of c.missing){
    const review=reviews.find(r=>r.status==='current'&&r.review.kind===c.kind&&r.review.requirements.includes(requirement.id));
    const item=h('div',{class:'coverage-item','data-requirement':requirement.id,'data-review-status':review?'current':'unreviewed'});
    item.append(h('button',{type:'button',class:'small',onclick:()=>showCandidate(item,requirement,review?record(review):h('p',{},'未評価: この項目について人の評価はまだ記録されていません。'))},'未網羅: '+requirement.id+' — '+(review?reviewLabels[review.review.status]:'未評価')));
    if(review)item.append(record(review));
    d.append(item);
   }
   for(const result of stale){const node=record(result);node.prepend(h('h4',{},'再評価が必要な記録: '+result.id));node.append(h('p',{},'評価対象: '+result.review.requirements.join(' / ')));d.append(node);}
   if(c.infeasible?.length)d.append(h('details',{},h('summary',{},'ユースケースの終了に到達できないため除外した経路: '+c.infeasible.length),h('pre',{},c.infeasible.map(x=>x.id).join('\n'))));
   box.append(d);
  }
  const hash=h('input',{type:'text',id:'review-basis-'+u.id,class:'search',readOnly:true,value:u.reviewBasisHash,onclick:e=>e.target.select()});
  box.append(h('details',{},h('summary',{},'評価記録を更新するための情報'),h('p',{},'現在の仕様を確認して評価記録を編集するときに使用します。'),h('label',{for:hash.id},'basisHash（選択してコピー）'),hash));
  return box;
 };
 let diagram=null;
 function showCandidate(item,requirement,reviewNode){
  el('detail')?.remove();
  const w=requirement.witness;
  const box=h('section',{id:'detail',class:'candidate'},h('h2',{},w?(requirement.state?'未網羅状態のシナリオ候補（未承認）':'未網羅パス候補（未承認）'):'未網羅項目'));
  if(w){box.append(h('p',{},'開始: '+vertexText(w.start)),witnessTable(w));diagram.select({path:w.transitions,states:requirement.state?[requirement.state]:[]},'未網羅候補の経路を表示しています。');}
  else{box.append(h('pre',{},JSON.stringify(requirement,null,2)));diagram.select({path:requirement.path||[],states:requirement.state?[requirement.state]:[]});}
  box.append(reviewNode);item.after(box);
 }
 const preconditions=u=>u.starts.map(p=>`${u.actor}が「${vertexText(p)}」の画面を開いている。`);
 const pages={
  overview(){
   const fig=overviewDiagram('64vh');
   return {wide:true,title:spec.project?.name||report.name,body:[
    h('p',{class:'lede'},`${Object.keys(spec.usecases).length}件のユースケースと${report.scenarios.length}件のシナリオ（回帰を含む）を、画面・状態遷移の構造の上で確かめた仕様書です。`),
    fig,
    section('ユースケース',h('div',{class:'cards'},Object.entries(spec.usecases).map(([id,u])=>{const c=usecaseResult(id)?.coverage.find(x=>x.kind==='transitions');return h('a',{class:'card',href:'#uc/'+id,'data-page':'uc/'+id},h('strong',{},u.name),h('span',{},u.goal),h('small',{},`${u.actor} ・ シナリオ ${scenariosOf(id).length}`+(c?` ・ 遷移 ${c.covered}/${c.total}`:'')));}))),
    h('p',{class:'notice'},'ここでの網羅率はシナリオ定義の網羅性です。実装のテスト実行結果ではありません。検証済みは、定義した状態の構造とシナリオの範囲に限ります。モデルの期待結果は実装の保証ではなく、未網羅候補は自動承認されません。')]};
  },
  diagram(){
   const fig=overviewDiagram('calc(100vh - var(--bar-h) - 150px)');
   const body=[fig];
   if(pending){const w=pending;pending=null;diagram.select({path:w.transitions},w.title+' の経路を表示しています。');body.push(section(w.title,h('p',{},'開始: '+vertexText(w.start)),witnessTable(w)));}
   return {wide:true,title:'画面・状態遷移図',body};
  },
  uc(id){
   const u=spec.usecases[id];if(!u)return null;
   diagram=new Diagram({owner:id});
   return {side:diagram.el,crumbs:['ユースケース',spec.usecasePackages[u.package]?.name],title:u.name,body:[
    h('p',{class:'lede'},u.goal),
    facts([['アクター',[u.actor,spec.actors[u.actor]?.description?h('span',{class:'mutedtext'},' — '+spec.actors[u.actor].description):'']],u.package&&['パッケージ',spec.usecasePackages[u.package]?.name]]),
    section('事前条件',list(preconditions(u))),
    section('事後条件',list(u.ends.map(e=>describeTransition(e)+'となって終わる。')),h('p',{class:'mutedtext small'},'終わり方ごとの結果は、各シナリオの期待する結果に記します。')),
    section('扱うイベント',list(u.scope.map(e=>`${describeTransition(e)}（${(spec.transitions[e]?.actors||[]).join('・')}）`))),
    section('シナリオ',h('div',{class:'scrolltable'},h('table',{},h('tr',{},['シナリオ','目的','期待する結果',''].map(v=>h('th',{},v))),scenariosOf(id).map(s=>h('tr',{},h('td',{},link('sc/'+s.id,s.name)),h('td',{},s.goal),h('td',{},s.expectedOutcome),h('td',{},pill(s.valid))))))),
    coverageSection(id)]};
  },
  sc(id){
   const s=scenarioByID(id);if(!s)return null;
   const u=s.usecase?spec.usecases[s.usecase]:null;
   const last=s.trace?.[s.trace.length-1]?.state;
   const base={path:s.witness?.transitions||[],states:last?[last]:[]};
   diagram=new Diagram({owner:s.usecase||'',base});
   const steps=narrate(s);
   const items=steps.map(st=>h('li',{class:'step'+(st.reject?' reject':''),tabindex:'0',onclick:e=>activate(e.currentTarget,st)},
    h('div',{class:'step-head'},h('span',{class:'step-no'},st.no),h('span',{class:'step-event'},st.step.event),st.step.branches?.length?h('span',{class:'step-branch'},st.step.branches.map(b=>'［'+b+'］').join(' ')):''),
    h('p',{class:'step-text'},st.sentences.join('')),st.step.note?h('p',{class:'step-note small mutedtext'},st.step.note):''));
   const activate=(node,st)=>{items.forEach(n=>n.classList.toggle('active',n===node));diagram.select(st.transitions.length?{path:st.transitions,states:st.to?[st.to]:[]}:{states:st.to?[st.to]:[]},'手順'+st.no+'の遷移を表示しています。');};
   return {side:diagram.el,crumbs:u?['ユースケース',spec.usecasePackages[u.package]?.name,link('uc/'+s.usecase,u.name)]:['回帰シナリオ'],title:s.name,body:[
    h('p',{class:'lede scenario-goal'},s.goal),
    facts([[u?'ユースケース':'種類',u?link('uc/'+s.usecase,u.name):'回帰シナリオ（ユースケース横断）'],['アクター',s.actor],['期待する結果',h('span',{class:'scenario-outcome'},s.expectedOutcome)],['検査',pill(s.valid)],['識別子',h('code',{},s.id)]]),
    s.error?h('p',{class:'notice'},s.error):'',
    h('p',{class:'small mutedtext'},'目的と期待する結果は、人がシナリオを確認するための記述です。'),
    section('事前条件',list([`${s.actor}が「${vertexText(s.witness?.start)}」の画面を開いている。`])),
    section('手順',h('ol',{class:'steps'},items),h('p',{class:'small mutedtext'},'手順をクリックすると、その遷移を図で強調します。')),
    section('事後条件',h('p',{},last?`「${vertexText(last)}」の画面で終わる。`:'終了状態はありません。'))]};
  },
  actors(){
   return {title:'アクター',body:[h('table',{},h('tr',{},['アクター','説明','起こせるイベント'].map(v=>h('th',{},v))),Object.entries(spec.actors).map(([a,v])=>h('tr',{},h('td',{},a),h('td',{},v?.description||''),h('td',{},[...new Set(Object.values(spec.transitions).filter(t=>t.kind==='event'&&t.actors?.includes(a)).map(t=>t.name))].join('、')))))]};
  },
  rules(){return {title:'共通ルール',body:Object.values(spec.commonRules).map(r=>section(r.name,h('p',{},r.description)))};},
  checks(){
   const path=(title,w)=>w?h('button',{type:'button',onclick:()=>{pending={...w,title};go('diagram');}},'経路を表示'):'';
   return {title:'検査結果',body:[
    h('p',{class:'lede'},['検証結果: ',h('span',{class:'status '+report.status},report.status),`　到達できる末端状態 ${report.reachableStates}`]),
    section('モデル検査',report.checks.map(c=>h('details',{},h('summary',{},`${c.status}: ${c.id} (${c.kind})`),h('p',{},c.message),path(c.id,c.witness)))),
    section('診断',report.diagnostics.length?report.diagnostics.map(d=>h('details',{},h('summary',{},`${d.severity}: ${d.code} — ${d.subject||''}`),h('p',{},d.message),d.source?.file?h('code',{},d.source.file+(d.source.pointer||'')):'',path(d.code,d.witness))):h('p',{},'診断はありません。')),
    report.impact?section('変更後の再確認候補',h('p',{},'依存関係から抽出した候補です。変更の意味や、目的・期待結果・設計要求への再合意を自動で判断したものではありません。'),h('pre',{},JSON.stringify(report.impact,null,2))):'',
    section('再現性',h('p',{},'ツール: '+(report.toolVersion||'')),h('p',{},['入力 SHA-256: ',h('code',{},report.inputHash||'')]),h('button',{type:'button',id:'download-json',onclick:download},'検査結果JSONを書き出す'))]};
  }
 };
 // The diagram with filters for the whole system: use case, scenario, actor.
 function overviewDiagram(height){
  const uc=h('select',{id:'view-select','aria-label':'表示するユースケース'},h('option',{value:''},'全体'),Object.entries(spec.usecases).map(([id,u])=>h('option',{value:id},u.name)));
  const sc=h('select',{id:'scenario-select','aria-label':'強調するシナリオ'});
  const actor=h('select',{id:'actor-select','aria-label':'アクター'},h('option',{value:''},'すべてのアクター'),Object.keys(spec.actors).map(a=>h('option',{value:a},a)));
  const fill=()=>sc.replaceChildren(h('option',{value:''},'シナリオを選ばない'),report.scenarios.filter(s=>!uc.value||s.usecase===uc.value).map(s=>h('option',{value:s.id},(s.usecase?'':'回帰: ')+s.name)));
  fill();
  diagram=new Diagram({toolbar:[h('label',{for:'view-select'},'ユースケース'),uc,h('label',{for:'scenario-select'},'シナリオ'),sc,h('label',{for:'actor-select'},'アクター'),actor]});
  uc.onchange=()=>{fill();diagram.zoomed=false;diagram.base=empty;diagram.sel=empty;diagram.set({owner:uc.value});diagram.selection.textContent='';};
  sc.onchange=()=>{const s=scenarioByID(sc.value);const owner=s?s.usecase||'':uc.value;diagram.base=s?{path:s.witness.transitions,states:[]}:empty;if(owner!==diagram.owner)diagram.zoomed=false;diagram.owner=owner;diagram.select(diagram.base,s?'シナリオ「'+s.name+'」の経路を表示しています。':'');};
  actor.onchange=()=>diagram.set({actor:actor.value});
  diagram.el.style.setProperty('--figure-h',height);diagram.el.classList.add('wide');
  return diagram.el;
 }
 function download(){const blob=new Blob([JSON.stringify(report,null,2)],{type:'application/json'});const a=h('a',{href:URL.createObjectURL(blob),download:'report.json'});a.click();setTimeout(()=>URL.revokeObjectURL(a.href),1000);}

 // ---- routing -------------------------------------------------------------------
 const crumbs=el('crumbs');
 function go(page,push=true){
  const [kind,...rest]=page.split('/');const arg=rest.join('/');
  diagram=null;
  const view=(kind==='uc'||kind==='sc')?pages[kind](arg):pages[kind]?.()||null;
  const v=view||pages.overview();if(!view)page='overview';
  const article=h('article',{class:'doc'},h('h1',{},v.title),v.body);
  el('main').className=v.side?'spread':v.wide?'wide':'narrow';
  el('main').replaceChildren(...(v.side?[h('div',{class:'side'},v.side),article]:[article]));
  crumbs.replaceChildren(...[...(v.crumbs||[]),page==='overview'?'':v.title].filter(Boolean).map(c=>h('span',{},c)));
  nav.querySelectorAll('.nav-link').forEach(a=>a.classList.toggle('current',a.dataset.page===page));
  document.body.classList.remove('toc-open');el('toc-button').setAttribute('aria-expanded','false');
  if(push){try{history.pushState(null,'','#'+page);}catch{}}
  window.scrollTo(0,0);
 }
 document.addEventListener('click',e=>{const a=e.target.closest?.('a[data-page]');if(!a||e.button!==0||e.ctrlKey||e.metaKey||e.shiftKey)return;e.preventDefault();go(a.dataset.page);});
 window.addEventListener('popstate',()=>go(decodeURIComponent(location.hash.slice(1))||'overview',false));
 go(decodeURIComponent(location.hash.slice(1))||'overview',false);
})();
