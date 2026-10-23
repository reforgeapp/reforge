import { test, expect } from '@playwright/test'

const org='00000000-0000-4000-8000-000000000001'
const promotion='00000000-0000-4000-8000-000000000002'
const base=`/api/v1/orgs/${org}`

test('overview opens complete work and GitOps links select promotion detail',async({page})=>{
 await page.route(`**${base}/**`,route=>route.fulfill({json:{items:[],complete:true,total:0}}))
 await page.route('**/api/v1/session',route=>route.fulfill({json:{user:{id:'owner',name:'Owner'},organisations:[{id:org,name:'Local'}],memberships:[{org_id:org,role:'owner',all_repositories:true}],csrf_token:'csrf'}}))
 await page.route('**/api/v1/meta',route=>route.fulfill({json:{development:true,fixture_auth:true}}))
 await page.route(`**${base}/overview`,route=>route.fulfill({json:{counts:{needs_decision:0,running:3,blocked:1,ready_for_review:0,verified_deployments:0,accessible_repositories:1,stale_repositories:0,queued_jobs:1},attention:[],portfolio:[],capacity:{queued_jobs:1,running_jobs:1,active_pools:1,active_runners:1,reserved_micro_usd:0}}}))
 await page.route(`**${base}/work?**`,route=>{
  const query=new URL(route.request().url()).searchParams
  const next=query.get('cursor')
  if(query.get('state')==='blocked') return route.fulfill({json:{items:[{id:'run:waiting',repository_id:'repo',repository:'shop',kind:'repair',state:'waiting',bucket:'blocked',title:'Redis master label repair',reason:'Waiting for a safe validation slot',url:'runs?run=waiting',created_at:new Date().toISOString()}],complete:true}})
  const common={repository_id:'repo',repository:'shop',state:'verifying',bucket:'active',reason:'Waiting for application health',created_at:new Date().toISOString()}
  return route.fulfill({json:{items:next?[{...common,id:'gitops:'+promotion,kind:'gitops',title:'Deliver staging',url:`deployments?promotion=${promotion}`}]:[{...common,id:'deployment:1',kind:'native',title:'Deploy production',url:'deployments?deployment=native'},{...common,id:'run:1',kind:'repair',title:'Fix checkout',url:'runs?run=repair'}],complete:!!next,...(!next&&{next_cursor:'next'})}})
 })
 let requested=false
 await page.route(`**${base}/gitops-promotions/${promotion}`,route=>{requested=true;return route.fulfill({status:404,json:{code:'not_found',message:'Fixture detail unavailable',retryable:false,request_id:'fixture'}})})
 await page.goto(`/org/${org}/overview`)
 const counts=page.getByRole('region',{name:'Portfolio counts'})
 await expect(counts.getByRole('link',{name:/Needs you/})).toHaveAttribute('href',`/org/${org}/runs?state=needs`)
 await expect(counts.getByRole('link',{name:/Fixes being checked/})).toHaveAttribute('href',`/org/${org}/runs?state=review`)
 await counts.getByRole('link',{name:/In progress/}).click()
 await expect(page.getByRole('link',{name:'Fix checkout'})).toBeVisible()
 await expect(page.getByRole('link',{name:'Deploy production'})).toBeVisible()
 await page.getByRole('button',{name:'Load more work'}).click()
 await page.getByRole('link',{name:'Deliver staging'}).click()
 await expect(page.getByRole('tab',{name:'GitOps promotions'})).toHaveAttribute('aria-selected','true')
 await expect.poll(()=>requested).toBe(true)
 await page.goto(`/org/${org}/overview`)
 await page.getByRole('button', { name: 'More details', exact: true }).click()
 await page.getByRole('link',{name:/Waiting work/}).click()
 const waiting=page.getByRole('row',{name:/Redis master label repair/})
 await expect(waiting.getByRole('cell').nth(2)).toHaveText('Fix')
 await expect(waiting.getByRole('cell').nth(3)).toHaveText('Waiting')
 await expect(waiting.getByText('No verified fix yet')).toBeVisible()
 await expect(waiting.getByText('Waiting for a safe validation slot')).toBeVisible()
 await expect(waiting.getByRole('link',{name:'Redis master label repair'})).toHaveAttribute('href',`/org/${org}/runs?run=waiting`)
})
