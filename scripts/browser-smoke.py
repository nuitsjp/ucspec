import argparse
import importlib.metadata
import json
from pathlib import Path

from playwright.sync_api import expect, sync_playwright


root = Path(__file__).resolve().parents[1]
parser = argparse.ArgumentParser(
    description="Optional browser smoke test. Not model-to-implementation conformance."
)
parser.add_argument(
    "--report",
    type=Path,
    default=root / "examples/customer-crud/artifacts/ucspec/index.html",
)
parser.add_argument("--chromium", default=None)
parser.add_argument(
    "--output", type=Path, default=root / "validation/browser"
)
args = parser.parse_args()
args.output.mkdir(parents=True, exist_ok=True)

results = []
with sync_playwright() as p:
    browser = p.chromium.launch(
        executable_path=args.chromium,
        headless=True,
        args=["--no-sandbox"],
    )
    errors = []

    report_page = browser.new_page(viewport={"width": 1440, "height": 1000})
    report_page.set_default_timeout(5000)
    report_page.on("pageerror", lambda error: errors.append(str(error)))
    report_page.on(
        "console",
        lambda message: errors.append(message.text)
        if message.type == "error"
        else None,
    )
    report_page.set_content(
        args.report.read_bytes().decode("utf-8"), wait_until="load"
    )

    assert report_page.locator("#nav .nav-package").count() == 2
    assert (
        report_page.locator(
            '#nav .nav-package[data-package="customerManagement"]'
        ).count()
        == 1
    )
    assert (
        report_page.locator('#nav .nav-package[data-package="settings"]').count()
        == 1
    )
    assert report_page.locator("#nav .nav-package .nav-package").count() == 0
    assert report_page.locator("#nav .nav-usecase-button").count() == 5
    assert report_page.locator("#nav .nav-scenario").count() == 25
    view_ids = report_page.locator("#view-select option").evaluate_all(
        "options => options.map(option => option.value)"
    )
    assert len(view_ids) == 6
    for view_id in view_ids:
        report_page.locator("#view-select").select_option(view_id)
        start = report_page.locator("#graph svg").evaluate(
            """(svg, id) => {
                const {spec} = JSON.parse(document.getElementById('spec-data').textContent);
                const expected = [...new Set((id ? spec.usecases[id].starts : spec.project.initial).map(s => s.state))].sort();
                const nodes = [...svg.querySelectorAll('.start-node')];
                const bounds = nodes.map(node => { const c=node.querySelector('circle'); const t=node.transform.baseVal.consolidate().matrix; return {x:t.e+c.cx.baseVal.value,y:t.f+c.cy.baseVal.value,r:c.r.baseVal.value,fill:getComputedStyle(c).fill}; });
                const center = (Math.min(...bounds.map(b=>b.x-b.r))+Math.max(...bounds.map(b=>b.x+b.r)))/2;
                const top = Math.min(...[...svg.querySelectorAll(':scope > .node, :scope > .choice')].map(node=>node.transform.baseVal.consolidate().matrix.f));
                return {expected,actual:nodes.map(n=>n.dataset.startState).sort(),center,middle:svg.viewBox.baseVal.width/2,above:bounds.every(b=>b.y+b.r<top),dark:bounds.every(b=>b.fill==='rgb(23, 43, 58)'),edges:svg.querySelectorAll('.start-edge path').length};
            }""", view_id
        )
        assert start["expected"] == start["actual"]
        assert start["actual"] and start["edges"] == len(start["actual"])
        assert abs(start["center"] - start["middle"]) < 1
        assert start["above"] and start["dark"]
        report_page.locator("#graph").screenshot(path=str(args.output / ((view_id or "system") + ".png")))
    results.append("renderer: all six diagrams have their defined start markers and arrows at the top center")
    report_page.locator("#view-select").select_option("")
    hidden_ids = report_page.evaluate("() => Object.entries(JSON.parse(document.getElementById('spec-data').textContent).spec.transitions).filter(([,t])=>!t.defaultVisible).map(([id])=>id)")
    assert hidden_ids
    for transition_id in hidden_ids:
        assert report_page.locator(f'#graph [data-transition="{transition_id}"]').count() == 0
    show_all = report_page.locator("#show-all-transitions")
    show_all.check()
    for transition_id in hidden_ids:
        assert report_page.locator(f'#graph [data-transition="{transition_id}"]').count() > 0
    show_all.uncheck()
    for transition_id in hidden_ids:
        assert report_page.locator(f'#graph [data-transition="{transition_id}"]').count() == 0
    report_page.locator(
        '#nav .nav-scenario[data-scenario="regressions/customerEditNavigation"]'
    ).click()
    selected_menu = report_page.locator(
        '#graph [data-transition="openSettingsFromCustomerEdit"]'
    )
    expect(selected_menu).to_be_visible()
    expect(selected_menu).to_have_class("edge selected")
    report_page.locator("#zoom-fit").click()
    expect(selected_menu).to_be_visible()
    show_all.check()
    show_all.uncheck()
    expect(selected_menu).to_be_visible()
    results.append(
        "renderer: default-hidden menu transitions, selected path, all-transitions toggle and zoom preserve selection"
    )
    assert (
        report_page.locator(
            '#nav .nav-package[data-package="customerManagement"] .nav-usecase'
        ).count()
        == 4
    )
    assert (
        report_page.locator(
            '#nav .nav-package[data-package="settings"] .nav-usecase'
        ).count()
        == 1
    )

    report_page.locator("#view-select").select_option("editCustomer")
    assert (
        report_page.locator('#nav .nav-scenario[data-uc="editCustomer"]').count()
        == 3
    )
    report_page.locator('#nav .nav-scenario[data-uc="editCustomer"]').first.click()
    assert report_page.locator("#graph .edge.selected").count() > 0
    assert report_page.locator("#detail table").count() == 1
    results.append(
        "renderer: script CSP, usecase navigation, scenario table and SVG highlighting"
    )

    assert report_page.locator(".scenario-goal").inner_text()
    assert report_page.locator(".scenario-outcome").inner_text()
    assert report_page.locator(".scenario-outcome").evaluate("node => !!(node.compareDocumentPosition(document.querySelector('#detail table')) & Node.DOCUMENT_POSITION_FOLLOWING)")
    assert report_page.locator("#coverage .review-guidance").count() == 8
    report_page.locator("#detail").screenshot(path=str(args.output / "scenario.png"))
    candidate = report_page.locator("#coverage details[data-coverage-kind]").filter(has=report_page.locator(".coverage-item > button")).first
    candidate.locator("summary").first.click()
    candidate.locator(".coverage-item > button").first.click()
    expect(report_page.locator("#detail h2")).to_contain_text("未網羅")
    results.append("renderer: missing-path candidate navigation")
    report_page.evaluate("window.scrollTo(0,0)")
    report_page.screenshot(path=str(args.output / "renderer.png"), full_page=False)
    assert not errors, errors
    results.append(
        "renderer: no JavaScript or CSP errors; layout engine itself is outside this test"
    )

    # Keep the reference app on a separate page because the HTML is injected with
    # set_content to exercise its CSP-safe, serverless distribution form.
    app_page = browser.new_page(viewport={"width": 1440, "height": 1000})
    app_page.set_default_timeout(5000)
    app_page.on("pageerror", lambda error: errors.append(str(error)))
    app_page.on(
        "console",
        lambda message: errors.append(message.text)
        if message.type == "error"
        else None,
    )
    app_page.set_content(
        (root / "examples/customer-crud/app/index.html").read_bytes().decode("utf-8"),
        wait_until="load",
    )

    expect(app_page.locator("#screen h2")).to_have_text("顧客検索")
    customer_menu = app_page.locator("#customer-menu")
    settings_menu = app_page.locator("#settings-menu")
    assert customer_menu.count() == 1
    assert settings_menu.count() == 1
    assert app_page.get_by_role("button", name="顧客管理を開く", exact=True).count() == 0
    assert app_page.get_by_role("button", name="通知設定を開く", exact=True).count() == 0
    assert app_page.get_by_role("button", name="ホームへ", exact=True).count() == 0
    app_page.locator("#sample-controls > summary").click()

    app_page.get_by_role("button", name="検索", exact=True).click()
    app_page.get_by_role("button", name="閲覧", exact=True).click()
    app_page.get_by_role("button", name="編集", exact=True).click()
    app_page.locator("#name").fill("")
    app_page.get_by_role("button", name="更新", exact=True).click()
    expect(app_page.get_by_role("status")).to_contain_text("必須")
    results.append("sample: invalid name rejected")

    app_page.locator("#name").fill("更新後の顧客")
    app_page.locator("#fail").check()
    app_page.get_by_role("button", name="更新", exact=True).click()
    expect(app_page.locator('#screen[aria-busy="true"]')).to_be_visible()
    expect(app_page.locator("#customer-menu")).to_be_disabled()
    expect(app_page.locator("#settings-menu")).to_be_disabled()
    expect(app_page.get_by_role("status")).to_contain_text("システムエラーが発生しました。")
    expect(app_page.locator("#name")).to_have_value("更新後の顧客")
    expect(app_page.locator("#customer-menu")).to_be_enabled()
    app_page.get_by_role("button", name="更新", exact=True).click()
    expect(app_page.get_by_role("status")).to_contain_text("更新しました")
    expect(app_page.locator("#screen h2")).to_have_text("顧客を閲覧")
    results.append("sample: update failure preserves input; retry succeeds")

    app_page.locator("#fail").check()
    app_page.get_by_role("button", name="削除", exact=True).click()
    expect(app_page.locator("#delete-dialog")).to_be_visible()
    app_page.get_by_role("button", name="はい", exact=True).click()
    expect(app_page.locator('#screen[aria-busy="true"]')).to_be_visible()
    expect(app_page.locator("#customer-menu")).to_be_disabled()
    expect(app_page.locator("#settings-menu")).to_be_disabled()
    expect(app_page.get_by_role("status")).to_contain_text("システムエラーが発生しました。")
    expect(app_page.locator("#screen h2")).to_have_text("顧客を閲覧")

    app_page.get_by_role("button", name="削除", exact=True).click()
    expect(app_page.locator("#delete-dialog")).to_be_visible()
    app_page.get_by_role("button", name="はい", exact=True).click()
    expect(app_page.locator('#screen[aria-busy="true"]')).to_be_visible()
    expect(app_page.locator("#customer-menu")).to_be_disabled()
    expect(app_page.locator("#settings-menu")).to_be_disabled()
    expect(app_page.get_by_role("status")).to_contain_text("削除しました")
    expect(app_page.locator("#screen h2")).to_have_text("顧客検索")
    zero_search_dialog = []

    def capture_zero_search_alert(dialog):
        zero_search_dialog.append((dialog.type, dialog.message))
        dialog.dismiss()

    app_page.once("dialog", capture_zero_search_alert)
    app_page.get_by_role("button", name="検索", exact=True).click()
    expect(app_page.locator("#screen")).to_contain_text("見つかりません")
    assert zero_search_dialog == [("alert", "対象の顧客が見つかりません。")]
    results.append("sample: delete failure, retry, and no resurrection on search")

    app_page.get_by_role("button", name="データをリセット", exact=True).click()
    app_page.locator("#role").select_option("reader")
    app_page.get_by_role("button", name="検索", exact=True).click()
    app_page.get_by_role("button", name="閲覧", exact=True).click()
    app_page.get_by_role("button", name="編集", exact=True).click()
    expect(app_page.get_by_role("status")).to_contain_text("閲覧専用")
    results.append("sample: reader cannot edit")

    app_page.get_by_role("button", name="データをリセット", exact=True).click()
    app_page.locator("#role").select_option("editor")
    app_page.get_by_role("button", name="検索", exact=True).click()
    app_page.get_by_role("button", name="閲覧", exact=True).click()
    app_page.get_by_role("button", name="編集", exact=True).click()
    app_page.locator("#name").fill("未更新の顧客")
    app_page.once("dialog", lambda dialog: dialog.dismiss())
    settings_menu.click()
    expect(app_page.locator("#screen h2")).to_have_text("顧客編集")
    expect(app_page.locator("#name")).to_have_value("未更新の顧客")
    app_page.once("dialog", lambda dialog: dialog.accept())
    settings_menu.click()
    expect(app_page.locator("#screen h2")).to_have_text("通知設定")
    assert app_page.get_by_role("button", name="通知設定を編集", exact=True).count() == 0
    results.append("sample: customer update menu move confirmation preserves input")

    notification = app_page.locator("#notification-enabled")
    notification.check()
    app_page.once("dialog", lambda dialog: dialog.dismiss())
    customer_menu.click()
    expect(app_page.locator("#screen h2")).to_have_text("通知設定")
    expect(notification).to_be_checked()
    app_page.once("dialog", lambda dialog: dialog.accept())
    customer_menu.click()
    expect(app_page.locator("#screen h2")).to_have_text("顧客検索")
    settings_menu.click()
    expect(app_page.locator("#screen h2")).to_have_text("通知設定")

    notification.check()
    app_page.locator("#fail").check()
    app_page.get_by_role("button", name="通知設定を更新", exact=True).click()
    expect(app_page.locator('#screen[aria-busy="true"]')).to_be_visible()
    expect(app_page.locator("#customer-menu")).to_be_disabled()
    expect(app_page.locator("#settings-menu")).to_be_disabled()
    expect(app_page.get_by_role("status")).to_contain_text("システムエラーが発生しました。")
    expect(notification).to_be_checked()
    app_page.get_by_role("button", name="通知設定を更新", exact=True).click()
    expect(app_page.get_by_role("status")).to_contain_text("更新しました")
    expect(app_page.locator("#screen h2")).to_have_text("通知設定")
    expect(notification).to_be_checked()

    notification.uncheck()
    app_page.get_by_role("button", name="編集を取り消す", exact=True).click()
    expect(notification).to_be_checked()
    expect(app_page.get_by_role("status")).to_contain_text("編集を取り消しました")
    notification.uncheck()
    app_page.get_by_role("button", name="通知設定を更新", exact=True).click()
    expect(app_page.locator('#screen[aria-busy="true"]')).to_be_visible()
    expect(app_page.locator("#customer-menu")).to_be_disabled()
    expect(app_page.locator("#settings-menu")).to_be_disabled()
    expect(app_page.get_by_role("status")).to_contain_text("更新しました")
    expect(notification).not_to_be_checked()
    results.append(
        "sample: notification enable, failure retry, cancel restore, and disable update"
    )

    assert not errors, errors
    app_page.screenshot(path=str(args.output / "sample-app.png"))
    browser_version = browser.version
    browser.close()

(args.output / "results.json").write_text(
    json.dumps(
        {
            "checks": results,
            "count": len(results),
            "errors": errors,
            "chromium": browser_version,
            "playwright": importlib.metadata.version("playwright"),
            "method": "HTML injected with set_content; not file:// verification",
        },
        ensure_ascii=False,
        indent=2,
    ),
    encoding="utf-8",
)
print(json.dumps(results, ensure_ascii=False, indent=2))
