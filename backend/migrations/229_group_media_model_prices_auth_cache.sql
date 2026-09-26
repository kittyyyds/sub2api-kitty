-- Keep auth snapshots coherent when media model final prices are changed
-- outside the admin service (for example, by a controlled SQL backfill).
-- Migration 222 added image_model_prices to this trigger, but the video
-- column introduced in migration 217 was omitted from the comparison set.
CREATE OR REPLACE FUNCTION enqueue_group_auth_cache_invalidation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    target_group_id BIGINT;
BEGIN
    target_group_id := OLD.id;
    IF TG_OP = 'UPDATE'
       AND OLD.status IS NOT DISTINCT FROM NEW.status
       AND OLD.is_exclusive IS NOT DISTINCT FROM NEW.is_exclusive
       AND OLD.allow_image_generation IS NOT DISTINCT FROM NEW.allow_image_generation
       AND OLD.platform IS NOT DISTINCT FROM NEW.platform
       AND OLD.subscription_type IS NOT DISTINCT FROM NEW.subscription_type
       AND OLD.rate_multiplier IS NOT DISTINCT FROM NEW.rate_multiplier
       AND OLD.peak_rate_enabled IS NOT DISTINCT FROM NEW.peak_rate_enabled
       AND OLD.peak_start IS NOT DISTINCT FROM NEW.peak_start
       AND OLD.peak_end IS NOT DISTINCT FROM NEW.peak_end
       AND OLD.peak_rate_multiplier IS NOT DISTINCT FROM NEW.peak_rate_multiplier
       AND OLD.profit_control_enabled IS NOT DISTINCT FROM NEW.profit_control_enabled
       AND OLD.profit_min_margin IS NOT DISTINCT FROM NEW.profit_min_margin
       AND OLD.profit_safety_buffer IS NOT DISTINCT FROM NEW.profit_safety_buffer
       AND OLD.grok_prompt_enabled IS NOT DISTINCT FROM NEW.grok_prompt_enabled
       AND OLD.grok_prompt_mode IS NOT DISTINCT FROM NEW.grok_prompt_mode
       AND OLD.grok_prompt_content IS NOT DISTINCT FROM NEW.grok_prompt_content
       AND OLD.image_model_prices IS NOT DISTINCT FROM NEW.image_model_prices
       AND OLD.video_model_prices IS NOT DISTINCT FROM NEW.video_model_prices
       AND OLD.deleted_at IS NOT DISTINCT FROM NEW.deleted_at THEN
        RETURN NEW;
    END IF;

    INSERT INTO auth_cache_invalidation_outbox (cache_key)
    SELECT encode(sha256(convert_to(k.key, 'UTF8')), 'hex')
    FROM api_keys AS k
    WHERE k.group_id = target_group_id
      AND k.deleted_at IS NULL
      AND k.key <> '';
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;

COMMENT ON COLUMN groups.image_model_prices IS
    '按模型×分辨率设置客户最终图片单价（CNY/张）；未配置时回退 USD 单价与分组倍率';

COMMENT ON COLUMN groups.video_model_prices IS
    '按模型族×分辨率设置客户最终视频单价（CNY/秒）；未配置时回退 USD 单价与分组倍率';
