/** page_key → 页面组件。路由由后端菜单决定，这里只负责「这个 key 渲染哪个组件」。 */
import type { ComponentType } from "react";
import Workbench from "./workbench";
import Apps from "./apps";
import Market from "./market";
import Knowledge from "./knowledge";
import Build from "./build";
import Templates from "./templates";
import Materials from "./materials";
import { AppNew } from "./asset-editor";
import DshDownload from "./dsh-download";
import Setup from "./setup";
import Publish from "./publish";
import DevDocs from "./dev-docs";
import DevExamples from "./dev-examples";
import MyKeys from "./my-keys";
import AdminUsers from "./admin/users";
import AdminRoles from "./admin/roles";
import AdminMenus from "./admin/menus";
import AdminFields from "./admin/fields";
import AdminProviders from "./admin/providers";
import AdminQuotas from "./admin/quotas";
import AdminKeys from "./admin/keys";
import AdminDashboard from "./admin/dashboard";
import AdminStats from "./admin/overview";
import AdminAudit from "./admin/audit";
import AdminPlugins from "./admin/plugins";
import AdminConnections from "./admin/connections";
import AdminReview from "./admin/review";
import AdminRegistry from "./admin/registry";
import AdminUpstream from "./admin/upstream";
import AdminDistribution from "./admin/distribution";
import AdminSettings from "./admin/settings";
import AdminBrand from "./admin/brand";
import AdminAuth from "./admin/auth";
import AdminSigningKeys from "./admin/signing-keys";
import AdminExpertCenter from "./admin/expert-center";
import AdminDirectory from "./admin/directory";
import type { PageKey } from "../lib/page-meta";

export const PAGE_COMPONENTS: Record<PageKey, ComponentType> = {
  workbench: Workbench,
  apps: Apps,
  market: Market,
  knowledge: Knowledge,
  "app-new": AppNew,
  build: Build,
  templates: Templates,
  materials: Materials,
  "dsh-download": DshDownload,
  setup: Setup,
  publish: Publish,
  "dev-docs": DevDocs,
  "dev-examples": DevExamples,
  "my-keys": MyKeys,
  "admin-users": AdminUsers,
  "admin-roles": AdminRoles,
  "admin-menus": AdminMenus,
  "admin-fields": AdminFields,
  "admin-providers": AdminProviders,
  "admin-quotas": AdminQuotas,
  "admin-keys": AdminKeys,
  "admin-dashboard": AdminDashboard,
  "admin-stats": AdminStats,
  "admin-audit": AdminAudit,
  "admin-plugins": AdminPlugins,
  "admin-connections": AdminConnections,
  "admin-review": AdminReview,
  "admin-registry": AdminRegistry,
  "admin-upstream": AdminUpstream,
  "admin-distribution": AdminDistribution,
  "admin-brand": AdminBrand,
  "admin-auth": AdminAuth,
  "admin-signing-keys": AdminSigningKeys,
  "admin-expert-center": AdminExpertCenter,
  "admin-directory": AdminDirectory,
  "admin-settings": AdminSettings,
};
