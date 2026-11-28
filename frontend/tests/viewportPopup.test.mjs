import {test} from 'node:test'
import assert from 'node:assert/strict'
import {popupPosition} from '../src/lib/utils/viewportPopup.ts'
const bounds={left:0,top:0,width:940,height:700}
test('flips bottom/right anchors and clamps negative/outside anchors',()=>{
  assert.deepEqual(popupPosition(930,690,250,300,bounds),{x:680,y:390})
  assert.deepEqual(popupPosition(40,30,250,300,bounds),{x:40,y:30})
  assert.deepEqual(popupPosition(-50,-50,250,300,bounds),{x:8,y:8})
  assert.deepEqual(popupPosition(2000,2000,250,300,bounds),{x:682,y:392})
})
test('respects visual viewport offsets and oversized popup bounds',()=>{
  assert.deepEqual(popupPosition(400,800,344,224,{left:20,top:50,width:360,height:240}),{x:28,y:58})
})
