import { test, expect } from '@playwright/test'

const org='00000000-0000-4000-8000-000000000001'
const finding='00000000-0000-4000-8000-000000000002'
const base=`/api/v1/orgs/${org}`

test('Needs you shows full count, scoped handoff and one bounded retry',async({page})=>{
 await page.route(`**${base}/**`,route=>route.fulfill({json:{items:[],complete:true}}))
 await page.route('**/api/v1/session',route=>route.fulfill({json:{user:{id:'owner',name:'Owner'},organisations:[{id:org,name:'Local'}],memberships:[{org_id:org,role:'owner',all_repositories:true}],csrf_token:'csrf'}}))
 await page.route('**/api/v1/meta',route=>route.fulfill({json:{development:true,fixture_auth:true}}))
 await page.route(`**${base}/autopilot/needs?**`,route=>{
  const next=new URL(route.request().url()).searchParams.get('cursor')
  return route.fulfill({json:{items:[{id:next?'attempt:'+finding:'budget:repo:organisation',kind:next?'exhausted':'raise_limit',finding_id:next?finding:'',finding_version:1,repository_id:'repo',repository:'shop',title:next?'Checkout still failing':'Adjust maintenance spending limit',reason:next?'Six attempts failed':'Spending paused',url:next?`findings?finding=${finding}`:`usage?scope_kind=organisation&scope_id=${org}`,label:next?'Review evidence':'Adjust spending limit',owner:next?'owner':'owner'}],total:51,complete:!!next,...(!next&&{next_cursor:'next'})}})
 })
 let retry:any
 await page.route(`**${base}/autopilot/needs/${finding}/retry`,route=>{retry=route.request().postDataJSON();return route.fulfill({status:204})})
 await page.goto(`/org/${org}/repositories`)
 await page.getByRole('button',{name:'51 items need you Review'}).click()
 const dialog=page.getByRole('dialog',{name:'Needs you'})
 await expect(dialog.getByText('shop · owner')).toBeVisible()
 await expect(dialog.getByRole('link',{name:'Adjust spending limit',exact:true})).toHaveAttribute('href',`/org/${org}/usage?scope_kind=organisation&scope_id=${org}`)
 await dialog.getByRole('button',{name:'Load more decisions'}).click()
 await expect(dialog.getByText('Six attempts failed')).toBeVisible()
 await dialog.getByText('Try one more fix',{exact:true}).click()
 await dialog.getByLabel('What changed?').fill('Runner fixed; reviewed prior evidence')
 await dialog.getByRole('button',{name:'Retry one fix',exact:true}).click()
 await expect.poll(()=>retry).toEqual({finding_version:1,reason:'Runner fixed; reviewed prior evidence'})
})
