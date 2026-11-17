// @ts-nocheck
// 由 promo/openmotion/build-film.mjs 生成，不要手改：改文字去改那个脚本里的 COPY，再重新生成。
// 整部片子是时间的纯函数：createFilm(root) 返回 draw(秒)，root 是放着画面的影子根。
export const FILM_SECONDS = 61;
export const FILM_WIDTH = 1280;
export const FILM_HEIGHT = 720;
export function createFilm(root) {
 const $=s=>root.querySelector(s), $$=s=>[...root.querySelectorAll(s)];
 const clamp=x=>Math.max(0,Math.min(1,x)), lerp=(a,b,x)=>a+(b-a)*x;
 const ease=x=>1-Math.pow(1-clamp(x),3), smooth=x=>{x=clamp(x);return x*x*(3-2*x)};
 const ramp=(t,a,b)=>smooth((t-a)/(b-a));
 const tr=(s,x=0,y=0,z=1,r=0)=>{let e=typeof s==='string'?$(s):s;if(e)e.style.transform=`translate(${x}px,${y}px) scale(${z}) rotate(${r}deg)`};
 const op=(s,a)=>{let e=typeof s==='string'?$(s):s;if(e)e.style.opacity=clamp(a)};
 const settings={};
 const scenes=$$('.scene');
 function draw(t){
  // Scene visibility is also represented by native clip timing; assigning explicitly
  // keeps the independent browser preview deterministic during reverse seeking.
  scenes.forEach(el=>{const a=+el.dataset.start,b=a+(+el.dataset.duration);el.style.opacity=t>=a&&t<b?1:0;el.style.pointerEvents='none'});
  if(t<3.7){
   let zoom=1+5*ramp(t,2.3,2.85);tr('#opening-text',0,0,1-.15*ramp(t,2.3,2.7));op('#opening-text',1-ramp(t,2.3,2.7));
   op('#opening-differently',ramp(t,.49,.68));op('#opening-ai',ramp(t,1.04,1.22));
   tr('#opening-ribbon',0,0,1+.1*Math.sin(t*2));op('#opening-ribbon',1-ramp(t,2.4,2.85));
   $$('[data-orbit]').forEach((e,i)=>{let rad=(i<3?220:400+(i%3)*90),a=t*(i%2?1:-1)*.9+i*2.19;
    let x=640+Math.cos(a)*rad*zoom,y=340+Math.sin(a)*rad*.7*zoom,size=i<3?150:55+(i%4)*35;
    e.style.width=size+'px';e.style.height=size+'px';tr(e,x-size/2,y-size/2,zoom,a*57);op(e,(i<3?1:ramp(t,.45,1.5))*(1-ramp(t,2.5,2.85)));e.style.filter=i>7?'blur(9px)':'none';
   });
   let a=ramp(t,2.42,2.78),shrink=ramp(t,3,3.7),s=lerp(1.5,1,a)*lerp(1,200/570,shrink);tr('#opening-logo',0,lerp(450,0,ease((t-2.35)/.5))+92*shrink,s,lerp(-40,0,ramp(t,2.6,3.08)));op('#opening-logo',ramp(t,2.32,2.45));op('#opening-logo svg > g',ramp(t,2.7,2.96));
  }
  if(t>=3.7&&t<7.8){let q=t-3.7,out=ramp(t,6.7,7.8);tr('#intro-logo',0,lerp(328,0,ease(q/1.05))+452*out,lerp(2.5,1,ease(q/1.05))+.1*out);
   tr('#intro-title',0,lerp(560,0,ease(q/1.1))+552*out);op('#intro-title',ramp(t,3.7,3.9));tr('#dashboard',0,lerp(600,0,ease((q-.5)/.9))+700*out);
   tr('#introducing .scene-content',0,0,1);op('#introducing .sky',1-out);$$('#introducing .cloud').forEach(e=>op(e,.5*(1-out)));
  }
  if(t>=7.8&&t<12.7){let q=t-7.8;let intro=ease(q/.8),end=ramp(t,12.15,12.7);
   tr('#network-logo',0,lerp(206,0,intro),1-.95*ramp(t,9.55,9.95));op('#network-logo',1-ramp(t,9.6,9.92));
   tr('#network-wedge',0,0,1,lerp(-45,20,clamp(q/4.9)));
   $$('#network .network-card').forEach((el,i)=>tr(el,(i%2?1:-1)*50*clamp((q-2)/2),lerp(410,0,ease((q-.15-i*.06)/.8))-end*50));
   op('#network-title',ramp(t,9.72,10.05));tr('#network-title',0,20*(1-ramp(t,9.72,10.05)));
  }
  if(t>=12.2&&t<493/30){let q=t-12.2;$$('#departments .department,#departments .avatar,#departments .dept-icon').forEach((e,i)=>{const x=parseFloat(e.style.left)||0,delay=x<590?0:x<810?.7:x<1040?1.3:1.9;let a=ease((q-delay-(i%3)*.08)/.65);op(e,a);tr(e,0,(1-a)*140)});}
  for(const id of ['daily','opportunities']){let start=id==='daily'?493/30:1313/30,end=id==='daily'?694/30:50.6;if(t<start||t>=end)continue;let local=t-start;
   let typing=id==='daily'?19.75:47.05,full=(id==='daily'?settings.dailyPrompt:settings.opportunityPrompt)||($(`#${id} .typed`).dataset.full||'').replace(/<br>/g,'\n'),typed=Math.floor(clamp((t-typing)/(id==='daily'?1.6:1.45))*full.length);
   const span=$(`#${id} .typed`);span.textContent=full.slice(0,typed);span.style.whiteSpace='pre-line';op(`#${id} .placeholder`,t<typing?1:0);$(`#${id} .placeholder`).style.display=t<typing?'inline':'none';span.style.display=t>=typing?'inline':'none';
   const submit=$(`#${id} .submit`);submit.style.background=t>typing-.3?'#4d6bfe':'transparent';submit.style.color=t>typing-.3?'white':'#aaa';submit.style.transform=`scale(${t>typing-.3?.78:1})`;
   let scale= id==='daily'?lerp(.93,1.03,clamp(local/6.7)):lerp(1.04,.9,clamp(local/6.8));tr(`#${id} .scene-content`,0,0,scale);
   $$('#'+id+' .icon-card').forEach((e,i)=>{tr(e,Math.sin(local*.6+i)*16,Math.cos(local*.5+i)*18,1,Math.sin(local*.6+i)*4);tr(e.querySelector('svg'),0,0,.96,Math.sin(local*1.2+i)*28)});
   let click=id==='daily'?22.7:49.9,move=ramp(t,click-.5,click);tr(`#${id}-cursor`,lerp(0,135,move),lerp(0,-13,move));op(`#${id}-cursor`,ramp(t,start+.4,start+.8));
   if(t>click)tr(submit,0,0,.78-.08*Math.sin(clamp((t-click)/.25)*Math.PI));
  }
  if(t>=694/30&&t<914/30){let q=t-694/30,expand=ramp(t,24.7,25.6);tr('#build-column',0,0,1+expand*2);op('#build-column',1-expand);
   tr('#build-title',0,-460*ramp(t,24.7,25.6));op('#build-title',1-ramp(t,25.1,25.6));
   let y=lerp(1100,350,ramp(t,24.65,25.6))-350*ramp(t,26.1,26.9),sc=lerp(1.8,1,ramp(t,26.1,26.9));
   y-=ramp(t,27.8,28.6)*326+ramp(t,28.9,29.7)*326+ramp(t,30,913/30)*1968;
   tr('#flow-stack',0,y,sc);op('#flow-stack',ramp(t,24.7,25.1));
   $$('#building .flow-card p').forEach((el,i)=>{const at=i<4?[25.55,26.7,28,29.05][i]:29.8+(i-4)*.06;op(el,ramp(t,at,at+(i<4?.4:.1)))});
  }
  if(t>=914/30&&t<34.65){let q=t-914/30;tr('#builder',0,lerp(-40,0,ease(q/.5)),lerp(1.2,1,ease(q/.5)));tr('#builder-flow',0,-lerp(70,250,clamp(q/3.93)));}
  if(t>=34.13&&t<37.7){let q=t-34.13;tr('#organization .scene-content',0,0,lerp(1.09,1,clamp(q/3)));}
  if(t>=37.2&&t<40.7){let q=t-37.2;tr('#knowledge-slash',lerp(100,420,clamp(q/3.15)),0,1);op('#knowledge-title',ramp(t,37.65,38.0));}
  if(t>=40.3&&t<1313/30){let v=ramp(t,42.1,42.85);tr('#world-icon',-20*v,0,1-.12*v);$('#world-icon').style.color=v>.5?'#aaa':'#4d6bfe';tr('#both',-60*v,0);op('#world-copy',1-v);op('#work-copy',v);op('#data-switch',v);op('#work-icon',v);tr('#data-switch',0,0,lerp(.35,1,v));}
  if(t>=50.2&&t<53.05){if(settings.opportunityPrompt)$('#report h2').textContent=settings.opportunityPrompt;$('#report h2').style.whiteSpace='pre-line';let v=ramp(t,50.2,50.6);tr('#report',0,lerp(231,0,v),1);$('#report').style.clipPath=v>=1?'none':`inset(0 0 ${702*(1-v)}px 0 round ${lerp(8,26,v)}px)`;$('#report').style.paddingLeft=lerp(28,70,v)+'px';$('#report').style.paddingTop=lerp(52,46,v)+'px';op('#answer .report-background',ramp(t,50.4,50.65)*(1-ramp(t,52.4,52.65)));$$('#report > :not(h2)').forEach(e=>op(e,ramp(t,50.45,50.9)));}
  if(t>=52.4&&t<55.5){let q=t-52.4;tr('#lock',0,0,lerp(.78,1,ease(q/.5)));tr('#pupil',Math.sin(q*2.3)*32,Math.cos(q*2)*12);let blink=1-.92*Math.exp(-Math.pow((q-2.25)/.13,2));tr('#governance .eye',0,0,1);$('#governance .eye').style.transform=`scaleY(${blink})`;op('#governance-copy',ramp(t,53,53.65));}
  if(t>=55.5&&t<59.12){let q=t-55.5;tr('#mosaic-cards',0,0,lerp(1.8,1,ease(q/2.5)));op('#mosaic-cards',ramp(t,55.5,55.7));tr('#mosaic-title',0,0,lerp(1,1.01,q/3));}
  if(t>=58.7){let q=t-58.7;tr('#outro-diamond',lerp(-850,80,ease(q/.65)),0,1);let reveal=ramp(t,58.9,59.55);$('#wordmark').style.clipPath=`inset(0 ${(1-reveal)*100}% 0 0)`;}

  const wipe=(id,a,b)=>{let v=ramp(t,a,b),x=1560*(1-v)-280;$(id).style.clipPath=`polygon(${x}px 0,1560px 0,1560px 720px,${x+280}px 720px)`;};
  if(t>=12.2&&t<493/30)wipe('#departments',12.2,12.72);
  if(t>=34.13&&t<37.7){let v=ease((t-34.13)/.52);tr('#builder',0,-800*v,1+.6*v);$('#organization').style.clipPath=`inset(${720*(1-v)}px 0 0 0)`;tr('#organization .scene-content',0,80*(1-v),1.04);}
  if(t>=37.2&&t<40.7)wipe('#knowledge',37.2,37.65);
  if(t>=40.3&&t<1313/30)wipe('#data',40.3,40.65);
  if(t>=50.2&&t<50.65){op('#answer',1);}
  if(t>=52.4&&t<53.05){let v=clamp((t-52.4)/.65);tr('#report',0,850*v*v,1);}
  if(t>=55.5&&t<59.12){$$('#mosaic-title .mosaic-line').forEach((e,i)=>op(e,ramp(t,55.5+i*.12,55.72+i*.12)));}
  if(t>=58.7){let v=ramp(t,58.7,59.12);$('#outro').style.clipPath=`polygon(0 0,${v*1560-180}px 0,${v*1560+180}px 720px,0 720px)`;}
  $$('[data-dot]').forEach((e,i)=>{const id=+e.dataset.dot;tr(e,Math.sin(t*.42+id)*40,Math.cos(t*.37+id)*18)});
  $$('[data-trail]').forEach(e=>{const i=+e.dataset.trail;tr(e,((t*160+i*113)%1600)-400,0)});
 }
 return draw;
}
