// 把首页那部介绍短片（web/src/components/intro-film）逐帧渲染成 MP4。画面只有一份代码，这里只负责把帧号换成秒、把 1280×720 放大到 1920×1080。
import { AbsoluteFill, Composition, useCurrentFrame, useVideoConfig } from "remotion";
import { FILM_HEIGHT, FILM_SECONDS, FILM_WIDTH, IntroFilm } from "../../web/src/components/intro-film/film";
import type { FilmLang } from "../../web/src/components/intro-film/copy";

const FPS = 30;
const SCALE = 1.5;

function Film({ lang }: { lang: FilmLang }) {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  return (
    <AbsoluteFill>
      <div style={{ width: FILM_WIDTH, height: FILM_HEIGHT, transform: `scale(${SCALE})`, transformOrigin: "0 0" }}>
        <IntroFilm t={frame / fps} lang={lang} />
      </div>
    </AbsoluteFill>
  );
}

export const Root = () => (
  <>
    {(["zh", "en"] as const).map((lang) => (
      <Composition key={lang} id={`intro-${lang}`} component={Film} durationInFrames={FILM_SECONDS * FPS} fps={FPS} width={FILM_WIDTH * SCALE} height={FILM_HEIGHT * SCALE} defaultProps={{ lang }} />
    ))}
  </>
);
