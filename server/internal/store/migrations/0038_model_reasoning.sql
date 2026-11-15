-- 思考模式能力声明：模型是否支持思考模式，以及可用的思考强度列表。
-- 这两列只用于 /v1/models、/v1/capabilities 与发现文档的能力公布；
-- 网关对 thinking / reasoning_effort 始终原样透传，不按这里的配置校验请求。
ALTER TABLE models
  ADD COLUMN supports_reasoning TINYINT(1) NOT NULL DEFAULT 0,
  -- 逗号分隔的 reasoning_effort 值（如 low,high,max）；空串表示按上游/模型默认。
  ADD COLUMN reasoning_efforts VARCHAR(128) NOT NULL DEFAULT '';
