-- 专家 · 技能 · 连接器（docs/expert-center.md §5.1）：受管分类、精选、禁用策略。

-- 分类按 Tab 分组：app（专家与微应用）/ skill（技能与提示词）/ connection（连接器）。
-- 分类有自己的可用范围：不在范围内的人看不到这个分类下的能力；只能收窄，不能放宽资产自己的范围。
CREATE TABLE asset_categories (
  id          BIGINT       NOT NULL AUTO_INCREMENT,
  grp         VARCHAR(16)  NOT NULL,
  name        VARCHAR(32)  NOT NULL,
  sort        INT          NOT NULL DEFAULT 0,
  scope       TEXT         NOT NULL,
  created_at  DATETIME(3)  NOT NULL,
  updated_at  DATETIME(3)  NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_asset_categories_name (grp, name)
);

-- category 文本列保留（旧页面、导出文件在用），设了 category_id 时两者同步
ALTER TABLE assets
  ADD COLUMN category_id BIGINT     NULL,
  ADD COLUMN featured    TINYINT(1) NOT NULL DEFAULT 0;

-- 禁用策略：范围内的人即使在资产的可用范围里也用不了 asset_ids 里的能力
CREATE TABLE asset_blocks (
  id          BIGINT        NOT NULL AUTO_INCREMENT,
  scope       TEXT          NOT NULL,
  -- JSON 数组：被禁用的资产 id
  asset_ids   TEXT          NOT NULL,
  note        VARCHAR(500)  NOT NULL DEFAULT '',
  updated_by  BIGINT        NULL,
  created_at  DATETIME(3)   NOT NULL,
  updated_at  DATETIME(3)   NOT NULL,
  PRIMARY KEY (id)
);

-- 已有的分类文本归并成受管分类，范围默认全公司
INSERT INTO asset_categories (grp, name, sort, scope, created_at, updated_at)
SELECT DISTINCT
  CASE kind WHEN 'connection' THEN 'connection' WHEN 'app' THEN 'app' ELSE 'skill' END,
  category, 0, '{"all":true,"departments":[],"roles":[],"users":[]}', UTC_TIMESTAMP(3), UTC_TIMESTAMP(3)
FROM assets WHERE category <> '';

UPDATE assets a JOIN asset_categories c
  ON c.name = a.category
 AND c.grp = CASE a.kind WHEN 'connection' THEN 'connection' WHEN 'app' THEN 'app' ELSE 'skill' END
SET a.category_id = c.id
WHERE a.category <> '';

-- 已有部署：在「待审核」后面挂上「专家与连接器策略」页。全新部署由 rbac.DefaultMenus 写入，下面不生效。
INSERT INTO menus (parent_id, kind, title, path, page_key, icon, permission, sort, visible, created_at, updated_at)
SELECT c.parent_id, 'page', '专家与连接器策略', '/admin/expert-center', 'admin-expert-center', 'GraduationCap', 'plugins.manage', c.sort + 5, 1, UTC_TIMESTAMP(3), UTC_TIMESTAMP(3)
FROM menus c
WHERE c.page_key = 'admin-review'
  AND NOT EXISTS (SELECT 1 FROM menus WHERE page_key = 'admin-expert-center')
ORDER BY c.id LIMIT 1;
