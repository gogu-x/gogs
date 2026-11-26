const el=id=>document.getElementById(id);
let socket,phase='offline',battle='',control=null,hero=null,allies=[],enemies=[],target=-1;
let sequence=0,cursor=0,pending=0,clock=0,next=0,started=null,hits=[],preferred='',fallback=false;
let automatic=false;
let casts=0,damage=0,elapsed=0;
let pendingBody=null;
let attackTimer,requestTimer,claim='',offers=[],claimed=[];
const swings=new Map();
const queuedSwings=new Map();
const message=text=>{el('log').textContent=text};
function status(value,title){
  phase=value;el('place').textContent=title;
  el('start').disabled=phase!=='lobby';
  el('start').textContent=phase==='play'?(automatic?'自动战斗中':'手动战斗中'):phase==='starting'?'等待入场':'进入战斗';
  el('login').hidden=phase!=='offline';el('disconnect').hidden=phase==='offline';
}
function send(name,body){socket.send(battleWire.encode(name,body))}
function clearInput(){
  pending=0;pendingBody=null;
  clearTimeout(requestTimer);el('retry-input').hidden=true;
}
function stopTimers(){clearTimeout(attackTimer);clearInput()}
function transmitInput(retry){
  if(phase!=='play'||!pendingBody)return;
  clearTimeout(requestTimer);el('retry-input').hidden=true;
  send('BattleInputReq',pendingBody);
  requestTimer=setTimeout(()=>{
    if(!pendingBody||phase!=='play')return;
    if(!retry){message('仍在等待出手确认，正在重发同一次出招。');transmitInput(true);return}
    el('retry-input').hidden=false;
    message('第 '+pending+' 次出招尚未收到确认，可重试。');
  },15000);
}
el('retry-input').onclick=()=>transmitInput(true);
function resetBattle(){
  automatic=false;swings.clear();queuedSwings.clear();casts=damage=elapsed=0;
  stopTimers();battle='';control=null;hero=null;allies=[];enemies=[];target=-1;sequence=cursor=pending=0;
  started=null;hits=[];preferred='';fallback=false;claim='';offers=[];claimed=[];render();
}
function disconnect(reason){
  const old=socket;socket=null;
  if(old)old.close();
  resetBattle();status('offline','山门');el('session').textContent='尚未连接';message(reason);
}
function deadline(label){
  clearTimeout(requestTimer);
  requestTimer=setTimeout(()=>disconnect(label+'超时，请重新登录。'),15000);
}
el('disconnect').onclick=()=>disconnect('已断开连接。');
el('login').onsubmit=event=>{
  event.preventDefault();
  const register=event.submitter?.value==='register';
  const login={account:el('account').value.trim(),password:el('password').value,server_id:Number(el('server').value)};
  let ws;
  try{
    const url=new URL(el('address').value);
    if(!['ws:','wss:'].includes(url.protocol))throw Error('Gate 地址需使用 ws 或 wss');
    ws=new WebSocket(url,'protobuf');
  }catch(err){message(err.message);return}
  socket=ws;ws.binaryType='arraybuffer';
  status('connecting','连接中');message('正在连接 Gate…');deadline('连接');
  ws.onopen=()=>{
    if(socket!==ws)return;
    message(register?'正在注册账号…':'正在登录…');
    send(register?'RegisterReq':'LoginReq',login);deadline(register?'注册':'区服登录');
  };
  ws.onmessage=event=>{
    if(socket!==ws)return;
    try{
      const packet=battleWire.decode(event.data);if(!packet)return;
      const {name,data}=packet;
      if(name==='RegisterAck'||name==='LoginAck'){
        if(data.code){disconnect('账号请求失败，服务端错误码 '+data.code);return}
        if(name==='RegisterAck'){
          message('注册成功，正在进入 '+login.server_id+' 区…');send('LoginReq',login);deadline('区服登录');return;
        }
        login.password='';el('password').value='';clearTimeout(requestTimer);
        el('session').textContent=login.account+' · '+login.server_id+' 区';
        status('lobby','准备就绪');message('已登录，点击进入战斗。');return;
      }
      receive(name,data);
    }catch(err){disconnect('处理网关消息失败：'+err.message)}
  };
  ws.onerror=()=>{if(socket===ws)disconnect('无法连接 Gate，请检查地址及服务状态。')};
  ws.onclose=()=>{if(socket===ws)disconnect('网关连接已断开，请重新登录。')};
};
el('start').onclick=()=>{
  if(phase!=='lobby')return;
  resetBattle();status('starting','正在入场');message('等待战斗快照…');
  send('StartBattleReq',{});deadline('进战');
};
function attack(skill,defaultTarget=false){
  if(phase!=='play'||pending||!hero?.hp||!control)return;
  if(performance.now()<clock+next){message('尚未到下一次出手时间。');return}
  const body={battle_id:battle,unit_id:control.unit_id,sequence:++sequence,skill_id:skill};
  if(skill===control.basic_skill&&enemies[target]?.hp&&!defaultTarget)body.target_id=enemies[target].instance_id;
  pending=sequence;pendingBody=body;transmitInput(false);render();
}
function schedule(){
  clearTimeout(attackTimer);
  if(!automatic||phase!=='play'||pending||!hero?.hp||!control)return;
  const wait=clock+next-performance.now();
  if(wait>0){attackTimer=setTimeout(schedule,Math.ceil(wait));return}
  attack(fallback||!preferred?control.basic_skill:preferred,fallback);
}
el('auto').onclick=()=>{
  if(phase!=='play')return;
  automatic=!automatic;fallback=false;
  status('play','交战中');render();schedule();
};
function useControl(updated){
  if(!updated||updated.unit_id!==hero?.instance_id)return;
  control=updated;
  hero.action_interval_ms=updated.action_interval_ms;
  if(preferred&&!(updated.active_skills||[]).includes(preferred))preferred='';
}
function selectTarget(){
  if(!enemies[target]?.hp)target=enemies.findIndex(e=>e.hp>0);
}
function receive(name,m){
  if(name==='StartBattleAck'||name==='BattleAdmissionFailedNtf'){
    if(phase==='starting'&&m.code){clearTimeout(requestTimer);status('lobby','入场失败');message('服务端错误码 '+m.code)}
    return;
  }
  if(name==='BattleCreatedNtf'){
    if(!['starting','play'].includes(phase)||(battle&&battle!==m.battle_id)||(m.snapshot_sequence||0)<cursor)return;
    const own=m.controls?.[0];
    if(!own)throw Error('缺少角色控制信息');
    const selected=enemies[target]?.instance_id;
    hero=m.units.find(u=>u.instance_id===own.unit_id);
    if(!hero)throw Error('缺少角色快照');
    allies=m.units.filter(u=>u.team===1&&u.instance_id!==own.unit_id);
    enemies=m.units.filter(u=>u.team===2);
    target=enemies.findIndex(e=>e.instance_id===selected);selectTarget();
    useControl(own);
    offers=own.available_grants||[];claimed=hero.received_grants||[];
    if(!battle){preferred=own.active_skills?.[0]||'';started=performance.now()}
    battle=m.battle_id;cursor=m.snapshot_sequence||0;sequence=Math.max(sequence,own.last_input_sequence||0);
    if(!pending||pending<=(own.last_input_sequence||0)){clearInput()}
    clock=performance.now()-(m.current_ms||0);next=own.next_ready_ms||0;
    status('play','交战中');message(automatic?'自动出招中。':'点击普攻或技能出招。');render();schedule();return;
  }
  if(!battle||m.battle_id!==battle)return;
  if(name==='BattleFinishedNtf'){
    stopTimers();const units=m.units||[];hero=units.find(u=>u.instance_id===hero?.instance_id)||hero;
    allies=units.filter(u=>u.team===1&&u.instance_id!==hero?.instance_id);
    enemies=units.filter(u=>u.team===2);offers=[];claim='';battle='';selectTarget();render();
    const result=m.outcome===1?'获胜':m.outcome===2?'落败':'平局';
    status('lobby',result);
    message(result+' · 战斗 '+(elapsed/1000).toFixed(2)+' 秒 · 我方施招 '+casts+' 次 · 造成伤害 '+damage);
    return;
  }
  if(name==='ClaimBattleGrantAck'){
    if(claim===m.grant_id)claim='';
    if(!m.code)offers=offers.filter(g=>g.grant_id!==m.grant_id);
    message(m.code?'领取失败，错误码 '+m.code:'已领取');render();return;
  }
  if(name==='BattleInputAck'){
    if(m.unit_id!==control.unit_id||m.sequence!==pending)return;
    clearInput();
    clock=performance.now()-(m.current_ms||0);next=m.ready_ms||0;
    // 技能冷却或目标刚死亡时，下次尝试服务端默认选敌的普攻。
    fallback=Boolean(m.code);
    if(m.code)next=Math.max(next,(m.current_ms||0)+100);
    if(!automatic&&m.code)message('本次出招未被接受，可尝试普攻或其他技能。');
    render();schedule();return;
  }
  if(name!=='BattleActionNtf')return;
  const e=m.event;if(!e||e.sequence<=cursor)return;cursor=e.sequence;
  elapsed=Math.max(elapsed,e.elapsed_ms||0);
  if(e.actor_id===hero.instance_id){
    if(e.type===2)casts++;
    if(e.type===3)damage+=e.amount||0;
  }
  if(e.joined_unit){
    const group=e.joined_unit.team===2?enemies:allies;
    if(e.joined_unit.instance_id!==hero.instance_id&&!group.some(u=>u.instance_id===e.joined_unit.instance_id))group.push(e.joined_unit);
  }
  const idx=enemies.findIndex(u=>u.instance_id===e.target_id);
  const unit=idx>=0?enemies[idx]:hero?.instance_id===e.target_id?hero:allies.find(u=>u.instance_id===e.target_id);
  if((e.type===3||e.type===4)&&unit){
    unit.hp=e.hp_after||0;
    hits.push({from:enemies.findIndex(u=>u.instance_id===e.actor_id),to:idx>=0?idx:unit===hero?-1:-2-allies.indexOf(unit),time:performance.now(),
      text:(e.type===3?'−':'+')+(e.amount||0),heal:e.type===4});
    selectTarget();
  }
  if(e.updated_unit && e.updated_unit.instance_id!==hero.instance_id){
    const updated=[...allies,...enemies].find(u=>u.instance_id===e.updated_unit.instance_id);
    if(updated)Object.assign(updated,e.updated_unit,{hp:e.updated_unit.hp??0});
  }
  if(e.updated_unit?.instance_id===hero.instance_id){
    Object.assign(hero,e.updated_unit,{hp:e.updated_unit.hp??0});
    if(e.grant){claimed.push(e.grant);offers=offers.filter(g=>g.grant_id!==e.grant.grant_id)}
  }
  if(e.updated_control){
    const updated=e.updated_control;
    if(updated.unit_id===hero.instance_id){
      useControl(updated);clock=performance.now()-(e.elapsed_ms||0);next=control.next_ready_ms||0;schedule();
    }else{
      const unit=[...allies,...enemies].find(u=>u.instance_id===updated.unit_id);
      if(unit)unit.action_interval_ms=updated.action_interval_ms;
    }
  }
  if(e.type===2){
    const enemyIndex=enemies.findIndex(u=>u.instance_id===e.actor_id);
    const allyIndex=allies.findIndex(u=>u.instance_id===e.actor_id);
    const self=e.actor_id===hero.instance_id;
    const actor=self?-1:enemyIndex>=0?enemyIndex:-2-allyIndex;
    const unit=self?hero:enemyIndex>=0?enemies[enemyIndex]:allies[allyIndex];
    if(unit)queuedSwings.set(actor,(queuedSwings.get(actor)||0)+1);
    message(e.actor_id+' 施放技能 '+e.skill_id);
  }
  if(((e.type===3||e.type===4)&&unit)||e.updated_unit||e.updated_control||e.joined_unit)render();
}
function button(label,action,disabled=false){
  const b=document.createElement('button');b.textContent=label;b.onclick=action;b.disabled=disabled;return b;
}
function grantName(g){
  if(g.equip_config_id===4)return '缚灵护腕';
  if(g.equip_config_id!==undefined)return '装备 '+g.equip_config_id;
  if(g.talent_node_id!==undefined)return '天赋 '+g.talent_node_id;
  return '状态 '+(g.status_config_id??'');
}
let hudDirty=true;
function render(){hudDirty=true}
function paintHud(){
  el('auto').disabled=phase!=='play';
  el('auto').textContent=automatic&&phase==='play'?'切回手动':'开启自动';
  el('auto').setAttribute('aria-pressed',automatic&&phase==='play');
  el('hero').textContent=hero?.instance_id||'执剑人';
  el('hp').max=hero?.max_hp||1;el('hp').value=hero?.hp||0;
  el('life').textContent=hero?(hero.hp||0)+' / '+hero.max_hp:'等待入场';
  el('gear').textContent=[control?.action_interval_ms?'出手间隔 '+control.action_interval_ms+' 毫秒':'',...claimed.map(grantName)].filter(Boolean).join(' · ');
  el('targets').replaceChildren(...enemies.map((e,i)=>{
    const b=button(e.instance_id+' · '+(e.hp||0)+' / '+e.max_hp,()=>{target=i;render()},!e.hp);
    b.setAttribute('aria-pressed',i===target);return b;
  }));
  el('actions').replaceChildren(...(control?[control.basic_skill,...(control.active_skills||[])]:[]).map(id=>{
    const basic=id===control.basic_skill;
    const b=button((basic?'普攻 ':automatic?'优先技能 ':'技能 ')+id,()=>{
      if(automatic){preferred=basic?'':id;render()}else attack(id);
    },phase!=='play'||(!automatic&&Boolean(pending)));
    if(automatic)b.setAttribute('aria-pressed',basic?!preferred:preferred===id);return b;
  }));
  el('grants').replaceChildren(...offers.map(g=>button('领取 '+grantName(g),()=>{
    claim=g.grant_id;send('ClaimBattleGrantReq',{battle_id:battle,unit_id:control.unit_id,grant_id:claim});render();
  },Boolean(claim))));
}
function animate(now){
  if(hudDirty){hudDirty=false;paintHud()}
  hits=hits.filter(h=>now-h.time<640);
  // 同一帧收到多次施招时逐次播放，血量仍在收到事件时更新。
  for(const [actor,count] of queuedSwings){
    const unit=actor===-1?hero:actor>=0?enemies[actor]:allies[-2-actor];
    if(now-(swings.get(actor)??-Infinity)<swingDuration(unit?.action_interval_ms))continue;
    swings.set(actor,now);
    if(count===1)queuedSwings.delete(actor);
    else queuedSwings.set(actor,count-1);
  }
  drawScene(enemies,target,hits,started,now,swings,phase==='play',hero,allies);
  requestAnimationFrame(animate);
}
requestAnimationFrame(animate);
