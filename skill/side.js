/* whatnow: the bar on the left of every live page. Sessions, Features and Favorites at the top, then every feature,
   one click away. Served by the server as /side.js and loaded by both pages; a page opened as a file has no bar. */
(function(){
if(window.wnSide)return;
var CSS="body{--sw:0px}body.sb{--sw:256px;padding-left:var(--sw)}body.sb.sm{--sw:54px}body.sb .back{display:none}"+
".wns{position:fixed;z-index:35;left:0;top:0;bottom:0;width:var(--sw);box-sizing:border-box;background:#0f1b16;color:#cfe3d8;overflow-y:auto;padding:10px 8px 20px;font:13px/1.3 Geologica,system-ui,sans-serif;box-shadow:inset -1px 0 0 #ffffff14}"+
".wns a{text-decoration:none;color:#dfe9e3;border-radius:9px}.wns a:focus-visible,.wns button:focus-visible{outline:3px solid #7ab8ff;outline-offset:-2px}"+
".wns .wtop{display:flex;align-items:center;margin:0 4px 6px 8px;min-height:26px;font-weight:600;color:#fff;font-size:14px}"+
".wns .wtop button{all:unset;cursor:pointer;margin-left:auto;color:#9fb3a8;padding:1px 8px;border-radius:6px;font-size:15px}.wns .wtop button:hover{background:#ffffff1f;color:#fff}"+
".wns a.wm{display:flex;align-items:center;gap:8px;padding:7px 8px;font-weight:500}.wns a.wm:hover,.wns a.wf:hover{background:#ffffff14}"+
".wns a.wm em,.wns a.wm small{margin-left:auto}.wns a.wm small{color:#8ea398;font-size:12px}.wns a.wm[aria-current] small{color:#586860}"+
".wns a[aria-current]{background:#fff;color:#16241e;font-weight:600}.wns a[aria-current]:hover{background:#fff}"+
".wns h4{margin:14px 8px 4px;font-size:11.5px;font-weight:600;color:#7f968a}"+
".wns a.wf{display:grid;grid-template-columns:10px 1fr auto;gap:2px 9px;align-items:baseline;padding:7px 8px;border-left:3px solid transparent}"+
".wns a.wf span{display:-webkit-box;-webkit-line-clamp:3;-webkit-box-orient:vertical;overflow:hidden;overflow-wrap:anywhere}"+
".wns a.wf small{grid-column:2/4;font-size:11.5px;color:#8ea398;font-weight:400}.wns a.wf[aria-current] small{color:#586860}"+
".wns a.wdn:not([aria-current]){opacity:.55}"+
".wns i{width:8px;height:8px;border-radius:50%;flex:none;box-shadow:inset 0 0 0 2px #8c9a92;display:inline-block}"+
".wns i.work{background:#3fbf86;box-shadow:none}.wns i.bg{background:#7fb0ff;box-shadow:none}.wns i.wait{background:#e8b13a;box-shadow:none}"+
".wns em{font-style:normal;font-size:11px;font-weight:700;background:#e8b13a;color:#2b2000;border-radius:999px;padding:0 7px;white-space:nowrap}"+
/* Claude wrote and waits for an answer */
".wns a.wnew{border-left-color:#e8b13a;background:#e8b13a1f}"+
".wns a.wnew small{color:#f3cf7a;font-weight:600}"+
".wns u{text-decoration:none;font-size:14px;line-height:1;position:relative}"+
".wns a.wnew u::after{content:'';position:absolute;right:-4px;top:-3px;width:8px;height:8px;border-radius:50%;background:#e0452f;box-shadow:0 0 0 2px #0f1b16}"+
"body.sm .wns{padding:10px 6px}body.sm .wns h4,body.sm .wns .wtop b,body.sm .wns a small,body.sm .wns a span,body.sm .wns a.wm em{display:none}"+
"body.sm .wns .wtop{margin:0 0 6px;justify-content:center}body.sm .wns .wtop button{margin:0}"+
".wns .wmo{all:unset;box-sizing:border-box;cursor:pointer;display:block;width:100%;margin-top:18px;padding:7px 8px;border-radius:8px;color:#9fb3a8;font-size:12px}.wns .wmo:hover{background:#ffffff14;color:#fff}.wns .wmo:focus-visible{outline:3px solid #7ab8ff;outline-offset:-2px}"+
"body.sm .wns .wmo{text-align:center;padding:8px 0}body.sm .wns .wmo span{display:none}body.sm .wns .wmo::before{content:attr(data-ab);font-size:14px}"+
"body.sm .wns a.wm,body.sm .wns a.wf{display:grid;grid-template-columns:1fr;justify-items:center;padding:8px 0;position:relative;margin-bottom:2px}"+
"body.sm .wns a.wm::after,body.sm .wns a.wf::after{content:attr(data-ab);font-size:11px;font-weight:700}"+
"body.sm .wns a.wf i{position:absolute;right:4px;top:5px}body.sm .wns a.wf u{position:absolute;left:1px;top:1px;font-size:11px}"+
"body.sm .wns hr{margin:8px 6px}.wns hr{border:0;border-top:1px solid #ffffff1f;margin:8px}"+
/* narrow screen: a row of chips above the page */
"@media(max-width:860px){body.sb,body.sb.sm{--sw:0px}.wns{position:static;width:auto;display:flex;gap:6px;align-items:center;overflow-x:auto;padding:8px 12px;box-shadow:none}"+
".wns .wtop,.wns h4,.wns hr,.wns a small{display:none!important}.wns a.wm,.wns a.wf{display:inline-flex!important;flex:none;gap:7px;align-items:center;padding:4px 12px!important;border-radius:999px;background:#ffffff14;white-space:nowrap;border-left:0;margin:0!important}"+
".wns a::after{display:none}.wns a span{display:inline!important;max-width:200px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.wns a.wf i,.wns a.wf u{position:static!important}.wns a[aria-current]{background:#fff}}";
var st=document.createElement('style');st.textContent=CSS;document.head.appendChild(st);
var el=document.createElement('nav');el.className='wns';el.setAttribute('aria-label','Sessions and features');document.body.insertBefore(el,document.body.firstChild);
var STX={work:'working',bg:'background task running',wait:'waits for you',idle:'session open'},GRP=['Favorites','Active now','Other','Finished'];
var small=false,data=window.WN_SIDE||null,key='',title=document.title,first=true;
try{small=localStorage.getItem('wn-side')==='sm'}catch(e){}
// Motion: the pages animate what changes. "on" shows it whatever the system says (the default: the animations are
// small and tell what happened), "sys" follows the system's "reduce motion", "off" shows none. The class on <html>
// is set before anything is drawn; the pages' styles read it.
var MO=['on','sys','off'],MOT={on:'Motion: on',sys:'Motion: as the system',off:'Motion: off'},MOH={on:'Changes are animated, whatever the system setting says. Click to follow the system.',sys:'Changes are animated unless the system asks to reduce motion. Click to turn them off.',off:'Nothing is animated. Click to turn animations on.'},mo='on';
try{var m0=localStorage.getItem('wn-motion');if(MO.indexOf(m0)>=0)mo=m0}catch(e){}
function moSet(){var c=document.documentElement.classList;MO.forEach(function(x){c.toggle('mo-'+x,x===mo)})}
moSet();
function esc(t){return String(t==null?'':t).replace(/[&<>"]/g,function(c){return{'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[c]})}
function abbr(n){var w=String(n).replace(/[^0-9A-Za-zЀ-ӿ\s]/g,' ').split(/\s+/).filter(Boolean);return ((w[0]||'?').charAt(0)+((w[1]||'').charAt(0))).toUpperCase()}
function grp(x){return x.fav?GRP[0]:x.state?GRP[1]:x.done?GRP[3]:GRP[2]}
function here(){var m=/^\/f\/([^/]+)/.exec(location.pathname);return m?decodeURIComponent(m[1]):''}
// a reply is new until the page of its feature has been open after it
function seen(id){try{return +localStorage.getItem('wn-visit-'+id)||0}catch(e){return 0}}
function visit(){var c=here();if(c&&document.visibilityState==='visible')try{localStorage.setItem('wn-visit-'+c,Date.now())}catch(e){}}
function draw(){if(!data)return;visit();var cur=here(),tab=cur?'':(window.WN_TAB||''),f=data.features||[],asks=0;
 document.body.classList.add('sb');document.body.classList.toggle('sm',small);
 var h='<div class="wtop"><b>whatnow</b><button type="button" aria-expanded="'+!small+'" aria-label="'+(small?'Show the names':'Collapse the bar')+'" title="'+(small?'Show the names':'Collapse the bar')+'">'+(small?'›':'‹')+'</button></div>'+
  '<a class="wm" href="/#sessions" data-tab="S" data-ab="S"'+(tab==='S'?' aria-current="page"':'')+' title="Every Claude Code session"><span>Sessions</span>'+(data.waiting?'<em title="Sessions that wait for you">'+data.waiting+' wait</em>':'')+'</a>'+
  '<a class="wm" href="/#features" data-tab="F" data-ab="F"'+(tab==='F'?' aria-current="page"':'')+' title="Every feature with its readiness"><span>Features</span></a>'+
  '<a class="wm" href="/#favorites" data-tab="V" data-ab="★"'+(tab==='V'?' aria-current="page"':'')+' title="The sessions and features you starred"><span>Favorites</span>'+(data.favorites?'<small>'+data.favorites+'</small>':'')+'</a><hr>';
 h+=GRP.map(function(g){var it=f.filter(function(x){return grp(x)===g});
  return it.length?'<h4>'+g+'</h4>'+it.map(function(x){var mine=x.id===cur,ask=x.ask||'',fresh=!!ask&&!mine&&x.saidT>seen(x.id);if(fresh)asks++;
   var sub=ask==='permission'?'asks for your permission':ask?(fresh?'new reply, waits for your answer':'waits for your answer'):x.state?STX[x.state]:'no open session';
   var tip=x.name+'\n'+(ask==='permission'?'Claude asks for a permission in the terminal.':ask?'Claude wrote something and waits for your answer.':x.state?'Its session: '+STX[x.state]+'.':'No open session.')+(x.waiting?'\n'+x.waiting+' open item'+(x.waiting>1?'s':'')+' only you can do.':'')+(x.done?'\nReady to release.':'');
   return '<a class="wf'+(x.done?' wdn':'')+(ask?' wask':'')+(fresh?' wnew':'')+'" href="/f/'+encodeURIComponent(x.id)+'/" data-ab="'+esc(abbr(x.name))+'"'+(mine?' aria-current="page"':'')+' title="'+esc(tip)+'"><i class="'+x.state+'"></i><span>'+esc(x.name)+'</span>'+(ask?'<u aria-hidden="true">'+(ask==='permission'?'✋':'💬')+'</u>':'')+
    '<small>'+sub+(x.waiting?' · '+x.waiting+' item'+(x.waiting>1?'s':'')+' for you':'')+'</small></a>'}).join(''):''}).join('');
 h+='<button type="button" class="wmo" data-mo="1" data-ab="'+{on:'◉',sys:'◐',off:'○'}[mo]+'" title="'+MOH[mo]+'"><span>'+MOT[mo]+'</span></button>';
 var k=h+small;if(k!==key){key=k;el.innerHTML=h;
  el.querySelector('.wtop button').onclick=function(){small=!small;try{localStorage.setItem('wn-side',small?'sm':'')}catch(e){}draw();el.querySelector('.wtop button').focus()};
  if(first){first=false;var c=el.querySelector('a.wf[aria-current]');if(c&&c.scrollIntoView)c.scrollIntoView({block:'nearest',inline:'center'})}}
 document.title=(asks?'('+asks+') ':'')+title;
 var bk=document.querySelector('.back');if(bk)bk.style.display='none'}
function load(){fetch('/api/features',{cache:'no-store'}).then(function(r){return r.json()}).then(function(j){data=j;draw()}).catch(function(){})}
// on the home page Sessions and Features switch its tab without loading it again
el.addEventListener('click',function(e){var mb=e.target.closest('button[data-mo]');if(mb){mo=MO[(MO.indexOf(mo)+1)%MO.length];try{localStorage.setItem('wn-motion',mo)}catch(x){}moSet();draw();var nb=el.querySelector('button[data-mo]');if(nb)nb.focus();return}
 var a=e.target.closest('a[data-tab]');if(a&&location.pathname==='/'&&typeof window.wnShowTab==='function'){e.preventDefault();window.wnShowTab(a.dataset.tab)}});
window.wnSide={refresh:draw};
// a page from the server comes with the data: draw at once, before the first paint
if(data)draw();else load();setInterval(load,5000);
})();
