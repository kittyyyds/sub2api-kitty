-- Tibo question for the administrator test catalog (scheduled_test_definitions).
-- Same prompt as the gateway's background Tibo probe (openAITiboProbePrompt):
-- gpt-6-astra knows who Tibo is, a degraded model usually does not. The answer
-- is compared as a True/False verdict (capability match mode "tibo"), so
-- decorated answers such as "True." or "**True**" still pass.
--
-- The definition is inserted disabled and is not added to any saved plan, so
-- this migration by itself never sends a request or pauses an account. An
-- administrator enables it and selects it in a test plan when wanted.
-- Idempotent: an existing 'tibo' definition (including an archived one) is
-- left untouched.
INSERT INTO scheduled_test_definitions
    (key, name, description, prompt, output_kind, enabled, sort_order, max_output_tokens)
SELECT
    'tibo',
    'Tibo 降智检测',
    '询问模型是否知道 OpenAI 的 Tibo，只允许回答 True 或 False；回答 True 通过，其他回答不通过；执行异常不算通过。建议在测试计划中选用 gpt-6-astra 并把思考强度设为 xhigh，与网关后台的 Tibo 探测一致。默认停用，启用后需在测试计划中手动勾选。',
    $tibo${"version":1,"samples":1,"required_passes":1,"families":[],"questions":[{"key":"tibo","prompt":"Tell me whether you know who Tibo is in OpenAI based on your knowledge without searching online. Only return True or  False.","expected":"True","match":"tibo"}]}$tibo$,
    'capability',
    false,
    COALESCE(MAX(sort_order), -1) + 1,
    4096
FROM scheduled_test_definitions
ON CONFLICT (key) DO NOTHING;
