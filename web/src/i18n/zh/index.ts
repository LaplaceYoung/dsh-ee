import common from "./common";
import config from "./config";
import infra from "./infra";
import ops from "./ops";
import llm_admin from "./llm-admin";
import menus_admin from "./menus-admin";
import access from "./access";
import landing from "./landing";
import distribution from "./distribution";
import market from "./market";
import portal from "./portal";
import keys from "./keys";
import auth from "./auth";
import registry from "./registry";
import audit from "./audit";
import hub from "./hub";
import tokens from "./tokens";
import onboarding from "./onboarding";
import getStarted from "./get-started";
import knowledge from "./knowledge";
import build from "./build";
import sso from "./sso";
import requests from "./requests";
import studio from "./studio";
import appGuide from "./app-guide";
import extensions from "./extensions";

/** 中文文案（源语言）。按页面分模块，这里合并成一棵树。 */
export default {
  ...common,
  ...config,
  ...infra,
  ...ops,
  ...llm_admin,
  ...menus_admin,
  ...access,
  ...landing,
  ...distribution,
  ...market,
  ...portal,
  ...keys,
  ...auth,
  ...registry,
  ...audit,
  ...hub,
  ...tokens,
  ...onboarding,
  ...knowledge,
  ...getStarted,
  ...build,
  ...sso,
  ...requests,
  ...studio,
  ...appGuide,
  ...extensions,
};
