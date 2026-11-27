// 用分层山脊、剪影和剑光绘制山门。
const canvas=document.querySelector('canvas'), ctx=canvas.getContext('2d');
function polygon(points,color){
  ctx.fillStyle=color;ctx.beginPath();
  points.forEach(([x,y],i)=>i?ctx.lineTo(x,y):ctx.moveTo(x,y));
  ctx.closePath();ctx.fill();
}
// 挥刀和收招占出手间隔的一部分，极高攻速时保留可见的动作周期。
function swingDuration(intervalMs){
  return intervalMs>0 ? Math.min(intervalMs, 700, Math.max(300, intervalMs*0.55)) : 300;
}
function drawScene(enemies,target,hits,started,now,swings,inCombat=false,player=null,allies=[],battleMs=0){
  const w=canvas.clientWidth,h=canvas.clientHeight,d=Math.min(devicePixelRatio,2);
  if(canvas.width!==Math.round(w*d)||canvas.height!==Math.round(h*d)){canvas.width=Math.round(w*d);canvas.height=Math.round(h*d)}
  ctx.setTransform(canvas.width/900,0,0,canvas.height/350,0,0);
  const sky=ctx.createLinearGradient(0,0,0,350);
  sky.addColorStop(0,'#183541');sky.addColorStop(.65,'#a4b6a4');sky.addColorStop(1,'#e5c898');
  ctx.fillStyle=sky;ctx.fillRect(0,0,900,350);
  ctx.fillStyle='#efd8a0';ctx.beginPath();ctx.arc(604,91,46,0,Math.PI*2);ctx.fill();
  for(let k=0;k<3;k++){
    let ridge=[[0,350]];
    for(let x=0;x<=910;x+=10)ridge.push([x,150+k*37+Math.sin(x*.014+k*2)*36+Math.cos(x*.031+k)*17+Math.sin(x*.08)*5]);
    polygon([...ridge,[910,350]],['#76948e','#526f70','#314e52'][k]);
  }
  for(let i=0;i<4;i++){
    let y=218-i*24,r=52-i*10;
    polygon([[435-r,y],[435-r*.38,y-10],[435,y-21],[435+r*.4,y-10],[435+r,y],[435+r*.6,y+3],[435-r*.6,y+3]],'#233f43');
    ctx.fillStyle='#283f40';ctx.fillRect(435-r*.35,y,r*.7,20)
  }
  polygon([[0,273],[210,250],[610,254],[900,267],[900,350],[0,350]],'#263c40');
  for(let i=0;i<7;i++)polygon([[0,278+i*11],[900,274+i*12],[900,276+i*12]],'#4d615c');
  const approach=started===null?0:Math.min(1,Math.max(0,(now-started)/550));
  const close=1-(1-approach)**2;
  const positions=[];
  for(let id=-1-allies.length;id<enemies.length;id++){
    const hero=id===-1;
    const ally=id< -1;
    const unit=hero?player:ally?allies[-id-2]:enemies[id];
    const side=ally?1:hero?(target===1?-1:1):(id===0?-1:1);
    const home=ally?150-80*(-id-2)/Math.max(1,allies.length):hero?192:635+205*id/Math.max(1,enemies.length-1);
    const center=ally?350-160*(-id-2)/Math.max(1,allies.length):hero?450:id===0?497:403+Math.max(0,id-1)*43;
    const attackTime=swings ? swings.get(id) : hits.findLast(hit=>hit.from===id&&!hit.heal)?.time;
    const age=now-attackTime;
    const duration=swingDuration(unit?.action_interval_ms);
    const phase=Math.min(1,Math.max(0,Number.isFinite(age)?age/duration:1));
    // 举刀时后撤，劈下时踏前，最后收回架势。
    const ease=t=>t*t*(3-2*t);
    const wind=ease(Math.min(1,phase/.28));
    const cut=ease(Math.min(1,Math.max(0,(phase-.28)/.32)));
    const recover=ease(Math.max(0,(phase-.60)/.40));
    const windup=inCombat&&unit?.hp?unit.windup:null;
    const guarding=inCombat&&unit?.hp&&unit.guard_until_ms>battleMs;
    const lift=windup?Math.min(1,Math.max(0,(battleMs-(windup.start_ms||0))/230)):0;
    const swing=windup?-.46*lift:guarding?-.18:(-.38*wind+1.38*cut)*(1-recover);
    const blade=windup?-1.55*lift:guarding?-.72:(-1.25*wind+2.7*cut)*(1-recover);
    const stagger=hits.some(hit=>hit.to===id&&!hit.heal&&now-hit.time<140);
    // 步法、重心和持剑姿势使用错开的相位，让换向时仍能看到举剑动作。
    const engaged=inCombat&&(unit?.hp>0);
    const beat=(now-started)/(hero?235:265+id*13)+id*2;
    const stride=engaged?Math.sin(beat)*close:0;
    const guard=engaged?Math.cos(beat)*close:0;
    const x=home+(center-home)*close+side*(stride*11+swing*14-(stagger?4:0));
    const y=262+(hero?0:id===0?-8:8)*close-Math.abs(guard)*4
      +Math.sin(now/85+id)*3*(1-close)*approach;
    positions.push({unit,x,y,side:approach===0?(hero?1:-1):side,swing,blade,phase,stagger,stride,guard,id,windup,guarding});
  }
  for(const {unit,x,y,side,swing,blade,phase,stagger,stride,guard,id,windup,guarding} of [...positions].sort((a,b)=>a.y-b.y)){
    if(id>=0&&!enemies[id].hp&&!hits.some(hit=>hit.to===id&&now-hit.time<200))continue;
    if(inCombat&&enemies.some(e=>e.hp&&e.windup&&e.windup.target_id===unit?.instance_id)){
      ctx.save();ctx.strokeStyle='#ff9b76';ctx.lineWidth=3;
      ctx.beginPath();ctx.ellipse(x,y+3,22,6,0,0,Math.PI*2);ctx.stroke();ctx.restore();
    }
    const color=id<0?'#8cd9c6':'#d99372';
    ctx.save();ctx.translate(x,y);ctx.scale(side,1);
    polygon([[-12,-15],[-4,-15],[-10-stride*7,1],[-19-stride*7,1]],'#162b30');
    polygon([[0,-15],[8,-14],[12+stride*7,2],[2+stride*7,2]],'#182e32');
    const friendly=id<0;
    const trim=friendly?'#def6cd':'#ffcf87';
    const armor=friendly?'#316863':'#874a43';
    ctx.rotate(stride*.06+swing*.17-(stagger ? .10 : 0));
    // 披风的下摆滞后于身体，近身挥砍时仍能看清重心和朝向。
    const hem=-24-swing*13-guard*3;
    polygon([[-7,-47],[-13,-40],[hem,-14],[hem+9,-17],[-17,-6],[1,-20]],armor);
    polygon([[-9,-47],[5,-47],[12,-24],[15,-8],[0,-13],[-14,-6],[-11,-26]],color);
    polygon([[-11,-29],[9,-31],[11,-26],[-11,-24]],'#26373a');
    polygon([[-6,-46],[0,-47],[6,-32],[2,-31]],trim);
    ctx.fillStyle='#24383c';ctx.beginPath();ctx.arc(0,-53,8,0,7);ctx.fill();
    polygon([[2,-55],[9,-55],[8,-51],[4,-51]],trim);
    if(friendly){
      polygon([[-17,-55],[-3,-66],[2,-65],[15,-54]],armor);
      polygon([[-17,-55],[15,-55],[12,-53],[-14,-53]],trim);
    }else{
      polygon([[-10,-52],[-10,-63],[-4,-59],[2,-63],[9,-63],[7,-54]],armor);
      polygon([[-11,-47],[-4,-50],[3,-43],[-6,-39],[-14,-41]],trim);
    }
    ctx.translate(2,-41+guard*2);
    const handX=9+swing*7,handY=6+blade*4;
    polygon([[-4,-3],[2,-4],[5,3],[handX,handY-2],[handX+1,handY+2],[2,10],[-5,4]],armor);
    ctx.translate(handX,handY);ctx.rotate(-.78+guard*.12+blade);
    if(!windup&&!guarding&&phase>.28&&phase<.7){
      ctx.save();ctx.globalAlpha=.65*Math.sin(Math.PI*(phase-.28)/.42);
      ctx.strokeStyle=trim;ctx.lineWidth=3;ctx.beginPath();
      ctx.arc(0,0,42,-.95,0);ctx.stroke();ctx.restore();
    }
    polygon([[-6,-2],[6,-2],[6,2],[-6,2]],'#473f36');
    polygon([[4,-6],[7,-5],[8,5],[5,6]],trim);
    polygon([[7,-2],[39,-3],[48,-6],[41,1],[7,3]],windup?'#ffab77':guarding?'#adf3e5':'#e5efdf');
    ctx.restore();
    if(guarding){
      ctx.save();ctx.strokeStyle='#9ee6d5';ctx.lineWidth=3;
      ctx.beginPath();ctx.arc(x,y-32,22,side>0?-1.2:Math.PI-1.2,side>0?1.2:Math.PI+1.2);ctx.stroke();ctx.restore();
    }
    if(windup){
      const start=windup.start_ms||0;
      const progress=Math.min(1,Math.max(0,(battleMs-start)/Math.max(1,windup.impact_ms-start)));
      ctx.fillStyle='#492a2a';ctx.fillRect(x-23,y+7,46,6);
      ctx.fillStyle='#ffab77';ctx.fillRect(x-22,y+8,44*progress,4);
    }
    if(id>=0&&id===target)polygon([[x-5,y-79],[x+5,y-79],[x,y-72]],'#ffdc93');
  }
  for(const hit of hits){
    const position=positions.find(p=>p.id===hit.to);
    if(!position)continue;
    const {x,y}=position,age=now-hit.time;
    ctx.save();ctx.globalAlpha=Math.min(1,(640-age)/180);
    ctx.fillStyle=hit.heal?'#a9ffe1':'#ffe2aa';ctx.strokeStyle='#23383b';ctx.lineWidth=3;
    ctx.font='bold 21px serif';ctx.textAlign='center';
    const tx=x+(hit.to===-1?hit.from*20:0),ty=y-84-age*.055;
    ctx.strokeText(hit.text,tx,ty);ctx.fillText(hit.text,tx,ty);ctx.restore();
  }
  // 将相邻单位的生命条分层放置，避免近身交锋时数值互相遮挡。
  const labels=[];
  ctx.save();
  ctx.font='bold 15px sans-serif';ctx.textAlign='center';
  for(const {unit,id,x,y} of positions){
    if(!unit)continue;
    const hp=unit.hp??0,max=unit.max_hp??unit.max;
    if(!max)continue;
    const value=hp+' / '+max,width=Math.max(68,ctx.measureText(value).width+12);
    const left=Math.max(0,Math.min(900-width,x-width/2));
    let top=y-110;
    while(labels.some(r=>left<r.right+3 && left+width>r.left-3 && Math.abs(top-r.top)<27))top-=27;
    labels.push({left,right:left+width,top});
    ctx.fillStyle='#13282e';ctx.fillRect(left,top,width,24);
    ctx.fillStyle=id<0?'#84d8bd':'#eea080';
    ctx.fillRect(left+2,top+18,(width-4)*Math.min(1,Math.max(0,hp/max)),4);
    ctx.fillStyle='#fff4df';ctx.fillText(value,left+width/2,top+14);
  }
  ctx.restore();

}
