import {api,postJSON} from './api';
export type NumericProfile={quantity:string;semantic_role:string;allowed_shapes:string[];allowed_operators:string[];canonical_unit:string;declared_input_unit:string;numeric_format:string;schema_version:number;parser_version:string;unit_registry_version:string};
export type SearchProfile={version:number;kind:'text'|'choice'|'multi_choice'|'numeric';text_rule?:string;delimiter?:string;operators:string[];numeric?:NumericProfile};
export type SpecSearchProfile={spec_id:string;spec_semantic_version:number;revision:number;filterable:boolean;profile:SearchProfile};
export type SpecCapabilities={version:number;text_rule:string;kinds:SearchProfile['kind'][];numeric_profiles:NumericProfile[]};
export type NormalizationExpectation={product_revision:number;source_revision:number;spec_semantic_version:number;profile_revision:number;selection_revision:number};
export type SourceSelection={product_id:string;spec_id:string;spec_value_id:string;revision:number};
export type NormalizationContext={spec_value_id:string;product_id:string;spec_id:string;raw:string;source_locale:string;profile:NumericProfile;expected:NormalizationExpectation;source_selection:SourceSelection;selection:{selected_normalized_value_id:string;preference:string;revision:number}};
export type NormalizationOutcome={normalized_value_id:string;reason_code?:string;product_revision:number;selection_revision:number;publication_pending:boolean};
export type NormalizationWork={id:string;spec_id:string;status:string;profile_revision:number;semantic_version:number;processed:number;failed:number;skipped:number;total:number};
export type WorkItem={spec_value_id:string;state:string;reason:string};
const specPath=(id:string)=>`/admin/api/specs/${encodeURIComponent(id)}`;
const valuePath=(id:string)=>`/admin/api/spec-values/${encodeURIComponent(id)}`;
export const specCommands={
 sourceState:async(productID:string)=>{const path=`/admin/api/products/${encodeURIComponent(productID)}`;const [product,values]=await Promise.all([api<{revision:number}>(path),api<import("./types").SpecValueDetail[]>(`${path}/spec-values`)]);return {product,values};},
 capabilities:()=>api<SpecCapabilities>('/admin/api/spec-search-capabilities'),
 searchProfile:(id:string)=>api<SpecSearchProfile>(`${specPath(id)}/search-profile`),
 configureSearchProfile:(id:string,expectedRevision:number,desired:Pick<SpecSearchProfile,'filterable'|'profile'>)=>postJSON<SpecSearchProfile>(`${specPath(id)}/search-profile`,{expected_revision:expectedRevision,...desired}),
 startNormalizationWork:(id:string,expectedRevision:number)=>postJSON<NormalizationWork>(`${specPath(id)}/normalization-work`,{expected_revision:expectedRevision}),
 normalizationWork:(id:string)=>api<NormalizationWork>(`/admin/api/spec-normalization-work/${encodeURIComponent(id)}`),
 workItems:(id:string,page:number)=>api<{items:WorkItem[];total:number}>(`/admin/api/spec-normalization-work/${encodeURIComponent(id)}/items?page=${page}&page_size=20`),
 context:(id:string)=>api<NormalizationContext>(`${valuePath(id)}/normalization-context`),
 normalize:(id:string,expected:NormalizationExpectation)=>postJSON<NormalizationOutcome>(`${valuePath(id)}/normalize`,{expected}),
 adoptManual:(id:string,expected:NormalizationExpectation,value:unknown)=>postJSON<NormalizationOutcome>(`${valuePath(id)}/normalized`,{expected,value_json:JSON.stringify(value)}),
 selectAutomatic:(id:string,expected:NormalizationExpectation,candidateID:string)=>postJSON<NormalizationOutcome>(`${valuePath(id)}/normalization/automatic`,{expected,candidate_id:candidateID}),
 selectSource:(productID:string,input:{spec_id:string;spec_value_id:string;expected_product_revision:number;expected_source_revision:number;expected_source_selection_revision:number})=>postJSON<SourceSelection>(`/admin/api/products/${encodeURIComponent(productID)}/spec-source`,input),
};
