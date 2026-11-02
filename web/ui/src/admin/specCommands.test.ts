import {afterEach,beforeEach,expect,it,vi} from 'vitest';
vi.mock('./locales',()=>({normalizeAdminLocale:(locale:string)=>locale}));
beforeEach(()=>{vi.resetModules();vi.stubGlobal('document',{documentElement:{lang:'en-US'},querySelector:()=>({content:'csrf'})});});
afterEach(()=>vi.unstubAllGlobals());
it('configureSearchProfile preserves the captured revision and propagates 409 without retry',async()=>{
 const fetch=vi.fn().mockResolvedValue(new Response('{"error":"revision conflict"}',{status:409}));vi.stubGlobal('fetch',fetch);
 const {specCommands}=await import('./specCommands');
 const profile={version:1,kind:'text' as const,text_rule:'unicode-fold-space-1',operators:['equals','contains']};
 await expect(specCommands.configureSearchProfile('spec/opaque',7,{filterable:true,profile})).rejects.toMatchObject({status:409});
 expect(fetch).toHaveBeenCalledTimes(1);
 const [url,init]=fetch.mock.calls[0];expect(url).toBe('/admin/api/specs/spec%2Fopaque/search-profile');expect(JSON.parse(init.body)).toEqual({expected_revision:7,filterable:true,profile});
});
