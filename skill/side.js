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
/* the menu of a feature: three vertical dots at the right edge of its row, shown when the row is pointed at */
".wns .wr{position:relative}.wns .wr a.wf{padding-right:24px}.wns .wk{all:unset;cursor:pointer;position:absolute;right:3px;top:5px;width:20px;height:24px;text-align:center;border-radius:6px;color:#9fb3a8;font-size:16px;line-height:24px;opacity:0}"+
".wns .wr:hover .wk,.wns .wk:focus-visible,.wns .wk[aria-expanded=true]{opacity:1}.wns .wk:hover,.wns .wk[aria-expanded=true]{background:#ffffff26;color:#fff}.wns .wk:focus-visible{outline:3px solid #7ab8ff}"+
".wns .wr a[aria-current]~.wk{color:#586860}.wns .wr a[aria-current]~.wk:hover,.wns .wr a[aria-current]~.wk[aria-expanded=true]{background:#0000001a;color:#17231d}"+
".wkm{position:fixed;z-index:90;background:#fff;border:1px solid #d7dfda;border-radius:10px;box-shadow:0 8px 24px rgba(22,36,30,.28);padding:4px}.wkm button{all:unset;cursor:pointer;display:block;padding:7px 12px;border-radius:7px;color:#17231d;font:600 13px/1.4 inherit;white-space:nowrap}"+
".wkm button:hover,.wkm button:focus-visible{background:#eef3f0}.wkm small{display:block;font-weight:400;color:#586860;font-size:11.5px}body.sm .wns .wk{display:none}"+
".wns i{width:8px;height:8px;border-radius:50%;flex:none;box-shadow:inset 0 0 0 2px #8c9a92;display:inline-block}"+
".wns i.work{background:#3fbf86;box-shadow:none}.wns i.bg{background:#7fb0ff;box-shadow:none}.wns i.wait{background:#e8b13a;box-shadow:none}"+
".wns em{font-style:normal;font-size:11px;font-weight:700;background:#e8b13a;color:#2b2000;border-radius:999px;padding:0 7px;white-space:nowrap}"+
/* Claude wrote and waits for an answer */
".wns a.wnew{border-left-color:#e8b13a;background:#e8b13a1f}"+
".wns a.wnew small{color:#f3cf7a;font-weight:600}"+
/* the open feature with a new reply: its row stays white, the amber edge and a dark amber line say "new" */
".wns a.wnew[aria-current],.wns a.wnew[aria-current]:hover{background:#fff}.wns a.wnew[aria-current] small{color:#8a5a00}.wns a.wnew[aria-current] u::after{box-shadow:0 0 0 2px #fff}"+
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
".wns .wtop,.wns h4,.wns hr,.wns a small,.wns .wk{display:none!important}.wns .wr{display:contents}.wns a.wm,.wns a.wf{display:inline-flex!important;flex:none;gap:7px;align-items:center;padding:4px 12px!important;border-radius:999px;background:#ffffff14;white-space:nowrap;border-left:0;margin:0!important}"+
".wns a::after{display:none}.wns a span{display:inline!important;max-width:200px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.wns a.wf i,.wns a.wf u{position:static!important}.wns a[aria-current]{background:#fff}}";
CSS+=".nfw{position:fixed;inset:0;z-index:80;background:rgba(15,27,22,.5);display:grid;place-items:center;padding:20px}.nfw[hidden]{display:none}.nf{background:var(--paper);border-radius:16px;box-shadow:0 20px 60px rgba(0,0,0,.35);width:min(620px,100%);max-height:calc(100vh - 40px);overflow-y:auto;padding:22px 24px 20px;font-size:13.5px;line-height:1.5}.nf h2{margin:0 0 6px;font-size:18px}.nf .nfm{margin:0 0 10px;color:var(--mut)}.nf .nfm b{color:var(--ink);font-weight:600}.nfl{list-style:none;margin:0 0 14px;padding:10px 12px;background:#f5f8f6;border-radius:10px;display:grid;gap:6px}.nfl li{display:grid;grid-template-columns:96px 1fr;gap:10px;align-items:baseline}.nfl li>span{color:var(--mut);font-size:12.5px}.nfl li>div{min-width:0;line-height:1.7}.nfl code{background:#fff;border:1px solid var(--line);border-radius:6px;padding:1px 7px;font:12.5px ui-monospace,SFMono-Regular,Menlo,monospace;overflow-wrap:anywhere}.nfl small{color:var(--mut)}.nf .nfb{display:block;font-weight:600;margin-bottom:4px}.nf textarea{width:100%;min-height:92px;border:1px solid #c3cec7;border-radius:9px;padding:9px 11px;font:inherit;resize:vertical;background:#fff}.nf textarea:focus-visible{outline:3px solid #2a8cff;outline-offset:1px}.nfp{border:0;margin:14px 0 0;padding:0;display:grid;gap:8px}.nfp legend{font-weight:600;padding:0;margin-bottom:6px}.nfp label{display:grid;grid-template-columns:18px 1fr;gap:10px;align-items:start;border:1.5px solid var(--line);border-radius:10px;padding:10px 12px;cursor:pointer}.nfp label.on{border-color:#1f5fd0;background:#f1f5fe}.nfp input{margin:3px 0 0}.nfp b{font-weight:600}.nfp em{font-style:normal;font-size:11.5px;font-weight:600;color:#0f6b40;background:#e6f4ec;border-radius:99px;padding:1px 8px;margin-left:4px}.nfp small{display:block;color:var(--mut);margin-top:2px;font-size:12.5px}.nf .row{display:flex;gap:10px;align-items:center;margin-top:16px}.nf .go{all:unset;cursor:pointer;background:var(--side);color:#fff;font-weight:600;padding:8px 18px;border-radius:10px}.nf .go[disabled]{opacity:.55;cursor:default}.nf .cx{all:unset;cursor:pointer;color:var(--mut);padding:8px 8px}.nf .go:focus-visible,.nf .cx:focus-visible{outline:3px solid #2a8cff;outline-offset:2px}.nf .nferr{color:var(--bad);margin:12px 0 0}.nf .nfok{margin:8px 0 0}.nf .axp{font-weight:700;padding:1px 9px;border-radius:99px;background:#e8eeea}.nf .axp.warn{background:#fbeccb;color:#5a4300}.nf .axp.bad{background:#fbe5e3;color:#b3261e}.nfs{display:inline-flex;background:#e8eeea;border-radius:99px;padding:3px;margin:6px 0 12px}.nfs button{all:unset;cursor:pointer;padding:4px 14px;border-radius:99px;font-weight:600;font-size:13px;color:#586860}.nfs button[aria-pressed=true]{background:#fff;color:#17231d;box-shadow:0 1px 3px rgba(0,0,0,.15)}.nfs button:focus-visible{outline:3px solid #2a8cff;outline-offset:1px}.nfd{display:flex;gap:8px}.nfd input,.nf select{flex:1;min-width:0;border:1px solid #c3cec7;border-radius:9px;padding:7px 10px;font:inherit;background:#fff}.nfd button{all:unset;cursor:pointer;border:1px solid #c3cec7;border-radius:9px;padding:6px 12px;font-weight:600;font-size:13px;white-space:nowrap}.nfd button:hover{border-color:#17231d}.nf select{display:block;width:100%;margin-bottom:12px}.nf .nfh{color:#586860;font-size:12.5px;margin:4px 0 12px}.nf .nfb em{font-style:normal;font-weight:400;color:#586860}.wns .wnew2{all:unset;box-sizing:border-box;cursor:pointer;display:flex;align-items:center;gap:8px;width:100%;padding:7px 8px;margin:2px 0 4px;border-radius:9px;border:1px dashed #ffffff45;color:#dfe9e3;font-weight:600;font-size:13px}.wns .wnew2:hover{background:#ffffff14;border-color:#ffffff80}.wns .wnew2:focus-visible{outline:3px solid #7ab8ff}body.sm .wns .wnew2{justify-content:center;padding:6px 0}body.sm .wns .wnew2 span{display:none}";
var st=document.createElement('style');st.textContent=CSS;document.head.appendChild(st);
var el=document.createElement('nav');el.className='wns';el.setAttribute('aria-label','Sessions and work');document.body.insertBefore(el,document.body.firstChild);
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
// a reply is new until it was read in the window of its session (wn-read, written by the page of the feature);
// for a feature whose window was never opened here: until its page was open after the reply
function seen(id){try{return +localStorage.getItem('wn-read-'+id)||+localStorage.getItem('wn-visit-'+id)||0}catch(e){return 0}}
function visit(){var c=here();if(c&&document.visibilityState==='visible')try{localStorage.setItem('wn-visit-'+c,Date.now())}catch(e){}}
function draw(){if(!data)return;visit();var cur=here(),tab=cur?'':(window.WN_TAB||''),f=data.features||[],asks=0;
 document.body.classList.add('sb');document.body.classList.toggle('sm',small);
 var h='<div class="wtop"><b>whatnow</b><button type="button" aria-expanded="'+!small+'" aria-label="'+(small?'Show the names':'Collapse the bar')+'" title="'+(small?'Show the names':'Collapse the bar')+'">'+(small?'›':'‹')+'</button></div>'+
  '<a class="wm" href="/#sessions" data-tab="S" data-ab="S"'+(tab==='S'?' aria-current="page"':'')+' title="Every Claude Code session"><span>Sessions</span>'+(data.waiting?'<em title="Sessions that wait for you">'+data.waiting+' wait</em>':'')+'</a>'+
  '<a class="wm" href="/#features" data-tab="F" data-ab="F"'+(tab==='F'?' aria-current="page"':'')+' title="All the work, with its readiness"><span>Work</span></a>'+
  '<a class="wm" href="/#favorites" data-tab="V" data-ab="★"'+(tab==='V'?' aria-current="page"':'')+' title="The sessions and the work you starred"><span>Favorites</span>'+(data.favorites?'<small>'+data.favorites+'</small>':'')+'</a><hr>';
 h+='<button type="button" class="wnew2" data-nw="1" data-ab="+" title="Start a new piece of work: a new Claude Code session in a folder you choose, or with the access of an existing one"><b>+</b><span>New work</span></button>';
 // sessions started from the dashboard that have not made their feature yet
 var st=data.starting||[];if(st.length)h+='<h4>Starting</h4>'+st.map(function(x){return '<a class="wf" href="/#sessions" data-ab="'+esc(abbr(x.name||'new'))+'" title="'+esc((x.name||'A new session')+'\nA new session, started from the dashboard. It gets a page of its own when it has written its plan.\n\n'+(x.task||''))+'"><i class="work"></i><span>'+esc(x.name||'new session')+'</span><small>starting, no page yet</small></a>'}).join('');
 h+=GRP.map(function(g){var it=f.filter(function(x){return grp(x)===g});
  return it.length?'<h4>'+g+'</h4>'+it.map(function(x){var mine=x.id===cur,ask=x.ask||'',fresh=!!ask&&x.saidT>seen(x.id);if(fresh)asks++;
   var sub=ask==='permission'?'asks for your permission':ask?(fresh?'new reply, waits for your answer':'waits for your answer'):x.state?STX[x.state]:'no open session';
   var tip=x.name+'\n'+(ask==='permission'?'Claude asks for a permission in the terminal.':ask?'Claude wrote something and waits for your answer.':x.state?'Its session: '+STX[x.state]+'.':'No open session.')+(x.waiting?'\n'+x.waiting+' open item'+(x.waiting>1?'s':'')+' only you can do.':'')+(x.done?'\nReady to release.':'');
   return '<div class="wr"><a class="wf'+(x.done?' wdn':'')+(ask?' wask':'')+(fresh?' wnew':'')+'" href="/f/'+encodeURIComponent(x.id)+'/" data-ab="'+esc(abbr(x.name))+'"'+(mine?' aria-current="page"':'')+' title="'+esc(tip)+'"><i class="'+x.state+'"></i><span>'+esc(x.name)+'</span>'+(ask?'<u aria-hidden="true">'+(ask==='permission'?'✋':'💬')+'</u>':'')+
    '<small>'+sub+(x.waiting?' · '+x.waiting+' item'+(x.waiting>1?'s':'')+' for you':'')+'</small></a><button type="button" class="wk" data-k="'+esc(x.id)+'" aria-haspopup="menu" aria-expanded="false" aria-label="More for '+esc(x.name)+'" title="More">⋮</button></div>'}).join(''):''}).join('');
 if(window.WN_TERM&&data.alerts)h+='<button type="button" class="wmo" data-al="1" data-ab="'+{sound:'🔔',banner:'🔕',off:'✕'}[data.alerts]+'" title="'+ALH[data.alerts]+'"><span>'+ALT[data.alerts]+'</span></button>';
 h+='<button type="button" class="wmo" data-mo="1" data-ab="'+{on:'◉',sys:'◐',off:'○'}[mo]+'" title="'+MOH[mo]+'"><span>'+MOT[mo]+'</span></button>';
 var k=h+small;if(k!==key){kmShut();key=k;el.innerHTML=h;
  el.querySelector('.wtop button').onclick=function(){small=!small;try{localStorage.setItem('wn-side',small?'sm':'')}catch(e){}draw();el.querySelector('.wtop button').focus()};
  if(first){first=false;var c=el.querySelector('a.wf[aria-current]');if(c&&c.scrollIntoView)c.scrollIntoView({block:'nearest',inline:'center'})}}
 document.title=(asks?'('+asks+') ':'')+title;
 var bk=document.querySelector('.back');if(bk)bk.style.display='none'}
function load(){focusSay();fetch('/api/features',{cache:'no-store'}).then(function(r){return r.json()}).then(function(j){data=j;draw()}).catch(function(){})}
// The app window tells the app whether it is in front: an answer that comes while it is not is announced by the
// system (a notification, a sound). The bar has the switch: with a sound, without, off.
var AL=['sound','banner','off'],ALT={sound:'Alerts: with sound',banner:'Alerts: silent',off:'Alerts: off'},ALH={sound:'When a session answers while this window is not in front, the system shows a notification and plays a sound. Click for no sound.',banner:'When a session answers while this window is not in front, the system shows a notification, without a sound. Click to turn alerts off.',off:'No notifications. Click to turn them on.'};
function tpost(b){return fetch('/api/app',{method:'POST',headers:{'Content-Type':'application/json','X-Whatnow':'1'},body:JSON.stringify(b)})}
var FOC=null;function focusSay(){if(!window.WN_TERM)return;var on=document.visibilityState==='visible'&&document.hasFocus()?1:0;if(on===FOC&&Date.now()-focusSay.t<20000)return;FOC=on;focusSay.t=Date.now();tpost({op:'focus',on:on}).catch(function(){})}
['focus','blur','pageshow'].forEach(function(n){window.addEventListener(n,function(){setTimeout(focusSay,0)})});document.addEventListener('visibilitychange',focusSay);
// on the home page Sessions and Features switch its tab without loading it again
// the menu of a feature in the bar
var KM=null;function kmShut(){if(KM){KM.m.remove();var b=el.querySelector('.wk[aria-expanded=true]');if(b)b.setAttribute('aria-expanded','false');KM=null}}
function kmOpen(b){kmShut();var id=b.dataset.k,m=document.createElement('div'),r=b.getBoundingClientRect();m.className='wkm';m.setAttribute('role','menu');
 m.innerHTML='<button type="button" role="menuitem">New work with the same access<small>A clean session: same folders and permissions, empty memory</small></button>';
 document.body.appendChild(m);m.style.left=Math.max(8,Math.min(r.left,innerWidth-m.offsetWidth-8))+'px';m.style.top=Math.min(r.bottom+4,innerHeight-m.offsetHeight-8)+'px';b.setAttribute('aria-expanded','true');KM={m:m,id:id};
 m.firstChild.onclick=function(){kmShut();nwOpen({from:id})};m.firstChild.focus();
 m.addEventListener('keydown',function(e){if(e.key==='Escape'){kmShut();b.focus()}})}
document.addEventListener('click',function(e){if(KM&&!e.target.closest('.wkm')&&!e.target.closest('.wk'))kmShut()});window.addEventListener('scroll',kmShut,true);
// New work: a new Claude Code session in a folder the user names, or with the access of an existing piece of work
// (its folder, its added folders, its permission mode). The task is optional: with one, the session makes its own
// page and names it; without one it starts and waits, and the dashboard opens an empty page for it.
var NW=null,NMODES=[['','Ask before edits and commands','the default of Claude Code'],['acceptEdits','Edit files without asking','asks before a command that no rule allows'],['plan','Plan only','reads and proposes, changes nothing'],['auto','Work without asking','a safety check stops the risky actions']],
 NMT={'default':['','Asks before it edits a file or runs a command that no rule allows.'],acceptEdits:['','Edits files without asking; asks before a command that no rule allows.'],plan:['','Reads and proposes; changes nothing.'],auto:['warn','Works without asking. A safety check looks at each action and stops the risky ones.'],bypassPermissions:['bad','Does everything without asking and without a safety check.'],dontAsk:['','Does what the rules allow and refuses the rest without asking.']};
function nwGet(k){try{return localStorage.getItem(k)||''}catch(e){return ''}}function nwSet(k,v){try{if(v)localStorage.setItem(k,v);else localStorage.removeItem(k)}catch(e){}}
function nwClose(){if(!NW||NW.busy)return;NW.w.remove();NW=null}
function nwPlan(){if(!NW)return;var src=NW.src,key=src==='work'?NW.from:NW.dir.trim();NW.plan=null;NW.perr='';if(!key){nwDraw();return}var tok=NW.tok=Date.now();nwDraw();
 fetch('/api/spawn?'+(src==='work'?'feature=':'dir=')+encodeURIComponent(key),{cache:'no-store'}).then(function(r){return r.json().then(function(j){return{ok:r.ok,j:j}})}).then(function(r){if(!NW||NW.tok!==tok)return;if(r.ok){NW.plan=r.j;if(!r.j.git)NW.place='same'}else NW.perr=r.j.error||'could not read it';nwDraw()}).catch(function(){if(NW&&NW.tok===tok){NW.perr='no connection to the server';nwDraw()}})}
function nwDraw(){if(!NW)return;var p=NW.plan,box=NW.w.firstChild,ta=box.querySelector('#nwt'),di=box.querySelector('#nwd'),foc=document.activeElement&&document.activeElement.id;
 if(ta)NW.task=ta.value;if(di)NW.dir=di.value;
 var h='<h2 id="nfh">New work</h2>';
 if(NW.done){box.innerHTML=h+'<p class="nfok"><b>Started: '+esc(NW.done.name)+'.</b> It is in the bar on the left under "Starting" and gets a page of its own when it has written its plan. Its session is in the Sessions list.</p><div class="row"><button type="button" class="go" data-nw="close">Close</button></div>';box.querySelector('button').focus();return}
 h+='<div class="nfs" role="group" aria-label="Where it works"><button type="button" data-src="dir" aria-pressed="'+(NW.src==='dir')+'">In a folder</button><button type="button" data-src="work" aria-pressed="'+(NW.src==='work')+'">Like existing work</button></div>';
 if(NW.src==='dir'){
  h+='<label class="nfb" for="nwd">Folder</label><div class="nfd"><input id="nwd" type="text" spellcheck="false" autocomplete="off" placeholder="/path/to/the/project" value="'+esc(NW.dir)+'">'+(window.WN_TERM?'<button type="button" data-nw="pick">Choose…</button>':'')+'</div>'+
   '<p class="nfh">'+(NW.perr?'<span style="color:#b3261e">'+esc(NW.perr)+'</span>':p?(p.git?'A git repository.':'Not a git repository.')+' A new Claude Code session starts here.':'The folder the new session works in.')+'</p>'+
   '<label class="nfb" for="nwm">Permissions</label><select id="nwm">'+NMODES.map(function(m){return '<option value="'+m[0]+'"'+(NW.mode===m[0]?' selected':'')+'>'+m[1]+' ('+m[2]+')</option>'}).join('')+'</select>'}
 else{var fs=(data&&data.features)||[];
  h+='<label class="nfb" for="nwf">With the access of</label><select id="nwf">'+(NW.from?'':'<option value="">Choose…</option>')+fs.map(function(f){return '<option value="'+esc(f.id)+'"'+(f.id===NW.from?' selected':'')+'>'+esc(f.name)+'</option>'}).join('')+'</select>';
  if(NW.perr)h+='<p class="nferr" style="margin:0 0 12px">'+esc(NW.perr)+'</p>';
  else if(p){var md=NMT[p.mode]||null;h+='<p class="nfm">A new session with an empty memory. It gets the same access:</p><ul class="nfl"><li><span>Works in</span><div><code>'+esc(p.rootShown)+'</code></div></li>'+(p.dirsShown.length?'<li><span>Also reaches</span><div>'+p.dirsShown.map(function(d){return '<code>'+esc(d)+'</code>'}).join(' ')+'</div></li>':'')+
   (p.mode?'<li><span>Permissions</span><div><b class="axp '+(md?md[0]:'')+'">'+esc(p.mode)+'</b>'+(md?' <small>'+md[1]+'</small>':'')+'</div></li>':'')+'<li><span>And</span><div><small>the rules, outside services and instruction files of that folder.</small></div></li></ul>'}}
 h+='<label class="nfb" for="nwt">What should it do? <em>(you can leave this empty)</em></label><textarea id="nwt" placeholder="Describe the task: the work gets its name from it. Without a task the session starts and waits for you to write to it.">'+esc(NW.task)+'</textarea>';
 if(p&&p.git)h+='<fieldset class="nfp"><legend>Where does it work?</legend>'+
  '<label class="'+(NW.place==='worktree'?'on':'')+'"><input type="radio" name="nwp" value="worktree"'+(NW.place==='worktree'?' checked':'')+'><span><b>In its own copy of the code</b> <em>safer</em><small>Claude Code makes a second working copy of this repository, on a new branch. Another session in the same repository and the new one cannot overwrite each other\'s files. The new work reaches the main code when its branch is merged.</small></span></label>'+
  '<label class="'+(NW.place==='same'?'on':'')+'"><input type="radio" name="nwp" value="same"'+(NW.place==='same'?' checked':'')+'><span><b>In the folder itself</b><small>The session changes the files in '+esc(p.rootShown)+' directly. Fine when no other session works there; otherwise they get in each other\'s way.</small></span></label></fieldset>';
 h+=(NW.err?'<p class="nferr">'+esc(NW.err)+'</p>':'')+'<div class="row"><button type="button" class="go" data-nw="go"'+(NW.busy||!p?' disabled':'')+'>'+(NW.busy?'Starting…':'Start')+'</button><button type="button" class="cx" data-nw="close">Cancel</button></div>';
 box.innerHTML=h;var f=foc&&box.querySelector('#'+foc);if(f){f.focus();if(f.setSelectionRange&&f.type!=='radio')try{f.setSelectionRange(f.value.length,f.value.length)}catch(e){}}}
function nwOpen(o){o=o||{};if(NW)nwClose();var w=document.createElement('div');w.className='nfw';w.innerHTML='<div class="nf" role="dialog" aria-modal="true" aria-labelledby="nfh"></div>';document.body.appendChild(w);
 NW={w:w,src:o.from?'work':(nwGet('wn-nw-src')==='work'?'work':'dir'),from:o.from||'',dir:nwGet('wn-nw-dir'),mode:nwGet('wn-nw-mode'),task:nwGet('wn-nw-task'),place:'worktree',plan:null,perr:'',err:'',busy:false,done:null,tok:0};
 w.addEventListener('click',function(e){if(e.target===w){nwClose();return}var b=e.target.closest('button');if(!b)return;
  if(b.dataset.src){NW.src=b.dataset.src;nwSet('wn-nw-src',NW.src);NW.err='';nwPlan();return}
  if(b.dataset.nw==='close'){nwClose();return}
  if(b.dataset.nw==='pick'){fetch('/api/app',{method:'POST',headers:{'Content-Type':'application/json','X-Whatnow':'1'},body:JSON.stringify({op:'pickdir'})}).then(function(r){return r.json()}).then(function(j){if(NW&&j.dir){NW.w.querySelector('#nwd').value=j.dir;NW.dir=j.dir;nwSet('wn-nw-dir',j.dir);nwPlan()}}).catch(function(){});return}
  if(b.dataset.nw==='go'&&!NW.busy&&NW.plan){nwDraw();NW.busy=true;NW.err='';nwDraw();var body={action:'spawn',text:NW.task,place:NW.place};if(NW.src==='work')body.feature=NW.from;else{body.dir=NW.plan.root;body.mode=NW.mode}
   fetch('/api/act',{method:'POST',headers:{'Content-Type':'application/json','X-Whatnow':'1'},body:JSON.stringify(body)}).then(function(r){return r.json().then(function(j){return{ok:r.ok,j:j}})}).then(function(r){if(!NW)return;NW.busy=false;
    if(!r.ok){NW.err=r.j.error||'could not start the session';nwDraw();return}nwSet('wn-nw-task','');if(r.j.feature){location.href='/f/'+encodeURIComponent(r.j.feature)+'/';return}NW.done=r.j;nwDraw();load()}).catch(function(){if(NW){NW.busy=false;NW.err='no connection to the server';nwDraw()}})}});
 w.addEventListener('change',function(e){var t=e.target;if(t.name==='nwp'){NW.place=t.value;nwDraw()}else if(t.id==='nwm'){NW.mode=t.value;nwSet('wn-nw-mode',NW.mode)}else if(t.id==='nwf'){NW.from=t.value;nwPlan()}else if(t.id==='nwd'){NW.dir=t.value;nwSet('wn-nw-dir',NW.dir.trim());nwPlan()}});
 w.addEventListener('input',function(e){if(e.target.id==='nwt')nwSet('wn-nw-task',e.target.value)});
 w.addEventListener('keydown',function(e){if(e.key==='Escape')nwClose();else if(e.key==='Enter'&&e.target.id==='nwd'){e.preventDefault();NW.dir=e.target.value;nwSet('wn-nw-dir',NW.dir.trim());nwPlan()}});
 nwDraw();nwPlan();var first=w.querySelector(NW.src==='dir'?'#nwd':'#nwt');if(first)first.focus()}
window.wnNewWork=nwOpen;
el.addEventListener('click',function(e){var nb=e.target.closest('button[data-nw]');if(nb){nwOpen();return}
 var kb=e.target.closest('button.wk');if(kb){e.preventDefault();if(KM&&KM.id===kb.dataset.k)kmShut();else kmOpen(kb);return}
 var ab=e.target.closest('button[data-al]');if(ab&&data){var nm=AL[(AL.indexOf(data.alerts)+1)%AL.length];data.alerts=nm;draw();var nb2=el.querySelector('button[data-al]');if(nb2)nb2.focus();tpost({op:'alerts',mode:nm}).catch(function(){});return}
 var mb=e.target.closest('button[data-mo]');if(mb){mo=MO[(MO.indexOf(mo)+1)%MO.length];try{localStorage.setItem('wn-motion',mo)}catch(x){}moSet();draw();var nb=el.querySelector('button[data-mo]');if(nb)nb.focus();return}
 var a=e.target.closest('a[data-tab]');if(a&&location.pathname==='/'&&typeof window.wnShowTab==='function'){e.preventDefault();window.wnShowTab(a.dataset.tab)}});
window.wnSide={refresh:draw};
// a page from the server comes with the data: draw at once, before the first paint
if(data)draw();else load();setInterval(load,5000);
})();
