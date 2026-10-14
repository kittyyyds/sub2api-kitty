<script setup lang="ts">
import { computed, shallowRef } from 'vue'
import { useI18n } from 'vue-i18n'

import CustomerCodeBlock from '@/components/customer-console/CustomerCodeBlock.vue'
import CustomerFeatureHeader from '@/components/customer-console/CustomerFeatureHeader.vue'
import Icon from '@/components/icons/Icon.vue'
import { kittySite } from '@/config/site'
import { useAppStore } from '@/stores/app'
import { sanitizeUrl } from '@/utils/url'

const { t } = useI18n()
const appStore = useAppStore()
const apiBaseUrl = computed(() =>
  (sanitizeUrl(appStore.apiBaseUrl) || kittySite.primaryUrl).replace(/\/+$/, '').replace(/\/v1$/i, '')
)
const compatibleApiBaseUrl = computed(() => `${apiBaseUrl.value}/v1`)
const endpointDescription = computed(() =>
  appStore.cachedPublicSettings?.api_base_url_description || t('native.docs.endpoints.description')
)
const additionalEndpoints = computed(() => {
  const configured = (appStore.cachedPublicSettings?.custom_endpoints || [])
    .map((endpoint) => ({ ...endpoint, endpoint: sanitizeUrl(endpoint.endpoint).replace(/\/+$/, '') }))
    .filter((endpoint) => endpoint.endpoint)
  return configured.length ? configured : [{
    name: t('native.docs.endpoints.domestic'),
    endpoint: kittySite.domesticUrl,
    description: '',
  }]
})

type GuideId = 'claude' | 'codex' | 'openai'
type SdkId = 'curl' | 'node' | 'python'
type SystemId = 'windows' | 'linux' | 'macos'

interface GuideTab {
  id: GuideId
  label: string
  note: string
}

interface OptionTab<T extends string> {
  id: T
  label: string
}

const guideTabs = computed<readonly GuideTab[]>(() => [
  { id: 'claude', label: 'Claude Code', note: t('native.docs.guideNote.claude') },
  { id: 'codex', label: 'Codex', note: t('native.docs.guideNote.codex') },
  { id: 'openai', label: 'OpenAI SDK', note: t('native.docs.guideNote.openai') }
])

const systemTabs: readonly OptionTab<SystemId>[] = [
  { id: 'windows', label: 'Windows' },
  { id: 'linux', label: 'Linux' },
  { id: 'macos', label: 'macOS' }
]

const sdkTabs: readonly OptionTab<SdkId>[] = [
  { id: 'curl', label: 'cURL' },
  { id: 'node', label: 'Node.js' },
  { id: 'python', label: 'Python' }
]

const activeGuide = shallowRef<GuideId>('claude')
const activeSystem = shallowRef<SystemId>('windows')
const activeSdk = shallowRef<SdkId>('curl')

const claudeCommands = computed<Readonly<Record<SystemId, string>>>(() => ({
  windows: [
    '# Windows PowerShell',
    '$env:ANTHROPIC_AUTH_TOKEN="sk-your-key"',
    `$env:ANTHROPIC_BASE_URL="${apiBaseUrl.value}"`,
    'claude'
  ].join('\n'),
  linux: [
    '# Linux / Bash',
    'export ANTHROPIC_AUTH_TOKEN="sk-your-key"',
    `export ANTHROPIC_BASE_URL="${apiBaseUrl.value}"`,
    'claude'
  ].join('\n'),
  macos: [
    '# macOS / zsh',
    'export ANTHROPIC_AUTH_TOKEN="sk-your-key"',
    `export ANTHROPIC_BASE_URL="${apiBaseUrl.value}"`,
    'claude'
  ].join('\n')
}))

const codexCommands = computed<Readonly<Record<SystemId, string>>>(() => ({
  windows: [
    '# Windows PowerShell',
    '$env:OPENAI_API_KEY="sk-your-key"',
    `$env:OPENAI_BASE_URL="${compatibleApiBaseUrl.value}"`,
    'codex'
  ].join('\n'),
  linux: [
    '# Linux / Bash',
    'export OPENAI_API_KEY="sk-your-key"',
    `export OPENAI_BASE_URL="${compatibleApiBaseUrl.value}"`,
    'codex'
  ].join('\n'),
  macos: [
    '# macOS / zsh',
    'export OPENAI_API_KEY="sk-your-key"',
    `export OPENAI_BASE_URL="${compatibleApiBaseUrl.value}"`,
    'codex'
  ].join('\n')
}))

const openAiExamples = computed<Readonly<Record<SdkId, string>>>(() => ({
  curl: [
    `curl ${compatibleApiBaseUrl.value}/chat/completions \\`,
    '  -H "Authorization: Bearer sk-your-key" \\',
    '  -H "Content-Type: application/json" \\',
    "  -d '{",
    '    "model": "GPT-5.5",',
    '    "messages": [',
    `      {"role": "user", "content": "${t('native.docs.examplePrompt')}"}`,
    '    ],',
    '    "stream": true',
    "  }'"
  ].join('\n'),
  node: [
    'import OpenAI from "openai";',
    '',
    'const client = new OpenAI({',
    '  apiKey: "sk-your-key",',
    `  baseURL: "${compatibleApiBaseUrl.value}",`,
    '});',
    '',
    'const response = await client.chat.completions.create({',
    '  model: "GPT-5.5",',
    '  messages: [{ role: "user", content: "Hello kitty" }],',
    '});',
    '',
    'console.log(response.choices[0].message.content);'
  ].join('\n'),
  python: [
    'from openai import OpenAI',
    '',
    'client = OpenAI(',
    '    api_key="sk-your-key",',
    `    base_url="${compatibleApiBaseUrl.value}"`,
    ')',
    '',
    'response = client.chat.completions.create(',
    '    model="GPT-5.5",',
    '    messages=[{"role": "user", "content": "Hello kitty"}]',
    ')',
    '',
    'print(response.choices[0].message.content)'
  ].join('\n')
}))

const providerConfig = computed(() => [
  '{',
  '  "providers": {',
  '    "kitty-claude": {',
  '      "type": "anthropic",',
  `      "base_url": "${apiBaseUrl.value}",`,
  '      "api_key": "sk-your-key"',
  '    },',
  '    "kitty-openai": {',
  '      "type": "openai",',
  `      "base_url": "${compatibleApiBaseUrl.value}",`,
  '      "api_key": "sk-your-key"',
  '    }',
  '  }',
  '}'
].join('\n'))

const currentGuide = computed(() =>
  guideTabs.value.find((guide) => guide.id === activeGuide.value) ?? guideTabs.value[0]
)

const currentCode = computed(() => {
  if (activeGuide.value === 'claude') return claudeCommands.value[activeSystem.value]
  if (activeGuide.value === 'codex') return codexCommands.value[activeSystem.value]
  return openAiExamples.value[activeSdk.value]
})

const currentCodeLabel = computed(() => {
  if (activeGuide.value === 'openai') {
    return 'OpenAI Compatible / ' + (sdkTabs.find((tab) => tab.id === activeSdk.value)?.label ?? '')
  }

  const system = systemTabs.find((tab) => tab.id === activeSystem.value)?.label ?? ''
  return currentGuide.value.label + ' / ' + system
})

const currentLanguage = computed(() => {
  if (activeGuide.value === 'openai') {
    if (activeSdk.value === 'node') return 'javascript'
    if (activeSdk.value === 'python') return 'python'
    return 'shell'
  }
  return activeSystem.value === 'windows' ? 'powershell' : 'shell'
})

const codeBlockKey = computed(
  () => activeGuide.value + ':' + activeSystem.value + ':' + activeSdk.value
)
</script>

<template>
  <div class="docs-panel">
    <CustomerFeatureHeader
      eyebrow="Operator manual / Quick start"
      :title="t('native.docs.title')"
      :description="t('native.docs.description')"
      icon="book"
      :metrics="[
        { value: apiBaseUrl, label: t('native.docs.metrics.claude') },
        { value: compatibleApiBaseUrl, label: t('native.docs.metrics.compatible') },
        { value: 'Bearer sk-your-key', label: t('native.docs.metrics.placeholder') }
      ]"
    />

    <section class="docs-panel__start" aria-labelledby="docs-start-title">
      <div class="docs-panel__section-heading">
        <span class="docs-panel__index">01</span>
        <div>
          <h2 id="docs-start-title">{{ t('native.docs.before.title') }}</h2>
          <p>{{ t('native.docs.before.description') }}</p>
        </div>
      </div>

      <ol class="start-steps">
        <li>
          <span>01</span>
          <strong>{{ t('native.docs.before.keyTitle') }}</strong>
          <p>{{ t('native.docs.before.keyBody') }}</p>
        </li>
        <li>
          <span>02</span>
          <strong>{{ t('native.docs.before.modelTitle') }}</strong>
          <p>{{ t('native.docs.before.modelBody') }}</p>
        </li>
        <li>
          <span>03</span>
          <strong>{{ t('native.docs.before.urlTitle') }}</strong>
          <p>{{ t('native.docs.before.urlBody') }}</p>
        </li>
        <li>
          <span>04</span>
          <strong>{{ t('native.docs.before.restartTitle') }}</strong>
          <p>{{ t('native.docs.before.restartBody') }}</p>
        </li>
      </ol>
    </section>

    <section class="docs-panel__endpoints" aria-labelledby="docs-endpoints-title">
      <div class="docs-panel__section-heading">
        <span class="docs-panel__index">02</span>
        <div>
          <h2 id="docs-endpoints-title">{{ t('native.docs.endpoints.title') }}</h2>
          <p>{{ endpointDescription }}</p>
        </div>
      </div>

      <div class="endpoint-grid">
        <CustomerCodeBlock
          :code="apiBaseUrl"
          label="Claude Code Base URL"
          language="url"
          compact
        />
        <CustomerCodeBlock
          :code="compatibleApiBaseUrl"
          label="Codex / OpenAI Compatible"
          language="url"
          compact
        />
        <CustomerCodeBlock
          v-for="endpoint in additionalEndpoints"
          :key="endpoint.endpoint"
          :code="endpoint.endpoint"
          :label="[endpoint.name || endpoint.endpoint, endpoint.description].filter(Boolean).join(' · ')"
          language="url"
          compact
        />
      </div>
    </section>

    <section class="docs-panel__workbench" aria-labelledby="docs-workbench-title">
      <div class="docs-panel__section-heading">
        <span class="docs-panel__index">03</span>
        <div>
          <h2 id="docs-workbench-title">{{ t('native.docs.workbench.title') }}</h2>
          <p>{{ t('native.docs.workbench.description') }}</p>
        </div>
      </div>

      <div class="guide-workbench">
        <div class="guide-workbench__rail" role="tablist" :aria-label="t('native.docs.workbench.toolAria')">
          <button
            v-for="guide in guideTabs"
            :id="'guide-tab-' + guide.id"
            :key="guide.id"
            :data-testid="'tool-' + guide.id"
            type="button"
            role="tab"
            :aria-selected="activeGuide === guide.id"
            :aria-controls="'guide-panel-' + guide.id"
            :class="{ 'is-active': activeGuide === guide.id }"
            @click="activeGuide = guide.id"
          >
            <span>{{ guide.label }}</span>
            <small>{{ guide.note }}</small>
          </button>
        </div>

        <div
          :id="'guide-panel-' + activeGuide"
          class="guide-workbench__panel"
          role="tabpanel"
          :aria-labelledby="'guide-tab-' + activeGuide"
        >
          <div class="guide-workbench__panel-head">
            <div>
              <span>{{ activeGuide === 'openai' ? 'SDK / REQUEST' : 'SYSTEM' }}</span>
              <strong>{{ currentGuide.label }}</strong>
            </div>

            <div
              v-if="activeGuide !== 'openai'"
              class="segmented-tabs"
              role="tablist"
              :aria-label="t('native.docs.workbench.osAria')"
            >
              <button
                v-for="system in systemTabs"
                :key="system.id"
                :data-testid="'system-' + system.id"
                type="button"
                role="tab"
                :aria-selected="activeSystem === system.id"
                :class="{ 'is-active': activeSystem === system.id }"
                @click="activeSystem = system.id"
              >
                {{ system.label }}
              </button>
            </div>

            <div v-else class="segmented-tabs" role="tablist" :aria-label="t('native.docs.workbench.sdkAria')">
              <button
                v-for="sdk in sdkTabs"
                :key="sdk.id"
                :data-testid="'sdk-' + sdk.id"
                type="button"
                role="tab"
                :aria-selected="activeSdk === sdk.id"
                :class="{ 'is-active': activeSdk === sdk.id }"
                @click="activeSdk = sdk.id"
              >
                {{ sdk.label }}
              </button>
            </div>
          </div>

          <CustomerCodeBlock
            :key="codeBlockKey"
            :code="currentCode"
            :label="currentCodeLabel"
            :language="currentLanguage"
          />

          <div class="guide-workbench__note">
            <Icon name="infoCircle" size="sm" :stroke-width="1.8" />
            <p v-if="activeGuide === 'claude'">
              {{ t('native.docs.workbench.claudeNote') }}
            </p>
            <p v-else-if="activeGuide === 'codex'">
              {{ t('native.docs.workbench.codexNote') }}
            </p>
            <p v-else>
              {{ t('native.docs.workbench.openaiNote') }}
            </p>
          </div>
        </div>
      </div>
    </section>

    <section class="docs-panel__request" aria-labelledby="docs-request-title">
      <div class="docs-panel__section-heading">
        <span class="docs-panel__index">04</span>
        <div>
          <h2 id="docs-request-title">{{ t('native.docs.request.title') }}</h2>
          <p>{{ t('native.docs.request.description') }}</p>
        </div>
      </div>

      <div class="request-table-wrap">
        <table class="request-table">
          <thead>
            <tr>
              <th>{{ t('native.docs.request.field') }}</th>
              <th>{{ t('native.docs.request.location') }}</th>
              <th>{{ t('native.docs.request.purpose') }}</th>
              <th>{{ t('native.docs.request.example') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr>
              <td><code>Authorization</code></td>
              <td>Header</td>
              <td>{{ t('native.docs.request.auth') }}</td>
              <td><code>Bearer sk-your-key</code></td>
            </tr>
            <tr>
              <td><code>Content-Type</code></td>
              <td>Header</td>
              <td>{{ t('native.docs.request.format') }}</td>
              <td><code>application/json</code></td>
            </tr>
            <tr>
              <td><code>model</code></td>
              <td>Body</td>
              <td>{{ t('native.docs.request.model') }}</td>
              <td><code>GPT-5.5</code></td>
            </tr>
            <tr>
              <td><code>messages</code></td>
              <td>Body</td>
              <td>{{ t('native.docs.request.context') }}</td>
              <td><code>[{"role":"user"}]</code></td>
            </tr>
            <tr>
              <td><code>stream</code></td>
              <td>Body</td>
              <td>{{ t('native.docs.request.stream') }}</td>
              <td><code>true</code></td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <section class="docs-panel__multi" aria-labelledby="docs-multi-title">
      <div class="docs-panel__section-heading">
        <span class="docs-panel__index">05</span>
        <div>
          <h2 id="docs-multi-title">{{ t('native.docs.multi.title') }}</h2>
          <p>{{ t('native.docs.multi.description') }}</p>
        </div>
      </div>

      <div class="multi-layout">
        <CustomerCodeBlock
          :code="providerConfig"
          label="Provider mapping"
          language="json"
        />
        <div class="multi-layout__rules">
          <div>
            <span>CLAUDE</span>
            <strong>{{ t('native.docs.multi.rootTitle') }}</strong>
            <code>{{ apiBaseUrl }}</code>
          </div>
          <div>
            <span>OPENAI</span>
            <strong>{{ t('native.docs.multi.compatibleTitle') }}</strong>
            <code>{{ compatibleApiBaseUrl }}</code>
          </div>
          <div>
            <span>KEY</span>
            <strong>{{ t('native.docs.multi.isolationTitle') }}</strong>
            <p>{{ t('native.docs.multi.isolationBody') }}</p>
          </div>
        </div>
      </div>
    </section>

    <section class="docs-panel__troubleshooting" aria-labelledby="docs-troubleshooting-title">
      <div class="docs-panel__section-heading">
        <span class="docs-panel__index">06</span>
        <div>
          <h2 id="docs-troubleshooting-title">{{ t('native.docs.troubleshooting.title') }}</h2>
          <p>{{ t('native.docs.troubleshooting.description') }}</p>
        </div>
      </div>

      <div class="troubleshooting-grid">
        <details open>
          <summary>
            <span><b>401</b> {{ t('native.docs.troubleshooting.unauthorized') }}</span>
            <Icon name="chevronDown" size="sm" />
          </summary>
          <div>
            <ol>
              <li>{{ t('native.docs.troubleshooting.unauthorized1') }}</li>
              <li>{{ t('native.docs.troubleshooting.unauthorized2') }}</li>
              <li>{{ t('native.docs.troubleshooting.unauthorized3') }}</li>
            </ol>
          </div>
        </details>

        <details>
          <summary>
            <span><b>404</b> {{ t('native.docs.troubleshooting.notFound') }}</span>
            <Icon name="chevronDown" size="sm" />
          </summary>
          <div>
            <ol>
              <li>{{ t('native.docs.troubleshooting.notFound1') }}</li>
              <li>{{ t('native.docs.troubleshooting.notFound2') }}</li>
              <li>{{ t('native.docs.troubleshooting.notFound3') }}</li>
            </ol>
          </div>
        </details>

        <details>
          <summary>
            <span><b>429</b> {{ t('native.docs.troubleshooting.rateLimit') }}</span>
            <Icon name="chevronDown" size="sm" />
          </summary>
          <div>
            <ol>
              <li>{{ t('native.docs.troubleshooting.rateLimit1') }}</li>
              <li>{{ t('native.docs.troubleshooting.rateLimit2') }}</li>
              <li>{{ t('native.docs.troubleshooting.rateLimit3') }}</li>
            </ol>
          </div>
        </details>

        <details>
          <summary>
            <span><b>TIMEOUT</b> {{ t('native.docs.troubleshooting.timeout') }}</span>
            <Icon name="chevronDown" size="sm" />
          </summary>
          <div>
            <ol>
              <li>{{ t('native.docs.troubleshooting.timeout1') }}</li>
              <li>{{ t('native.docs.troubleshooting.timeout2') }}</li>
              <li>{{ t('native.docs.troubleshooting.timeout3') }}</li>
            </ol>
          </div>
        </details>

        <details>
          <summary>
            <span><b>CONFIG</b> {{ t('native.docs.troubleshooting.config') }}</span>
            <Icon name="chevronDown" size="sm" />
          </summary>
          <div>
            <ol>
              <li>{{ t('native.docs.troubleshooting.config1') }}</li>
              <li>{{ t('native.docs.troubleshooting.config2') }}</li>
              <li>{{ t('native.docs.troubleshooting.config3') }}</li>
            </ol>
          </div>
        </details>

        <details>
          <summary>
            <span><b>COST</b> {{ t('native.docs.troubleshooting.cost') }}</span>
            <Icon name="chevronDown" size="sm" />
          </summary>
          <div>
            <ol>
              <li>{{ t('native.docs.troubleshooting.cost1') }}</li>
              <li>{{ t('native.docs.troubleshooting.cost2') }}</li>
              <li>{{ t('native.docs.troubleshooting.cost3') }}</li>
            </ol>
          </div>
        </details>
      </div>
    </section>
  </div>
</template>

<style scoped>
.docs-panel {
  width: 100%;
  min-width: 0;
  color: var(--customer-ink, #142126);
}

.docs-panel > * + * {
  margin-top: 22px;
}

.docs-panel__start,
.docs-panel__endpoints,
.docs-panel__workbench,
.docs-panel__request,
.docs-panel__multi,
.docs-panel__troubleshooting {
  padding-top: 22px;
  border-top: 1px solid var(--customer-line, rgba(20, 33, 38, 0.12));
}

.docs-panel__section-heading {
  display: flex;
  gap: 12px;
  align-items: flex-start;
  margin-bottom: 16px;
}

.docs-panel__section-heading h2 {
  margin: 0;
  color: var(--customer-ink, #142126);
  font-size: 19px;
  font-weight: 680;
  letter-spacing: 0;
  line-height: 1.35;
}

.docs-panel__section-heading p {
  margin: 4px 0 0;
  color: var(--customer-muted, rgba(20, 33, 38, 0.66));
  font-size: 13px;
  line-height: 1.65;
}

.docs-panel__index {
  display: inline-grid;
  flex: 0 0 30px;
  width: 30px;
  height: 24px;
  color: var(--customer-cyan, #0a7f90);
  font-family: var(--customer-mono, monospace);
  font-size: 10px;
  font-weight: 700;
  place-items: center;
  background: var(--customer-cyan-soft, rgba(10, 127, 144, 0.1));
  border-left: 2px solid var(--customer-cyan, #0a7f90);
}

.start-steps {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 10px;
  padding: 0;
  margin: 0;
  list-style: none;
}

.start-steps li {
  min-width: 0;
  padding: 16px;
  background: var(--customer-surface, rgba(248, 251, 252, 0.82));
  border: 1px solid var(--customer-line, rgba(20, 33, 38, 0.12));
  border-radius: 7px;
}

.start-steps li > span {
  display: block;
  color: var(--customer-cyan, #0a7f90);
  font-family: var(--customer-mono, monospace);
  font-size: 9px;
  font-weight: 700;
}

.start-steps li:nth-child(even) > span {
  color: var(--customer-copper, #9c641f);
}

.start-steps strong {
  display: block;
  margin-top: 7px;
  color: var(--customer-ink, #142126);
  font-size: 13px;
  font-weight: 680;
}

.start-steps p {
  margin: 5px 0 0;
  color: var(--customer-muted, rgba(20, 33, 38, 0.66));
  font-size: 11px;
  line-height: 1.6;
}

.endpoint-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.guide-workbench {
  display: grid;
  grid-template-columns: minmax(190px, 0.28fr) minmax(0, 1fr);
  overflow: hidden;
  background: var(--customer-surface, rgba(248, 251, 252, 0.82));
  border: 1px solid var(--customer-line, rgba(20, 33, 38, 0.12));
  border-radius: 8px;
  box-shadow: var(--customer-shadow, 0 14px 40px rgba(25, 47, 55, 0.1));
}

.guide-workbench__rail {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 12px;
  background:
    linear-gradient(155deg, var(--customer-cyan-soft, rgba(10, 127, 144, 0.1)), transparent 52%),
    var(--customer-surface-soft, rgba(228, 237, 240, 0.82));
  border-right: 1px solid var(--customer-line, rgba(20, 33, 38, 0.12));
}

.guide-workbench__rail button {
  position: relative;
  display: block;
  width: 100%;
  min-height: 58px;
  padding: 10px 12px;
  color: var(--customer-muted, rgba(20, 33, 38, 0.66));
  font: inherit;
  text-align: left;
  cursor: pointer;
  background: transparent;
  border: 1px solid transparent;
  border-radius: 6px;
}

.guide-workbench__rail button:hover {
  color: var(--customer-ink, #142126);
  background: var(--customer-cyan-soft, rgba(10, 127, 144, 0.1));
}

.guide-workbench__rail button.is-active {
  color: var(--customer-ink, #142126);
  background: var(--customer-surface-raised, #f8fbfc);
  border-color: var(--customer-line-strong, rgba(10, 127, 144, 0.36));
  box-shadow: inset 3px 0 0 var(--customer-cyan, #0a7f90);
}

.guide-workbench__rail button span,
.guide-workbench__rail button small {
  display: block;
}

.guide-workbench__rail button span {
  font-size: 12px;
  font-weight: 680;
}

.guide-workbench__rail button small {
  margin-top: 4px;
  color: var(--customer-faint, rgba(20, 33, 38, 0.48));
  font-size: 9px;
  line-height: 1.35;
}

.guide-workbench__panel {
  min-width: 0;
  padding: 18px;
}

.guide-workbench__panel-head {
  display: flex;
  gap: 16px;
  align-items: flex-end;
  justify-content: space-between;
  margin-bottom: 12px;
}

.guide-workbench__panel-head > div:first-child span,
.guide-workbench__panel-head > div:first-child strong {
  display: block;
}

.guide-workbench__panel-head > div:first-child span {
  color: var(--customer-copper, #9c641f);
  font-family: var(--customer-mono, monospace);
  font-size: 9px;
  font-weight: 700;
}

.guide-workbench__panel-head > div:first-child strong {
  margin-top: 3px;
  color: var(--customer-ink, #142126);
  font-size: 14px;
  font-weight: 680;
}

.segmented-tabs {
  display: inline-flex;
  max-width: 100%;
  padding: 3px;
  overflow-x: auto;
  background: var(--customer-surface-soft, rgba(228, 237, 240, 0.82));
  border: 1px solid var(--customer-line, rgba(20, 33, 38, 0.12));
  border-radius: 7px;
}

.segmented-tabs button {
  min-width: 68px;
  min-height: 30px;
  padding: 5px 9px;
  color: var(--customer-muted, rgba(20, 33, 38, 0.66));
  font: inherit;
  font-size: 10px;
  font-weight: 650;
  white-space: nowrap;
  cursor: pointer;
  background: transparent;
  border: 1px solid transparent;
  border-radius: 5px;
}

.segmented-tabs button:hover {
  color: var(--customer-cyan, #0a7f90);
}

.segmented-tabs button.is-active {
  color: var(--customer-cyan-ink, #f4fbfc);
  background: var(--customer-cyan, #0a7f90);
  border-color: var(--customer-cyan, #0a7f90);
  box-shadow: 0 4px 12px var(--customer-cyan-shadow, rgba(10, 127, 144, 0.16));
}

.guide-workbench button:focus-visible,
.troubleshooting-grid summary:focus-visible {
  outline: 2px solid var(--customer-cyan, #0a7f90);
  outline-offset: 2px;
}

.guide-workbench__note {
  display: flex;
  gap: 8px;
  align-items: flex-start;
  padding-top: 12px;
  color: var(--customer-muted, rgba(20, 33, 38, 0.66));
}

.guide-workbench__note svg {
  flex: 0 0 auto;
  color: var(--customer-cyan, #0a7f90);
}

.guide-workbench__note p {
  margin: 0;
  font-size: 10px;
  line-height: 1.55;
}

.guide-workbench__note code,
.request-table code,
.multi-layout__rules code,
.troubleshooting-grid code {
  color: var(--customer-copper, #9c641f);
  font-family: var(--customer-mono, monospace);
  font-size: 0.95em;
}

.request-table-wrap {
  max-width: 100%;
  overflow-x: auto;
  background: var(--customer-surface, rgba(248, 251, 252, 0.82));
  border: 1px solid var(--customer-line, rgba(20, 33, 38, 0.12));
  border-radius: 7px;
}

.request-table {
  width: 100%;
  min-width: 660px;
  border-collapse: collapse;
  font-size: 11px;
}

.request-table th,
.request-table td {
  padding: 12px 14px;
  color: var(--customer-muted, rgba(20, 33, 38, 0.66));
  text-align: left;
  border-bottom: 1px solid var(--customer-line, rgba(20, 33, 38, 0.12));
}

.request-table th {
  color: var(--customer-faint, rgba(20, 33, 38, 0.48));
  font-family: var(--customer-mono, monospace);
  font-size: 9px;
  font-weight: 700;
  text-transform: uppercase;
  background: var(--customer-surface-soft, rgba(228, 237, 240, 0.82));
}

.request-table tr:last-child td {
  border-bottom: 0;
}

.request-table td:first-child {
  color: var(--customer-ink, #142126);
  font-weight: 650;
}

.multi-layout {
  display: grid;
  grid-template-columns: minmax(0, 1.2fr) minmax(250px, 0.8fr);
  gap: 12px;
  align-items: stretch;
}

.multi-layout__rules {
  display: grid;
  grid-template-rows: repeat(3, minmax(0, 1fr));
  overflow: hidden;
  background: var(--customer-surface, rgba(248, 251, 252, 0.82));
  border: 1px solid var(--customer-line, rgba(20, 33, 38, 0.12));
  border-radius: 7px;
}

.multi-layout__rules > div {
  min-width: 0;
  padding: 13px 15px;
  border-top: 1px solid var(--customer-line, rgba(20, 33, 38, 0.12));
}

.multi-layout__rules > div:first-child {
  border-top: 0;
}

.multi-layout__rules span {
  color: var(--customer-cyan, #0a7f90);
  font-family: var(--customer-mono, monospace);
  font-size: 9px;
  font-weight: 700;
}

.multi-layout__rules strong {
  margin-left: 8px;
  color: var(--customer-ink, #142126);
  font-size: 11px;
}

.multi-layout__rules code,
.multi-layout__rules p {
  display: block;
  margin: 6px 0 0;
  overflow-wrap: anywhere;
  color: var(--customer-muted, rgba(20, 33, 38, 0.66));
  font-size: 10px;
  line-height: 1.55;
}

.troubleshooting-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
}

.troubleshooting-grid details {
  min-width: 0;
  overflow: hidden;
  background: var(--customer-surface, rgba(248, 251, 252, 0.82));
  border: 1px solid var(--customer-line, rgba(20, 33, 38, 0.12));
  border-radius: 7px;
}

.troubleshooting-grid details[open] {
  border-color: var(--customer-line-strong, rgba(10, 127, 144, 0.36));
}

.troubleshooting-grid summary {
  display: flex;
  min-height: 48px;
  gap: 12px;
  align-items: center;
  justify-content: space-between;
  padding: 11px 13px;
  color: var(--customer-ink, #142126);
  font-size: 11px;
  font-weight: 650;
  cursor: pointer;
  list-style: none;
  background: var(--customer-surface-soft, rgba(228, 237, 240, 0.82));
}

.troubleshooting-grid summary::-webkit-details-marker {
  display: none;
}

.troubleshooting-grid summary b {
  display: inline-block;
  min-width: 56px;
  margin-right: 7px;
  color: var(--customer-copper, #9c641f);
  font-family: var(--customer-mono, monospace);
  font-size: 9px;
}

.troubleshooting-grid summary svg {
  flex: 0 0 auto;
  color: var(--customer-cyan, #0a7f90);
  transition: transform 160ms ease;
}

.troubleshooting-grid details[open] summary svg {
  transform: rotate(180deg);
}

.troubleshooting-grid details > div {
  padding: 12px 14px;
  border-top: 1px solid var(--customer-line, rgba(20, 33, 38, 0.12));
}

.troubleshooting-grid ol {
  padding-left: 18px;
  margin: 0;
}

.troubleshooting-grid li {
  color: var(--customer-muted, rgba(20, 33, 38, 0.66));
  font-size: 10px;
  line-height: 1.65;
}

.troubleshooting-grid li + li {
  margin-top: 4px;
}

@media (max-width: 960px) {
  .start-steps {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .guide-workbench {
    grid-template-columns: 1fr;
  }

  .guide-workbench__rail {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    border-right: 0;
    border-bottom: 1px solid var(--customer-line, rgba(20, 33, 38, 0.12));
  }

  .guide-workbench__rail button.is-active {
    box-shadow: inset 0 -3px 0 var(--customer-cyan, #0a7f90);
  }

  .multi-layout {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 700px) {
  .docs-panel > * + * {
    margin-top: 18px;
  }

  .endpoint-grid,
  .troubleshooting-grid {
    grid-template-columns: 1fr;
  }

  .guide-workbench__panel-head {
    align-items: flex-start;
    flex-direction: column;
  }

  .segmented-tabs {
    width: 100%;
  }

  .segmented-tabs button {
    flex: 1 0 auto;
  }
}

@media (max-width: 520px) {
  .start-steps {
    grid-template-columns: 1fr;
  }

  .guide-workbench__rail {
    grid-template-columns: 1fr;
  }

  .guide-workbench__rail button {
    min-height: 48px;
  }

  .guide-workbench__rail button.is-active {
    box-shadow: inset 3px 0 0 var(--customer-cyan, #0a7f90);
  }

  .guide-workbench__panel {
    padding: 14px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .troubleshooting-grid summary svg {
    transition: none;
  }
}
</style>
