import {test} from 'node:test'
import assert from 'node:assert/strict'
import {readFileSync} from 'node:fs'
import ts from 'typescript'
const source=readFileSync(new URL('../src/lib/filters/scope.ts',import.meta.url),'utf8')
const {outputText}=ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.ESNext,target:ts.ScriptTarget.ES2022}})
const {filterAccountIDs}=await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)
test('filter scope matches backend legacy fallback and explicit-list precedence',()=>{
  assert.deepEqual(filterAccountIDs({accountId:'legacy'}),['legacy'])
  assert.deepEqual(filterAccountIDs({accountId:'legacy',accountIds:[]}),['legacy'])
  assert.deepEqual(filterAccountIDs({accountId:'legacy',accountIds:['','']}),['legacy'])
  assert.deepEqual(filterAccountIDs({accountId:'legacy',accountIds:['work','work','']}),['work'])
  assert.deepEqual(filterAccountIDs({accountId:'legacy',accountIds:['work','home','work']}),['work','home'])
  assert.deepEqual(filterAccountIDs({}),[])
})
