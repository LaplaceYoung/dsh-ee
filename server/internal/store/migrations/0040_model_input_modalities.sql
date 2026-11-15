-- 模型输入模态声明：该模型接受哪些请求输入模态（text / image / file，逗号分隔）。
-- text 始终存在（默认值）；image 对应 OpenAI chat completions 的 image_url / file 图片块，
-- file 对应 Files API 的文档引用（file 块 / document 块）。
-- 这些值只用于 /v1/models、/v1/capabilities、发现文档与 dsh-connect 生成配置的能力公布；
-- 网关仍原样透传请求，不按声明校验/转换内容或做服务端工具逻辑。
ALTER TABLE models
  ADD COLUMN input_modalities VARCHAR(128) NOT NULL DEFAULT 'text';
