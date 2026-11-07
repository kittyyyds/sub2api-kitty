<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { getCommunityGuide, type CommunityGuide } from '@/api/community'
import AppLayout from '@/components/layout/AppLayout.vue'
import { useAuthStore } from '@/stores/auth'
import { useAppStore } from '@/stores/app'
import { useTheme } from '@/composables/useTheme'
import { communityGroupContact } from '@/config/customerContact'

const auth = useAuthStore()
const app = useAppStore()
const { isDark, toggleTheme } = useTheme()
const guide = ref<CommunityGuide | null>(null)
const loading = ref(true)
const failed = ref(false)
const bindingMethod = ref<'email' | 'code'>('email')
const copied = ref('')
const copyError = ref(false)
const bindingTarget = computed(() => auth.isAuthenticated ? '/profile#qq-binding' : '/login?redirect=%2Fprofile%23qq-binding')
const emailAutomatic = computed(() => guide.value?.enabled && guide.value.auto_email_binding)
const joinRule = computed(() => guide.value?.join_review)
const emailSteps = computed(() => emailAutomatic.value ? [
  { title: '申请入群，填写注册邮箱', text: '在 QQ 群的入群问题中，填写你自己在 Kitty 注册的邮箱。消费资格按该网站账号的后台累计实际消费审核。' },
  { title: '入群后，查收确认邮件', text: '符合条件并实际入群后，系统会尝试向注册邮箱发送绑定确认邮件。以欢迎提示和实际收到的邮件为准；也可检查垃圾邮件。' },
  { title: '核对 QQ 身份，确认绑定', text: '打开邮件中的 10 分钟有效确认链接，核对 QQ 身份，点击确认后才完成绑定。无需再生成网站绑定码。' },
  { title: '开始使用群福利', text: '确认成功后，在群里 @机器人 发送「我的余额」或「签到」。网站个人资料中也可以查看绑定状态。' },
] : [
  { title: '先加入 QQ 群', text: '按群内要求申请入群。管理员邀请或人工放行入群的用户，也需要完成本人账号绑定。' },
  { title: '在群里发起邮箱绑定', text: '在群内 @机器人 发送「绑定 你的注册邮箱」。如果不方便在群里发送邮箱，可改用下方的网站绑定码方式。' },
  { title: '查收邮件，核对后确认', text: '打开绑定邮件中的 10 分钟有效链接，核对 QQ 身份并点击确认。只有确认成功才算完成绑定。' },
  { title: '查看绑定状态', text: '回到网站个人资料查看 QQ 绑定状态，或在群内 @机器人 发送「我的余额」。' },
])
const codeSteps = [
  { title: '登录本人网站账号', text: '打开「个人资料 → QQ 账号绑定」，点击「生成 / 更新绑定码」。绑定码 10 分钟有效，更新后旧码失效。' },
  { title: '在群里发送绑定指令', text: '在允许的 QQ 群内 @机器人 发送「绑定 你的绑定码」。请使用刚刚从本人网站账号生成的码。' },
  { title: '回网站刷新并核对身份', text: '点击「刷新绑定状态」，确认显示的待绑定 QQ 身份属于你本人。此时还没有完成正式绑定。' },
  { title: '点击「确认绑定此 QQ 身份」', text: '确认成功后才完成账号关联，可以使用群内账户查询、签到和其他已开启活动。' },
]
const faqs = computed(() => [
  { q: '通过进群审核，就已经绑定成功了吗？', a: '进群审核只核验填写的邮箱是否符合消费资格。邮箱确认方式需要收件人核对 QQ 身份并点击确认；网站绑定码方式需要回网站确认。收到欢迎语或发出绑定指令，都不等于已经绑定成功。' },
  { q: '管理员邀请我入群，怎么绑定？', a: '管理员邀请、人工放行或没有自动审核记录的入群用户，使用网站绑定码流程；也可以在群内 @机器人 发送「绑定 你的注册邮箱」，按邮件提示确认。已有相符绑定可直接沿用。' },
  { q: '为什么机器人有时不回复？', a: '请先 @机器人，再发送完整指令，例如「签到」或「帮助」。普通群消息的接收还受 QQ 权限和群设置影响；遇到冷却或额度限制，以机器人实际提示为准。' },
  { q: '没有收到邮件，或者链接过期了怎么办？', a: '检查注册邮箱和垃圾邮件；超过 10 分钟的确认链接需重新发起。也可以改用网站绑定码流程。如果提示绑定冲突，请联系管理员核实，不要确认陌生的 QQ 身份。' },
  { q: '多个 QQ、多个群可以重复领签到奖励吗？', a: '签到和成长奖励按网站账号核算。同一个网站账号换群、重新绑定或重复发送指令，不会额外增加领取次数。' },
  { q: '本页的奖励以后会变吗？', a: '福利内容读取站点当前活动配置，重新打开或点击「刷新活动」即可获取最新规则。金额按站内账户额度展示；奖励到账、个人资格和活动参与结果以后台记录及机器人回复为准。' },
])

async function load() {
  loading.value = true
  failed.value = false
  try { guide.value = await getCommunityGuide() }
  catch { guide.value = null; failed.value = true }
  finally { loading.value = false }
}
async function copyCommand(command: string) {
  copyError.value = false
  try { await navigator.clipboard.writeText(command); copied.value = command }
  catch { copyError.value = true }
}
onMounted(load)
</script>

<template>
  <component :is="auth.isAuthenticated ? AppLayout : 'div'">
    <div class="community-page">
      <header v-if="!auth.isAuthenticated" class="community-public-nav">
        <RouterLink to="/home" class="community-brand"><img :src="app.brandLogoUrl || '/logo.svg'" alt="Kitty" /><span>Kitty 用户社区</span></RouterLink>
        <div class="community-nav-actions"><button type="button" class="community-quiet-button" :aria-label="isDark ? '切换浅色主题' : '切换深色主题'" @click="toggleTheme">{{ isDark ? '浅色' : '深色' }}</button><RouterLink to="/login?redirect=%2Fcommunity" class="community-button community-button-small">登录账号</RouterLink></div>
      </header>

      <div class="community-content">
        <section class="community-hero" aria-labelledby="community-title">
          <div class="community-hero-copy">
            <span class="community-eyebrow">KITTY 用户社区</span>
            <h1 id="community-title">一起交流，<br />把福利领回账户。</h1>
            <p class="community-lead">加入 Kitty QQ 用户群，交流模型使用经验、了解站点动态。绑定网站账号后，在群内查余额、看用量，参与当前开放的福利活动。</p>
            <div class="community-hero-actions"><a :href="communityGroupContact.href" target="_blank" rel="noopener noreferrer" class="community-button">打开 QQ 加群 <span aria-hidden="true">↗</span></a><a href="#binding" class="community-secondary-button">查看绑定步骤 <span aria-hidden="true">↓</span></a></div>
            <p class="community-caption">加群资格与账号绑定分两步完成，邮箱确认后再开启账户服务。</p>
          </div>
          <aside class="community-join-card" aria-label="扫码加入 QQ 群">
            <div class="community-join-heading"><span class="community-dot" aria-hidden="true"></span> QQ 交流群 · {{ communityGroupContact.valueKind === 'number' ? communityGroupContact.value : 'Kitty' }}</div>
            <div class="community-qr"><img :src="communityGroupContact.qrSrc" alt="Kitty QQ 用户群加群二维码" /></div>
            <p>使用手机 QQ 扫码申请入群</p>
            <p v-if="joinRule" class="community-join-rule">当前门槛：后台累计实际消费 ≥ <strong>{{ joinRule.min_spent }}</strong><br />填写本人的网站注册邮箱</p>
            <p v-else class="community-join-rule">入群请按 QQ 群显示的问题填写，<br />具体资格以群内审核要求为准。</p>
          </aside>
        </section>

        <nav class="community-section-nav" aria-label="本页内容"><a href="#why-join">加群能做什么</a><a href="#benefits">当前福利</a><a href="#joining">如何入群</a><a href="#binding">绑定教程</a><a href="#commands">常用指令</a></nav>

        <section id="why-join" class="community-section" aria-labelledby="why-title">
          <div class="community-section-heading"><div><span class="community-eyebrow">交流 · 查询 · 福利</span><h2 id="why-title">把常用的服务，放进一个群。</h2></div></div>
          <div class="community-purpose-grid">
            <article><span class="community-number">01</span><h3>交流使用经验</h3><p>分享客户端配置、模型体验和接入问题，和其他用户交流实用方法。</p></article>
            <article><span class="community-number">02</span><h3>随手查账户与模型</h3><p>绑定后通过机器人查询本人余额、今日用量；也能查看模型价格和公开渠道状态。</p></article>
            <article><span class="community-number">03</span><h3>参与群内福利</h3><p>签到、成长任务、抽奖和排行榜，具体开放项目与奖励见下方当前活动。</p></article>
            <article><span class="community-number">04</span><h3>了解站点动态</h3><p>关注群内发布的功能、模型和活动通知。{{ guide?.chat_enabled ? '当前还可 @机器人 进行 AI 交流，受群配额及实际回复限制。' : '有疑问可在群内交流，账户问题请联系管理员。' }}</p></article>
          </div>
        </section>

        <section id="benefits" class="community-section" aria-labelledby="benefits-title" :aria-busy="loading">
          <div class="community-section-heading"><div><span class="community-eyebrow">与后台活动同步</span><h2 id="benefits-title">现在有哪些福利？</h2><p>先完成本人账号绑定，再按活动规则参与。</p></div><button type="button" class="community-quiet-button" :disabled="loading" @click="load">{{ loading ? '正在读取…' : '刷新活动' }}</button></div>
          <div v-if="loading" class="community-state" role="status">正在读取当前活动规则…</div>
          <div v-else-if="failed" class="community-state community-error" role="alert"><strong>暂时无法读取活动规则</strong><p>请点击「刷新活动」重试；当前不展示未经确认的奖励金额。</p></div>
          <div v-else-if="!guide?.enabled" class="community-state">机器人目前未开放服务。加群与绑定教程可以先查看，活动请等待站点开放。</div>
          <div v-else-if="!guide.benefits.length" class="community-state">当前暂无已开启的福利活动，后续开放后会在这里展示。</div>
          <div v-else class="community-benefit-grid">
            <article v-for="(benefit, index) in guide.benefits" :key="benefit.id" class="community-benefit" :class="{ 'community-benefit-featured': index === 0 }" :data-benefit="benefit.id">
              <span class="community-benefit-label">{{ benefit.title }}</span><h3>{{ benefit.highlight }}</h3>
              <ul><li v-for="(line, lineIndex) in benefit.lines" :key="lineIndex">{{ line }}</li></ul>
              <div class="community-command-footer"><span>群内 @机器人</span><button type="button" :aria-label="`复制指令：${benefit.command}`" @click="copyCommand(benefit.command)">{{ copied === benefit.command ? '已复制' : benefit.command }} <span aria-hidden="true">⧉</span></button></div>
            </article>
          </div>
          <p class="community-caption">活动按北京时间统计。余额奖励进入已绑定的网站账号，活动赠额不计入有效充值；个人领取资格与结果以实际记录为准。</p>
        </section>

        <section id="joining" class="community-section community-joining" aria-labelledby="joining-title">
          <div><span class="community-eyebrow">先入群，再绑定</span><h2 id="joining-title">如何加入 Kitty QQ 群</h2><p v-if="joinRule">当前要求：网站账号后台累计实际消费达到 <strong>{{ joinRule.min_spent }}</strong>。审核查看的是实际消费，充值额和剩余额度不能代替消费。</p><p v-else>打开加群链接或扫描二维码，按群里显示的入群问题提交申请。</p></div>
          <ol><li>点击「打开 QQ 加群」，或用手机 QQ 扫描本页二维码。</li><li>在入群问题中填写<strong>本人在 Kitty 注册的邮箱</strong>。<span v-if="joinRule">当前问题：{{ joinRule.question }}</span></li><li>等待审核结果；填写邮箱只用于核验资格，进入群后仍需确认账号绑定。</li></ol>
          <a :href="communityGroupContact.href" target="_blank" rel="noopener noreferrer" class="community-button">申请加入 QQ 群 ↗</a>
        </section>

        <section id="binding" class="community-section" aria-labelledby="binding-title">
          <div class="community-section-heading"><div><span class="community-eyebrow">只绑定属于你自己的账号</span><h2 id="binding-title">QQ 和网站账号，怎么绑定？</h2><p>确认前不会开放账户查询或发放绑定奖励。已有相符绑定的用户可以直接沿用。</p></div><RouterLink :to="bindingTarget" class="community-secondary-button">前往个人资料 ↗</RouterLink></div>
          <div class="community-tabs" role="tablist" aria-label="选择绑定方式"><button id="email-tab" type="button" role="tab" :aria-selected="bindingMethod === 'email'" aria-controls="binding-steps" :class="{ active: bindingMethod === 'email' }" @click="bindingMethod = 'email'">{{ emailAutomatic ? '审核入群 · 邮箱确认' : '群内邮箱确认' }}</button><button id="code-tab" type="button" role="tab" :aria-selected="bindingMethod === 'code'" aria-controls="binding-steps" :class="{ active: bindingMethod === 'code' }" @click="bindingMethod = 'code'">网站绑定码</button></div>
          <div id="binding-steps" role="tabpanel" :aria-labelledby="bindingMethod === 'email' ? 'email-tab' : 'code-tab'" class="community-steps"><article v-for="(step, index) in bindingMethod === 'email' ? emailSteps : codeSteps" :key="`${bindingMethod}-${index}`"><span>{{ String(index + 1).padStart(2, '0') }}</span><div><h3>{{ step.title }}</h3><p>{{ step.text }}</p></div></article></div>
          <p v-if="emailAutomatic && bindingMethod === 'email'" class="community-binding-note">管理员邀请、人工放行，或入群后未收到自动确认邮件：切换「网站绑定码」完成绑定；也可 @机器人 发送「绑定 你的注册邮箱」发起邮件确认。</p>
          <p class="community-caption">不要公开邮箱确认链接、验证码或账户密码；遇到陌生 QQ 身份或已有绑定冲突时，先联系管理员核实。</p>
        </section>

        <section id="commands" class="community-section" aria-labelledby="commands-title">
          <div class="community-section-heading"><div><span class="community-eyebrow">复制后，在群内 @机器人 发送</span><h2 id="commands-title">常用指令，随用随查。</h2></div></div>
          <div v-if="guide?.enabled && !failed" class="community-command-grid"><button v-for="item in guide.commands" :key="item.command" type="button" :aria-label="`复制指令：${item.command}`" @click="copyCommand(item.command)"><span><code>{{ item.command }}</code><small>{{ item.description }}</small></span><span class="community-copy-label">{{ copied === item.command ? '已复制' : '复制' }}</span></button></div>
          <p v-else class="community-caption">活动配置恢复或机器人开放后，这里会展示当前可用的常用指令。</p>
          <p class="community-caption" aria-live="polite">{{ copyError ? '复制未成功，请选中指令文字手动复制。' : copied ? `已复制「${copied}」，请到 QQ 群内 @机器人 发送。` : '复制不会自动发送消息或执行签到、抽奖。查询本人余额和用量需要先完成绑定。' }}</p>
        </section>

        <section class="community-section" aria-labelledby="faq-title"><div class="community-section-heading"><div><span class="community-eyebrow">常见问题</span><h2 id="faq-title">把这几件事先说明白。</h2></div></div><div class="community-faq"><details v-for="item in faqs" :key="item.q"><summary>{{ item.q }}<span aria-hidden="true">＋</span></summary><p>{{ item.a }}</p></details></div></section>
        <footer class="community-footer"><span>Kitty 用户社区</span><RouterLink :to="auth.isAuthenticated ? '/dashboard' : '/home'">{{ auth.isAuthenticated ? '返回控制台' : '返回首页' }} ↗</RouterLink></footer>
      </div>
    </div>
  </component>
</template>

<style scoped>
.community-page{--community-bg:#f2f1ec;--community-card:#fff;--community-soft:#eae8e0;--community-border:#deddd5;--community-text:#252821;--community-muted:#656a5f;--community-accent:#c45100;--community-accent-soft:#fff2e7;color:var(--community-text);background:var(--community-bg);min-height:100vh;border-radius:18px;overflow-wrap:anywhere}
:global(.dark) .community-page{--community-bg:#161715;--community-card:#20221f;--community-soft:#292c26;--community-border:#383c33;--community-text:#f1f0e9;--community-muted:#b3b9aa;--community-accent:#e9a67b;--community-accent-soft:#34291f}
.community-content{width:100%;max-width:1480px;margin:0 auto;padding:44px 40px 24px}.community-public-nav{display:flex;align-items:center;justify-content:space-between;gap:16px;padding:18px 5%;border-bottom:1px solid var(--community-border)}.community-brand,.community-nav-actions{display:flex;align-items:center;gap:12px}.community-brand{font-weight:750}.community-brand img{width:34px;height:34px;border-radius:10px}.community-hero{display:grid;grid-template-columns:minmax(0,1fr) 280px;gap:64px;align-items:center;padding:24px 0 44px}.community-eyebrow{display:block;color:var(--community-accent);font-size:11px;font-weight:800;letter-spacing:.13em;margin-bottom:15px}.community-hero h1{font-size:clamp(34px,3.8vw,58px);font-weight:780;letter-spacing:-.045em;line-height:1.15;margin:0 0 22px}.community-lead{font-size:16px;line-height:1.9;max-width:650px;color:var(--community-muted)}.community-hero-actions{display:flex;flex-wrap:wrap;gap:12px;margin:28px 0 16px}.community-button,.community-secondary-button{display:inline-flex;justify-content:center;align-items:center;gap:14px;border-radius:11px;padding:13px 20px;font-size:13px;font-weight:750;text-decoration:none;transition:background .15s}.community-button{background:var(--community-accent);color:#fff}.community-button:hover{filter:brightness(.94)}:global(.dark) .community-button{color:#251a12}.community-button-small{padding:9px 14px}.community-secondary-button{background:var(--community-card);border:1px solid var(--community-border);color:var(--community-text)}.community-caption{color:var(--community-muted);font-size:12px;line-height:1.8;margin-top:16px}.community-join-card{background:var(--community-card);border:1px solid var(--community-border);border-radius:20px;padding:20px;text-align:center;box-shadow:0 8px 30px #25282106}.community-join-heading{display:flex;align-items:center;justify-content:center;gap:8px;font-size:13px;font-weight:750}.community-dot{width:7px;height:7px;border-radius:50%;background:var(--community-accent)}.community-qr{margin:18px auto 12px;padding:10px;background:#fff;border-radius:12px;width:192px;height:192px}.community-qr img{width:100%;height:100%;object-fit:contain}.community-join-card>p{font-size:12px;color:var(--community-muted);line-height:1.8}.community-join-rule{margin-top:14px;padding-top:13px;border-top:1px solid var(--community-border)}.community-section-nav{display:flex;flex-wrap:wrap;gap:8px 26px;padding:17px 0;border-top:1px solid var(--community-border);border-bottom:1px solid var(--community-border);font-size:12px;color:var(--community-muted)}.community-section-nav a:hover{color:var(--community-accent)}.community-section{padding-top:54px;scroll-margin-top:90px}.community-section-heading{display:flex;align-items:flex-end;justify-content:space-between;gap:24px;margin-bottom:24px}.community-section h2{font-size:clamp(23px,2.2vw,30px);font-weight:750;line-height:1.4;letter-spacing:-.03em;margin:0}.community-section-heading p,.community-joining p{color:var(--community-muted);font-size:13px;line-height:1.8;margin-top:10px}.community-quiet-button{font-size:12px;font-weight:650;color:var(--community-muted);padding:9px 12px;border:1px solid var(--community-border);border-radius:9px;background:var(--community-card);white-space:nowrap}.community-quiet-button:disabled{opacity:.6}.community-purpose-grid{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:25px}.community-purpose-grid article{padding:8px 0}.community-number{display:block;font-size:11px;letter-spacing:.08em;color:var(--community-accent);font-weight:750;margin-bottom:18px}.community-purpose-grid h3{font-size:15px;font-weight:700;margin-bottom:10px}.community-purpose-grid p{font-size:13px;line-height:1.9;color:var(--community-muted)}.community-benefit-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:16px}.community-benefit{display:flex;flex-direction:column;min-width:0;padding:23px;background:var(--community-card);border:1px solid var(--community-border);border-radius:15px}.community-benefit-featured{background:var(--community-accent-soft);border-color:color-mix(in srgb,var(--community-accent) 30%,var(--community-border))}.community-benefit-label{font-size:12px;font-weight:700;color:var(--community-muted)}.community-benefit h3{font-size:21px;font-weight:750;letter-spacing:-.025em;margin:14px 0 18px;line-height:1.5;color:var(--community-accent)}.community-benefit ul{font-size:12px;line-height:1.9;color:var(--community-muted);display:grid;gap:8px;padding-left:15px;list-style-type:disc;margin-bottom:24px}.community-command-footer{margin-top:auto;display:flex;align-items:center;justify-content:space-between;gap:8px;padding-top:14px;border-top:1px solid var(--community-border);font-size:10px;color:var(--community-muted)}.community-command-footer button{color:var(--community-text);font-size:12px;font-weight:700;background:var(--community-soft);border-radius:7px;padding:6px 10px}.community-state{background:var(--community-card);border:1px solid var(--community-border);padding:30px;border-radius:14px;font-size:14px;color:var(--community-muted);line-height:1.9}.community-error{border-color:#d8a077}.community-joining{margin-top:52px;padding:28px;display:grid;grid-template-columns:1fr 1fr;gap:24px;background:var(--community-soft);border-radius:16px}.community-joining h2{font-size:24px}.community-joining ol{font-size:13px;line-height:1.9;display:grid;gap:10px;padding-left:18px;list-style:decimal;color:var(--community-muted)}.community-joining li span{display:block;font-size:12px;margin-top:5px}.community-joining>.community-button{justify-self:start}.community-tabs{display:inline-flex;flex-wrap:wrap;background:var(--community-soft);padding:5px;border-radius:12px;gap:4px;margin-bottom:24px}.community-tabs button{padding:10px 15px;border-radius:8px;font-size:12px;font-weight:700;color:var(--community-muted)}.community-tabs button.active{color:var(--community-accent);background:var(--community-card);box-shadow:0 2px 5px #00000008}.community-steps{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:22px}.community-steps article{border-top:2px solid var(--community-border);padding-top:18px}.community-steps article>span{font-size:12px;font-weight:800;color:var(--community-accent);display:block;margin-bottom:14px}.community-steps h3{font-size:14px;font-weight:750;line-height:1.65;margin-bottom:10px}.community-steps p{color:var(--community-muted);font-size:12px;line-height:1.95}.community-binding-note{background:var(--community-accent-soft);padding:14px 18px;margin-top:22px;border-radius:10px;color:var(--community-muted);font-size:12px;line-height:1.9}.community-command-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:10px}.community-command-grid>button{display:flex;justify-content:space-between;align-items:center;gap:12px;background:var(--community-card);border:1px solid var(--community-border);padding:17px;border-radius:11px;text-align:left}.community-command-grid>button:hover{border-color:var(--community-accent)}.community-command-grid code{font-size:13px;font-weight:700}.community-command-grid small{display:block;color:var(--community-muted);font-size:11px;line-height:1.7;margin-top:6px}.community-copy-label{font-size:10px;white-space:nowrap;color:var(--community-accent)}.community-faq{border-top:1px solid var(--community-border)}.community-faq details{border-bottom:1px solid var(--community-border)}.community-faq summary{padding:19px 0;cursor:pointer;font-size:14px;font-weight:650;list-style:none;display:flex;justify-content:space-between;gap:20px}.community-faq summary::-webkit-details-marker{display:none}.community-faq summary span{color:var(--community-accent)}.community-faq details[open] summary span{transform:rotate(45deg)}.community-faq p{padding:0 35px 21px 0;font-size:13px;line-height:1.95;color:var(--community-muted);max-width:1000px}.community-footer{display:flex;justify-content:space-between;gap:20px;margin-top:54px;padding-top:23px;border-top:1px solid var(--community-border);font-size:12px;color:var(--community-muted)}.community-page :is(a,button,summary):focus-visible{outline:2px solid var(--community-accent);outline-offset:4px}
@media(min-width:1700px){.community-content{padding-top:54px}.community-hero{gap:100px}.community-purpose-grid{gap:44px}}
@media(max-width:1100px){.community-content{padding:28px 24px}.community-hero{gap:28px;grid-template-columns:minmax(0,1fr) 250px}.community-qr{width:175px;height:175px}.community-purpose-grid,.community-steps{grid-template-columns:repeat(2,minmax(0,1fr))}.community-benefit-grid{grid-template-columns:repeat(2,minmax(0,1fr))}.community-command-grid{grid-template-columns:repeat(2,minmax(0,1fr))}}
@media(max-width:640px){.community-page{border-radius:12px}.community-content{padding:24px 16px}.community-public-nav{padding:14px 16px}.community-brand{font-size:12px;gap:8px}.community-nav-actions{gap:6px}.community-button-small{font-size:11px;padding:9px 10px}.community-hero{grid-template-columns:1fr;gap:28px;padding:6px 0 28px}.community-hero h1{font-size:38px}.community-lead{font-size:14px}.community-join-card{width:100%;display:grid;grid-template-columns:125px 1fr;gap:4px 18px;text-align:left;padding:18px}.community-join-heading{grid-column:2;grid-row:1;justify-content:flex-start;font-size:12px}.community-qr{grid-column:1;grid-row:1/4;margin:0;width:125px;height:125px;padding:6px}.community-join-card>p{font-size:11px;margin:0}.community-join-rule{padding-top:7px;margin-top:4px!important}.community-section-nav{gap:14px 19px;font-size:11px}.community-section{padding-top:38px}.community-section-heading{gap:12px;align-items:flex-start;flex-wrap:wrap;margin-bottom:20px}.community-section h2{font-size:24px}.community-purpose-grid{gap:22px 18px}.community-purpose-grid h3{font-size:13px}.community-purpose-grid p{font-size:12px}.community-benefit-grid,.community-command-grid{grid-template-columns:1fr}.community-benefit{padding:21px}.community-joining{grid-template-columns:1fr;padding:23px;margin-top:38px;gap:20px}.community-tabs{display:flex}.community-tabs button{flex:1;font-size:11px;padding:10px 8px}.community-steps{grid-template-columns:1fr;gap:20px}.community-steps article{display:flex;gap:16px;padding-top:16px}.community-steps article>span{padding-top:4px}.community-faq summary{font-size:13px;line-height:1.7}.community-footer{margin-top:36px}}
</style>
