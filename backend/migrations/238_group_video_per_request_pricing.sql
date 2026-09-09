-- TToken 视频计费从按秒调整为按生成次数；保留现有列和 JSON 结构，
-- 仅更新数据库注释，避免破坏已有分组配置。
COMMENT ON COLUMN groups.video_price_480p IS '480p 视频生成单次价格 (USD/次)，Grok 平台使用';
COMMENT ON COLUMN groups.video_price_720p IS '720p 视频生成单次价格 (USD/次)，Grok 平台使用';
COMMENT ON COLUMN groups.video_price_1080p IS '1080p 视频生成单次价格 (USD/次)，Grok 平台使用';
COMMENT ON COLUMN groups.video_model_prices IS '可选：按模型族×分辨率覆盖视频单次价格 (USD/次)。key 为规范模型族，value 为分辨率→单次价格映射；NULL/空表示回退到 video_price_* 列或内置默认价';
