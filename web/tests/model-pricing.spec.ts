import { test, expect } from '@playwright/test'

const org='00000000-0000-4000-8000-000000000001'
const connection='00000000-0000-4000-8000-000000000002'
const base=`/api/v1/orgs/${org}`

test('owner applies sourced pricing without token arithmetic and retains custom override',async({page})=>{
 await page.route(`**${base}/**`,route=>route.fulfill({json:{items:[],complete:true,total:0}}))
 await page.route('**/api/v1/session',route=>route.fulfill({json:{user:{id:'owner',name:'Owner'},organisations:[{id:org,name:'Local'}],memberships:[{org_id:org,role:'owner',all_repositories:true}],csrf_token:'csrf'}}))
 await page.route('**/api/v1/meta',route=>route.fulfill({json:{development:true,fixture_auth:true}}))
 const model={id:connection,org_id:org,kind:'model',provider:'openai',name:'Approved API',endpoint:'https://api.openai.com/v1',settings:{model:'gpt-6-luna',billing_route:'direct_api',auth_kind:'api_key'},state:'healthy',version:1,credential_version:1,capabilities:{}}
 await page.route(`**${base}/connections?**`,route=>route.fulfill({json:{items:[model],complete:true}}))
 await page.route(`**${base}/connections/${connection}`,route=>route.fulfill({json:model}))
 const preset={id:'openai-standard-ceiling-2026-10-13-luna',provider:'openai',model:'gpt-6-luna',source:'https://developers.openai.com/api/docs/pricing',checked_at:'2026-10-13',input_micro_usd_per_million:250000,output_micro_usd_per_million:750000}
 let saved=false
 const priced={connection_id:connection,model:'gpt-6-luna',name:'default',mode:'priced',pricing_version:preset.id,pricing_preset_id:preset.id,pricing_source:preset.source,pricing_checked_at:preset.checked_at,cost_basis:'published_ceiling',input_micro_usd_per_million:250000,output_micro_usd_per_million:750000,request_micro_usd:0,max_input_tokens:64000,max_output_tokens:4096,max_milliseconds:600000,max_requests:1,qualified:false,qualification_ref:'',paused:false,version:1}
 await page.route(`**${base}/model-pricing/${connection}`,route=>route.fulfill({json:{presets:[preset],manual:saved,automatic:saved,reason:saved?'Approved spending ceilings apply':'Known pricing required'}}))
 await page.route(`**${base}/budget-routes/${connection}?**`,route=>saved?route.fulfill({json:priced}):route.fulfill({status:409,json:{code:'budget_unconfigured',message:'Pricing required',retryable:false,request_id:'fixture'}}))
 await page.route(`**${base}/budget-routes/${connection}/price-preset`,route=>{expect(route.request().postDataJSON()).toEqual({preset_id:preset.id});expect(route.request().headers()['if-match']).toBe('"0"');saved=true;return route.fulfill({json:priced})})
 await page.goto(`/org/${org}/connections?connection=${connection}`)
 const pricing=page.getByRole('form',{name:'Model pricing'})
 await expect(pricing.getByRole('button',{name:'Use published price ceiling'})).toBeVisible()
 await expect(pricing.getByRole('link',{name:'Source · 2026-10-13'})).toHaveAttribute('href',preset.source)
 await expect(pricing.getByLabel('Input $ per 1M tokens')).toBeHidden()
 await pricing.getByRole('button',{name:'Use published price ceiling'}).click()
 await expect(pricing.getByText('Published price ceiling saved.')).toBeVisible()
 await expect(pricing.getByText(/Automatic maintenance supported/)).toBeVisible()
 await pricing.getByText('Custom prices',{exact:true}).click()
 await expect(pricing.getByLabel('Input $ per 1M tokens')).toBeVisible()
})
