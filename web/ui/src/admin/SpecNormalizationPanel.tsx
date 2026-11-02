import {useEffect,useState} from 'react';
import {Alert,Button,Card,Checkbox,Form,Input,Modal,Select,Space,Tag,Typography} from 'antd';
import {APIError} from './api';
import {specMessages,type AdminLocale} from './locales';
import {specCommands,type NormalizationContext,type NormalizationOutcome,type SpecSearchProfile} from './specCommands';
import {manualValue,factLabel,type ManualFields} from './specManualValue';
import type {SpecDefinition,SpecValueDetail} from './types';
export function SpecNormalizationPanel({spec,details,productID,productRevision,locale,canEdit,onClose,onChanged}:{spec:SpecDefinition;details:SpecValueDetail[];productID:string;productRevision:number;locale:AdminLocale;canEdit:boolean;onClose:()=>void;onChanged:()=>Promise<void>}) {
 const text=specMessages(locale);const [form]=Form.useForm<ManualFields>();
 const [sources,setSources]=useState(details);
 const [selected,setSelected]=useState(details.find(d=>d.source_selection.spec_value_id===d.value.id)?.value.id??details[0]?.value.id);
 const [captured,setCaptured]=useState<NormalizationContext>();const [profile,setProfile]=useState<SpecSearchProfile>();const [busy,setBusy]=useState(false);const [conflict,setConflict]=useState(false);const [error,setError]=useState('');const [message,setMessage]=useState('');const [candidate,setCandidate]=useState<string>();
 const [sourceExpected,setSourceExpected]=useState({product:productRevision,revision:details[0]?.source_selection.revision??0});
 const shape=Form.useWatch('shape',form);const detail=sources.find(d=>d.value.id===selected);
 const load=async()=>{if(!selected)return;setBusy(true);try{const [p,fresh]=await Promise.all([specCommands.searchProfile(spec.id),specCommands.sourceState(productID)]);setSources(fresh.values.filter(d=>d.value.spec_id===spec.id));setProfile(p);const c=p.profile.kind==='numeric'?await specCommands.context(selected):undefined;setCaptured(c);setConflict(false);setError('');setSourceExpected({product:c?.expected.product_revision??fresh.product.revision,revision:c?.source_selection.revision??fresh.values.find(d=>d.value.spec_id===spec.id)?.source_selection.revision??0});if(c&&!form.getFieldValue('shape'))form.setFieldValue('shape',c.profile.allowed_shapes[0]);}catch(e){setError(String(e));}finally{setBusy(false);}};
 useEffect(()=>{void load();},[selected]);
 const fail=(e:unknown)=>{if(e instanceof APIError&&e.status===409){setConflict(true);setError(text.conflict);}else{setError(String(e));}};
 const commit=async(operation:()=>Promise<NormalizationOutcome|unknown>)=>{setBusy(true);setError('');let result:unknown;try{result=await operation();}catch(e){fail(e);setBusy(false);return;}
  const outcome=result as Partial<NormalizationOutcome>;setMessage(outcome.reason_code?`${text.reason}: ${outcome.reason_code}`:text.saved);
  try{await onChanged();await load();}catch{setError(text.refreshFailed);}finally{setBusy(false);}
 };
 const adopt=(fields:ManualFields)=>{if(!captured)return;let value:Record<string,unknown>;try{value=manualValue(captured.profile,fields);}catch(e){setError(String(e));return;}void commit(()=>specCommands.adoptManual(captured.spec_value_id,captured.expected,value));};
 const chooseSource=()=>{if(!detail)return;void commit(()=>specCommands.selectSource(productID,{spec_id:spec.id,spec_value_id:detail.value.id,expected_product_revision:sourceExpected.product,expected_source_revision:detail.value.source_revision,expected_source_selection_revision:sourceExpected.revision}));};
 const automatic=(detail?.normalized??[]).filter(n=>n.source==='automatic'&&n.status==='current');
 const active=(detail?.normalized??[]).find(n=>n.id===captured?.selection.selected_normalized_value_id);
 return <Modal open title={`${text.normalization}: ${spec.name}`} onCancel={onClose} footer={<Button onClick={onClose}>{text.close}</Button>} width={820}>
  {error&&<Alert type='error' title={error} showIcon/>}{message&&<Alert type='success' title={message}/>}
  <Space direction='vertical' style={{width:'100%'}}>
   <Typography.Text>{text.source}</Typography.Text><Select style={{width:'100%'}} value={selected} onChange={setSelected} options={sources.map(d=>({value:d.value.id,label:`${d.value.raw_value} · ${d.value.source_locale??''} · r${d.value.source_revision}${d.source_selection.spec_value_id===d.value.id?` · ${text.selected}`:''}`}))}/>
   <Typography.Paragraph copyable>{detail?.value.raw_value}</Typography.Paragraph>
   <Space>{canEdit&&<Button disabled={conflict||!detail||detail.source_selection.spec_value_id===detail.value.id} loading={busy} onClick={chooseSource}>{text.source}</Button>}<Button loading={busy} onClick={()=>void load()}>{text.recheck}</Button></Space>
   <Typography.Paragraph>{text.selected}: {active?factLabel(active.value_json):'—'} <Tag>{captured?.selection.preference}</Tag><Tag>{active?.status}</Tag></Typography.Paragraph>
   <Typography.Text type='secondary'>{text.manualPreserved}</Typography.Text>
   {(detail?.normalized??[]).map(n=><div key={n.id}><Tag color={n.status==='stale'?'orange':n.status==='failed'?'red':'blue'}>{n.source} · {n.status}{n.id===captured?.selection.selected_normalized_value_id?` · ${text.selected}`:''}</Tag><Typography.Text>{factLabel(n.value_json)} {n.failure_reason}</Typography.Text></div>)}
   {captured&&<>
    <Typography.Paragraph>{text.profile}: {captured.profile.quantity} / {captured.profile.semantic_role} / {captured.profile.canonical_unit} · {captured.profile.parser_version} · {captured.profile.unit_registry_version}</Typography.Paragraph>
    {canEdit&&<Button loading={busy} disabled={conflict} onClick={()=>void commit(()=>specCommands.normalize(captured.spec_value_id,captured.expected))}>{text.normalize}</Button>}
    {automatic.length>0&&<Space wrap><Select style={{minWidth:240}} value={candidate} onChange={setCandidate} placeholder={text.automatic} options={automatic.map(n=>({value:n.id,label:factLabel(n.value_json)}))}/>{canEdit&&<Button disabled={!candidate||conflict} loading={busy} onClick={()=>void commit(()=>specCommands.selectAutomatic(captured.spec_value_id,captured.expected,candidate!))}>{text.automatic}</Button>}</Space>}
    <Card size='small' title={text.manual}>
    <Form form={form} layout='vertical' initialValues={{inclusive:true,direction:'upper',tolerance_type:'absolute',intervals:[{lower_inclusive:true,upper_inclusive:true}]}} onFinish={adopt} disabled={!canEdit}>
     <Form.Item name='shape' label={text.shape} rules={[{required:true}]}><Select options={captured.profile.allowed_shapes.map(value=>({value,label:value.replaceAll('_',' ')}))}/></Form.Item>
     <Typography.Text>{text.unit}: {captured.profile.canonical_unit}</Typography.Text>
     {(shape==='scalar'||shape==='bound_claim')&&<Form.Item name='value' label={text.value} rules={[{required:true}]}><Input inputMode='decimal'/></Form.Item>}
     {shape==='discrete_set'&&<Form.Item name='values' label={`${text.value} (,)`} rules={[{required:true}]}><Input/></Form.Item>}
     {shape==='interval_set'&&<Form.List name='intervals'>{(fields,{add,remove})=><>{fields.map(field=><Space key={field.key} align='baseline' wrap><Form.Item name={[field.name,'lower']} label={text.lower} rules={[{required:true}]}><Input inputMode='decimal'/></Form.Item><Form.Item name={[field.name,'upper']} label={text.upper} rules={[{required:true}]}><Input inputMode='decimal'/></Form.Item><Form.Item name={[field.name,'lower_inclusive']} valuePropName='checked'><Checkbox>{text.lower} · {text.inclusive}</Checkbox></Form.Item><Form.Item name={[field.name,'upper_inclusive']} valuePropName='checked'><Checkbox>{text.upper} · {text.inclusive}</Checkbox></Form.Item>{canEdit&&<Button onClick={()=>remove(field.name)}>{text.remove}</Button>}</Space>)}{canEdit&&<Button onClick={()=>add({lower_inclusive:true,upper_inclusive:true})}>{text.add}</Button>}</>}</Form.List>}
     {shape==='bound_claim'&&<Space><Form.Item name='direction' label={text.shape}><Select style={{minWidth:120}} disabled={captured.profile.semantic_role==='error_magnitude'} options={[{value:'upper',label:text.upper},{value:'lower',label:text.lower}]}/></Form.Item><Form.Item name='inclusive' valuePropName='checked'><Checkbox>{text.inclusive}</Checkbox></Form.Item></Space>}
     {shape==='tolerance'&&<><Form.Item name='nominal' label={text.nominal} rules={[{required:true}]}><Input inputMode='decimal'/></Form.Item><Form.Item name='amount' label={text.amount} rules={[{required:true}]}><Input inputMode='decimal'/></Form.Item><Form.Item name='tolerance_type' label={text.unit}><Select options={[{value:'absolute',label:captured.profile.canonical_unit},{value:'relative',label:'%'}]}/></Form.Item></>}
     <Form.Item name='condition' label={text.condition}><Input/></Form.Item>
     {canEdit&&<Button type='primary' htmlType='submit' disabled={conflict} loading={busy}>{text.manual}</Button>}
    </Form></Card>
   </>}
   {profile?.profile.kind!=='numeric'&&<Tag>{profile?.profile.kind}</Tag>}
  </Space>
 </Modal>;
}
