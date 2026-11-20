import {test} from 'node:test'
import assert from 'node:assert/strict'
import {readFileSync} from 'node:fs'
test('active palette view label has real English and Finnish translations',()=>{
  for(const [lang,expected] of [['en','active'],['fi','aktiivinen']]){
    const locale=JSON.parse(readFileSync(new URL(`../src/lib/i18n/locales/${lang}.json`,import.meta.url),'utf8'))
    assert.equal(locale.common.active,expected)
  }
})
