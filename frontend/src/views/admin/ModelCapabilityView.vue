<template>
  <AppLayout>
    <div class="w-full min-w-0 space-y-5 pb-8">
      <header class="flex flex-wrap items-start justify-between gap-4 border-b border-gray-200 pb-4 dark:border-dark-700">
        <div>
          <div class="flex items-center gap-2">
            <span class="inline-flex h-8 w-8 items-center justify-center rounded-lg bg-blue-50 text-blue-600 dark:bg-blue-900/30 dark:text-blue-300"><Icon name="shield" size="sm" /></span>
            <h1 class="text-xl font-bold text-gray-900 dark:text-white">神机</h1>
          </div>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">汇总真实流量证据，并对账号映射后的模型执行受控主动探测。能力支持与身份可信度分开判断。</p>
        </div>
        <div class="flex items-center gap-2">
          <button type="button" class="btn btn-secondary" :disabled="loading" title="刷新审计结果" @click="refreshAll"><Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />刷新记录</button>
          <button type="button" class="btn btn-primary" :disabled="running || loading || previewing" title="批量探测启用账号和已观测模型" @click="prepareBatchProbe"><Icon name="beaker" size="sm" />{{ running ? '探测中…' : previewing ? '计算范围…' : '一键探测' }}</button>
        </div>
      </header>

      <div v-if="errorMessage" class="border-l-4 border-red-500 bg-red-50 px-4 py-3 text-sm text-red-800 dark:bg-red-950/20 dark:text-red-200">{{ errorMessage }}</div>

      <section class="grid gap-4 lg:grid-cols-2">
        <div class="border-l-4 border-sky-500 bg-sky-50 px-4 py-3 dark:bg-sky-950/20"><div class="flex items-center gap-2 text-sm font-semibold text-sky-900 dark:text-sky-200"><Icon name="database" size="sm" />被动真实流量证据</div><p class="mt-1 text-xs leading-5 text-sky-800/80 dark:text-sky-200/70">来自已完成请求的请求模型、映射模型和上游响应模型，不产生额外上游请求。</p></div>
        <div class="border-l-4 border-violet-500 bg-violet-50 px-4 py-3 dark:bg-violet-950/20"><div class="flex items-center gap-2 text-sm font-semibold text-violet-900 dark:text-violet-200"><Icon name="beaker" size="sm" />主动受控探测</div><p class="mt-1 text-xs leading-5 text-violet-800/80 dark:text-violet-200/70">对启用的 OpenAI 兼容账号发送短请求，记录协议、工具调用、状态码和延迟。</p></div>
      </section>

      <section class="flex flex-wrap items-end gap-3 border-b border-gray-200 pb-4 dark:border-dark-700">
        <label class="block min-w-32"><span class="mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400">观察窗口</span><select v-model.number="days" class="input w-full" :disabled="running" @change="refreshAll"><option :value="7">最近 7 天</option><option :value="30">最近 30 天</option><option :value="90">最近 90 天</option></select></label>
        <label class="flex h-9 items-center gap-2 px-1 text-sm text-gray-700 dark:text-gray-200"><input v-model="activeOnly" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500" :disabled="running" />仅探测启用账号</label>
        <label class="block w-28"><span class="mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400">探测上限</span><input v-model.number="maxItems" type="number" min="1" max="240" class="input w-full" :disabled="running" /></label>
        <span v-if="lastRun" class="pb-2 text-xs text-gray-500 dark:text-gray-400">上次运行 #{{ lastRun.id }}：{{ runStatusLabel(lastRun.status) }}，{{ lastRun.completed_items ?? lastRun.completed_count }}/{{ lastRun.total_items ?? lastRun.total_count }} 项协议证据</span>
      </section>

      <section class="grid grid-cols-2 gap-3 md:grid-cols-5"><div v-for="card in summaryCards" :key="card.label" class="border border-gray-200 bg-white px-4 py-3 dark:border-dark-700 dark:bg-dark-800"><p class="text-xs text-gray-500 dark:text-gray-400">{{ card.label }}</p><p class="mt-1 text-2xl font-semibold" :class="card.color">{{ card.value }}</p></div></section>

      <section class="border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800">
        <div class="flex flex-wrap items-center justify-between gap-3 border-b border-gray-200 px-4 py-3 dark:border-dark-700">
          <div>
            <h2 class="text-sm font-semibold text-gray-900 dark:text-white">模型审计矩阵</h2>
            <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">每项协议独立展示能力、身份边界和最近一次证据；点击任意行查看同模型的完整证据。</p>
          </div>
          <div class="text-right text-xs text-gray-500 dark:text-gray-400">
            <div>{{ summary?.generated_at ? formatTime(summary.generated_at) : '尚未加载' }}</div>
            <div v-if="summary" class="mt-1">
              当前 {{ summary.returned_models ?? summary.rows.length }} / 总计 {{ summary.total_models }} 个模型映射
              <span v-if="summary.rows_truncated" class="text-amber-600 dark:text-amber-300">（已按上限截断）</span>
            </div>
          </div>
        </div>
        <div v-if="loading" class="px-4 py-12 text-center text-sm text-gray-500 dark:text-gray-400">正在读取审计记录…</div>
        <div v-else-if="protocolRows.length === 0" class="px-4 py-12 text-center text-sm text-gray-500 dark:text-gray-400">暂无证据。先积累真实请求，或点击“一键探测”生成主动记录。</div>
        <div v-else class="overflow-x-auto">
          <table class="w-full min-w-[1260px] text-left text-sm">
            <thead class="bg-gray-50 text-xs font-semibold text-gray-500 dark:bg-dark-900/50 dark:text-gray-400">
              <tr>
                <th class="px-4 py-3">账号 / 模型映射</th>
                <th class="px-4 py-3">协议 / 能力</th>
                <th class="px-4 py-3">能力 / 身份判定</th>
                <th class="px-4 py-3">证据来源</th>
                <th class="px-4 py-3">最近证据</th>
                <th class="px-4 py-3">响应详情</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr
                v-for="item in protocolRows"
                :key="item.key"
                class="cursor-pointer align-top transition hover:bg-gray-50 dark:hover:bg-dark-900/40"
                tabindex="0"
                @click="selectedRow = item.row"
                @keydown.enter="selectedRow = item.row"
              >
                <td class="px-4 py-3">
                  <div class="font-medium text-gray-900 dark:text-white">{{ item.row.account_name || ('账号 #' + item.row.account_id) }}</div>
                  <div class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{{ item.row.platform }} · {{ item.row.group_name || '未分组' }}</div>
                  <div class="mt-2 font-mono text-xs text-gray-800 dark:text-gray-200">{{ item.row.requested_model }}</div>
                  <div class="mt-0.5 font-mono text-[11px] text-gray-500 dark:text-gray-400">映射：{{ item.row.mapped_model || '直通' }}</div>
                </td>
                <td class="px-4 py-3">
                  <div class="font-medium text-gray-900 dark:text-white">{{ item.protocol ? protocolLabel(item.protocol) : '被动真实流量' }}</div>
                  <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ item.evidence?.capability || (item.protocol ? '基础协议响应' : '请求与响应模型记录') }}</div>
                  <div v-if="item.protocol === 'responses_tools'" class="mt-1 text-xs" :class="item.evidence?.tool_call_found && item.evidence?.tool_arguments_valid ? 'text-emerald-600' : 'text-gray-500'">
                    工具调用：{{ item.evidence?.tool_call_found && item.evidence?.tool_arguments_valid ? '结构有效' : '未验证' }}
                  </div>
                </td>
                <td class="px-4 py-3">
                  <span v-if="item.evidence" class="rounded px-2 py-1 text-xs font-medium" :class="statusClass(item.evidence.status)">{{ protocolStatus(item.evidence.status) }}</span>
                  <span v-else class="text-xs text-gray-500 dark:text-gray-400">{{ item.row.passive_requests > 0 ? '已观测请求' : '尚无证据' }}</span>
                  <div class="mt-2 text-xs" :class="capabilityClass(itemCapabilityStatus(item))">{{ capabilityLabel(itemCapabilityStatus(item)) }}</div>
                  <div class="mt-1 max-w-56 text-xs" :class="identityTextClass(itemIdentityStatus(item))">{{ identityLabel(itemIdentityStatus(item)) }}</div>
                  <div class="mt-0.5 text-[11px] text-gray-400">{{ confidenceLabel(itemConfidence(item)) }}</div>
                </td>
                <td class="px-4 py-3 text-xs text-gray-600 dark:text-gray-300">
                  <div class="font-medium text-gray-800 dark:text-gray-200">{{ evidenceSourceLabel(item.evidence?.evidence_source || item.evidence?.source) }}</div>
                  <div v-if="item.evidence" class="mt-1">累计：尝试 {{ evidenceAttempts(item.evidence) }} / 成功 {{ evidenceSuccesses(item.evidence) }} / 失败 {{ evidenceFailures(item.evidence) }}</div>
                  <div class="mt-1">真实流量：{{ item.row.passive_requests }} 次<span v-if="item.row.passive_mismatches" class="ml-1 text-amber-600">· 偏差 {{ item.row.passive_mismatches }}</span></div>
                </td>
                <td class="whitespace-nowrap px-4 py-3 text-xs text-gray-500 dark:text-gray-400">
                  <div>{{ formatTime(evidenceTime(item)) }}</div>
                  <div v-if="item.evidence?.run_id" class="mt-1 font-mono text-[11px]">运行 #{{ item.evidence.run_id }}</div>
                </td>
                <td class="max-w-80 px-4 py-3 text-xs text-gray-600 dark:text-gray-300">
                  <div>HTTP {{ item.evidence?.http_status ?? '-' }} · {{ item.evidence?.latency_ms ?? '-' }} ms</div>
                  <div class="mt-1 break-all font-mono">返回模型：{{ evidenceResponseModel(item) }}</div>
                  <div v-if="item.evidence" class="mt-1 break-words text-gray-500 dark:text-gray-400">证据：{{ item.evidence.evidence_code || item.evidence.evidence || '-' }}</div>
                  <div v-if="item.evidence?.error" class="mt-1 break-words text-red-600 dark:text-red-300">{{ item.evidence.error }}</div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <section class="border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800"><div class="border-b border-gray-200 px-4 py-3 dark:border-dark-700"><h2 class="text-sm font-semibold text-gray-900 dark:text-white">探测历史</h2></div><div v-if="runs.length === 0" class="px-4 py-8 text-center text-sm text-gray-500 dark:text-gray-400">还没有主动探测记录</div><div v-else class="overflow-x-auto"><table class="min-w-[720px] w-full text-left text-sm"><thead class="bg-gray-50 text-xs text-gray-500 dark:bg-dark-900/50 dark:text-gray-400"><tr><th class="px-4 py-2.5">运行</th><th class="px-4 py-2.5">状态</th><th class="px-4 py-2.5">进度</th><th class="px-4 py-2.5">结果</th><th class="px-4 py-2.5">时间</th></tr></thead><tbody class="divide-y divide-gray-100 dark:divide-dark-700"><tr v-for="run in runs" :key="run.id" class="cursor-pointer hover:bg-gray-50 dark:hover:bg-dark-900/40" @click="openRun(run.id)"><td class="px-4 py-2.5 font-mono text-xs">#{{ run.id }}</td><td class="px-4 py-2.5"><span class="rounded px-2 py-1 text-xs" :class="runStatusClass(run.status)">{{ runStatusLabel(run.status) }}</span></td><td class="px-4 py-2.5 text-xs text-gray-600 dark:text-gray-300">{{ run.completed_count }}/{{ run.total_count }}</td><td class="px-4 py-2.5 text-xs text-gray-600 dark:text-gray-300">通过 {{ run.verified_count }} · 偏差 {{ run.mismatch_count }} · 不支持 {{ run.unsupported_count }} · 异常 {{ run.error_count }}</td><td class="px-4 py-2.5 text-xs text-gray-500 dark:text-gray-400">{{ formatTime(run.created_at) }}</td></tr></tbody></table></div></section>

      <section class="border-t border-gray-200 pt-5 dark:border-dark-700">
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">成对行为审计</h2>
            <p class="mt-1 max-w-4xl text-xs leading-5 text-gray-500 dark:text-gray-400">使用固定基准题，把第三方 API Key 账号与支持同一模型的 api.openai.com 官方锚点成对比较。保存正确性、响应模型、延迟和输出哈希，不保存完整回答；行为接近仍不能证明底层模型身份。</p>
          </div>
          <button type="button" class="btn btn-secondary" :disabled="behaviorLoading" title="刷新成对审计" @click="refreshBehavior"><Icon name="refresh" size="sm" :class="behaviorLoading ? 'animate-spin' : ''" />刷新审计</button>
        </div>
        <div v-if="behaviorErrorMessage" class="mt-4 border-l-4 border-red-500 bg-red-50 px-4 py-3 text-sm text-red-800 dark:bg-red-950/20 dark:text-red-200">{{ behaviorErrorMessage }}</div>
        <div class="mt-4 grid gap-3 md:grid-cols-[minmax(0,2fr)_minmax(8rem,0.7fr)_minmax(9rem,0.8fr)_auto] md:items-end">
          <label class="block"><span class="mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400">目标模型</span><input v-model.trim="behaviorModel" class="input w-full font-mono" maxlength="200" placeholder="例如 gpt-5" :disabled="behaviorRunning" /></label>
          <label class="block"><span class="mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400">基准用例数</span><input v-model.number="behaviorCaseCount" type="number" min="1" max="5" class="input w-full" :disabled="behaviorRunning" /></label>
          <label class="block"><span class="mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400">推理强度</span><select v-model="behaviorEffort" class="input w-full" :disabled="behaviorRunning"><option value="minimal">Minimal</option><option value="low">Low</option><option value="medium">Medium</option><option value="high">High</option><option value="xhigh">XHigh</option></select></label>
          <button type="button" class="btn btn-primary h-9" :disabled="behaviorRunning || behaviorPreviewing || !behaviorModel" @click="prepareBehaviorAudit"><Icon name="beaker" size="sm" />{{ behaviorRunning ? '审计中…' : behaviorPreviewing ? '计算范围…' : '开始审计' }}</button>
        </div>
        <div v-if="latestBehaviorRun" class="mt-4 bg-gray-50 px-4 py-3 text-xs text-gray-600 dark:bg-dark-900/40 dark:text-gray-300">
          <div class="flex flex-wrap items-center gap-x-4 gap-y-2">
            <span class="font-medium text-gray-900 dark:text-white">运行 #{{ latestBehaviorRun.id }}</span>
            <span :class="runStatusClass(latestBehaviorRun.status)" class="rounded px-2 py-1">{{ runStatusLabel(latestBehaviorRun.status) }}</span>
            <span>{{ latestBehaviorRun.completed_pairs }}/{{ latestBehaviorRun.total_pairs }} 对</span>
            <span>请求 {{ latestBehaviorRun.attempt_count }}/{{ latestBehaviorRun.total_requests }}</span>
            <span>一致 {{ latestBehaviorRun.matched_count }}</span>
            <span>偏离 {{ latestBehaviorRun.diverged_count }}</span>
            <span>待复核 {{ latestBehaviorRun.inconclusive_count }}</span>
            <span>异常 {{ latestBehaviorRun.error_count }}</span>
          </div>
          <p v-if="latestBehaviorRun.error_message" class="mt-2 break-words text-red-600 dark:text-red-300">{{ latestBehaviorRun.error_message }}</p>
        </div>
        <div v-if="behaviorRuns.length" class="mt-3 flex flex-wrap gap-2">
          <button
            v-for="run in behaviorRuns"
            :key="run.id"
            type="button"
            class="border border-gray-200 px-3 py-1.5 text-xs text-gray-600 hover:bg-gray-50 dark:border-dark-700 dark:text-gray-300 dark:hover:bg-dark-900/40"
            :class="behaviorDetail?.run.id === run.id ? 'border-blue-500 text-blue-600 dark:border-blue-400 dark:text-blue-300' : ''"
            @click="openBehaviorRun(run.id)"
          >#{{ run.id }} · {{ run.model_id }} · {{ runStatusLabel(run.status) }}</button>
        </div>
        <div v-if="behaviorDetail?.results.length" class="mt-4 overflow-x-auto border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800">
          <table class="w-full min-w-[1180px] text-left text-sm">
            <thead class="bg-gray-50 text-xs text-gray-500 dark:bg-dark-900/50 dark:text-gray-400">
              <tr><th class="px-4 py-3">目标 / 官方锚点</th><th class="px-4 py-3">基准</th><th class="px-4 py-3">结论</th><th class="px-4 py-3">端点结果</th><th class="px-4 py-3">延迟</th><th class="px-4 py-3">映射 / 自报模型</th><th class="px-4 py-3">工具结果证据</th></tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="item in behaviorDetail.results" :key="item.id" class="align-top">
                <td class="px-4 py-3"><div class="font-medium text-gray-900 dark:text-white">{{ item.target_account_name || `账号 #${item.target_account_id}` }}</div><div class="mt-0.5 text-xs text-gray-500">对 {{ item.anchor_account_name || `官方锚点 #${item.anchor_account_id}` }}</div></td>
                <td class="px-4 py-3"><div>{{ item.case_label || item.case_key }}</div><div class="mt-0.5 font-mono text-[11px] text-gray-400">{{ item.case_key }}</div></td>
                <td class="px-4 py-3"><span class="rounded px-2 py-1 text-xs font-medium" :class="behaviorStatusClass(item.comparison_status)">{{ behaviorStatusLabel(item.comparison_status) }}</span><div class="mt-2 text-[11px] text-gray-500">尝试 {{ item.attempt_count }} / 成功 {{ item.success_count }} / 失败 {{ item.failure_count }}</div></td>
                <td class="px-4 py-3 text-xs"><div>目标 {{ behaviorObservationLabel(item.target_status) }} · HTTP {{ item.target_http_status || '-' }}</div><div class="mt-1">锚点 {{ behaviorObservationLabel(item.anchor_status) }} · HTTP {{ item.anchor_http_status || '-' }}</div></td>
                <td class="px-4 py-3 text-xs"><div>目标 {{ item.target_latency_ms }} ms</div><div class="mt-1">锚点 {{ item.anchor_latency_ms }} ms</div></td>
                <td class="max-w-64 px-4 py-3 font-mono text-xs"><div class="break-all">目标 {{ item.target_mapped_model || '-' }} → {{ item.target_response_model || '-' }}</div><div class="mt-1 break-all text-gray-400">锚点 {{ item.anchor_mapped_model || '-' }} → {{ item.anchor_response_model || '-' }}</div></td>
                <td class="max-w-72 px-4 py-3 font-mono text-[11px] text-gray-500 dark:text-gray-400"><div :title="item.target_output_hash">目标 {{ shortHash(item.target_output_hash) }}</div><div class="mt-1" :title="item.anchor_output_hash">锚点 {{ shortHash(item.anchor_output_hash) }}</div><div class="mt-1" :title="item.expected_output_hash">预期 {{ shortHash(item.expected_output_hash) }}</div><div v-if="item.error" class="mt-2 break-words font-sans text-red-600 dark:text-red-300">{{ item.error }}</div></td>
              </tr>
            </tbody>
          </table>
        </div>
        <div v-else-if="behaviorDetail" class="mt-4 border border-dashed border-gray-300 px-4 py-8 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400">{{ isBehaviorRunActive(behaviorDetail.run) ? '审计正在后台运行，首批结果完成后会自动显示。' : '该次审计没有生成可展示的成对结果。' }}</div>
        <div v-else-if="behaviorRuns.length === 0" class="mt-4 border border-dashed border-gray-300 px-4 py-8 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400">还没有成对行为审计记录</div>
      </section>
    </div>

    <div v-if="pendingEstimate" class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4" role="dialog" aria-modal="true" aria-labelledby="probe-confirm-title" @click.self="pendingEstimate = null"><section class="w-full max-w-md border border-gray-200 bg-white shadow-xl dark:border-dark-700 dark:bg-dark-800"><div class="flex items-start justify-between border-b border-gray-200 px-5 py-4 dark:border-dark-700"><div><h2 id="probe-confirm-title" class="font-semibold text-gray-900 dark:text-white">确认一键探测</h2><p class="mt-1 text-xs text-gray-500 dark:text-gray-400">主动探测会产生上游用量和费用</p></div><button type="button" class="inline-flex h-9 w-9 shrink-0 items-center justify-center" title="取消探测" @click="pendingEstimate = null"><Icon name="x" size="sm" /></button></div><div class="space-y-4 p-5"><div class="grid grid-cols-3 gap-3 text-center"><div class="border border-gray-200 px-2 py-3 dark:border-dark-700"><p class="text-xl font-semibold text-gray-900 dark:text-white">{{ pendingEstimate.target_count }}</p><p class="mt-1 text-xs text-gray-500">账号模型</p></div><div class="border border-gray-200 px-2 py-3 dark:border-dark-700"><p class="text-xl font-semibold text-gray-900 dark:text-white">{{ pendingEstimate.request_count }}</p><p class="mt-1 text-xs text-gray-500">上游请求</p></div><div class="border border-gray-200 px-2 py-3 dark:border-dark-700"><p class="text-xl font-semibold text-gray-900 dark:text-white">{{ estimateDuration(pendingEstimate.estimated_max_seconds) }}</p><p class="mt-1 text-xs text-gray-500">最长预计</p></div></div><p class="text-xs leading-5 text-gray-600 dark:text-gray-300">每个账号模型按 {{ pendingEstimate.protocol_count }} 项协议发送最小非流式请求。实际扣费由上游模型价格、输出量和账号计费规则决定。</p><div class="flex justify-end gap-2"><button type="button" class="btn btn-secondary" @click="pendingEstimate = null">取消</button><button type="button" class="btn btn-primary" @click="confirmBatchProbe"><Icon name="play" size="sm" />确认探测</button></div></div></section></div>

    <div v-if="pendingBehaviorEstimate" class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4" role="dialog" aria-modal="true" aria-labelledby="behavior-confirm-title" @click.self="pendingBehaviorEstimate = null"><section class="w-full max-w-md border border-gray-200 bg-white shadow-xl dark:border-dark-700 dark:bg-dark-800"><div class="flex items-start justify-between border-b border-gray-200 px-5 py-4 dark:border-dark-700"><div><h2 id="behavior-confirm-title" class="font-semibold text-gray-900 dark:text-white">确认成对行为审计</h2><p class="mt-1 text-xs text-gray-500 dark:text-gray-400">目标端与官方锚点都会产生上游费用</p></div><button type="button" class="inline-flex h-9 w-9 shrink-0 items-center justify-center" title="取消审计" @click="pendingBehaviorEstimate = null"><Icon name="x" size="sm" /></button></div><div class="space-y-4 p-5"><div class="grid grid-cols-3 gap-3 text-center"><div class="border border-gray-200 px-2 py-3 dark:border-dark-700"><p class="text-xl font-semibold text-gray-900 dark:text-white">{{ pendingBehaviorEstimate.pair_count }}</p><p class="mt-1 text-xs text-gray-500">成对用例</p></div><div class="border border-gray-200 px-2 py-3 dark:border-dark-700"><p class="text-xl font-semibold text-gray-900 dark:text-white">{{ pendingBehaviorEstimate.request_count }}</p><p class="mt-1 text-xs text-gray-500">上游请求</p></div><div class="border border-gray-200 px-2 py-3 dark:border-dark-700"><p class="text-xl font-semibold text-gray-900 dark:text-white">{{ estimateDuration(pendingBehaviorEstimate.estimated_max_seconds) }}</p><p class="mt-1 text-xs text-gray-500">最长预计</p></div></div><p class="text-xs leading-5 text-gray-600 dark:text-gray-300">将比较 {{ pendingBehaviorEstimate.target_count }} 个第三方账号与 {{ pendingBehaviorEstimate.anchor_count }} 个可用官方锚点。结果是可复现的行为证据，不是模型身份认证。</p><p v-if="pendingBehaviorEstimate.targets_truncated" class="border-l-4 border-amber-400 bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:bg-amber-950/20 dark:text-amber-200">共有 {{ pendingBehaviorEstimate.available_target_count }} 个账号符合条件，本次按安全上限只审计前 {{ pendingBehaviorEstimate.target_count }} 个。</p><div class="flex justify-end gap-2"><button type="button" class="btn btn-secondary" @click="pendingBehaviorEstimate = null">取消</button><button type="button" class="btn btn-primary" @click="confirmBehaviorAudit"><Icon name="play" size="sm" />确认审计</button></div></div></section></div>

    <div v-if="selectedRow" class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4" @click.self="selectedRow = null">
      <section class="max-h-[90vh] w-full max-w-3xl overflow-y-auto border border-gray-200 bg-white shadow-xl dark:border-dark-700 dark:bg-dark-800" role="dialog" aria-modal="true" aria-labelledby="model-detail-title" tabindex="-1">
        <div class="flex items-start justify-between gap-4 border-b border-gray-200 px-5 py-4 dark:border-dark-700">
          <div class="min-w-0">
            <h2 id="model-detail-title" class="break-all font-semibold text-gray-900 dark:text-white">{{ selectedRow.requested_model }}</h2>
            <p class="mt-1 break-words text-xs text-gray-500 dark:text-gray-400">{{ selectedRow.account_name || ('账号 #' + selectedRow.account_id) }} · {{ selectedRow.platform }} · {{ selectedRow.group_name || '未分组' }}</p>
          </div>
          <button type="button" class="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded hover:bg-gray-100 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:hover:bg-dark-700" title="关闭详情" aria-label="关闭模型详情" @click="selectedRow = null"><Icon name="x" size="sm" /></button>
        </div>
        <div class="grid gap-3 border-b border-gray-200 p-5 text-sm sm:grid-cols-3 dark:border-dark-700">
          <div><span class="text-xs text-gray-500">能力</span><p class="mt-1 font-medium" :class="capabilityClass(selectedRow.capability_status)">{{ capabilityLabel(selectedRow.capability_status) }}</p></div>
          <div><span class="text-xs text-gray-500">身份</span><p class="mt-1 font-medium" :class="identityTextClass(selectedRow.identity_status)">{{ identityLabel(selectedRow.identity_status) }} · {{ confidenceLabel(selectedRow.confidence) }}</p></div>
          <div><span class="text-xs text-gray-500">真实流量</span><p class="mt-1 font-medium text-gray-900 dark:text-white">{{ selectedRow.passive_requests }} 次 / 偏差 {{ selectedRow.passive_mismatches }}</p></div>
        </div>
        <div class="p-5">
          <h3 class="mb-3 text-xs font-semibold uppercase tracking-wide text-gray-500">协议证据</h3>
          <div class="space-y-2">
            <div v-for="(evidence, protocol) in selectedRow.protocols" :key="protocol" class="border border-gray-200 p-3 dark:border-dark-700">
              <div class="flex flex-wrap items-center justify-between gap-2"><span class="font-medium text-gray-900 dark:text-white">{{ protocolLabel(protocol) }}</span><span class="rounded px-2 py-1 text-xs" :class="statusClass(evidence.status)">{{ protocolStatus(evidence.status) }}</span></div>
              <div class="mt-2 grid gap-2 text-xs text-gray-600 sm:grid-cols-3 dark:text-gray-300"><span>HTTP {{ evidence.http_status || '-' }}</span><span>{{ evidence.latency_ms ?? '-' }} ms</span><span class="break-all">返回 {{ evidence.response_model || evidence.upstream_model || '-' }}</span></div>
              <p class="mt-2 break-words text-xs text-gray-500 dark:text-gray-400">证据：{{ evidence.evidence_code || evidence.evidence || '-' }}<span v-if="evidence.error"> · {{ evidence.error }}</span></p>
            </div>
            <p v-if="Object.keys(selectedRow.protocols).length === 0" class="text-sm text-gray-500">尚无主动探测证据</p>
          </div>
          <div v-if="selectedRow.passive_response_models?.length" class="mt-5">
            <h3 class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">被动流量返回模型</h3>
            <div class="flex flex-wrap gap-2"><span v-for="model in selectedRow.passive_response_models" :key="model" class="break-all rounded bg-gray-100 px-2 py-1 font-mono text-xs text-gray-700 dark:bg-dark-700 dark:text-gray-200">{{ model }}</span></div>
          </div>
        </div>
      </section>
    </div>

    <div v-if="runDetail" class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4" @click.self="runDetail = null">
      <section class="max-h-[85vh] w-full max-w-2xl overflow-y-auto border border-gray-200 bg-white shadow-xl dark:border-dark-700 dark:bg-dark-800" role="dialog" aria-modal="true" aria-labelledby="run-detail-title" tabindex="-1">
        <div class="flex items-center justify-between gap-4 border-b border-gray-200 px-5 py-4 dark:border-dark-700">
          <h2 id="run-detail-title" class="font-semibold text-gray-900 dark:text-white">探测运行 #{{ runDetail.run.id }}</h2>
          <button type="button" class="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded hover:bg-gray-100 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:hover:bg-dark-700" title="关闭详情" aria-label="关闭运行详情" @click="runDetail = null"><Icon name="x" size="sm" /></button>
        </div>
        <div class="space-y-2 p-5">
          <div v-for="(item, index) in runDetail.evidence" :key="item.checked_at + '-' + index" class="border-b border-gray-100 py-2 text-xs dark:border-dark-700">
            <div class="flex justify-between gap-3"><span class="font-medium">{{ protocolLabel(item.protocol) }}</span><span :class="statusTextClass(item.status)">{{ protocolStatus(item.status) }}</span></div>
            <div class="mt-1 break-words text-gray-500">HTTP {{ item.http_status || '-' }} · {{ item.latency_ms ?? '-' }} ms · {{ item.response_model || item.upstream_model || '未返回模型' }} · {{ formatTime(item.checked_at) }}</div>
            <div v-if="item.error" class="mt-1 break-words text-red-600">{{ item.error }}</div>
          </div>
        </div>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { extractApiErrorMessage } from '@/utils/apiError'
import { adminAPI } from '@/api/admin'
import {
  normalizeCapabilityIdentityStatus,
  passiveCapabilityStatus,
  passiveConfidence,
  passiveIdentityStatus,
} from './modelCapabilityPresentation'
import type {
  BehaviorAuditDetail,
  BehaviorAuditEstimate,
  BehaviorAuditRequest,
  BehaviorAuditRun,
  CapabilityAuditEstimate,
  CapabilityAuditEvidence,
  CapabilityAuditProbeRequest,
  CapabilityAuditRow,
  CapabilityAuditRun,
  CapabilityAuditRunDetail,
  CapabilityAuditSummary,
} from '@/api/admin/modelCapabilityAudit'

interface CapabilityProtocolRow {
  key: string
  row: CapabilityAuditRow
  protocol: string | null
  evidence: CapabilityAuditEvidence | null
}

type BehaviorEffort = NonNullable<BehaviorAuditRequest['reasoning_effort']>

const summary = ref<CapabilityAuditSummary | null>(null)
const runs = ref<CapabilityAuditRun[]>([])
const lastRun = ref<CapabilityAuditRun | null>(null)
const selectedRow = ref<CapabilityAuditRow | null>(null)
const runDetail = ref<CapabilityAuditRunDetail | null>(null)
const days = ref(30)
const maxItems = ref(60)
const activeOnly = ref(true)
const loading = ref(false)
const running = ref(false)
const previewing = ref(false)
const errorMessage = ref('')
const pendingEstimate = ref<CapabilityAuditEstimate | null>(null)
let pendingProbeRequest: CapabilityAuditProbeRequest | null = null
let pollTimer: ReturnType<typeof setTimeout> | undefined
let pollDeadline = 0
let pollVersion = 0

const behaviorRuns = ref<BehaviorAuditRun[]>([])
const behaviorDetail = ref<BehaviorAuditDetail | null>(null)
const behaviorModel = ref('')
const behaviorCaseCount = ref(3)
const behaviorEffort = ref<BehaviorEffort>('low')
const behaviorLoading = ref(false)
const behaviorRunning = ref(false)
const behaviorPreviewing = ref(false)
const behaviorErrorMessage = ref('')
const pendingBehaviorEstimate = ref<BehaviorAuditEstimate | null>(null)
let pendingBehaviorRequest: BehaviorAuditRequest | null = null
let behaviorPollTimer: ReturnType<typeof setTimeout> | undefined
let behaviorPollDeadline = 0
let behaviorPollVersion = 0

const rows = computed(() => summary.value?.rows ?? [])
const protocolRows = computed<CapabilityProtocolRow[]>(() => {
  const result: CapabilityProtocolRow[] = []
  const protocolOrder: Record<string, number> = {
    chat_completions: 1,
    responses: 2,
    responses_tools: 3,
  }

  for (const row of rows.value) {
    if (row.passive_requests > 0) {
      result.push({ key: `${row.key}:passive`, row, protocol: null, evidence: null })
    }

    const evidenceEntries = Object.entries(row.protocols ?? {}).sort(([left], [right]) => {
      const leftOrder = protocolOrder[left] ?? 99
      const rightOrder = protocolOrder[right] ?? 99
      return leftOrder === rightOrder ? left.localeCompare(right) : leftOrder - rightOrder
    })
    for (const [protocol, evidence] of evidenceEntries) {
      result.push({ key: `${row.key}:protocol:${protocol}`, row, protocol, evidence })
    }

    if (row.passive_requests === 0 && evidenceEntries.length === 0) {
      result.push({ key: `${row.key}:unobserved`, row, protocol: null, evidence: null })
    }
  }
  return result
})
const latestBehaviorRun = computed(() => behaviorRuns.value[0] ?? null)
const summaryCards = computed(() => [
  { label: '审计模型', value: summary.value?.total_models ?? 0, color: 'text-gray-900 dark:text-white' },
  { label: '能力已验证', value: summary.value?.capability_verified ?? 0, color: 'text-emerald-600' },
  { label: '自报一致（弱证据）', value: summary.value?.self_report_consistent ?? 0, color: 'text-sky-600' },
  { label: '疑似替换', value: summary.value?.mismatch_suspected ?? 0, color: 'text-amber-600' },
  { label: '未验证', value: summary.value?.unverified ?? 0, color: 'text-gray-500' },
])
function formatTime(value?: string | null): string { if (!value) return '-'; const date = new Date(value); return Number.isNaN(date.getTime()) ? value : date.toLocaleString() }
function protocolLabel(value: string): string { return value === 'chat_completions' ? 'Chat Completions' : value === 'responses_tools' ? 'Responses + Tools' : value === 'responses' ? 'Responses' : value }
function protocolStatus(value: string): string { return value === 'verified' ? '通过' : value === 'model_mismatch' ? '偏差' : value === 'unsupported' ? '不支持' : value === 'needs_retest' ? '结论不足' : value === 'timeout' ? '超时' : value === 'error' ? '异常' : '待复测' }
function statusClass(value: string): string { return value === 'verified' ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300' : value === 'model_mismatch' ? 'bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300' : value === 'unsupported' || value === 'needs_retest' ? 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300' : 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300' }
function statusTextClass(value: string): string { return value === 'verified' ? 'text-emerald-600' : value === 'model_mismatch' ? 'text-amber-600' : value === 'unsupported' || value === 'needs_retest' ? 'text-gray-500' : 'text-red-600' }
function capabilityLabel(value: string): string { return value === 'verified' ? '能力支持' : value === 'observed' ? '真实请求已完成' : value === 'unsupported' ? '能力不支持' : '能力未验证' }
function capabilityClass(value: string): string { return value === 'verified' || value === 'observed' ? 'text-emerald-600' : value === 'unsupported' ? 'text-gray-500' : 'text-amber-600' }
function identityLabel(value: string): string { const normalized = normalizeCapabilityIdentityStatus(value); return normalized === 'consistent' || normalized === 'self_report_consistent' ? '目录与自报一致，身份未验真' : normalized === 'mismatch_suspected' ? '疑似模型替换' : normalized === 'evidence_only' ? '仅有能力证据' : '未验证' }
function identityTextClass(value: string): string { const normalized = normalizeCapabilityIdentityStatus(value); return normalized === 'consistent' || normalized === 'self_report_consistent' ? 'text-blue-600' : normalized === 'mismatch_suspected' ? 'text-amber-600' : 'text-gray-600' }
function confidenceLabel(value: string): string { return value === 'high' ? '较强异常信号' : value === 'medium' ? '弱一致性证据' : '证据不足' }
function estimateDuration(seconds: number): string { if (!seconds) return '< 1 分钟'; const minutes = Math.max(1, Math.ceil(seconds / 60)); return minutes >= 60 ? `${Math.floor(minutes / 60)} 小时 ${minutes % 60} 分` : `${minutes} 分钟` }
function runStatusLabel(value: string): string { return value === 'completed' ? '已完成' : value === 'partial' ? '部分完成' : value === 'running' ? '运行中' : value === 'failed' ? '失败' : value }
function runStatusClass(value: string): string { return value === 'completed' ? 'bg-emerald-100 text-emerald-700' : value === 'partial' ? 'bg-amber-100 text-amber-700' : value === 'running' ? 'bg-blue-100 text-blue-700' : 'bg-red-100 text-red-700' }
function evidenceSourceLabel(value?: string): string { return value === 'active_probe' ? '主动受控探测' : value === 'paired_behavior' ? '成对行为审计' : value === 'passive_traffic' || value === 'usage_log' ? '被动真实流量' : value ? value : '被动真实流量' }
function evidenceAttempts(evidence: CapabilityAuditEvidence): number { return evidence.attempt_count === undefined ? 1 : Math.max(0, evidence.attempt_count) }
function evidenceSuccesses(evidence: CapabilityAuditEvidence): number { return evidence.success_count === undefined ? (evidence.status === 'verified' || evidence.status === 'model_mismatch' ? 1 : 0) : Math.max(0, evidence.success_count) }
function evidenceFailures(evidence: CapabilityAuditEvidence): number { return evidence.failure_count === undefined ? Math.max(0, evidenceAttempts(evidence) - evidenceSuccesses(evidence)) : Math.max(0, evidence.failure_count) }
function evidenceResponseModel(item: CapabilityProtocolRow): string { return item.evidence ? (item.evidence.response_model || item.evidence.upstream_model || '-') : (item.row.passive_response_models?.join(', ') || '-') }
function evidenceTime(item: CapabilityProtocolRow): string | null | undefined { return item.evidence?.checked_at || item.row.passive_last_seen }
function itemCapabilityStatus(item: CapabilityProtocolRow): string { return item.evidence?.capability_status || (item.evidence ? item.row.capability_status : passiveCapabilityStatus(item.row.passive_requests)) }
function itemIdentityStatus(item: CapabilityProtocolRow): string { return item.evidence ? normalizeCapabilityIdentityStatus(item.evidence.identity_status) : passiveIdentityStatus(item.row.passive_requests, item.row.passive_mismatches, item.row.passive_response_models) }
function itemConfidence(item: CapabilityProtocolRow): string { return item.evidence?.confidence || (item.evidence ? item.row.confidence : passiveConfidence(item.row.passive_requests, item.row.passive_mismatches, item.row.passive_response_models)) }
function behaviorStatusLabel(value: string): string { return value === 'matched' ? '一致' : value === 'diverged' ? '偏离' : value === 'inconclusive' ? '待复核' : value === 'error' ? '异常' : value }
function behaviorStatusClass(value: string): string { return value === 'matched' ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300' : value === 'diverged' ? 'bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300' : value === 'inconclusive' ? 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300' : 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300' }
function behaviorObservationLabel(value: string): string { return value === 'correct' ? '正确' : value === 'incorrect' ? '错误答案' : value === 'inconclusive' ? '结论不足' : value === 'invalid' ? '工具结构无效' : value === 'timeout' ? '超时' : value === 'error' ? '异常' : value || '未执行' }
function shortHash(value?: string): string { return value ? value.slice(0, 16) : '-' }
function isRunActive(run?: CapabilityAuditRun | null): boolean { return run?.status === 'running' }
function isBehaviorRunActive(run?: BehaviorAuditRun | null): boolean { return run?.status === 'running' }
function stopPolling() { if (pollTimer) clearTimeout(pollTimer); pollTimer = undefined; pollDeadline = 0; pollVersion++ }
function stopBehaviorPolling() { if (behaviorPollTimer) clearTimeout(behaviorPollTimer); behaviorPollTimer = undefined; behaviorPollDeadline = 0; behaviorPollVersion++ }
function upsertProbeRun(run: CapabilityAuditRun) {
  const index = runs.value.findIndex((item) => item.id === run.id)
  if (index >= 0) runs.value.splice(index, 1, run)
  else runs.value.push(run)
  runs.value.sort((left, right) => right.id - left.id)
  lastRun.value = runs.value[0] ?? null
}
function upsertBehaviorRun(run: BehaviorAuditRun) {
  const index = behaviorRuns.value.findIndex((item) => item.id === run.id)
  if (index >= 0) behaviorRuns.value.splice(index, 1, run)
  else behaviorRuns.value.push(run)
  behaviorRuns.value.sort((left, right) => right.id - left.id)
}
async function loadAuditData(showLoading = true) {
  if (showLoading) loading.value = true
  errorMessage.value = ''
  const [summaryResult, runsResult] = await Promise.allSettled([
    adminAPI.modelCapabilityAudit.getSummary(days.value),
    adminAPI.modelCapabilityAudit.listRuns(),
  ])
  const failures: string[] = []
  if (summaryResult.status === 'fulfilled') summary.value = summaryResult.value
  else failures.push(extractApiErrorMessage(summaryResult.reason))
  if (runsResult.status === 'fulfilled') {
    runs.value = runsResult.value
    lastRun.value = runsResult.value[0] ?? null
    running.value = runsResult.value.some((item) => isRunActive(item))
  } else failures.push(extractApiErrorMessage(runsResult.reason))
  errorMessage.value = [...new Set(failures)].join('；')
  if (showLoading) loading.value = false
}
async function refreshAll() {
  await loadAuditData(true)
  const activeRun = runs.value.find((item) => isRunActive(item))
  if (activeRun) startPolling(activeRun.id)
  else { stopPolling(); running.value = false }
}
async function pollRun(runID: number, version: number) {
  if (version !== pollVersion) return
  try {
    const detail = await adminAPI.modelCapabilityAudit.getRun(runID)
    if (version !== pollVersion) return
    upsertProbeRun(detail.run)
    if (runDetail.value?.run.id === runID) runDetail.value = detail
    running.value = isRunActive(detail.run)
    if (!running.value) {
      stopPolling()
      const stoppedVersion = pollVersion
      await loadAuditData(false)
      if (stoppedVersion !== pollVersion) return
      const nextActiveRun = runs.value.find((item) => isRunActive(item))
      if (nextActiveRun) startPolling(nextActiveRun.id)
      return
    }
    errorMessage.value = ''
  } catch (error) {
    if (version !== pollVersion) return
    errorMessage.value = extractApiErrorMessage(error)
  }
  if (Date.now() >= pollDeadline) { stopPolling(); running.value = false; errorMessage.value = '后台探测仍在运行，请稍后刷新记录。'; return }
  pollTimer = setTimeout(() => void pollRun(runID, version), 2500)
}
function startPolling(runID: number) {
  stopPolling()
  pollDeadline = Date.now() + 80 * 60 * 1000
  running.value = true
  const version = pollVersion
  pollTimer = setTimeout(() => void pollRun(runID, version), 800)
}
function currentProbeRequest(): CapabilityAuditProbeRequest { return { active_only: activeOnly.value, max_items: Math.max(1, Math.min(240, Number(maxItems.value) || 60)) } }
async function prepareBatchProbe() {
  previewing.value = true
  errorMessage.value = ''
  pendingProbeRequest = currentProbeRequest()
  try {
    const estimate = await adminAPI.modelCapabilityAudit.estimateProbes(pendingProbeRequest)
    if (estimate.target_count === 0) { errorMessage.value = '没有符合条件的启用账号模型可供主动探测。'; pendingProbeRequest = null; return }
    pendingEstimate.value = estimate
  } catch (error) { errorMessage.value = extractApiErrorMessage(error); pendingProbeRequest = null }
  finally { previewing.value = false }
}
async function confirmBatchProbe() {
  const request = pendingProbeRequest ?? currentProbeRequest()
  pendingEstimate.value = null
  pendingProbeRequest = null
  running.value = true
  errorMessage.value = ''
  try {
    const started = await adminAPI.modelCapabilityAudit.runProbes(request)
    upsertProbeRun(started)
    if (isRunActive(started)) startPolling(started.id)
    else { running.value = false; await loadAuditData(false) }
  } catch (error) {
    running.value = false
    errorMessage.value = extractApiErrorMessage(error)
  }
}
async function openRun(id: number) { try { runDetail.value = await adminAPI.modelCapabilityAudit.getRun(id) } catch (error) { errorMessage.value = extractApiErrorMessage(error) } }

async function loadBehaviorData(showLoading = true) {
  if (showLoading) behaviorLoading.value = true
  behaviorErrorMessage.value = ''
  try {
    behaviorRuns.value = await adminAPI.modelCapabilityAudit.listBehaviorRuns()
    behaviorRunning.value = behaviorRuns.value.some((item) => isBehaviorRunActive(item))
  } catch (error) {
    behaviorErrorMessage.value = extractApiErrorMessage(error)
  } finally {
    if (showLoading) behaviorLoading.value = false
  }
}
async function refreshBehavior() {
  await loadBehaviorData(true)
  const selectedID = behaviorDetail.value?.run.id
  const detailID = selectedID && behaviorRuns.value.some((item) => item.id === selectedID)
    ? selectedID
    : behaviorRuns.value[0]?.id
  if (detailID) await openBehaviorRun(detailID)
  const activeRun = behaviorRuns.value.find((item) => isBehaviorRunActive(item))
  if (activeRun) startBehaviorPolling(activeRun.id)
  else { stopBehaviorPolling(); behaviorRunning.value = false }
}
async function pollBehaviorRun(runID: number, version: number) {
  if (version !== behaviorPollVersion) return
  try {
    const detail = await adminAPI.modelCapabilityAudit.getBehaviorRun(runID)
    if (version !== behaviorPollVersion) return
    upsertBehaviorRun(detail.run)
    if (!behaviorDetail.value || behaviorDetail.value.run.id === runID) behaviorDetail.value = detail
    behaviorRunning.value = isBehaviorRunActive(detail.run)
    if (!behaviorRunning.value) {
      stopBehaviorPolling()
      const stoppedVersion = behaviorPollVersion
      await loadBehaviorData(false)
      if (stoppedVersion !== behaviorPollVersion) return
      const nextActiveRun = behaviorRuns.value.find((item) => isBehaviorRunActive(item))
      if (nextActiveRun) startBehaviorPolling(nextActiveRun.id)
      return
    }
    behaviorErrorMessage.value = ''
  } catch (error) {
    if (version !== behaviorPollVersion) return
    behaviorErrorMessage.value = extractApiErrorMessage(error)
  }
  if (Date.now() >= behaviorPollDeadline) {
    stopBehaviorPolling()
    behaviorRunning.value = false
    behaviorErrorMessage.value = '后台行为审计仍在运行，请稍后刷新记录。'
    return
  }
  behaviorPollTimer = setTimeout(() => void pollBehaviorRun(runID, version), 2500)
}
function startBehaviorPolling(runID: number) {
  stopBehaviorPolling()
  behaviorPollDeadline = Date.now() + 80 * 60 * 1000
  behaviorRunning.value = true
  const version = behaviorPollVersion
  behaviorPollTimer = setTimeout(() => void pollBehaviorRun(runID, version), 800)
}
function currentBehaviorRequest(): BehaviorAuditRequest {
  return {
    model_id: behaviorModel.value.trim(),
    case_count: Math.max(1, Math.min(5, Number(behaviorCaseCount.value) || 3)),
    reasoning_effort: behaviorEffort.value,
  }
}
async function prepareBehaviorAudit() {
  behaviorPreviewing.value = true
  behaviorErrorMessage.value = ''
  pendingBehaviorRequest = currentBehaviorRequest()
  try {
    const estimate = await adminAPI.modelCapabilityAudit.estimateBehavior(pendingBehaviorRequest)
    if (!estimate.can_run || estimate.pair_count === 0) {
      behaviorErrorMessage.value = estimate.reason || '没有可配对的第三方账号与官方 OpenAI 锚点。'
      pendingBehaviorRequest = null
      return
    }
    pendingBehaviorEstimate.value = estimate
  } catch (error) {
    behaviorErrorMessage.value = extractApiErrorMessage(error)
    pendingBehaviorRequest = null
  } finally {
    behaviorPreviewing.value = false
  }
}
async function confirmBehaviorAudit() {
  const request = pendingBehaviorRequest ?? currentBehaviorRequest()
  pendingBehaviorEstimate.value = null
  pendingBehaviorRequest = null
  behaviorRunning.value = true
  behaviorErrorMessage.value = ''
  try {
    const started = await adminAPI.modelCapabilityAudit.runBehavior(request)
    upsertBehaviorRun(started)
    behaviorDetail.value = { run: started, results: [] }
    if (isBehaviorRunActive(started)) startBehaviorPolling(started.id)
    else { behaviorRunning.value = false; await refreshBehavior() }
  } catch (error) {
    behaviorRunning.value = false
    behaviorErrorMessage.value = extractApiErrorMessage(error)
  }
}
async function openBehaviorRun(id: number) {
  try {
    behaviorDetail.value = await adminAPI.modelCapabilityAudit.getBehaviorRun(id)
  } catch (error) {
    behaviorErrorMessage.value = extractApiErrorMessage(error)
  }
}
function closeTransientPanels(event: KeyboardEvent) {
  if (event.key !== 'Escape') return
  pendingEstimate.value = null
  pendingProbeRequest = null
  pendingBehaviorEstimate.value = null
  pendingBehaviorRequest = null
  selectedRow.value = null
  runDetail.value = null
}
onMounted(() => {
  document.addEventListener('keydown', closeTransientPanels)
  void Promise.all([refreshAll(), refreshBehavior()]).then(() => {
    if (!behaviorModel.value && rows.value.length > 0) behaviorModel.value = rows.value[0].requested_model
  })
})
onBeforeUnmount(() => {
  stopPolling()
  stopBehaviorPolling()
  document.removeEventListener('keydown', closeTransientPanels)
})
</script>
