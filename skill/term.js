/* The terminal of the app window: a dock at the bottom that shows a background session through `claude attach`.
   Put into the page by the server only in the app window (window.WN_TERM); a browser tab never gets it.
   The session keeps running in the background whatever happens to this dock or to the page.
   Every feature has its own terminal: the page of a feature shows the terminals of that feature's sessions, and
   going to another feature shows that one's. The home page shows all of them. */
(function(){
if(!window.WN_TERM||window.wnTerm)return;
var CSS=
"body.tm{padding-bottom:var(--th)}"+
".wnt{position:fixed;z-index:38;left:var(--sw,0px);right:0;bottom:0;height:var(--th);background:#0f1b16;color:#cfe3d8;display:flex;flex-direction:column;box-shadow:0 -6px 24px #0005;font:13px/1.3 Geologica,system-ui,sans-serif}"+
".wnt[hidden]{display:none}"+
".wnt .wtg{position:absolute;left:0;right:0;top:-4px;height:9px;cursor:ns-resize}"+
".wnt .wth{display:flex;align-items:center;gap:4px;padding:6px 10px 0;flex:none}"+
".wnt .wtt{all:unset;cursor:pointer;padding:6px 12px;border-radius:8px 8px 0 0;color:#9fb3a8;font-weight:500;white-space:nowrap;max-width:240px;overflow:hidden;text-overflow:ellipsis}"+
".wnt .wtt[aria-selected=true]{background:#16241e;color:#fff}.wnt .wtt:hover{color:#fff}"+
".wnt .wtt:focus-visible,.wnt .wta:focus-visible,.wnp:focus-visible{outline:3px solid #7ab8ff;outline-offset:-2px}"+
".wnt .wts{flex:1}"+
".wnt .wta[hidden]{display:none}"+
".wnt .wta{all:unset;cursor:pointer;color:#cfe3d8;font-size:12.5px;font-weight:600;padding:3px 10px;border-radius:99px;border:1px solid #ffffff3d;white-space:nowrap}.wnt .wta:hover{background:#ffffff1f}"+
".wnt .wtb{flex:1;min-height:0;background:#16241e;padding:8px 4px 6px 12px;position:relative}"+
".wnt .wtx{position:absolute;inset:8px 4px 6px 12px}"+
".wnt .wtm{max-width:640px;margin:26px auto;padding:0 16px;line-height:1.5;font-size:14px;color:#dfe9e3}.wnt .wtm h3{margin:0 0 8px;font-size:16px;color:#fff}.wnt .wtm p{margin:0 0 10px}"+
".wnt .wtm code{background:#ffffff1c;border-radius:6px;padding:2px 8px;font:13px ui-monospace,SFMono-Regular,Menlo,monospace;color:#fff}"+
".wnt .wtm .wte{color:#ffb4ab}.wnt .wtm small{color:#9fb3a8}"+
".wnp{all:unset;cursor:pointer;position:fixed;z-index:38;left:calc(var(--sw,0px) + 16px);bottom:16px;background:#0f1b16;color:#fff;font:600 12.5px Geologica,system-ui,sans-serif;padding:7px 14px;border-radius:99px;box-shadow:0 6px 20px #0005}"+
".wnp[hidden]{display:none}.wnp i{display:inline-block;width:8px;height:8px;border-radius:50%;background:#2f9e6a;margin-right:8px}"+
"body.tm .cw:not(.wide){bottom:calc(var(--th) + 16px)}body.tm .cw.wide{bottom:var(--th)}"+
"button[data-term].on{background:#2f9e6a!important;border-color:#2f9e6a!important;color:#fff!important}"+
"button[data-term] .tgi{font:700 11px ui-monospace,SFMono-Regular,Menlo,monospace;margin-right:6px;letter-spacing:-.5px;opacity:.8}";
var st=document.createElement('style');st.textContent=CSS;document.head.appendChild(st);
// SCOPE: the feature of this page, '' on the home page. What is open and which tab is shown is kept per scope.
var SCOPE=(function(){var m=/^\/f\/([^/]+)/.exec(location.pathname);return m?decodeURIComponent(m[1]):''})();
// Kept too: the session that was in the terminal of each scope (s). The terminals end when the app is closed (the
// sessions go on in the background); the next time the page is opened, a terminal that was open here is opened
// again for the same session, at the same height.
var ST={o:{},c:{},s:{},h:0};try{var sv=JSON.parse(localStorage.getItem('wn-term')||'{}');ST.o=sv.o||{};ST.c=sv.c||{};ST.s=sv.s||{};ST.h=sv.h||0}catch(e){}
var S={open:ST.o[SCOPE]!==false,cur:ST.c[SCOPE]||'',h:ST.h};   // open unless it was hidden here: a feature with a terminal shows it
function save(){ST.o[SCOPE]=S.open;ST.c[SCOPE]=S.cur;ST.h=S.h;var t=ALL.filter(function(x){return x.id===S.cur})[0];if(t)ST.s[SCOPE]=t.session;try{localStorage.setItem('wn-term',JSON.stringify(ST))}catch(e){}}
function forget(){delete ST.s[SCOPE];try{localStorage.setItem('wn-term',JSON.stringify(ST))}catch(e){}}   // the user left the terminal: it is not opened again by itself
function esc(t){return String(t==null?'':t).replace(/[&<>"]/g,function(c){return{'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[c]})}
function post(body){return fetch('/api/term',{method:'POST',headers:{'Content-Type':'application/json','X-Whatnow':'1'},body:JSON.stringify(body)}).then(function(r){return r.json().then(function(j){return{ok:r.ok,j:j}})})}

// a web address opens in the browser: the window itself never leaves the dashboard
function link(u){if(/^https?:\/\//i.test(u))post({op:'link',url:u}).catch(function(){})}
document.addEventListener('click',function(e){var a=e.target.closest&&e.target.closest('a[href]');if(!a)return;var u;try{u=new URL(a.getAttribute('href'),location.href)}catch(x){return}
 if(/^https?:$/.test(u.protocol)&&u.origin!==location.origin){e.preventDefault();link(u.href)}},true);
var dock=document.createElement('section');dock.className='wnt';dock.hidden=true;dock.setAttribute('aria-label','Terminal');
dock.innerHTML='<div class="wtg" title="Drag to resize, double click to reset"></div><div class="wth"><div class="wtl" role="tablist" style="display:flex;gap:2px;min-width:0;overflow-x:auto"></div><span class="wts"></span><button type="button" class="wta" data-x="leave" title="Close this terminal. The session keeps running in the background.">Disconnect</button><button type="button" class="wta" data-x="hide" title="Hide the terminal, keep it connected">Hide</button></div><div class="wtb"><div class="wtx"></div><div class="wtm" hidden></div></div>';
var pill=document.createElement('button');pill.type='button';pill.className='wnp';pill.hidden=true;
document.body.appendChild(dock);document.body.appendChild(pill);
var tabs=dock.querySelector('.wtl'),xt=dock.querySelector('.wtx'),msg=dock.querySelector('.wtm');

var ALL=[],HERE={},WAIT=null;      // every terminal of the window; the ones opened on this page; a session we wait for
// the terminals this page shows: the ones of its feature (and what was opened right here), all of them on the home page
function mine(){return SCOPE?ALL.filter(function(t){return t.feature===SCOPE||HERE[t.id]}):ALL}
var TERMS=[];
var X=null,FIT=null,RUN=0;         // the xterm on screen and the number of its read loop
function height(){var max=Math.max(160,window.innerHeight-120);return Math.max(160,Math.min(max,S.h||Math.round(window.innerHeight*0.45)))}
// EMB: on a feature page the terminal has no dock of its own. It is one of the two views of the window of the
// session (Messages / Terminal): the page gives the place for it (window.WN_EMBED) and decides when it is shown.
var EMB=null,LAST='';
function layout(){if(EMB){TERMS=ALL;mark();if(X)fit();if(EMB.state)EMB.state();return}
 TERMS=mine();var on=S.open&&(TERMS.length||WAIT);dock.hidden=!on;pill.hidden=on||!(TERMS.length||WAIT);
 document.body.classList.toggle('tm',!!on);document.body.style.setProperty('--th',on?height()+'px':'0px');
 pill.innerHTML='<i></i>Terminal'+(TERMS.length>1?' · '+TERMS.length:TERMS.length?': '+esc(TERMS[0].name):'');
 tabs.innerHTML=TERMS.map(function(t){return '<button type="button" class="wtt" role="tab" data-id="'+t.id+'" aria-selected="'+(t.id===S.cur&&!WAIT)+'" title="'+esc(t.name)+'">'+esc(t.name)+'</button>'}).join('')+(WAIT?'<button type="button" class="wtt" role="tab" aria-selected="true">'+esc(WAIT.name||'session')+'</button>':'');
 dock.querySelector('[data-x=leave]').hidden=!!WAIT||!TERMS.length;mark();if(on&&X)fit()}
function has(sid){return ALL.some(function(t){return t.session===sid})}
// the buttons "Terminal" on the page: green while that session is in this terminal
function mark(){document.querySelectorAll('button[data-term]').forEach(function(b){var on=has(b.dataset.term);if(b.classList.contains('on')!==on)b.classList.toggle('on',on);var tx=on?'In terminal':'Terminal';if(b.dataset.tg!==tx){b.dataset.tg=tx;b.innerHTML='<span class="tgi" aria-hidden="true">&gt;_</span>'+tx}
 b.title=on?'This session is open in the terminal below':'Open this session in the terminal of the app'})}
new MutationObserver(function(){mark()}).observe(document.body,{childList:true,subtree:true});

var libs=null;
function lib(){if(libs)return libs;libs=new Promise(function(ok,bad){var l=document.createElement('link');l.rel='stylesheet';l.href='/xterm.css';document.head.appendChild(l);
  var a=document.createElement('script');a.src='/xterm.js';a.onerror=bad;a.onload=function(){var b=document.createElement('script');b.src='/xterm-addon-fit.js';b.onerror=bad;b.onload=function(){var c=document.createElement('script');c.src='/xterm-addon-web-links.js';c.onerror=bad;c.onload=ok;document.head.appendChild(c)};document.head.appendChild(b)};document.head.appendChild(a)});return libs}
function bytes(b64){var s=atob(b64),u=new Uint8Array(s.length);for(var i=0;i<s.length;i++)u[i]=s.charCodeAt(i);return u}
var SENT={cols:0,rows:0};
function fit(){if(!X||(!EMB&&dock.hidden)||!xt.offsetWidth)return;try{FIT.fit()}catch(e){}if(X.cols!==SENT.cols||X.rows!==SENT.rows){SENT={cols:X.cols,rows:X.rows};post({op:'resize',id:S.cur,cols:X.cols,rows:X.rows}).catch(function(){})}}
function say(html){msg.hidden=!html;xt.hidden=!!html;msg.innerHTML=html||''}
// show the terminal t in a fresh xterm. A terminal that was already running (another page, another tab) is replayed
// from the output the app keeps: the program does not draw its screen again by itself. While the replay is written,
// nothing goes back to the program: old output holds questions to the terminal, and their answers would be typed in.
function show(t,isNew){var replay=!isNew,quiet=replay;S.cur=t.id;S.open=true;WAIT=null;save();say('');layout();var run=++RUN;
 lib().then(function(){if(run!==RUN)return;if(X){X.dispose();X=null}xt.innerHTML='';
  X=new Terminal({fontFamily:'ui-monospace,SFMono-Regular,Menlo,monospace',fontSize:13,cursorBlink:false /* a blinking cursor makes the browser restyle on every frame, all the time the terminal is open */,scrollback:5000,allowProposedApi:true,linkHandler:{activate:function(e,uri){link(uri)}},
   theme:{background:'#16241e',foreground:'#e6efe9',cursor:'#e6efe9',selectionBackground:'#3b7be066'}});
  FIT=new FitAddon.FitAddon();X.loadAddon(FIT);X.loadAddon(new WebLinksAddon.WebLinksAddon(function(e,uri){link(uri)}));X.open(xt);try{FIT.fit()}catch(e){}
  var q=Promise.resolve();                       // keys go out one request after another, in the order typed
  function send(d){if(quiet)return;for(var i=0;i<d.length;i+=4000)(function(part){q=q.then(function(){return post({op:'write',id:t.id,data:part})}).catch(function(){})})(d.slice(i,i+4000))}
  X.onData(send);
  // Shift+Enter is a new line in Claude Code, not "send": a terminal has to say so with ESC + Enter (what iTerm and
  // Terminal send after /terminal-setup); xterm.js alone sends a plain Enter for both
  X.attachCustomKeyEventHandler(function(e){if(e.key==='Enter'&&e.shiftKey&&!e.ctrlKey&&!e.metaKey&&!e.altKey){if(e.type==='keydown')send('\x1b\r');e.preventDefault();return false}return true});
  SENT={cols:0,rows:0};fit();
  X.focus();read(t,replay?0:t.off,run,function(){setTimeout(function(){quiet=false},150)})}).catch(function(){say('<p class="wte">Could not load the terminal.</p>')})}
function read(t,off,run,first){if(run!==RUN)return;
 fetch('/api/term/read?id='+t.id+'&off='+off,{cache:'no-store'}).then(function(r){if(r.status===404)throw 404;return r.json()}).then(function(j){if(run!==RUN)return;
  if(j.reset&&X)X.reset();if(j.data&&X)X.write(bytes(j.data),first);else if(first)first();
  if(j.done){gone(t);return}read(t,j.off,run)}).catch(function(e){if(run!==RUN)return;if(e===404){gone(t);return}setTimeout(function(){read(t,off,run,first)},1000)})}
function gone(t){ALL=ALL.filter(function(x){return x.id!==t.id});TERMS=mine();if(t.session===ST.s[SCOPE])forget();if(S.cur===t.id){RUN++;if(X){X.dispose();X=null}xt.innerHTML='';
  if(TERMS.length&&!EMB)show(TERMS[0]);else{S.cur='';save();layout()}}else layout()}
// the last thing the session said in the terminal, for the folded line of the window
function tail(){if(!X)return '';var b=X.buffer.active,fb='';for(var i=b.length-1;i>=0&&i>b.length-200;i--){var l=b.getLine(i);if(!l)continue;var s=l.translateToString(true).trim();if(!s)continue;
  var m=/^[⏺●]\s*(.+)$/.exec(s);if(m)return m[1];if(!fb&&!/^[❯>─╭╰│⏵✻✢✳✶✽·]/.test(s))fb=s}return fb}
// the view is taken away (the window is folded, or shows the messages): the terminal itself goes on running
function detach(){RUN++;if(X){LAST=tail()||LAST;X.dispose();X=null}xt.innerHTML=''}
// show the terminal of a session: the one that runs, or a new one
function showSes(sid,quiet){var t=ALL.filter(function(x){return x.session===sid})[0];if(t){HERE[t.id]=1;show(t);return Promise.resolve({term:t})}return open(sid,null,quiet)}
function leave(){var t=TERMS.filter(function(x){return x.id===S.cur})[0];if(!t)return;post({op:'close',id:t.id}).catch(function(){});gone(t);forget()}

// open the terminal of a session; a session that is open in another terminal has to be moved to the background there
// quiet: the page opens it by itself (it was open before the app was closed); if the session cannot be attached
// now, nothing is shown and the user is not asked for anything
function open(sid,cancelled,quiet){var d=X?{cols:X.cols,rows:X.rows}:{cols:Math.max(60,Math.floor((window.innerWidth-300)/8)),rows:Math.max(12,Math.floor(height()/17)-3)};
 return post({op:'open',session:sid,cols:d.cols,rows:d.rows,running:quiet?1:0}).then(function(r){if(cancelled&&cancelled())return null;var j=r.j||{};if(quiet&&!j.term)return j;
  if(j.term){HERE[j.term.id]=1;if(!ALL.some(function(x){return x.id===j.term.id}))ALL.push(j.term);show(j.term,!!j.fresh);return j}
  if(j.need==='background'){var first=!WAIT||WAIT.sid!==sid;WAIT={sid:sid,name:j.name,t:first?Date.now():WAIT.t};S.open=true;save();RUN++;
   say('<h3>'+esc(j.name||'This session')+' is open in another terminal</h3><p>A terminal cannot be taken over from outside. In that terminal type <code>/background</code> and press Enter: the session moves to the background and opens here by itself. You can attach to it from any terminal later with <code>claude attach</code>.</p><p><button type="button" class="wta" data-x="copy">Copy /background</button> <button type="button" class="wta" data-x="stop">Cancel</button></p><p><small>Waiting for the session to move…</small></p>');
   layout();var w=WAIT;setTimeout(function(){if(WAIT===w)open(sid,function(){return WAIT!==w})},2000);return j}
  WAIT=WAIT||{sid:sid,name:''};S.open=true;say('<p class="wte">'+esc(j.error||'Could not open the terminal.')+'</p><p><button type="button" class="wta" data-x="stop">Close</button></p>');layout();WAIT=null;return j
 }).catch(function(){say('<p class="wte">No connection to the app.</p>');layout();return null})}

document.addEventListener('click',function(e){var b=e.target.closest('button[data-term]');if(!b)return;e.preventDefault();e.stopPropagation();
 var sid=b.dataset.term;if(EMB&&EMB.request){EMB.request(sid);return}   // the window of the session shows it
 var t=ALL.filter(function(x){return x.session===sid})[0];if(t){HERE[t.id]=1;show(t);return}open(sid)},true);
dock.addEventListener('click',act);
function act(e){var b=e.target.closest('button');if(!b)return;var x=b.dataset.x;
 if(b.dataset.id){var t=TERMS.filter(function(q){return q.id===b.dataset.id})[0];if(t&&(t.id!==S.cur||WAIT))show(t);return}
 if(x==='hide'){S.open=false;save();layout()}
 else if(x==='leave')leave()
 else if(x==='stop'){WAIT=null;say('');if(!EMB&&TERMS.length)show(TERMS.filter(function(q){return q.id===S.cur})[0]||TERMS[0]);layout()}
 else if(x==='copy'){try{navigator.clipboard.writeText('/background');b.textContent='Copied'}catch(err){}}}
pill.addEventListener('click',function(){S.open=true;save();var t=TERMS.filter(function(q){return q.id===S.cur})[0]||TERMS[0];if(t&&!X)show(t);else{layout();if(X)X.focus()}});
// the top edge resizes the dock
var gr=dock.querySelector('.wtg');
gr.addEventListener('pointerdown',function(e){e.preventDefault();gr.setPointerCapture(e.pointerId);var y=e.clientY,h=height();
 function mv(ev){S.h=h+(y-ev.clientY);layout()}function up(){gr.removeEventListener('pointermove',mv);gr.removeEventListener('pointerup',up);save();fit()}
 gr.addEventListener('pointermove',mv);gr.addEventListener('pointerup',up)});
gr.addEventListener('dblclick',function(){S.h=0;save();layout();fit()});
window.addEventListener('resize',function(){layout()});

window.wnTerm={open:open,has:has,show:showSes,detach:detach,leave:leave,fit:fit,tail:function(){return (X?tail():'')||LAST},focus:function(){if(X)X.focus()},
 waiting:function(){return !!WAIT},live:function(){return !!X},stop:function(){WAIT=null;say('')},cur:function(){return ALL.filter(function(t){return t.id===S.cur})[0]||null}};
if(window.WN_EMBED){EMB=window.WN_EMBED;xt=EMB.xt;msg=EMB.msg;msg.addEventListener('click',act);dock.hidden=true;pill.hidden=true;
 if(window.ResizeObserver)new ResizeObserver(function(){fit()}).observe(xt)}
// The wheel over the terminal never scrolls the page. xterm lets the event through when its own screen has
// nothing to scroll in that direction, and Claude Code draws a full screen with no scrollback, so the page behind
// scrolled instead. (On the terminal element only: such a listener on the page would slow down its scrolling.)
xt.addEventListener('wheel',function(e){if(e.cancelable)e.preventDefault()},{passive:false});
// a page opened while terminals run (another feature, the home page): connect to them again
fetch('/api/term/list',{cache:'no-store'}).then(function(r){return r.json()}).then(function(j){ALL=(j.terms||[]).filter(function(t){return !t.done});TERMS=mine();
 if(EMB){layout();if(EMB.ready)EMB.ready(window.wnTerm);return}
 if(!TERMS.length){layout();if(ST.o[SCOPE]===true&&ST.s[SCOPE])open(ST.s[SCOPE],null,true);return}   // it was open here before the app was closed
 var t=TERMS.filter(function(q){return q.id===S.cur})[0]||TERMS[0];if(S.open)show(t);else{S.cur=t.id;layout()}}).catch(function(){});
})();
