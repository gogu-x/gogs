const el=id=>document.getElementById(id);
const monsters=[
  {name:'山门刀客',max:830,atk:84,def:37,speed:110},
  {name:'石阶守卫',max:860,atk:88,def:39,speed:105},
  {name:'夜行术士',max:890,atk:92,def:41,speed:110},
  {name:'掌灯医师',max:920,atk:96,def:43,speed:105}
];
const floors=[[0],[1,2],[2,3]];
const places=['山门初照','雾隐石阶','月下灯台'];
const skills=[['斩击','单体普攻',0],['横扫','攻击全体',3],['破军','单体重击',3],['回春','恢复自身',5]];
const maxHP=1250,attack=193;
let floor=0,hp,enemies,target,turn,ready,cooldowns,phase,hits,started,notice;
const loot=[
  {name:'疾风腕扣',speed:25},
  {name:'流光剑穗',speed:40},
  {name:'追影指环',speed:60}
];
const baseInterval=850;
let interval,dropState;
function showGear(item){
  el('gear').textContent=(item ? item.name+' · 攻速 +'+item.speed+'%' : '普通攻速')
    +' · '+(interval/1000).toFixed(2)+' 秒/次';
}
el('drop').onclick=()=>{
  if(phase!=='play'||dropState!=='available')return;
  const now=performance.now();
  const item=loot[floor];
  const nextInterval=baseInterval/(1+item.speed/100);
  ready=now+Math.max(0,ready-now)*nextInterval/interval;
  interval=nextInterval;
  dropState='equipped';
  el('drop').hidden=true;
  showGear(item);
};
let automatic=false;
el('auto').onclick=()=>{if(phase==='play')automatic=!automatic};
function prepare(){
  automatic=false;
  interval=baseInterval;dropState='waiting';el('drop').hidden=true;
  showGear(null);
  hp=maxHP;turn=0;ready=0;cooldowns=[0,0,0,0];target=0;hits=[];started=null;phase='ready';
  enemies=floors[floor].map(id=>({...monsters[id],action_interval_ms:100000/monsters[id].speed,id,hp:monsters[id].max,turn:0,next:0,skillReady:0,healReady:0}));
  el('floor').textContent='试炼层数　'+(floor+1)+' / 3';el('place').textContent=places[floor];
  notice='开战后点击普攻或技能出招。';
  el('targets').replaceChildren(...enemies.map((e,i)=>{
    const b=document.createElement('button');b.onclick=()=>{if(e.hp)target=i};return b
  }));
}
function end(won){
  el('drop').hidden=true;
  phase=won?'won':'lost';
  notice=won?(floor===2?'三层守关者已全部击败。':'守关者已击败，可以登上下一层。'):'剑修倒下了，可以重新挑战本层。';
}
function cast(i,now){
  if(phase!=='play'||now<ready||turn<cooldowns[i])return;
  if(i===3){const heal=Math.min(maxHP-hp,Math.floor(attack*.9)+10);hp+=heal;hits.push({from:-1,to:-1,text:'+'+heal,time:now,heal:true})}
  else{
    for(const [idx,e] of enemies.entries()){
      if(!e.hp||(i!==1&&idx!==target))continue;
      const amount=Math.max(1,Math.floor(attack*[1,.75,1.45][i])+[4,4,8][i]-e.def);
      e.hp=Math.max(0,e.hp-amount);hits.push({from:-1,to:idx,text:'−'+amount,time:now})
    }
  }
  notice='执剑人施展了'+skills[i][0]+'。';
  cooldowns[i]=turn+skills[i][2];turn++;ready=now+interval;
  if(dropState==='waiting' && enemies.some(enemy=>enemy.hp<=enemy.max*.75) && enemies.some(enemy=>enemy.hp>0)){
    dropState='available';
    const item=loot[floor];
    el('drop').textContent='拾取 '+item.name+' · 攻速 +'+item.speed+'%（本场）';
    el('drop').hidden=false;
  }
  if(!enemies[target].hp)target=enemies.findIndex(e=>e.hp);
  if(target<0)end(true)
}
function enemyAct(e,now){
  const allies=enemies.filter(a=>a.hp).sort((a,b)=>a.hp/a.max-b.hp/b.max);
  if(e.id===3&&allies[0].hp<=allies[0].max*.65&&e.turn>=e.healReady){
    const ally=allies[0],heal=Math.min(ally.max-ally.hp,Math.floor(e.atk*.9)+10);
    ally.hp+=heal;e.healReady=e.turn+5;
    hits.push({from:enemies.indexOf(e),to:enemies.indexOf(ally),text:'+'+heal,time:now,heal:true});notice=e.name+'为'+ally.name+'恢复生命。'
  }else{
    const heavy=(e.id===0&&e.turn%2===1||e.id===3&&hp<=maxHP*.4)&&e.turn>=e.skillReady;
    const dot=e.id===2&&e.turn>=e.skillReady;
    if(heavy||dot)e.skillReady=e.turn+(heavy?3:4);
    const damage=Math.max(1,Math.floor(e.atk*(heavy?1.45:dot?.85:.65))+(heavy?8:dot?4:0)-20);
    hp=Math.max(0,hp-damage);hits.push({from:enemies.indexOf(e),to:-1,text:'−'+damage,time:now});
    if(dot)e.poison={next:now+1000,left:3};
    notice=e.name+(heavy?'使出重击。':dot?'施放持续伤害。':'发起普攻。');
  }
  e.turn++;e.next=now+100000/e.speed;
  if(!hp)end(false)
}
function chooseSkill(){
  if(hp <= maxHP * 0.5 && turn >= cooldowns[3]) return 3;
  if(enemies.filter(enemy=>enemy.hp>0).length > 1 && turn >= cooldowns[1]) return 1;
  if(turn >= cooldowns[2]) return 2;
  return 0;
}
const actions=skills.map(([name],i)=>{
  const card=document.createElement('button');
  card.onclick=()=>{if(!automatic)cast(i,performance.now())};
  card.append(document.createTextNode(name),document.createElement('small'));
  el('actions').append(card);
  return card;
});
el('start').onclick=()=>{
  if(phase==='won'){floor=(floor+1)%3;prepare()}
  if(phase==='lost')prepare();
  phase='play';notice='战斗开始';started=performance.now();ready=started+550;enemies.forEach((e,i)=>e.next=ready+120+i*140)
};
function frame(now){
  for(const e of enemies){
    if(phase!=='play')break;
    if(e.poison&&now>=e.poison.next){hp=Math.max(0,hp-10);e.poison.next=now+1000;if(!--e.poison.left)e.poison=null;if(!hp){end(false);break}}
    if(e.hp&&now>=e.next)enemyAct(e,now);
  }
  if(automatic && phase==='play' && now>=ready) cast(chooseSkill(),now);
  hits=hits.filter(hit=>now-hit.time<640);
  drawScene(enemies,target,hits,started,now,undefined,phase==='play',{hp,max:maxHP,action_interval_ms:interval});
  el('hp').max=maxHP;el('hp').value=hp;el('life').textContent=hp+' / '+maxHP;
  enemies.forEach((e,i)=>{const b=el('targets').children[i];b.disabled=!e.hp;b.setAttribute('aria-pressed',target===i);b.textContent=e.name+' · '+e.hp+' / '+e.max});
  el('auto').disabled=phase!=='play';
  el('auto').textContent=automatic&&phase==='play'?'切回手动':'开启自动';
  el('auto').setAttribute('aria-pressed',automatic&&phase==='play');
  actions.forEach((b,i)=>{b.disabled=automatic||phase!=='play'||now<ready||turn<cooldowns[i];const cd=Math.max(0,cooldowns[i]-turn);b.lastChild.textContent=cd?'还需行动 '+cd+' 次':phase==='play'&&now<ready?'间隔 '+((ready-now)/1000).toFixed(1)+' 秒':skills[i][1]});
  el('start').hidden=phase==='play';el('start').textContent=phase==='ready'?'进入战斗':phase==='lost'?'重新挑战':floor===2?'再次登塔':'进入下一层';
  if(el('log').textContent!==notice)el('log').textContent=notice;
  requestAnimationFrame(frame)
}
prepare();requestAnimationFrame(frame);
