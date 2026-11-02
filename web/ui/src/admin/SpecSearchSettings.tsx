import {useEffect,useState} from 'react';
import {Alert,Button,Checkbox,Descriptions,Form,Input,Modal,Pagination,Progress,Select,Space,Table,Typography} from 'antd';
import {APIError} from './api';
import {specMessages,type AdminLocale} from './locales';
import {specCommands,type NumericProfile,type SpecCapabilities,type SpecSearchProfile,type NormalizationWork,type WorkItem,type SearchProfile} from './specCommands';
import type {SpecDefinition} from './types';
type Fields={filterable:boolean;kind:SearchProfile['kind'];operators:string[];delimiter?:string;template?:number};
export function SpecSearchSettings({spec,locale,canEdit,onClose,onChanged}:{spec:SpecDefinition;locale:AdminLocale;canEdit:boolean;onClose:()=>void;onChanged:()=>Promise<void>}) {
 const text=specMessages(locale);const [form]=Form.useForm<Fields>();
 const [captured,setCaptured]=useState<SpecSearchProfile>();const [capabilities,setCapabilities]=useState<SpecCapabilities>();
 const [profiles,setProfiles]=useState<NumericProfile[]>([]);const [busy,setBusy]=useState(false);const [error,setError]=useState('');const [conflict,setConflict]=useState(false);const [message,setMessage]=useState('');const [dirty,setDirty]=useState(false);
 const [work,setWork]=useState<NormalizationWork>();const [items,setItems]=useState<WorkItem[]>([]);const [total,setTotal]=useState(0);const [page,setPage]=useState(1);
 const kind=Form.useWatch('kind',form);const template=Form.useWatch('template',form);
 const load=async(preserve=false)=>{setBusy(true);try {const [next,caps]=await Promise.all([specCommands.searchProfile(spec.id),specCommands.capabilities()]);const choices=next.profile.numeric?[next.profile.numeric,...caps.numeric_profiles]:caps.numeric_profiles;setCaptured(next);setCapabilities(caps);setProfiles(choices);setConflict(false);setError('');if(!preserve){form.setFieldsValue({filterable:next.filterable,kind:next.profile.kind,operators:next.profile.operators,delimiter:next.profile.delimiter,template:0});setDirty(false);}}catch(e){setError(String(e));}finally{setBusy(false);}};
 useEffect(()=>{void load();},[spec.id]);
 const refreshWork=async()=>{if(!work)return;try{const [next,list]=await Promise.all([specCommands.normalizationWork(work.id),specCommands.workItems(work.id,page)]);setWork(next);setItems(list.items);setTotal(list.total);}catch(e){setError(String(e));}};
 useEffect(()=>{if(!work)return;void refreshWork();if(work.status!=='pending')return;const timer=setInterval(()=>void refreshWork(),2500);return()=>clearInterval(timer);},[work?.id,work?.status,page]);
 const failure=(e:unknown)=>{if(e instanceof APIError&&e.status===409){setConflict(true);setError(text.conflict);}else{setError(String(e));}};
 const save=async(values:Fields)=>{if(!captured)return;setBusy(true);setError('');let result:SpecSearchProfile;try {
  let profile:SearchProfile={version:1,kind:values.kind,text_rule:capabilities?.text_rule,operators:values.operators};
  if(values.kind==='multi_choice')profile.delimiter=values.delimiter;
  if(values.kind==='numeric'){const numeric=profiles[values.template??0];profile={version:1,kind:'numeric',operators:values.operators,numeric:{...numeric,allowed_operators:values.operators}};}
  result=await specCommands.configureSearchProfile(spec.id,captured.revision,{filterable:values.filterable,profile});
 }catch(e){failure(e);setBusy(false);return;}
 setCaptured(result);setDirty(false);setMessage(text.saved);setConflict(false);
 try{await onChanged();}catch{setError(text.refreshFailed);}finally{setBusy(false);}};
 const start=async()=>{if(!captured)return;setBusy(true);setError('');try{setWork(await specCommands.startNormalizationWork(spec.id,captured.revision));setPage(1);}catch(e){failure(e);}finally{setBusy(false);}};
 const allowed=kind==='numeric'?(profiles[template??0]?.allowed_operators??[]):kind==='text'?['equals','contains']:['equals'];
 return <Modal open title={`${text.settings}: ${spec.name}`} onCancel={onClose} footer={<Button onClick={onClose}>{text.close}</Button>} width={760}>
  {error&&<Alert type='error' showIcon title={error}/>}{message&&<Alert type='success' title={message}/>}
  <Descriptions size='small' items={[{key:'revision',label:'Revision',children:captured?.revision},{key:'semantic',label:'Semantic version',children:captured?.spec_semantic_version}]}/>
  <Form form={form} layout='vertical' onFinish={v=>void save(v)} onValuesChange={changed=>{setDirty(true);if(changed.kind){form.setFieldsValue({operators:changed.kind==='text'?['equals','contains']:changed.kind==='numeric'?(profiles[template??0]?.allowed_operators??[]):['equals']});}if(changed.template!==undefined)form.setFieldsValue({operators:profiles[changed.template]?.allowed_operators});}}>
   <Form.Item name='filterable' valuePropName='checked'><Checkbox disabled={!canEdit}>{text.filterable}</Checkbox></Form.Item>
   <Form.Item name='kind' label={text.kind} rules={[{required:true}]}><Select disabled={!canEdit} options={(capabilities?.kinds??[]).map(value=>({value,label:value.replaceAll('_',' ')}))}/></Form.Item>
   {kind==='multi_choice'&&<Form.Item name='delimiter' label={text.delimiter} rules={[{required:true}]}><Input disabled={!canEdit} maxLength={8}/></Form.Item>}
   {kind==='numeric'&&<><Form.Item name='template' label={text.profile} rules={[{required:true}]}><Select disabled={!canEdit} showSearch options={profiles.map((p,value)=>({value,label:`${p.quantity} / ${p.semantic_role.replaceAll('_',' ')} / ${p.canonical_unit}`}))} optionFilterProp='label'/></Form.Item><Typography.Paragraph type='secondary'>{profiles[template??0]?.parser_version} · {profiles[template??0]?.unit_registry_version} · {(profiles[template??0]?.allowed_shapes??[]).join(', ')}</Typography.Paragraph></>}
   <Form.Item name='operators' label={text.operators} rules={[{required:true}]}><Select mode='multiple' disabled={!canEdit} options={allowed.map(value=>({value,label:value}))}/></Form.Item>
   <Space wrap>{canEdit&&<Button type='primary' htmlType='submit' loading={busy} disabled={!captured||conflict}>{text.save}</Button>}<Button onClick={()=>void load(true)} loading={busy}>{text.recheck}</Button>{canEdit&&kind==='numeric'&&<Button disabled={!captured||dirty||conflict} onClick={()=>void start()} loading={busy}>{text.start}</Button>}</Space>
  </Form>
  {kind==='numeric'&&<Typography.Paragraph>{text.manualPreserved}</Typography.Paragraph>}
  {work&&<section><Typography.Title level={5}>{text.progress}: {work.status}</Typography.Title><Progress percent={work.total?Math.round(100*work.processed/work.total):100}/><Typography.Paragraph>{work.processed}/{work.total} · {text.failures}: {work.failed} · {text.manual}: {work.skipped}</Typography.Paragraph><Button onClick={()=>void refreshWork()}>{text.refresh}</Button><Table rowKey='spec_value_id' size='small' dataSource={items} pagination={false} columns={[{title:text.source,dataIndex:'spec_value_id'},{title:text.progress,dataIndex:'state'},{title:text.reason,dataIndex:'reason'}]}/><Pagination current={page} total={total} pageSize={20} onChange={setPage}/></section>}
 </Modal>;
}
