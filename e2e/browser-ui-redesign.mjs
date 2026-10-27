/**
 * Real Chromium UI audit, using Node's built-in fetch/WebSocket and CDP only.
 * Run against `make docker-up`: node e2e/browser-ui-redesign.mjs
 * Optional: CHROME_PATH, SENTINEL_URL, QA_OUTPUT_DIR, QA_SETTLE_MS (default 10000),
 * MAX_LAYOUT_SHIFT (default 0.1), --zero-shifts enforces an observed sum of zero.
 * --phase3 additionally checks real Canvas charts, parser-derived AST graph and
 * candidate selection. QA_PIPELINE_ID can select an existing pending healed run.
 * --baseline records the prior design without
 * enforcing redesigned controls or fonts. No API responses are mocked.
 */
import assert from 'node:assert/strict';
import {spawn} from 'node:child_process';
import {createHash} from 'node:crypto';
import {existsSync} from 'node:fs';
import {mkdir, mkdtemp, writeFile, rm} from 'node:fs/promises';
import {homedir, tmpdir} from 'node:os';
import path from 'node:path';

const baseline = process.argv.includes('--baseline');
const visualOnly = process.argv.includes('--visual-only');
const menuOnly = process.argv.includes('--menu-only');
const phase3 = process.argv.includes('--phase3');
const repairPipeline = process.env.QA_PIPELINE_ID;
const interactionsOnly = process.argv.includes('--interactions-only');
const recordShifts = process.argv.includes('--record-shifts');
assert(!(recordShifts&&process.argv.includes('--zero-shifts')),'Choose diagnostic recording or a strict zero-shift gate, not both');
const base = new URL(process.env.SENTINEL_URL ?? 'http://localhost:3000');
const settleMS = Number(process.env.QA_SETTLE_MS ?? 10000);
const maxShift = process.argv.includes('--zero-shifts') ? 0 : Number(process.env.MAX_LAYOUT_SHIFT ?? 0.1);
assert(Number.isFinite(settleMS) && settleMS >= 1000, 'QA_SETTLE_MS must be >=1000');
assert(Number.isFinite(maxShift) && maxShift >= 0, 'MAX_LAYOUT_SHIFT must be >=0');
const output = process.env.QA_OUTPUT_DIR ?? path.join(tmpdir(), baseline ? 'sentinel-ui-before' : 'sentinel-ui-after');
const chromePaths = [process.env.CHROME_PATH,
  path.join(homedir(), 'Library/Caches/ms-playwright/chromium-1248/chrome-mac-arm64/Google Chrome for Testing.app/Contents/MacOS/Google Chrome for Testing'),
  '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  '/usr/bin/chromium', '/usr/bin/chromium-browser', '/usr/bin/google-chrome'];
const executable = chromePaths.find(candidate => candidate && existsSync(candidate));
assert(executable, 'Set CHROME_PATH to an installed Chromium or Chrome executable');
await mkdir(output, {recursive: true});
const profile = await mkdtemp(path.join(tmpdir(), 'sentinel-ui-chrome-'));
const chrome = spawn(executable, ['--headless', '--disable-gpu', '--no-sandbox',
  '--remote-debugging-port=0', `--user-data-dir=${profile}`, 'about:blank'], {stdio: ['ignore', 'ignore', 'pipe']});
let socket, sequence = 0;
const pending = new Map(), exceptions = [], httpErrors = [], fonts = new Map(), webSocketPaths = new Map(), webSocketHandshakes = [];
const sanitize = value => String(value).replace(/([?&](?:token|access_token|jwt|authorization)=)[^&#\s]+/gi,'$1[redacted]').replace(/Bearer\s+[A-Za-z0-9_.-]+/gi,'Bearer [redacted]').replace(/\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b/g,'[redacted JWT]');
const report = {mode: baseline ? 'baseline' : menuOnly ? 'menu-only' : interactionsOnly ? 'interactions-only' : visualOnly ? 'visual-only' : 'redesign', base: base.origin,
  measuredAt: new Date().toISOString(), settleWindowMS: settleMS,
  maxAllowedLayoutShift: baseline || recordShifts ? null : maxShift,
  conditions: {cacheDisabled:true,networkThrottling:false,cpuThrottling:false,headless:true,platform:process.platform},
  layoutShiftDefinition: 'Sum of document-start layout-shift entries excluding hadRecentInput, through authentication, fonts, hydration, and the settle window; not a whole-session or all-network-conditions guarantee.',
  routes: [], interactions: {}, screenshots: [], exceptions, httpErrors, webSocketHandshakes};
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));
const observer = `(() => {
  window.__sentinelQA = {shifts: [], supported: PerformanceObserver.supportedEntryTypes.includes('layout-shift')};
  if (window.__sentinelQA.supported) new PerformanceObserver(list => {
    for (const entry of list.getEntries()) if (!entry.hadRecentInput) window.__sentinelQA.shifts.push({
      value: entry.value, startTime: entry.startTime,
      sources: (entry.sources || []).map(source => ({node: source.node?.className || source.node?.nodeName,
        previous: {x: source.previousRect.x,y: source.previousRect.y,width: source.previousRect.width,height: source.previousRect.height},
        current: {x: source.currentRect.x,y: source.currentRect.y,width: source.currentRect.width,height: source.currentRect.height}}))});
  }).observe({type: 'layout-shift', buffered: true});
})();`;
function call(method, params = {}) {
  const id = ++sequence;
  return new Promise((resolve, reject) => {
    const timeout = setTimeout(() => {pending.delete(id); reject(new Error(`CDP timeout: ${method}`));}, 15000);
    pending.set(id, {resolve, reject, timeout});
    socket.send(JSON.stringify({id, method, params}));
  });
}
async function evaluate(expression) {
  const result = await call('Runtime.evaluate', {expression, returnByValue: true, awaitPromise: true, userGesture: true});
  if (result.exceptionDetails) throw new Error(result.exceptionDetails.exception?.description ?? result.exceptionDetails.text);
  return result.result.value;
}
async function waitFor(expression, message, timeout = 12000) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    try {if (await evaluate(`Boolean(${expression})`)) return;} catch { /* Navigation replaces the execution context. */ }
    await sleep(100);
  }
  throw new Error(message);
}
async function clickExpression(expression) {
  const rect = await evaluate(`(() => {const element = (${expression});if (!element) throw new Error('Missing clickable control');element.scrollIntoView({block:'center',inline:'nearest'});const r=element.getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2};})()`);
  await call('Input.dispatchMouseEvent', {type:'mousePressed',button:'left',clickCount:1,...rect});
  await call('Input.dispatchMouseEvent', {type:'mouseReleased',button:'left',clickCount:1,...rect});
  await sleep(100);
}
const click = selector => clickExpression(`document.querySelector(${JSON.stringify(selector)})`);
const button = (container, text) => clickExpression(`[...document.querySelectorAll(${JSON.stringify(container+' button')})].find(element => element.textContent.trim().toLowerCase().startsWith(${JSON.stringify(text.toLowerCase())}))`);
async function key(key, code, virtual, modifiers = 0) {
  await call('Input.dispatchKeyEvent', {type:'keyDown',key,code,windowsVirtualKeyCode:virtual,modifiers});
  await call('Input.dispatchKeyEvent', {type:'keyUp',key,code,windowsVirtualKeyCode:virtual,modifiers});
}
async function screenshot(name) {
  const shot = await call('Page.captureScreenshot', {format:'png',captureBeyondViewport:false});
  const target = path.join(output, name+'.png');
  await writeFile(target, Buffer.from(shot.data, 'base64'));
  report.screenshots.push(target);
}
async function navigate(route) {
  await call('Page.navigate', {url: new URL(route, base).href});
  await waitFor(`location.pathname === ${JSON.stringify(route)} && document.querySelector('h1,h2') && (document.querySelector('.connection-pill:not(.disconnected)') || document.body.innerText.includes('Connected to gateway'))`, `Route did not become live: ${route}`);
}
async function viewport(width, height) {
  await call('Emulation.setDeviceMetricsOverride', {width,height,deviceScaleFactor:1,mobile:width<600});
}
async function selectRepair() {
  await navigate('/healing-studio');
  await waitFor(`document.querySelector('[aria-label="Select repair pipeline"]')?.options.length > 0`, 'Repair pipeline selector empty');
  await evaluate(`(() => {const select=document.querySelector('[aria-label="Select repair pipeline"]');const option=[...select.options].find(option=>${repairPipeline ? `option.value===${JSON.stringify(repairPipeline)}` : '/\\bHEALED\\b/i.test(option.textContent)'});if(!option)throw new Error('Run a healing simulation before browser QA');select.value=option.value;select.dispatchEvent(new Event('change',{bubbles:true}));})()`);
  await waitFor(`document.querySelector('.repair-pane .segmented') && document.querySelectorAll('.terminal-line').length > 0`, 'A real healed patch and logs did not load');
}
async function profileDismissal() {
  await click('[aria-label="Open user menu"]');await key('Escape','Escape',27);
  assert(await evaluate(`!document.querySelector('[role="menu"]')&&document.activeElement===document.querySelector('[aria-label="Open user menu"]')`),'Profile Escape did not close and restore focus');
  await click('[aria-label="Open user menu"]');
  const outside = await evaluate(`(()=>{const x=innerWidth/2,y=Math.min(400,innerHeight-80);return {x,y,hitClass:document.elementFromPoint(x,y)?.className};})()`);
  await call('Input.dispatchMouseEvent',{type:'mousePressed',button:'left',clickCount:1,x:outside.x,y:outside.y});
  await call('Input.dispatchMouseEvent',{type:'mouseReleased',button:'left',clickCount:1,x:outside.x,y:outside.y});
  await sleep(100);
  assert(await evaluate(`!document.querySelector('[role="menu"]')&&document.activeElement===document.querySelector('[aria-label="Open user menu"]')`),'Physical main-content click did not dismiss the profile menu and restore focus');
  return {escapeRestoresFocus:true,outsideDismisses:true,outsidePoint:outside};
}
try {
  const endpoint = await new Promise((resolve, reject) => {
    let stderr = '';
    const timeout = setTimeout(() => reject(new Error('Chromium did not start its debugging endpoint')), 15000);
    chrome.once('error', error => {clearTimeout(timeout);reject(error);});
    chrome.stderr.on('data', data => {stderr += data; const match = stderr.match(/DevTools listening on (ws:\/\/[^\s]+)/); if(match){clearTimeout(timeout);resolve(new URL(match[1]));}});
    chrome.once('exit', code => {clearTimeout(timeout);reject(new Error(`Chromium exited before startup: ${code}`));});
  });
  const targets = await fetch(`http://${endpoint.host}/json/list`).then(response => response.json());
  const page = targets.find(target => target.type === 'page');
  assert(page, 'Chromium page target missing');
  socket = new WebSocket(page.webSocketDebuggerUrl);
  await new Promise((resolve,reject) => {socket.onopen=resolve;socket.onerror=reject;});
  socket.onmessage = event => {
    const message = JSON.parse(String(event.data));
    if(message.id){const action=pending.get(message.id);if(action){clearTimeout(action.timeout);pending.delete(message.id);message.error?action.reject(new Error(JSON.stringify(message.error))):action.resolve(message.result);}return;}
    if(message.method==='Runtime.exceptionThrown')exceptions.push(sanitize(message.params.exceptionDetails.exception?.description ?? message.params.exceptionDetails.text));
    if(message.method==='Runtime.consoleAPICalled'&&message.params.type==='error')exceptions.push(sanitize('console.error: '+message.params.args.map(arg=>arg.value??arg.description??arg.type).join(' ')));
    if(message.method==='Network.webSocketCreated')webSocketPaths.set(message.params.requestId,new URL(message.params.url).pathname);
    if(message.method==='Network.webSocketHandshakeResponseReceived'){
      const handshake={path:webSocketPaths.get(message.params.requestId)??'/ws',status:message.params.response.status};
      webSocketHandshakes.push(handshake);
      if(handshake.status!==101)httpErrors.push({...handshake,context:'websocket_handshake'});
    }
    if(message.method==='Network.webSocketFrameError')exceptions.push(sanitize('WebSocket '+(webSocketPaths.get(message.params.requestId)??'/ws')+': '+message.params.errorMessage));
    if(message.method==='Network.responseReceived'){
      const response=message.params.response;
      if(response.status>=400)httpErrors.push({path:new URL(response.url).pathname,status:response.status});
      if(message.params.type==='Font')fonts.set(response.url,{path:new URL(response.url).pathname,status:response.status,local:new URL(response.url).origin===base.origin});
    }
  };
  await call('Page.enable');await call('Runtime.enable');await call('Network.enable');
  const version = await call('Browser.getVersion');
  report.browserVersion=version.product;
  await call('Network.setCacheDisabled',{cacheDisabled:true});
  await call('Page.addScriptToEvaluateOnNewDocument',{source:observer});
  await viewport(1440,1000);
  const defaultRoutes = baseline ? ['/overview','/healing-studio'] : ['/overview','/pipelines','/repositories','/healing-studio','/workers','/queue'];
  const routeArgument = process.argv.find(argument=>argument.startsWith('--routes='))?.slice('--routes='.length) ?? process.env.QA_ROUTES;
  const routes = menuOnly || interactionsOnly ? [] : routeArgument ? routeArgument.split(',') : defaultRoutes;
  assert(routes.every(route=>defaultRoutes.includes(route)),'QA_ROUTES must name supported routes');
  const profiles = menuOnly || interactionsOnly ? [] : baseline || process.argv.includes('--desktop-only') ? [{name:'desktop',width:1440,height:1000}] : [{name:'desktop',width:1440,height:1000},{name:'mobile',width:390,height:844}];
  for (const profile of profiles) {
  await viewport(profile.width,profile.height);
  for (const route of routes) {
    await navigate(route);
    await sleep(settleMS);
    const measurement = await evaluate(`(() => {const qa=window.__sentinelQA;const style=getComputedStyle(document.documentElement);return {path:location.pathname,viewport:{width:innerWidth,height:innerHeight},documentWidth:document.documentElement.scrollWidth,heading:[...document.querySelectorAll('h1,h2')].map(e=>e.textContent),live:!!document.querySelector('.connection-pill:not(.disconnected)')||document.body.innerText.includes('Connected to gateway'),layoutShiftSupported:qa.supported,layoutShift:qa.shifts.reduce((sum,entry)=>sum+entry.value,0),layoutShiftEntries:qa.shifts,measurementEndMS:performance.now(),fontFaces:[...document.fonts].map(face=>({family:face.family,status:face.status,weight:face.weight})),bodyFont:getComputedStyle(document.body).fontFamily,codeFont:getComputedStyle(document.querySelector('code')??document.body).fontFamily,bodyFontSize:getComputedStyle(document.body).fontSize,tokens:Object.fromEntries([...style].filter(name=>name.startsWith('--')).map(name=>[name,style.getPropertyValue(name).trim()]))};})()`);
    measurement.contrastSamples=await evaluate(`(() => {
      const rgb=value=>{const parts=value.match(/[\\d.]+/g)?.map(Number);return parts&&parts.length>=3?[parts[0],parts[1],parts[2],parts[3]??1]:null;};
      const composite=(fg,bg)=>fg.slice(0,3).map((value,index)=>value*fg[3]+bg[index]*(1-fg[3]));
      const luminance=values=>values.map(value=>{const channel=value/255;return channel<=.04045?channel/12.92:Math.pow((channel+.055)/1.055,2.4);}).reduce((sum,value,index)=>sum+value*[.2126,.7152,.0722][index],0);
      return [['body',document.body],['heading',document.querySelector('h1,h2')],['muted',document.querySelector('.muted')],['primary',document.querySelector('.primary:not(:disabled)')]].flatMap(([name,element])=>{
        if(!element||!element.getBoundingClientRect().height)return [];
        const chain=[];for(let current=element;current;current=current.parentElement){const color=rgb(getComputedStyle(current).backgroundColor);if(color)chain.push(color);}
        let background=[9,9,11];for(const color of chain.reverse())background=composite(color,background);
        const foreground=rgb(getComputedStyle(element).color);if(!foreground)return [];
        const light=luminance(composite(foreground,background)),dark=luminance(background);
        return [{name,ratio:(Math.max(light,dark)+.05)/(Math.min(light,dark)+.05),fontSize:getComputedStyle(element).fontSize,fontWeight:getComputedStyle(element).fontWeight,foreground:getComputedStyle(element).color,background}];
      });
    })()`);
    report.routes.push(measurement);
    measurement.findings=[];
    const routeAssert=(condition,message)=>{if(!condition&&recordShifts)measurement.findings.push(message);else assert(condition,message);};
    measurement.scriptAssets=await evaluate(`[...document.scripts].filter(script=>script.src).map(script=>new URL(script.src).pathname)`);
    measurement.chrome=await evaluate(`({headerHeight:document.querySelector('.header')?.getBoundingClientRect().height,ribbonHeight:document.querySelector('.telemetry-ribbon')?.getBoundingClientRect().height,sidebarWidth:document.querySelector('.sidebar')?.getBoundingClientRect().width})`);
    routeAssert(measurement.layoutShiftSupported,'Chromium does not support layout-shift entries');
    if(recordShifts&&measurement.layoutShift>0)measurement.findings.push(`Observed cumulative layout shift ${measurement.layoutShift}`);
    if(!baseline&&!recordShifts)assert(measurement.layoutShift<=maxShift, `${route} layout shift ${measurement.layoutShift} > ${maxShift}`);
    routeAssert(measurement.documentWidth<=profile.width, `${profile.name} ${route} overflows the document horizontally`);
    if (!baseline) {
      routeAssert(measurement.fontFaces.some(face=>/Inter/i.test(face.family)&&face.status==='loaded'), 'Inter was not actually loaded');
      routeAssert(measurement.fontFaces.some(face=>/JetBrains Mono/i.test(face.family)&&face.status==='loaded'), 'JetBrains Mono was not actually loaded');
      routeAssert(/Inter/i.test(measurement.bodyFont)&&/JetBrains Mono/i.test(measurement.codeFont),'CSS is not using the loaded fonts');
      const family = value => value.split(',')[0].trim().replace(/^['"]|['"]$/g,'').toLowerCase();
      for(const declared of [measurement.bodyFont,measurement.codeFont])routeAssert(measurement.fontFaces.some(face=>face.status==='loaded'&&family(face.family)===family(declared)),`Declared font ${declared} does not match a loaded face`);
      for(const token of ['--background','--surface','--border','--text','--text-muted','--accent'])routeAssert(measurement.tokens[token],`Missing applied design token ${token}`);
      routeAssert(Math.abs(measurement.chrome.headerHeight-44)<=.5,'Workspace header does not retain its 44px geometry');
      routeAssert(Math.abs(measurement.chrome.ribbonHeight-48)<=.5,'Telemetry ribbon does not retain its 48px geometry');
      for(const sample of measurement.contrastSamples)routeAssert(sample.ratio>=4.5,`${route} ${sample.name} sampled contrast ${sample.ratio.toFixed(3)}:1 is below 4.5:1`);
    }
    await screenshot(profile.name+'-'+route.slice(1));
    console.log(JSON.stringify({route,viewport:profile.name,layoutShift:measurement.layoutShift,windowMS:measurement.measurementEndMS,live:measurement.live}));
  }
  }
  if(menuOnly){await navigate('/workers');report.interactions.profileDismissal=await profileDismissal();await screenshot('profile-dismissal');}
  if (!baseline&&!visualOnly&&!menuOnly) {
    if(interactionsOnly)await navigate('/workers');
    await viewport(1440,1000);await sleep(150);
    await waitFor(`Math.abs(document.querySelector('.sidebar').getBoundingClientRect().width-240)<=.5`,'Expanded sidebar did not settle at 240px');
    const expanded=await evaluate(`document.querySelector('.sidebar').getBoundingClientRect().width`);
    await click('[aria-label="Collapse sidebar"]');
    await waitFor(`Math.abs(document.querySelector('.sidebar').getBoundingClientRect().width-64)<=.5`,'Collapsed sidebar did not settle at 64px');
    const collapsed=await evaluate(`document.querySelector('.sidebar').getBoundingClientRect().width`);
    assert(collapsed<expanded,'Sidebar did not collapse');
    await screenshot('sidebar-collapsed');await click('[aria-label="Expand sidebar"]');
    await waitFor(`Math.abs(document.querySelector('.sidebar').getBoundingClientRect().width-240)<=.5`,'Sidebar did not finish expanding to 240px');
    report.interactions.sidebar={expandedWidth:expanded,collapsedWidth:collapsed};
    await evaluate(`document.querySelector('[aria-label="Search workspace commands"]').focus()`);
    await key('k','KeyK',75,process.platform==='darwin'?4:2);
    await waitFor(`document.querySelector('[aria-label="Command palette"]')`,'Cmd+K did not open the palette');
    await waitFor(`document.activeElement===document.querySelector('[aria-label="Search commands"]')`,'Command palette did not focus its search field');
    const dialogControls = `(()=>{const dialog=document.querySelector('[aria-label="Command palette"]');return [...dialog.querySelectorAll('button:not(:disabled),input:not(:disabled),select:not(:disabled),textarea:not(:disabled),a[href],[tabindex]:not([tabindex="-1"])')].filter(element=>element.tabIndex>=0&&element.getBoundingClientRect().height>0);})()`;
    await evaluate(`(()=>{const controls=${dialogControls};controls.at(-1).focus();})()`);
    await key('Tab','Tab',9);
    assert(await evaluate(`(()=>{const controls=${dialogControls};return document.activeElement===controls[0];})()`),'Tab escaped the command dialog instead of wrapping');
    await evaluate(`(()=>{const controls=${dialogControls};controls[0].focus();})()`);
    await key('Tab','Tab',9,8);
    assert(await evaluate(`(()=>{const controls=${dialogControls};return document.activeElement===controls.at(-1);})()`),'Shift+Tab escaped the command dialog instead of wrapping');
    await key('Escape','Escape',27);
    await waitFor(`!document.querySelector('[aria-label="Command palette"]')`,'Escape did not close the command palette');
    assert(await evaluate(`document.activeElement===document.querySelector('[aria-label="Search workspace commands"]')`),'Escape did not restore the keyboard launcher focus');
    await key('k','KeyK',75,process.platform==='darwin'?4:2);
    await waitFor(`document.querySelector('[aria-label="Command palette"]')`,'Cmd+K did not reopen the palette');
    await waitFor(`document.activeElement===document.querySelector('[aria-label="Search commands"]')`,'Reopened command palette did not focus its search field');
    const groups=await evaluate(`document.querySelector('[aria-label="Command palette"]').innerText`);
    await call('Input.insertText',{text:'wrk nd'});
    await waitFor(`document.querySelector('.command-results')?.innerText.toLowerCase().includes('worker nodes')`,'Fuzzy command query did not match Worker nodes');
    await key('Enter','Enter',13);await waitFor(`location.pathname==='/workers'`,'Command Enter did not navigate');
    report.interactions.palette={fuzzyQuery:'wrk nd',navigatedTo:'/workers',grouped: /Navigation/i.test(groups)&&/Repair scenarios|Actions/i.test(groups),tabWrap:true,shiftTabWrap:true,escapeRestoresLauncher:true};
    assert(report.interactions.palette.grouped,'Palette results are not grouped');
    const dismissedProfile=await profileDismissal();
    await click('[aria-label="Open user menu"]');await button('[role="menu"]','Read-only Viewer');
    await waitFor(`document.querySelector('.statusbar')?.innerText.toLowerCase().includes('viewer')`,'Viewer session did not load');
    assert(await evaluate(`[...document.querySelectorAll('button')].filter(button=>/simulate build/i.test(button.textContent)).every(button=>button.disabled)`),'Viewer can mutate executions');
    await click('[aria-label="Open user menu"]');await button('[role="menu"]','Demo Admin');
    await waitFor(`document.querySelector('.statusbar')?.innerText.toLowerCase().includes('admin')`,'Admin session did not load');
    report.interactions.profile={viewerMutationDisabled:true,adminRestored:true,...dismissedProfile};
    await selectRepair();
    await button('.terminal-stream-tabs','stderr');
    const filtered=await evaluate(`({selected:document.querySelector('.terminal-stream-tabs button[aria-selected="true"]')?.textContent,rows:[...document.querySelectorAll('.terminal-line')].map(row=>row.className)})`);
    assert(filtered.selected.startsWith('stderr')&&filtered.rows.length>0&&filtered.rows.every(value=>value.includes('stderr')),'stderr filter is empty or leaked another stream');
    await button('.terminal-stream-tabs','All output');
    const logQuery=await evaluate(`document.querySelector('.terminal-line > span:last-child').textContent.trim().slice(0,18)`);
    await click('[aria-label="Search logs"]');await call('Input.insertText',{text:logQuery});
    await waitFor(`document.querySelectorAll('.terminal-line').length>0&&[...document.querySelectorAll('.terminal-line > span:last-child')].every(span=>span.textContent.toLowerCase().includes(${JSON.stringify(logQuery.toLowerCase())}))`,'Terminal text search did not filter real output');
    const searchRows=await evaluate(`document.querySelectorAll('.terminal-line').length`);
    await evaluate(`document.querySelector('[aria-label="Search logs"]').select()`);await key('Backspace','Backspace',8);
    await waitFor(`document.querySelector('[aria-label="Search logs"]').value===''`,'Terminal text search did not clear');
    const autoBefore=await evaluate(`document.querySelector('[aria-label="Toggle auto-scroll"]').getAttribute('aria-pressed')`);
    await click('[aria-label="Toggle auto-scroll"]');
    assert(await evaluate(`document.querySelector('[aria-label="Toggle auto-scroll"]').getAttribute('aria-pressed')`)!==autoBefore,'Auto-scroll did not toggle');
    await click('[aria-label="Toggle auto-scroll"]');
    await call('Browser.grantPermissions',{permissions:['clipboardReadWrite','clipboardSanitizedWrite'],origin:base.origin});
    const expectedCopy=await evaluate(`[...document.querySelectorAll('.terminal-line > span:last-child')].map(span=>span.textContent).join('\\n')`);
    await click('[aria-label="Copy terminal output"]');
    await waitFor(`document.querySelector('[aria-label="Copy terminal output"]').title==='Copied'`,'Terminal did not confirm a successful clipboard write');
    const copied=await evaluate(`navigator.clipboard.readText()`);
    assert(copied.length>0&&!/\u001b\[[0-9;]*m/.test(copied),'Terminal copy is empty or contains ANSI controls');
    assert.equal(copied,expectedCopy,'Copied terminal text does not match the displayed filtered output');
    report.interactions.terminal={filteredRows:filtered.rows.length,textFilterRows:searchRows,autoScrollToggle:true,clipboardLength:copied.length,clipboardSHA256:createHash('sha256').update(copied).digest('hex')};
    await button('.repair-pane .segmented','Unified');assert(await evaluate(`!!document.querySelector('.unified-diff')`),'Unified diff did not render');
    await button('.repair-pane .segmented','Split');assert(await evaluate(`!!document.querySelector('.split-diff')`),'Split diff did not render');
    report.interactions.diff={unified:true,split:true};await screenshot('studio-interactions');
    if(phase3){
      await button('.repair-tabs','diagnostics');
      await waitFor(`document.querySelector('[data-function-node]')`,'Actual parser AST did not render');
      const actual=await evaluate(`(async()=>{const auth=await fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({demo:true,role:'admin'})}).then(r=>r.json());const headers={Authorization:'Bearer '+auth.token};const id=document.querySelector('[aria-label="Select repair pipeline"]').value;const detail=await fetch('/api/v1/pipelines/'+id,{headers}).then(r=>r.json());const patch=detail.patches.find(p=>p.verification_status==='VERIFIED');return {graph:patch.diagnostics.call_graph,patch:patch.id,approval:patch.approval_status};})()`);
      const dom=await evaluate(`({nodes:[...document.querySelectorAll('[data-function-node]')].map(n=>n.getAttribute('data-function-node')),edges:document.querySelectorAll('.ast-call-edge').length,provenance:document.querySelector('.ast-graph-provenance').textContent})`);
      assert.deepEqual([...dom.nodes].sort(),actual.graph.nodes.map(n=>n.id).sort(),'AST DOM nodes differ from actual parser metadata');
      assert.equal(dom.edges,actual.graph.edges.length);assert(dom.provenance.includes(actual.graph.provenance));
      await click('[aria-label="Zoom in AST graph"]');assert(await evaluate(`document.querySelector('[aria-label="AST graph zoom"]').textContent`)!=='100%');
      await click('[aria-label="Reset AST graph viewport"]');assert.equal(await evaluate(`document.querySelector('[aria-label="AST graph zoom"]').textContent`),'100%');
      await click('[data-function-node]');assert.equal(await evaluate(`document.querySelector('[data-function-node]').getAttribute('aria-pressed')`),'true');
      await screenshot('phase3-ast');
      await button('.repair-tabs','candidates');
      await waitFor(`document.querySelectorAll('.repair-candidate').length===3`,'Actual candidates did not render');
      if(actual.approval==='PENDING'){
        const before=await evaluate(`document.querySelector('.repair-candidate.selected h4').textContent`);
        await clickExpression(`[...document.querySelectorAll('.repair-candidate:not(.selected) button')].find(b=>!b.disabled)`);
        await waitFor(`document.querySelector('.repair-candidate.selected h4').textContent!==${JSON.stringify(before)}`,'Candidate selection did not refresh visible selected state');
        assert.equal(await evaluate(`document.querySelectorAll('.repair-candidate.selected').length`),1);
      }
      report.interactions.phase3={parserNodes:dom.nodes.length,parserEdges:dom.edges,parserProvenance:actual.graph.provenance,candidates:3,selectionRefreshed:actual.approval==='PENDING',zoom:true};
      await screenshot('phase3-studio');
      await navigate('/workers');await waitFor(`document.querySelectorAll('canvas.telemetry-canvas').length===2`,'Actual Canvas worker charts missing');
      await waitFor(`Number(document.querySelector('.telemetry-panel-foot')?.textContent.match(/(\\d+) real samples/)?.[1])>=2`,'Live Canvas history did not append a second real sample',20000);
      report.interactions.phase3.actualSamples=await evaluate(`Number(document.querySelector('.telemetry-panel-foot').textContent.match(/(\\d+) real samples/)[1])`);
      const canvases=await evaluate(`Array.from(document.querySelectorAll('canvas.telemetry-canvas'),c=>({label:c.getAttribute('aria-label'),width:c.width,height:c.height,hasPaint:c.getContext('2d').getImageData(0,0,c.width,c.height).data.some((v,i)=>i%4===3&&v>0)}))`);
      assert(canvases.every(c=>c.width>0&&c.height>0&&c.hasPaint),'Canvas charts have no measured data paint');
      report.interactions.phase3.canvases=canvases;await screenshot('phase3-workers');
    }

    await navigate('/repositories');
    const repositoryName=await evaluate(`(async()=>{const auth=await fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({demo:true,role:'admin'})}).then(r=>r.json());const repos=await fetch('/api/v1/repos',{headers:{Authorization:'Bearer '+auth.token}}).then(r=>r.json());return repos[0].name;})()`);
    const generateSelector=`[aria-label=${JSON.stringify('Generate webhook secret for '+repositoryName)}]`;
    const secretSelector=`input[aria-label=${JSON.stringify('Webhook secret for '+repositoryName)}]`;
    await waitFor(`document.querySelector(${JSON.stringify(generateSelector)}) && !document.querySelector(${JSON.stringify(generateSelector)}).disabled`,'Actual repository generator did not hydrate as an enabled admin control');
    await click(generateSelector);
    const secret=await evaluate(`document.querySelector(${JSON.stringify(secretSelector)})?.value`);
    assert(typeof secret==='string'&&secret.length>=32,'Secret generator did not populate a strong secret');
    await click(generateSelector);
    assert(await evaluate(`document.querySelector(${JSON.stringify(secretSelector)})?.value`)!==secret,'Secret generator repeated the same value');
    report.interactions.repository={generatedSecretLength:secret.length,saved:false};
    await navigate('/pipelines');await button('[aria-label="Quick status filters"]','Healed');
    assert(await evaluate(`document.querySelectorAll('.execution-matrix tbody tr').length>0&&[...document.querySelectorAll('.execution-matrix tbody tr')].every(row=>row.querySelector('.badge')?.textContent.toUpperCase().includes('HEALED'))`),'Healed status filter is empty or leaked another status');
    await click('.execution-matrix button[aria-label^="Inspect "]');
    await waitFor(`document.querySelector('[aria-label="Execution inspection drawer"]')`,'Execution inspection drawer did not open');
    await screenshot('matrix-inspector');await click('[aria-label="Close execution inspector"]');
    assert(await evaluate(`!document.querySelector('[aria-label="Execution inspection drawer"]')`),'Inspection drawer did not close');
    report.interactions.matrix={statusFilter:true,drawerOpenClose:true};
    await viewport(390,844);await sleep(200);
    const mobile=await evaluate(`({documentWidth:document.documentElement.scrollWidth,viewport:innerWidth,scrollports:[...document.querySelectorAll('*')].filter(element=>element.scrollWidth>element.clientWidth+5&&['auto','scroll'].includes(getComputedStyle(element).overflowX)).map(element=>{element.scrollLeft=100;return {class:element.className,clientWidth:element.clientWidth,scrollWidth:element.scrollWidth,scrollLeft:element.scrollLeft}})})`);
    assert(mobile.documentWidth<=390,'Mobile page overflows the document');report.interactions.mobile=mobile;
    await screenshot('mobile-matrix');
  } else if(baseline) {await viewport(390,844);await screenshot('mobile-before');}
  report.fontRequests=[...fonts.values()];
  if(!baseline){assert(report.fontRequests.length>0,'No actual font requests were observed');assert(report.fontRequests.every(font=>font.local&&font.status===200),'Fonts are not self-hosted successfully');}
  assert(webSocketHandshakes.some(handshake=>handshake.status===101),'No live WebSocket connection was observed');
  assert.equal(exceptions.length,0,'Browser JavaScript exceptions occurred');
  assert.equal(httpErrors.length,0,'Browser HTTP errors occurred');
  report.result=recordShifts?'MEASURED':'PASS';
} catch(error) {
  report.result='FAIL';report.failure=sanitize(error.stack??error);process.exitCode=1;
  try{if(socket?.readyState===WebSocket.OPEN)await screenshot('failure');}catch{}
} finally {
  await writeFile(path.join(output,'report.json'),JSON.stringify(report,null,2));
  console.log(JSON.stringify({result:report.result,failure:report.failure,report:path.join(output,'report.json')}));
  if(socket?.readyState===WebSocket.OPEN)socket.close();
  for(const item of pending.values()){clearTimeout(item.timeout);item.reject(new Error('QA browser closing'));}
  if(chrome.pid&&chrome.exitCode===null&&chrome.signalCode===null){
    const exited = new Promise(resolve=>chrome.once('exit',resolve));
    chrome.kill('SIGTERM');await Promise.race([exited,sleep(3000)]);
    if(chrome.exitCode===null&&chrome.signalCode===null)chrome.kill('SIGKILL');
  }
  await rm(profile,{recursive:true,force:true});
}
