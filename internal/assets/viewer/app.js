'use strict';
(() => {
 const data=JSON.parse(document.getElementById('spec-data').textContent);
 const {spec,report,diagramVariants}=data;
 spec.usecasePackages ||= {}; spec.usecases ||= {}; spec.transitions ||= {}; spec.states ||= {}; spec.events ||= {}; spec.choices ||= {};
 report.usecases ||= []; report.scenarios ||= []; report.checks ||= []; report.diagnostics ||= [];
 const el=id=>document.getElementById(id);
 const text=(node,value)=>{node.textContent=value==null?'':String(value);return node;};
 const make=(tag,value,cls)=>{const n=document.createElement(tag);if(value!==undefined)text(n,value);if(cls)n.className=cls;return n;};
 const labels={states:'末端状態',compositeStates:'複合状態',transitions:'遷移',pairs:'遷移ペア',primePaths:'プライムパス',boundedPaths:'上限付き完了パス',conditions:'条件',multipleConditions:'複合条件'};
 const reviewLabels={addScenario:'シナリオ追加が必要',represented:'既存シナリオで代表',forbidden:'禁止すべき'};
 const reviewGuidance={
  states:'到達すべき末端状態と、その状態で利用者が確認できる結果を確認します。',
  compositeStates:'複合状態に含まれる業務のまとまりと、親状態に共通する制約を確認します。',
  transitions:'各操作の目的と結果を確認し、必要な振る舞いをシナリオに追加します。',
  pairs:'連続する2操作で、状態やデータの引継ぎによって結果が変わらないか確認します。',
  primePaths:'分岐と循環を含む経路から、業務上異なる目的や結果を見落としていないか確認します。',
  boundedPaths:'繰り返し回数による結果の違いを確認します。既存シナリオで代表させる場合は、その根拠を残します。',
  conditions:'各名前付き条件の真・偽について、必要な振る舞いと期待結果を確認します。',
  multipleConditions:'条件の組合せごとに、操作の許可・拒否と期待結果の違いを確認します。'
 };
 let currentUC='',zoom=1;
 let selection={path:[],states:[],choices:[]};
 const detail=(title,obj)=>{el('detail').replaceChildren(make('h2',title),make('pre',typeof obj==='string'?obj:JSON.stringify(obj,null,2)));};
 const statePath=id=>{const path=[];while(id&&(spec.states[id]||spec.choices[id])){path.unshift(id);id=spec.choices[id]?.parent||spec.states[id]?.parent||'';}return path;};
 const enteredState=id=>{while(spec.states[id]?.initial)id=spec.states[id].initial;return id;};
 const vertexLabel=id=>spec.choices[id]?.label||spec.states[id]?.label||id;
 function stateDetail(id){highlight([],[id]);detail(statePath(id).map(x=>spec.states[x].label+' ('+x+')').join(' / '),spec.states[id]);}
 function choiceDetail(id){const choice=spec.choices[id];if(!choice)return;highlight([],[],[id]);const branches=Object.entries(spec.transitions).filter(([,t])=>t.from===id).map(([transition,t])=>({transition,...t}));detail('ディシジョン: '+choice.label,{choice,branches});}
 function transitionDetail(id){const transition=spec.transitions[id];if(!transition)return;const choice=spec.choices[transition.from];detail(id,choice?{choice,transition}:{transition});}
 function stateCell(id){const td=make('td');statePath(id).forEach((state,i)=>{if(i)td.append(document.createTextNode(' / '));const b=make('button',vertexLabel(state)+' ('+state+')','state-link');b.type='button';b.onclick=()=>spec.choices[state]?choiceDetail(state):stateDetail(state);td.append(b);});return td;}
 function draw(id=''){
  currentUC=id;const diagram=currentDiagram(),template=el('diagram-'+diagram);el('graph').replaceChildren(template.content.cloneNode(true));el('graph').dataset.diagram=diagram;
  const svg=el('graph').querySelector('svg');if(!svg)return;const vb=svg.viewBox.baseVal;zoom=Math.min(1,Math.max(.15,(el('graph').clientWidth-22)/vb.width));setZoom();
  svg.querySelectorAll('[data-transition]').forEach(n=>{n.addEventListener('click',event=>{event.stopPropagation();const id=n.dataset.transition;highlight([id]);transitionDetail(id);});});
  svg.querySelectorAll('[data-choice]').forEach(n=>{n.addEventListener('click',event=>{event.stopPropagation();choiceDetail(n.dataset.choice);});});
  svg.querySelectorAll('[data-state]').forEach(n=>n.addEventListener('click',event=>{event.stopPropagation();stateDetail(n.dataset.state);}));
  applyHighlight();
 }
 function scopeTransitions(){return currentUC?spec.usecases[currentUC].scope:Object.keys(spec.transitions);}
 function currentDiagram(){
  if(el('show-all-transitions').checked)return diagramVariants[currentUC]['*'];
  const scope=new Set(scopeTransitions());
  const hidden=[...new Set(selection.path.filter(id=>scope.has(id)&&!spec.transitions[id].defaultVisible))].sort();
  return diagramVariants[currentUC][JSON.stringify(hidden)];
 }
 function setZoom(){const svg=el('graph').querySelector('svg');if(!svg)return;const v=svg.viewBox.baseVal;svg.style.width=(v.width*zoom)+'px';svg.style.height=(v.height*zoom)+'px';el('zoom-label').textContent=Math.round(zoom*100)+'%';}
 function updateTransitionVisibility(){
  const svg=el('graph').querySelector('svg');if(!svg)return;
  const selected=new Set(selection.path),showAll=el('show-all-transitions').checked;
  const hidden=scopeTransitions().filter(id=>!showAll&&!spec.transitions[id].defaultVisible&&!selected.has(id)).length;
  svg.querySelectorAll('[data-transition]').forEach(n=>{
   const visible=showAll||n.dataset.defaultVisible==='true'||selected.has(n.dataset.transition);
   n.classList.toggle('transition-hidden',!visible);
   n.setAttribute('aria-hidden',String(!visible));
  });
  el('transition-visibility').textContent='非表示の遷移: '+hidden;
 }
 function highlight(path,states=[],choices=[]){
  selection={path:[...path],states:[...states],choices:[...choices]};
  if(el('graph').dataset.diagram!==currentDiagram()){draw(currentUC);return;}
  applyHighlight();
 }
 function applyHighlight(){
  const {path,states,choices}=selection;
  const selected=new Set(path),active=new Set(states.flatMap(statePath)),activeChoices=new Set(choices);
  path.forEach(id=>{const from=spec.transitions[id]?.from;if(from&&spec.choices[from])activeChoices.add(from);});
  const svg=el('graph').querySelector('svg');if(!svg)return;
  svg.querySelectorAll('[data-transition]').forEach(n=>{n.classList.toggle('selected',selected.has(n.dataset.transition));n.classList.toggle('muted',selected.size>0&&!selected.has(n.dataset.transition));});
  svg.querySelectorAll('[data-state]').forEach(n=>n.classList.toggle('selected',active.has(n.dataset.state)));
  svg.querySelectorAll('[data-choice]').forEach(n=>n.classList.toggle('selected',activeChoices.has(n.dataset.choice)));
  updateTransitionVisibility();
 }
	 function guardText(t){if(t.guardLabel)return t.guardLabel;if(t.guard===undefined||t.guard===null)return '';return typeof t.guard==='string'?t.guard:JSON.stringify(t.guard);}
	 function transitionHistory(step,t){if(!t)return step.transition;if(!t.event){const choice=spec.choices[t.from];let value=choice?'ディシジョン: '+choice.label:step.transition;const guard=guardText(t);if(guard)value+=' ['+guard+']';return value;}const event=spec.events[t.event]||{};let value=event.label||t.event||step.transition;const guard=guardText(t);if(guard)value+=' ['+guard+']';if(step.input)value+=' '+JSON.stringify(step.input);if(step.reject)value+='（拒否を期待）';return value;}
 function witnessView(title,w,expect,trace,scenario){if(!w){detail(title,'実行可能性が未確定、または表示可能な反例がありません。');return;}highlight(w.steps.map(x=>x.transition),trace?.map(x=>x.state)||[]);const box=el('detail');box.replaceChildren(make('h2',title));if(scenario){box.append(make('h3','目的'),make('p',scenario.goal,'scenario-goal'),make('h3','期待する結果'),make('p',scenario.expectedOutcome,'scenario-outcome'),make('p','目的と期待する結果は、人がシナリオを確認するための記述です。','small mutedtext'));}box.append(make('pre','初期状態: '+JSON.stringify(w.start,null,2)));const table=make('table');const head=make('tr');['#','有効な状態（親 / 子）','イベント・入力','アクション','次状態・モデル上の期待式'].forEach(v=>head.append(make('th',v)));table.append(head);let active=enteredState(w.start.state);w.steps.forEach((s,i)=>{const t=spec.transitions[s.transition];const from=trace?.[i]?.state||active;const to=trace?.[i+1]?.state||(s.reject||t?.kind==='internal'?from:enteredState(t?.to));active=to;const tr=make('tr');tr.append(make('td',i+1),stateCell(from),make('td',transitionHistory(s,t)),make('td',t?.action||''));const next=stateCell(to);if(s.expect!==undefined)next.append(make('pre',JSON.stringify(s.expect)));tr.append(next);table.append(tr)});const scroll=make('div',undefined,'scrolltable');scroll.append(table);box.append(scroll);if(expect!==undefined)box.append(make('h3','モデル上の期待式'),make('pre',JSON.stringify(expect,null,2)),make('p','期待式の合格は、定義したモデル上の確認結果です。実装の動作を保証しません。','small mutedtext'));if(trace?.length)box.append(make('h3','モデル上の終端状態'),make('pre',JSON.stringify(trace[trace.length-1],null,2)));}
 function showScenario(s){witnessView(s.id+' — '+s.name,s.witness,s.expect,s.trace,s);if(s.error)el('detail').append(make('p',s.error,'notice'));}
 function renderUC(id){selection={path:[],states:[],choices:[]};draw(id);el('view-select').value=id;const u=spec.usecases[id];el('uc-summary').replaceChildren(make('h2',u?u.name:'システム全体'),make('p',u?u.goal:'画面・遷移の正本を統合した表示です。図の配置は検証結果に影響しません。'));if(u)el('uc-summary').append(make('p','利用者: '+u.actor),make('pre','終了条件: '+JSON.stringify(u.end)));
  renderCoverage(id);nav.querySelectorAll('.nav-overall,.nav-usecase-summary').forEach(n=>n.classList.toggle('active',n.dataset.uc===id));
 }
 function reviewRecord(u,result){
  const review=result.review;const box=make('div',undefined,'coverage-review');box.dataset.reviewStatus=result.status;
  box.append(make('p',result.status==='stale'?'再評価が必要 — 以前の評価: '+reviewLabels[review.status]:'評価: '+reviewLabels[review.status]));
  if(result.error)box.append(make('p',result.error,'notice'));
  box.append(make('p','理由: '+review.reason));
  if(review.scenario){
   const scenario=report.scenarios.find(s=>s.usecase===u.id&&s.id===u.id+'/'+review.scenario);
   if(scenario){const b=make('button','代表シナリオ: '+scenario.name+' ('+review.scenario+')','review-scenario');b.onclick=()=>{if(currentUC!==u.id)renderUC(u.id);showScenario(scenario)};box.append(b)}
   else box.append(make('p','代表シナリオ: '+review.scenario+'（現在の定義にありません）'));
  }
  if(review.architectureRequirements?.length){
   box.append(make('h4','必要な設計要求'),make('p','これらは評価の前提となる要求です。実装・検証の完了を示すものではありません。','small mutedtext'));
   const list=make('ul');for(const requirement of review.architectureRequirements)list.append(make('li',requirement));box.append(list);
  }
  return box;
 }
 function renderCoverage(id){
  const box=el('coverage');box.replaceChildren(make('h2','シナリオ定義の網羅性'),make('p','網羅率は登録シナリオをモデル上で再生した経路から計算した値です。人の評価を記録しても、未網羅項目を網羅済みには数えません。','notice'));
  const groups=report.usecases.filter(u=>!id||u.id===id);
  for(const u of groups){
   box.append(make('h3',spec.usecases[u.id]?.name||u.id));
   const reviews=u.coverageReviews||[];
   for(const c of u.coverage){
    const stale=reviews.filter(r=>r.status==='stale'&&r.review.kind===c.kind);
    const d=make('details');d.dataset.coverageKind=c.kind;const pct=c.percent===null?'対象なし':c.percent.toFixed(1)+'%';
    d.append(make('summary',`${labels[c.kind]||c.kind}: ${c.covered}/${c.total} (${pct})${c.complete?'':' — 解析未完了'}${stale.length?' — 再評価が必要: '+stale.length+'件':''}`),make('p',reviewGuidance[c.kind],'review-guidance'),make('p',c.note,'small mutedtext'));
    if(c.percent!==null){const p=make('progress');p.max=100;p.value=c.percent;d.append(p)}
    for(const requirement of c.missing){
     const review=reviews.find(r=>r.status==='current'&&r.review.kind===c.kind&&r.review.requirements.includes(requirement.id));
     const item=make('div',undefined,'coverage-item');item.dataset.requirement=requirement.id;item.dataset.reviewStatus=review?'current':'unreviewed';
     const b=make('button','未網羅: '+requirement.id+' — '+(review?reviewLabels[review.review.status]:'未評価'),'scenario small');
     b.onclick=()=>{if(currentUC!==u.id)renderUC(u.id);if(requirement.witness)witnessView(requirement.state?'未網羅状態のシナリオ候補（未承認）':'未網羅パス候補（未承認）',requirement.witness);else detail('未網羅項目',requirement);if(requirement.state)highlight(requirement.witness?requirement.witness.steps.map(x=>x.transition):(requirement.path||[]),[requirement.state]);if(review)el('detail').append(reviewRecord(u,review));else el('detail').append(make('p','未評価: この項目について人の評価はまだ記録されていません。'))};
     item.append(b);if(review)item.append(reviewRecord(u,review));d.append(item);
    }
    for(const result of stale){
     const record=reviewRecord(u,result);record.prepend(make('h4','再評価が必要な記録: '+result.id));record.append(make('p','評価対象: '+result.review.requirements.join(' / ')));d.append(record);
    }
    if(c.infeasible?.length){const x=make('details');x.append(make('summary','成立しないため除外した経路: '+c.infeasible.length),make('pre',c.infeasible.map(x=>x.id).join('\n')));d.append(x)}
    box.append(d);
   }
   const edit=make('details');edit.append(make('summary','評価記録を更新するための情報'),make('p','現在の仕様を確認して評価記録を編集するときに使用します。'));
   const label=make('label','basisHash（選択してコピー）');const hash=make('input');hash.id='review-basis-'+u.id;label.htmlFor=hash.id;hash.type='text';hash.readOnly=true;hash.value=u.reviewBasisHash;hash.className='search';hash.onclick=()=>hash.select();edit.append(label,hash);box.append(edit);
  }
 }
 const nav=el('nav');
 const scenariosByUC=new Map();
 const regressionScenarios=[];
 report.scenarios.forEach(s=>{if(s.usecase){if(!scenariosByUC.has(s.usecase))scenariosByUC.set(s.usecase,[]);scenariosByUC.get(s.usecase).push(s);}else regressionScenarios.push(s);});
 const navDetails=[];
 const scenarioNodes=[];
 const scenarioNames=new Map();
 const packageEntries=Object.entries(spec.usecasePackages);
 const packagesByParent=new Map(packageEntries.map(([id])=>[id,[]]));
 packageEntries.forEach(([id,pkg])=>{const parent=pkg.parent||'';if(parent)packagesByParent.get(parent).push(id);});
 const usecasesByPackage=new Map(packageEntries.map(([id])=>[id,[]]));
 Object.entries(spec.usecases).forEach(([id,u])=>{const packageID=u.package||'';if(packageID)usecasesByPackage.get(packageID).push(id);});
 const scenarioButton=s=>{const b=make('button',(s.valid?'✓ ':'✕ ')+s.name,'scenario nav-scenario');b.type='button';b.dataset.scenario=s.id;b.dataset.uc=s.usecase||'';scenarioNodes.push(b);scenarioNames.set(b,String(s.name||'').toLocaleLowerCase());b.onclick=()=>{const owner=s.usecase||'';renderUC(owner);showScenario(s);};return b;};
 const usecaseGroup=(id,u)=>{const group=make('details',undefined,'nav-group nav-usecase');group.dataset.uc=id;group.open=true;navDetails.push(group);const summary=make('summary',u.name,'nav-usecase-summary nav-usecase-button');summary.dataset.uc=id;summary.addEventListener('click',()=>renderUC(id));group.append(summary);const children=make('div',undefined,'nav-children');(scenariosByUC.get(id)||[]).forEach(s=>children.append(scenarioButton(s)));group.append(children);return group;};
 const packageGroup=id=>{const pkg=spec.usecasePackages[id];const group=make('details',undefined,'nav-group nav-package');group.dataset.package=id;group.open=true;navDetails.push(group);group.append(make('summary',pkg.name,'nav-package-summary'));const children=make('div',undefined,'nav-children');(packagesByParent.get(id)||[]).forEach(child=>children.append(packageGroup(child)));(usecasesByPackage.get(id)||[]).forEach(uid=>children.append(usecaseGroup(uid,spec.usecases[uid])));group.append(children);return group;};
 function buildNavigation(){
  nav.replaceChildren();scenarioNodes.length=0;navDetails.length=0;scenarioNames.clear();
  const overall=make('button','全体','navbutton nav-overall');overall.type='button';overall.dataset.uc='';overall.onclick=()=>renderUC('');nav.append(overall);
  packageEntries.filter(([id,pkg])=>!(pkg.parent||'')).forEach(([id])=>nav.append(packageGroup(id)));
  Object.entries(spec.usecases).filter(([,u])=>!(u.package||'')).forEach(([id,u])=>nav.append(usecaseGroup(id,u)));
  if(regressionScenarios.length){const group=make('details',undefined,'nav-group nav-regressions');group.open=true;navDetails.push(group);group.append(make('summary','回帰シナリオ','nav-regressions-summary'));const children=make('div',undefined,'nav-children');regressionScenarios.forEach(s=>children.append(scenarioButton(s)));group.append(children);nav.append(group);}
  const select=el('view-select');select.replaceChildren();const all=make('option','全体');all.value='';select.append(all);Object.entries(spec.usecases).forEach(([id,u])=>{const option=make('option',u.name);option.value=id;select.append(option);});
 }
 let savedOpenState=null;
 function applyScenarioFilter(value){const query=value.trim().toLocaleLowerCase();if(!query){scenarioNodes.forEach(n=>n.hidden=false);navDetails.forEach(n=>n.hidden=false);if(savedOpenState){savedOpenState.forEach((open,node)=>{node.open=open;});savedOpenState=null;}return;}if(!savedOpenState)savedOpenState=new Map(navDetails.map(n=>[n,n.open]));scenarioNodes.forEach(n=>{n.hidden=!scenarioNames.get(n).includes(query);});navDetails.forEach(group=>{const hasMatch=[...group.querySelectorAll('.nav-scenario')].some(n=>!n.hidden);group.hidden=!hasMatch;if(hasMatch)group.open=true;});}
 buildNavigation();
 el('view-select').onchange=e=>renderUC(e.target.value);
 el('show-all-transitions').onchange=()=>draw(currentUC);
 el('zoom-in').onclick=()=>{zoom=Math.min(2,zoom*1.2);setZoom()};el('zoom-out').onclick=()=>{zoom=Math.max(.1,zoom/1.2);setZoom()};el('zoom-fit').onclick=()=>draw(currentUC);
 el('filter').oninput=e=>applyScenarioFilter(e.target.value);
 const commonRules=el('common-rules');for(const rule of Object.values(spec.commonRules||{})){commonRules.hidden=false;el('common-rules-link').hidden=false;commonRules.append(make('h3',rule.name),make('p',rule.description));}
 const checks=el('checks');for(const c of report.checks){const d=make('details');d.append(make('summary',`${c.status}: ${c.id} (${c.kind})`));d.append(make('p',c.message));if(c.witness){const b=make('button','経路を表示');b.type='button';b.onclick=()=>{renderUC('');witnessView(c.id,c.witness)};d.append(b)};checks.append(d)}
 const diagnostics=el('diagnostics');for(const d of report.diagnostics){const row=make('details');row.append(make('summary',`${d.severity}: ${d.code} — ${d.subject||''}`));row.append(make('p',d.message));if(d.source?.file)row.append(make('code',d.source.file+(d.source.pointer||'')));if(d.witness){const b=make('button','反例の初期値・経路');b.type='button';b.onclick=()=>{renderUC('');witnessView(d.code,d.witness)};row.append(b)};diagnostics.append(row)}
 if(report.impact)el('impact').append(make('h2','変更後の再確認候補'),make('p','依存関係から抽出した候補です。変更の意味や、目的・期待結果・設計要求への再合意を自動で判断したものではありません。'),make('pre',JSON.stringify(report.impact,null,2)));
 el('download-json').onclick=()=>{const blob=new Blob([JSON.stringify(report,null,2)],{type:'application/json'});const a=make('a');const url=URL.createObjectURL(blob);a.href=url;a.download='report.json';a.click();setTimeout(()=>URL.revokeObjectURL(url),1000)};
 renderUC('');
})();
