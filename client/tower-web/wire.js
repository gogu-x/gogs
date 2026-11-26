// 与 tree/codec 的两字节大端消息号和 Protobuf 二进制正文互通。
// 字段号来自 protocol/cspb，仅解码页面使用的字段，其余按 wire type 跳过。
const battleWire = (() => {
  const encoder = new TextEncoder(), decoder = new TextDecoder();
  const auth = ['account:s','password:s','server_id'];
  const schemas = {
    RegisterReq:auth, LoginReq:auth, LoginAck:['code'], RegisterAck:['code'],
    StartBattleReq:[], StartBattleAck:['battle_id:s',null,'code'],
    BattleInputReq:['battle_id:s','unit_id:s','sequence','skill_id:s','target_id:s'],
    ClaimBattleGrantReq:['battle_id:s','unit_id:s','grant_id:s'],
    ClaimBattleGrantAck:['battle_id:s','unit_id:s','grant_id:s','code'],
    BattleAdmissionFailedNtf:['battle_id:s',null,'code'],
    BattleCreatedNtf:['battle_id:s',null,null,'units:*Unit','controls:*Control','snapshot_sequence','current_ms'],
    BattleActionNtf:['battle_id:s',null,'event:Event'],
    BattleInputAck:['battle_id:s','unit_id:s','sequence','code',null,'ready_ms',null,null,'current_ms'],
    BattleFinishedNtf:['battle_id:s',null,'outcome','units:*Unit'],
    Unit:['instance_id:s','team','hp','max_hp','alive',null,'received_grants:*Grant'],
    Grant:['grant_id:s','equip_config_id','talent_node_id','status_config_id'],
    Control:['unit_id:s','basic_skill:s','active_skills:*s',null,'last_input_sequence','next_ready_ms','available_grants:*Grant','attack','defense','speed','action_interval_ms'],
    Event:['sequence',null,'elapsed_ms','type','actor_id:s','target_id:s','skill_id:s',null,'amount',null,'hp_after',
      ...Array(14).fill(null),'joined_unit:Unit','grant:Grant','updated_unit:Unit','updated_control:Control']
  };
  const ids=new Map(), names=new Map();
  for(const name of Object.keys(schemas).filter(n=>/Req$|Ack$|Ntf$/.test(n))){
    const pkg=/^(Register|Login)/.test(name)?'gateway':'battle';
    let hash=2166136261;
    for(const ch of 'github.com/gogu-x/gogs/pb/cspb/pb_'+pkg+'.'+name)hash=Math.imul(hash^ch.charCodeAt(0),16777619);
    const id=hash&65535; ids.set(name,id); names.set(id,name);
  }
  function appendInt(bytes,value){
    let n=BigInt(value);
    if(n<0n)throw Error('请求数值越界');
    while(n>=128n){bytes.push(Number(n&127n)|128);n>>=7n}
    bytes.push(Number(n));
  }
  function encode(name,message){
    const id=ids.get(name),bytes=[id>>8,id&255];
    schemas[name].forEach((field,index)=>{
      if(!field)return;
      const [key,type]=field.split(':'),value=message[key];
      if(value==null)return;
      appendInt(bytes,(index+1)*8+(type==='s'?2:0));
      if(type==='s'){
        const body=encoder.encode(value);appendInt(bytes,body.length);
        for(const byte of body)bytes.push(byte);
      }else appendInt(bytes,value);
    });
    return Uint8Array.from(bytes);
  }
  function unpack(name,buf){
    const result={};let pos=0;
    function integer(){
      let n=0n;
      for(let shift=0n;shift<70n&&pos<buf.length;shift+=7n){
        const byte=buf[pos++];n|=BigInt(byte&127)<<shift;if(byte<128)return n;
      }
      throw Error('无效的协议整数');
    }
    while(pos<buf.length){
      const tag=Number(integer()),wire=tag%8;
      const field=schemas[name][Math.floor(tag/8)-1];
      let value;
      if(wire===0){const n=integer();if(field)value=field==='code'?Number(BigInt.asIntN(32,n)):Number(n)}
      else{
        const len=wire===2?Number(integer()):wire===1?8:wire===5?4:-1;
        if(len<0||len>buf.length-pos)throw Error('无效的协议长度');
        value=buf.subarray(pos,pos+len);pos+=len;
      }
      if(!field)continue;
      const [key,format='']=field.split(':'),type=format.replace('*','');
      if(type)value=type==='s'?decoder.decode(value):unpack(type,value);
      if(format.startsWith('*'))(result[key]??=[]).push(value);else result[key]=value;
    }
    return result;
  }
  function decode(buffer){
    const buf=new Uint8Array(buffer),name=names.get(buf[0]*256+buf[1]);
    return name?{name,data:unpack(name,buf.subarray(2))}:null;
  }
  return {encode,decode};
})();
