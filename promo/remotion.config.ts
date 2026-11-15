import path from "node:path";
import { Config } from "@remotion/cli/config";

// 短片的源码在 ../web/src 下，打包时会从 web/node_modules 找到另一份 React；统一指到这里的这一份。
Config.overrideWebpackConfig((config) => ({
  ...config,
  resolve: {
    ...config.resolve,
    alias: { ...(config.resolve?.alias ?? {}), react: path.resolve(process.cwd(), "node_modules/react") },
  },
}));
Config.setVideoImageFormat("png");
