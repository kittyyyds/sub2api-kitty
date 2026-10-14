import { ref, watch, onBeforeUnmount, type Ref } from 'vue'
import { getIQSummaries, type IQSummary } from '@/api/admin/iqInterval'
export function useIQSummaries(ids:Ref<number[]>) {
 const summaries=ref<Record<number,IQSummary>>({}),error=ref(false)
 let generation=0,busy=false,disposed=false
 async function refresh(){if(busy||disposed||document.hidden)return;busy=true;const g=generation;const current=[...ids.value];try{const rows:IQSummary[]=[];for(let i=0;i<current.length;i+=200)rows.push(...await getIQSummaries(current.slice(i,i+200)));if(g===generation&&!disposed){summaries.value=Object.fromEntries(rows.map(r=>[r.account_id,r]));error.value=false}}catch{if(g===generation)error.value=true}finally{busy=false;if(g!==generation&&!disposed)void refresh()}}
 watch(()=>ids.value.join(','),()=>{generation++;summaries.value={};void refresh()},{immediate:true})
 const timer=setInterval(()=>void refresh(),2000)
 onBeforeUnmount(()=>{disposed=true;generation++;clearInterval(timer)})
 return {summaries,error,refresh}
}
