"""UI-only Chromium checks. Start node scripts/ui-browser-fixture.mjs first."""
import json, pathlib, sys
from playwright.sync_api import sync_playwright
out=pathlib.Path(sys.argv[1]);out.mkdir(parents=True,exist_ok=True)
logs=[];results=[]
def check(name, condition):
 results.append({'check':name,'passed':bool(condition)})
 assert condition,name
with sync_playwright() as p:
 browser=p.chromium.launch(executable_path='/usr/bin/chromium',headless=True)
 page=browser.new_page(viewport={'width':1280,'height':900});page.set_default_timeout(7000)
 page.on('console',lambda m:logs.append({'type':m.type,'text':m.text}))
 page.on('pageerror',lambda e:logs.append({'type':'pageerror','text':str(e)}))
 try:
  page.goto('http://127.0.0.1:8766/__ui');page.locator('#admin-sidebar').wait_for();page.wait_for_timeout(600)
  nav=page.locator('#admin-sidebar');check('desktop expanded width',round(nav.bounding_box()['width'])==256)
  page.screenshot(path=str(out/'goadmin-desktop-expanded.png'),full_page=True)
  check('nested active link',page.locator('a[aria-current="page"]').count()==1)
  group=page.locator('#admin-sidebar nav button[aria-expanded]').first;group.focus();page.keyboard.press('Enter');check('nested keyboard collapse',group.get_attribute('aria-expanded')=='false');page.keyboard.press('Enter');check('nested keyboard expand',group.get_attribute('aria-expanded')=='true')
  page.get_by_label('Synthetic text').fill('Name <arbitrary> Русский')
  page.locator('select').select_option('en');check('English chosen',page.locator('html').get_attribute('lang')=='en');check('input retained after locale',page.get_by_label('Synthetic text').input_value()=='Name <arbitrary> Русский')
  page.get_by_role('button',name='Collapse',exact=True).click();page.wait_for_timeout(400);check('desktop collapsed width',round(nav.bounding_box()['width'])==80)
  page.screenshot(path=str(out/'goadmin-desktop-collapsed.png'),full_page=True)
  page.reload();page.wait_for_timeout(600);check('collapsed persists reload',round(nav.bounding_box()['width'])==80);check('locale persists reload',page.locator('select').input_value()=='en')
  page.locator('select').select_option('ru');check('Russian chosen',page.locator('html').get_attribute('lang')=='ru')
  page.set_viewport_size({'width':360,'height':800});page.wait_for_timeout(500);check('closed mobile sidebar inert',nav.get_attribute('inert') is not None)
  page.get_by_role('button',name='Открыть меню',exact=True).click();page.wait_for_timeout(700);print('mobile state',nav.bounding_box(),nav.get_attribute('inert'),flush=True);check('mobile drawer expanded',round(nav.bounding_box()['width'])==256 and nav.get_attribute('inert') is None)
  page.screenshot(path=str(out/'goadmin-mobile-open.png'),full_page=True)
  page.keyboard.press('Escape');check('Escape closes and inerts',nav.get_attribute('inert') is not None)
  page.get_by_role('button',name='Открыть меню',exact=True).click();page.wait_for_timeout(400);page.locator('.backdrop-blur-sm').click(position={'x':330,'y':400});check('overlay closes',nav.get_attribute('inert') is not None)
  check('mobile no horizontal overflow',page.evaluate('document.documentElement.scrollWidth <= innerWidth'))
  page.screenshot(path=str(out/'goadmin-mobile-closed.png'),full_page=True)
  check('no browser runtime errors',not any(x['type']=='pageerror' for x in logs))
 finally:
  (out/'goadmin-browser-console.json').write_text(json.dumps(logs,indent=2));(out/'goadmin-browser-results.json').write_text(json.dumps(results,indent=2));browser.close()
print(json.dumps(results,indent=2))
