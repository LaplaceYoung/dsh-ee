# 介绍短片的 MP4 导出

30 秒介绍短片只有一份代码：`web/src/components/intro-film/`（`film.tsx` 是画面，`copy.ts` 是中英文文字）。
整部片子是时间的纯函数，演示站首页直接用它播放；这里用 [Remotion](https://www.remotion.dev/) 把同一份代码逐帧渲染成 MP4，发社交媒体用。不进 dsh-ee 的构建。

改分镜、改文字都去改 `web/src/components/intro-film/`，首页和 MP4 一起变。人名、数字和 [示例场景](../docs/demo-company.md) 保持一致；
第 2 幕的 DSH 对话是动画，画面上标了「示意画面」。

## 渲染

```sh
cd promo
pnpm install
pnpm studio    # 浏览器里预览、拖时间轴
pnpm render    # 输出 out/intro-zh.mp4、out/intro-en.mp4（1920×1080，30 帧，H.264，30 秒，无声）
```

成品在 `out/`，不进 git。Remotion 第一次渲染会下载 Chrome Headless Shell；本机已有的话加 `--browser-executable=<路径>`。

## 转 GIF（README 第一屏）

Remotion 自带的 ffmpeg 是精简版，没有调色板滤镜，要用完整的 ffmpeg：

```sh
ffmpeg -i out/intro-zh.mp4 -vf "fps=8,scale=720:-1:flags=lanczos,split[a][b];[a]palettegen=max_colors=128:stats_mode=full[p];[b][p]paletteuse=dither=none:diff_mode=rectangle" ../docs/images/intro.zh-CN.gif
ffmpeg -i out/intro-en.mp4 -vf "（同上）" ../docs/images/intro.gif
```

## 一分钟产品短片（OpenMotion 模板）

首页现在播的是另一部片子：用 [OpenMotion](https://github.com/nexu-io/openmotion) 的模板「Glean Agents 发布」改的一分钟产品短片，英文、中文、日文三版。
文字、图标、三种语言都在 `openmotion/build-film.mjs` 的 `COPY` 里，改完重新生成：

```sh
node promo/openmotion/build-film.mjs   # 需要本机有 OpenMotion 源码，默认 ~/openmotion，可用 OPENMOTION_DIR 指定
```

- `web/src/components/product-film/generated/`：首页用代码播放的那一份（进 git，首页直接 import）。
- `out/dsh-ee-openmotion-film-<lang>/`：OpenMotion（Hyperframes）工程，可以在 Studio 里打开，或者渲染成 MP4：
  `hyperframes render out/dsh-ee-openmotion-film-zh --format png-sequence -o frames`，再用 ffmpeg 把帧编码成 H.264（1280×720，30 帧，61 秒，无声）。

上面那部 30 秒短片（`web/src/components/intro-film/`）还在，`pnpm render` 照旧能出 MP4，只是首页不再播它。
