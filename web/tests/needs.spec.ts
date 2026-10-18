import { test, expect } from '@playwright/test'

const org='00000000-0000-4000-8000-000000000001'
const finding='00000000-0000-4000-8000-000000000002'
const base=`/api/v1/orgs/${org}`

test('Needs you paginates owner actions and preserves scoped links',async({page})=>{
 await page.route(`**${base}/**`,route=>route.fulfill({json:{items:[],complete:true}}))
 await page.route('**/api/v1/session',route=>route.fulfill({json:{user:{id:'owner',name:'Owner'},organisations:[{id:org,name:'Local'}],memberships:[{org_id:org,role:'owner',all_repositories:true}],csrf_token:'csrf'}}))
 await page.route('**/api/v1/meta',route=>route.fulfill({json:{development:true,fixture_auth:true}}))
 await page.route(`**${base}/autopilot/needs?**`,route=>{
  const next=new URL(route.request().url()).searchParams.get('cursor')
  return route.fulfill({json:{items:[{id:next?'grant:'+finding:'budget:repo:organisation',kind:next?'grant':'raise_limit',finding_id:next?finding:'',finding_version:1,repository_id:'repo',repository:'shop',title:next?'Grant dependency access':'Adjust maintenance spending limit',reason:next?'Private package access required':'Spending paused',url:next?`repositories?repository=repo`:`usage?scope_kind=organisation&scope_id=${org}`,label:next?'Grant access':'Adjust spending limit',owner:'owner'}],total:51,complete:!!next,...(!next&&{next_cursor:'next'})}})
 })
 await page.goto(`/org/${org}/repositories`)
 await page.getByRole('button',{name:'51 items need you Review'}).click()
 const dialog=page.getByRole('dialog',{name:'Needs you'})
 await expect(dialog.getByText('shop · owner')).toBeVisible()
 await expect(dialog.getByRole('link',{name:'Adjust spending limit',exact:true})).toHaveAttribute('href',`/org/${org}/usage?scope_kind=organisation&scope_id=${org}`)
 await dialog.getByRole('button',{name:'Load more decisions'}).click()
 await expect(dialog.getByText('Private package access required')).toBeVisible()
 await expect(dialog.getByRole('link',{name:'Grant access'})).toHaveAttribute('href',`/org/${org}/repositories?repository=repo`)
})
