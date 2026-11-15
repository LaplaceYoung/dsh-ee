-- 用户语言偏好：规范短码 zh / en / es / pt / hi。
--
-- '' = 跟随组织默认语言（settings 里 app 行的 defaultLanguage，默认 zh），
-- 这样升级前已有账号不需要回填，也不会把某个人固定成中文。
-- 写入口径在 API：PUT /api/me/preferences 会先归一（zh-CN / zh_CN → zh）再保存。
ALTER TABLE users
  ADD COLUMN language VARCHAR(8) NOT NULL DEFAULT '';
