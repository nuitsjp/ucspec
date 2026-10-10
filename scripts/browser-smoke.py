import json
import argparse
import importlib.metadata
from pathlib import Path
from playwright.sync_api import sync_playwright
root=Path(__file__).resolve().parents[1]
parser=argparse.ArgumentParser(description="Optional browser smoke test. Not model-to-implementation conformance.")
parser.add_argument('--report',type=Path,default=root/'examples/customer-crud/artifacts/usecase-spec/index.html')
parser.add_argument('--chromium',default=None)
parser.add_argument('--output',type=Path,default=root/'validation/browser')
args=parser.parse_args()
args.output.mkdir(parents=True,exist_ok=True)
results=[]
with sync_playwright() as p:
 browser=p.chromium.launch(executable_path=args.chromium,headless=True,args=['--no-sandbox'])
 page=browser.new_page(viewport={"width":1440,"height":1000})
 page.set_default_timeout(5000)
 errors=[]
 page.on('pageerror',lambda e: errors.append(str(e)))
 page.on('console',lambda msg:errors.append(msg.text) if msg.type=='error' else None)
 page.set_content(args.report.read_text(encoding='utf-8'), wait_until='load')
 assert page.locator('#nav button').count()==4
 page.locator('[data-uc="editCustomer"]').click()
 assert page.locator('#scenario-list button').count()==5
 page.locator('#scenario-list button').first.click()
 assert page.locator('#graph .edge.selected').count()>0
 assert page.locator('#detail table').count()==1
 results.append('renderer: script CSP, usecase navigation, scenario table and SVG highlighting')
 page.locator('#coverage button').first.locator('xpath=..').locator('summary').first.click()
 page.locator('#coverage button').first.click()
 assert '未網羅' in page.locator('#detail h2').inner_text()
 results.append('renderer: missing-path candidate navigation')
 page.evaluate('window.scrollTo(0,0)')
 page.screenshot(path=str(args.output/'renderer.png'),full_page=False)
 assert not errors,errors
 results.append('renderer: no JavaScript or CSP errors; layout engine itself is outside this test')
 page.close()
 page=browser.new_page(viewport={"width":1440,"height":1000})
 page.on('pageerror',lambda e: errors.append(str(e)))
 page.set_content((root/'examples/customer-crud/app/index.html').read_text(encoding='utf-8'), wait_until='load')
 page.get_by_role('button',name='検索',exact=True).click()
 page.get_by_role('button',name='閲覧',exact=True).click()
 page.get_by_role('button',name='変更',exact=True).click()
 page.locator('#name').fill('')
 page.get_by_role('button',name='保存',exact=True).click()
 assert '必須' in page.get_by_role('status').inner_text()
 results.append('sample: invalid name rejected')
 page.locator('#name').fill('変更後の顧客')
 page.locator('#fail').check()
 page.get_by_role('button',name='保存',exact=True).click()
 assert page.locator('#name').input_value()=='変更後の顧客'
 assert '失敗' in page.get_by_role('status').inner_text()
 page.get_by_role('button',name='保存',exact=True).click()
 assert '保存しました' in page.get_by_role('status').inner_text()
 results.append('sample: save failure preserves input; retry succeeds')
 page.get_by_role('button',name='削除',exact=True).click()
 page.locator('#fail').check()
 page.get_by_role('button',name='削除を実行',exact=True).click()
 assert '失敗' in page.get_by_role('status').inner_text()
 page.get_by_role('button',name='削除を実行',exact=True).click()
 assert '削除しました' in page.get_by_role('status').inner_text()
 page.get_by_role('button',name='検索',exact=True).click()
 assert '見つかりません' in page.locator('#screen').inner_text()
 results.append('sample: delete failure, retry, and no resurrection on search')
 page.get_by_role('button',name='データをリセット',exact=True).click()
 page.locator('#role').select_option('reader')
 page.get_by_role('button',name='検索',exact=True).click()
 page.get_by_role('button',name='閲覧',exact=True).click()
 page.get_by_role('button',name='変更',exact=True).click()
 assert '閲覧専用' in page.get_by_role('status').inner_text()
 results.append('sample: reader cannot edit')
 assert not errors,errors
 page.screenshot(path=str(args.output/'sample-app.png'))
 browser_version=browser.version
 browser.close()
(args.output/'results.json').write_text(json.dumps({'checks':results,'count':len(results),'errors':errors,'chromium':browser_version,'playwright':importlib.metadata.version('playwright'),'method':'HTML injected with set_content; not file:// verification'},ensure_ascii=False,indent=2),encoding='utf-8')
print(json.dumps(results,ensure_ascii=False,indent=2))
