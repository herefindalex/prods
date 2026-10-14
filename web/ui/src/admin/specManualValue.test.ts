import {expect,it} from 'vitest';
import {manualValue} from './specManualValue';
import type {NumericProfile} from './specCommands';
const profile:NumericProfile={quantity:'voltage',semantic_role:'nominal',allowed_shapes:['tolerance','interval_set'],allowed_operators:['='],canonical_unit:'V',declared_input_unit:'V',numeric_format:'decimal-dot',schema_version:1,parser_version:'catalog-parser-2',unit_registry_version:'catalog-units-2'};
it('manual tolerance uses exact decimal arithmetic',()=>{expect(manualValue(profile,{shape:'tolerance',nominal:'0.1',amount:'10',tolerance_type:'relative'})).toMatchObject({tolerance:{interval:{lower:'0.09',upper:'0.11'}}});});
it('manual interval sets retain the gap',()=>{expect(manualValue(profile,{shape:'interval_set',intervals:[{lower:'3',upper:'4',lower_inclusive:true,upper_inclusive:false},{lower:'5',upper:'6',lower_inclusive:true,upper_inclusive:true}]})).toMatchObject({intervals:[{lower:'3',upper:'4',upper_inclusive:false},{lower:'5',upper:'6'}]});});
