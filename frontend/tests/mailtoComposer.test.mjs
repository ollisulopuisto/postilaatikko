import {test} from 'node:test'
import assert from 'node:assert/strict'
import {readFileSync} from 'node:fs'
import ts from 'typescript'
import {plainTextToHtml} from '../src/lib/components/composer/composerUtils.ts'
const source=readFileSync(new URL('../src/lib/components/composer/Composer.svelte',import.meta.url),'utf8')
const fn=source.match(/ {2}function initializeFromMessage\(\) \{[\s\S]*?\n {2}}/)[0]
const js=ts.transpileModule(fn,{compilerOptions:{target:ts.ScriptTarget.ES2022}}).outputText
function seed(message,plain=false){
  const state={html:''}
  new Function('initialMessage','plainTextToHtml','state','isPlainTextMode',`
 let toRecipients,ccRecipients,bccRecipients,showCc=false,subject,inReplyTo,references,gmailThreadId,initialRichBody,plainTextContent,composerImagesBlocked,signMessage,encryptMessage,pgpSignMessage,pgpEncryptMessage;
 const toSmtpAddress=x=>x; const editor={commands:{setContent:x=>state.html=x,focus:()=>{}}};const stripParagraphStyles=x=>x;const getAlwaysLoadImages=()=>false;const isImageAllowedSync=()=>false;
 ${js};initializeFromMessage();Object.assign(state,{toRecipients,ccRecipients,bccRecipients,subject,plainTextContent,initialRichBody});
 `)(message,plainTextToHtml,state,plain)
  return state
}
test('text-only mailto seeds rich editor safely and preserves cc/bcc',()=>{
  const body='Sent from Postilaatikko via a mailto link\n<img src="https://evil.example" onerror="alert(1)"><script>evil()</script>'
  const message={to:[{address:'to@example.com'}],cc:[{address:'cc@example.com'}],bcc:[{address:'bcc@example.com'}],subject:'test',text_body:body,html_body:''}
  const rich=seed(message);assert.equal(rich.html,plainTextToHtml(body));assert.ok(!rich.html.includes('<img'));assert.ok(!rich.html.includes('<script'));assert.deepEqual(rich.ccRecipients,message.cc);assert.deepEqual(rich.bccRecipients,message.bcc)
  assert.equal(seed(message,true).plainTextContent,body)
})
test('existing rich body stays authoritative',()=>{assert.equal(seed({html_body:'<p>rich</p>',text_body:'plain'}).html,'<p>rich</p>')})
