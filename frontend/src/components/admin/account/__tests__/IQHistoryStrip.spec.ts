import { mount } from '@vue/test-utils'
import { expect,it } from 'vitest'
import IQHistoryStrip from '../IQHistoryStrip.vue'
import type { IQSummary } from '@/api/admin/iqInterval'
it('always shows 20 cells; preserves latest chronological results and distinct request failures',()=>{
 const summary={account_id:1,enabled:true,interval_seconds:60,model:'fixture',reasoning:'medium',results:[{id:3,verdict:'error'},{id:2,verdict:'other',answer:'29'},{id:1,verdict:'correct',answer:'21'}].map(x=>({...x,model:'fixture',reasoning:'medium',finished_at:'2026-10-14T00:00:00Z'}))} as IQSummary
 const w=mount(IQHistoryStrip,{props:{summary}})
 expect(w.findAll('[data-verdict]')).toHaveLength(20)
 expect(w.findAll('[data-verdict="untested"]')).toHaveLength(17)
 expect(w.findAll('[data-verdict]').slice(-3).map(c=>c.attributes('data-verdict'))).toEqual(['correct','other','error'])
 expect(w.text()).toContain('1/3')
})
it('does not report a fetch error as an empty test history',()=>{
 const w=mount(IQHistoryStrip,{props:{error:true}})
 expect(w.text()).toContain('记录读取失败');expect(w.text()).not.toContain('0/0')
})
