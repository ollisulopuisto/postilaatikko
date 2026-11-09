import { test } from 'node:test'
import assert from 'node:assert/strict'
import { existsSync } from 'node:fs'
import { mkdtemp, readFile, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { execFileSync } from 'node:child_process'
import ts from 'typescript'

const chrome = ['/usr/bin/google-chrome', '/usr/bin/chromium', '/usr/bin/chromium-browser'].find(existsSync)
test('browser geometry: fifty arrivals keep the visible row fixed; jump newest leaves selection alone', { skip: !chrome }, async () => {
  const source = await readFile(new URL('../src/lib/components/list/scrollAnchor.ts', import.meta.url), 'utf8')
  const { outputText } = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } })
  const code = outputText.replace(/export /g, '')
  const dir = await mkdtemp(join(tmpdir(), 'mail-anchor-'))
  const html = `<!doctype html><style>
  body{background:#18191d;color:#e9e9ed;font:16px system-ui;margin:40px}#pane{display:flex;border:1px solid #464952;width:800px}#list{height:440px;width:340px;overflow:auto;overflow-anchor:none} .row{padding:18px;border-bottom:1px solid #333;min-height:32px}.selected{background:#26436f}#viewer{padding:24px}button{border:0;background:#4a8dff;color:white;border-radius:18px;padding:10px 18px;margin-bottom:12px}#status{color:#8fc9aa;padding:16px}
  </style><h1>Scroll-anchor browser regression</h1><button id="new">No new mail</button><div id="pane"><div id="list"></div><div id="viewer">Open message stays in reader<br><br>Delivery notice</div></div><div id="status"></div><script>
  ${code}
  const list=document.querySelector('#list');
  function row(key){const el=document.createElement('div');el.className='row'+(key==='open'?' selected':'');el.dataset.key=key;el.textContent=key==='open'?'Delivery notice · selected':key;return el}
  for(const key of ['open',...Array.from({length:20},(_,i)=>'older-'+i)])list.append(row(key));
  const oldTop=list.firstChild.getBoundingClientRect().top;
  const anchor=captureScrollAnchor([...list.children].map(el=>({key:el.dataset.key,top:el.getBoundingClientRect().top,bottom:el.getBoundingClientRect().bottom})),list.getBoundingClientRect().top,list.scrollTop);
  const previous=[...list.children].map(el=>el.dataset.key);
  const fresh=Array.from({length:50},(_,i)=>'new-'+i);
  const fragment=document.createDocumentFragment();for(const key of fresh)fragment.append(row(key));list.prepend(fragment);
  const selected=list.querySelector('.selected');
  list.scrollTop=anchoredScrollTop(anchor,selected.getBoundingClientRect().top,list.getBoundingClientRect().top,list.scrollTop);
  const delta=Math.abs(selected.getBoundingClientRect().top-oldTop);
  const newKeys=newRowsBeforeAnchor(previous,[...list.children].map(el=>el.dataset.key),anchor.key);
  document.querySelector('#new').textContent='↑ '+newKeys.length+' new above';
  let selection='open';document.querySelector('#new').onclick=()=>{list.scrollTop=0};
  const saved=list.scrollTop;document.querySelector('#new').click();const jump=list.scrollTop===0&&selection==='open';list.scrollTop=saved;
  const passed=delta<1&&newKeys.length===50&&jump;
  document.querySelector('#status').textContent=passed?'PASS: row offset unchanged; 50 new above; jump keeps open selection':'FAIL '+JSON.stringify({delta,newKeys,jump});
  </script>`
  const path = join(dir, 'test.html'); await writeFile(path, html)
  const args = ['--headless', '--no-sandbox', '--disable-gpu', '--allow-file-access-from-files', '--window-size=1000,800', '--virtual-time-budget=1000']
  const dom = execFileSync(chrome, [...args, '--dump-dom', `file://${path}`], { encoding: 'utf8', stdio: ['ignore','pipe','ignore'] })
  assert.match(dom, /id="status">PASS:/)
  // Optional visual evidence for local review, not a production-app screenshot.
  if (process.env.MAIL_ANCHOR_SCREENSHOT) execFileSync(chrome, [...args, `--screenshot=${process.env.MAIL_ANCHOR_SCREENSHOT}`, `file://${path}`], {stdio:'ignore'})
})
