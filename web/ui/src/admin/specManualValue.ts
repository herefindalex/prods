import type {NumericProfile} from './specCommands';
export type ManualInterval={lower:string;upper:string;lower_inclusive:boolean;upper_inclusive:boolean};
export type ManualFields={shape:string;value?:string;values?:string;intervals?:ManualInterval[];nominal?:string;amount?:string;tolerance_type?:'relative'|'absolute';direction?:'lower'|'upper';inclusive?:boolean;condition?:string};
function decimal(raw:string|undefined):{n:bigint;s:number} {
 if(!raw||raw.length>128||!/^[-+]?\d+(\.\d+)?$/.test(raw.trim()))throw new Error('Invalid decimal');
 const [whole,fraction='']=raw.trim().split('.');return {n:BigInt(whole+fraction),s:fraction.length};
}
function render(n:bigint,s:number):string {const sign=n<0n?'-':'';let digits=(n<0n?-n:n).toString().padStart(s+1,'0');if(s)digits=digits.slice(0,-s)+'.'+digits.slice(-s);if(digits.includes('.'))digits=digits.replace(/0+$/,'').replace(/\.$/,'');return sign+digits;}
function exact(raw:string|undefined):string{const d=decimal(raw);return render(d.n,d.s);}
export function manualValue(profile:NumericProfile,fields:ManualFields):Record<string,unknown> {
 if(!profile.allowed_shapes.includes(fields.shape))throw new Error('Unsupported shape');
 const result:Record<string,unknown>={schema_version:profile.schema_version,quantity:profile.quantity,semantic_role:profile.semantic_role,shape:fields.shape,unit:profile.canonical_unit,unit_registry_version:profile.unit_registry_version,context:fields.condition?.trim()?{kind:'conditional',raw:fields.condition.trim()}:{kind:'unconditional'}};
 switch(fields.shape){
 case 'scalar':result.scalar=exact(fields.value);break;
 case 'discrete_set':result.values=(fields.values??'').split(',').map(exact);break;
 case 'interval_set':result.intervals=(fields.intervals??[]).map(i=>({...i,lower:exact(i.lower),upper:exact(i.upper)}));break;
 case 'bound_claim':result.bound={direction:profile.semantic_role==='error_magnitude'?'upper':fields.direction??'upper',value:exact(fields.value),inclusive:fields.inclusive??true};break;
 case 'tolerance':{
  const nominal=decimal(fields.nominal),amount=decimal(fields.amount);
  let offset=amount;
  if(fields.tolerance_type==='relative')offset={n:(nominal.n<0n?-nominal.n:nominal.n)*amount.n,s:nominal.s+amount.s+2};
  const scale=Math.max(nominal.s,offset.s),n=nominal.n*10n**BigInt(scale-nominal.s),delta=offset.n*10n**BigInt(scale-offset.s);
  result.tolerance={nominal:render(nominal.n,nominal.s),amount:render(amount.n,amount.s),type:fields.tolerance_type??'absolute',interval:{lower:render(n-delta,scale),upper:render(n+delta,scale),lower_inclusive:true,upper_inclusive:true}};break;
 }
 default:throw new Error('Unsupported shape');
 }
 return result;
}
export function factLabel(encoded:string|undefined):string {
 if(!encoded)return '—';try{const v=JSON.parse(encoded);const payload=v.scalar??v.values?.join(', ')??v.intervals?.map((i:ManualInterval)=>`${i.lower_inclusive?'[':'('}${i.lower}, ${i.upper}${i.upper_inclusive?']':')'}`).join(' ∪ ')??(v.bound?`${v.bound.direction==='upper'?(v.bound.inclusive?'≤':'<'):(v.bound.inclusive?'≥':'>')} ${v.bound.value}`:v.tolerance?`${v.tolerance.nominal} ± ${v.tolerance.amount}${v.tolerance.type==='relative'?' %':''}`:'—');return `${payload} ${v.unit}${v.context?.kind==='conditional'?` (${v.context.raw})`:''}`;}catch{return '—';}
}
