async page => {
  const inputs = __INPUTS__;
  const checks = [];
  const errors = [];
  const assert = (condition, message) => {
    if (!condition) throw new Error(message);
  };
  const observe = target => {
    target.setDefaultTimeout(5000);
    target.on('pageerror', error => errors.push(String(error)));
    target.on('console', message => {
      if (message.type() === 'error') errors.push(message.text());
    });
  };
  const result = { passed: false, checks, errors, browser: page.context().browser().version() };
  try {
    observe(page);
    await page.setContent(inputs.report, { waitUntil: 'load' });
    assert(errors.length === 0, 'Renderer emitted JavaScript or CSP errors');
    await page.setViewportSize({ width: 1440, height: 1000 });
    assert(await page.locator('#nav .nav-package').count() === 2, 'Package navigation did not render');
    assert(await page.locator('#nav .nav-usecase-summary').count() === 5, 'Use case navigation did not render');
    assert(await page.locator('#nav .nav-scenario').count() === 25, 'Expected 14 use case scenarios and 11 regressions');
    const viewIDs = await page.locator('#view-select option').evaluateAll(options => options.map(option => option.value));
    assert(viewIDs.length === 6, 'Expected system plus five use case diagrams');
    for (const id of viewIDs) {
      await page.locator('#view-select').selectOption(id);
      const start = await page.locator('#graph svg').evaluate((svg, id) => {
        const { spec } = JSON.parse(document.getElementById('spec-data').textContent);
        const expected = [...new Set((id ? spec.usecases[id].starts : spec.project.initial).map(snapshot => snapshot.state))].sort();
        const markers = [...svg.querySelectorAll('.start-node')];
        const actual = markers.map(node => node.dataset.startState).sort();
        const bounds = markers.map(node => { const circle = node.querySelector('circle'); const t = node.transform.baseVal.consolidate().matrix; return { x: t.e + circle.cx.baseVal.value, y: t.f + circle.cy.baseVal.value, radius: circle.r.baseVal.value, fill: getComputedStyle(circle).fill }; });
        const center = (Math.min(...bounds.map(b => b.x - b.radius)) + Math.max(...bounds.map(b => b.x + b.radius))) / 2;
        const topState = Math.min(...[...svg.querySelectorAll(':scope > .node, :scope > .choice')].map(node => node.transform.baseVal.consolidate().matrix.f));
        return { expected, actual, center, middle: svg.viewBox.baseVal.width / 2, top: bounds.every(b => b.y + b.radius < topState), dark: bounds.every(b => b.fill === 'rgb(23, 43, 58)'), edges: svg.querySelectorAll('.start-edge path').length };
      }, id);
      assert(JSON.stringify(start.actual) === JSON.stringify(start.expected), 'Wrong start states in diagram: ' + (id || 'system'));
      assert(start.actual.length > 0 && start.edges === start.actual.length, 'Missing start marker or arrow: ' + (id || 'system'));
      assert(Math.abs(start.center - start.middle) < 1 && start.top && start.dark, 'Start markers must be dark and at the top center: ' + (id || 'system'));
      await page.locator('#graph').screenshot({ path: inputs.output + '/' + (id || 'system') + '.png' });
    }
    checks.push('renderer: system and all five use case diagrams show the defined start markers and arrows at the top center');
    await page.locator('#view-select').selectOption('');
    const hiddenIDs = await page.evaluate(() => Object.entries(JSON.parse(document.getElementById('spec-data').textContent).spec.transitions).filter(([,transition]) => !transition.defaultVisible).map(([id]) => id));
    assert(hiddenIDs.length > 0, 'Fixture needs default-hidden transitions');
    for (const id of hiddenIDs) assert(await page.locator('#graph [data-transition="' + id + '"]').count() === 0, 'Hidden transitions must be excluded from the default layout');
    await page.locator('#show-all-transitions').check();
    for (const id of hiddenIDs) assert(await page.locator('#graph [data-transition="' + id + '"]').count() > 0, 'All-transitions view must include hidden transitions');
    await page.locator('#show-all-transitions').uncheck();
    await page.locator('#nav [data-scenario="regressions/customerEditNavigation"]').click();
    const selectedMenu = page.locator('#graph [data-transition="openSettingsFromCustomerEdit"]');
    assert(await selectedMenu.isVisible(), 'Selected hidden transition must be visible');
    await page.locator('#zoom-fit').click();
    await page.locator('#show-all-transitions').check();
    await page.locator('#show-all-transitions').uncheck();
    assert(await selectedMenu.isVisible() && (await selectedMenu.getAttribute('class')).includes('selected'), 'Diagram switches must preserve selection');
    checks.push('renderer: package navigation, visible layout variants, selected hidden transition and zoom');
    assert(await page.locator('#graph svg').count() === 1, 'SVG graph did not render');
    await page.locator('#view-select').selectOption('editCustomer');
    assert(await page.locator('#nav .nav-scenario[data-uc="editCustomer"]').count() === 3, 'Unexpected edit scenarios');
    await page.locator('#nav .nav-scenario[data-scenario="editCustomer/update"]').click();
    assert(await page.locator('#graph .edge.selected').count() > 0, 'Scenario path was not highlighted');
    assert(await page.locator('#detail table').count() === 1, 'Scenario table did not render');
    checks.push('renderer: use case navigation, SVG, scenario table and path highlighting');
    assert((await page.locator('.scenario-goal').innerText()).length > 0, 'Scenario purpose is missing');
    assert((await page.locator('.scenario-outcome').innerText()).length > 0, 'Scenario outcome is missing');
    assert(await page.locator('.scenario-outcome').evaluate(node => !!(node.compareDocumentPosition(document.querySelector('#detail table')) & Node.DOCUMENT_POSITION_FOLLOWING)), 'Outcome must precede the operation table');
    assert((await page.locator('#detail').innerText()).includes('モデル上の期待式'), 'Model expectation must be distinguished from the human outcome');
    checks.push('renderer: purpose and expected outcome precede steps; model assertion is separate');
    await page.locator('#detail').screenshot({ path: inputs.output + '/scenario.png' });
    assert(await page.locator('#coverage .review-guidance').count() === 8, 'All eight metrics need review guidance');
    const candidate = page.locator('#coverage details[data-coverage-kind]').filter({ has: page.locator('.coverage-item > button') }).first();
    await candidate.locator('summary').first().click();
    await candidate.locator('.coverage-item > button').first().click();
    assert((await page.locator('#detail h2').innerText()).includes('未網羅'), 'Missing-path candidate did not open');
    checks.push('renderer: missing-path candidate navigation');
    assert(errors.length === 0, 'Renderer emitted JavaScript or CSP errors');
    checks.push('renderer: no JavaScript or CSP errors');

    const reviewPage = await page.context().newPage();
    try {
      observe(reviewPage);
      await reviewPage.setContent(inputs.reviews, { waitUntil: 'load' });
      await reviewPage.locator('#view-select').selectOption('editCustomer');
      const coverage = reviewPage.locator('[data-coverage-kind="boundedPaths"]');
      await coverage.locator('summary').first().click();
      const rawCoverage = await coverage.locator('summary').first().innerText();
      assert(await coverage.locator('.coverage-item[data-review-status="current"]').count() === 4, 'Grouped review requirements were not annotated');
      assert(await coverage.locator('.coverage-item[data-review-status="unreviewed"]').count() > 0, 'Unreviewed paths must remain visible');
      for (const label of ['シナリオ追加が必要', '既存シナリオで代表', '禁止すべき', '必要な設計要求']) {
        assert((await coverage.innerText()).includes(label), 'Missing review label: ' + label);
      }
      assert(await reviewPage.evaluate(() => window.injected === undefined), 'Review text executed as HTML');
      await coverage.locator('.review-scenario').first().click();
      assert((await reviewPage.locator('#detail h2').innerText()).includes('editCustomer/update'), 'Representative scenario did not open');
      assert((await reviewPage.locator('.scenario-goal').innerText()).length > 0, 'Representative purpose is missing');
      checks.push('renderer: grouped coverage reviews, reasons, design requirements and representative scenario navigation');
      await reviewPage.setContent(inputs.stale, { waitUntil: 'load' });
      await reviewPage.locator('#view-select').selectOption('editCustomer');
      const staleCoverage = reviewPage.locator('[data-coverage-kind="boundedPaths"]');
      await staleCoverage.locator('summary').first().click();
      assert(await staleCoverage.locator('.coverage-review[data-review-status="stale"]').count() === 3, 'Old review decisions must remain visible as stale');
      assert(await staleCoverage.locator('.coverage-item[data-review-status="current"]').count() === 0, 'Stale reviews must not mark paths reviewed');
      assert((await staleCoverage.innerText()).includes('再評価が必要'), 'Missing stale review explanation');
      assert((await staleCoverage.locator('summary').first().innerText()).startsWith(rawCoverage), 'Intent review must not change raw coverage');
      assert(errors.length === 0, 'Review pages emitted JavaScript or CSP errors');
      checks.push('renderer: changed intent invalidates review annotations while preserving raw coverage');
    } finally {
      await reviewPage.close();
    }

    const sample = await page.context().newPage();
    try {
      observe(sample);
      await sample.setContent(inputs.sample, { waitUntil: 'load' });
      await sample.setViewportSize({ width: 1440, height: 1000 });
      const button = name => sample.getByRole('button', { name, exact: true });
      const status = value => sample.getByRole('status').filter({ hasText: value }).waitFor();
      const heading = value => sample.locator('#screen h2').filter({ hasText: value }).waitFor();
      const busy = async () => {
        await sample.locator('#screen[aria-busy="true"]').waitFor();
        assert(await sample.locator('#customer-menu').isDisabled() && await sample.locator('#settings-menu').isDisabled(), 'Menus must be disabled during processing');
      };
      await heading('顧客検索');
      await sample.locator('#sample-controls > summary').click();
      await button('検索').click();
      await button('閲覧').click();
      await button('編集').click();
      await sample.locator('#name').fill('');
      await button('更新').click();
      await status('必須');
      checks.push('sample: invalid name rejected');
      await sample.locator('#name').fill('更新後の顧客');
      await sample.locator('#fail').check();
      await button('更新').click();
      await busy();
      await status('システムエラー');
      assert(await sample.locator('#name').inputValue() === '更新後の顧客', 'Update failure lost input');
      await button('更新').click();
      await status('更新しました');
      await heading('顧客を閲覧');
      checks.push('sample: update failure preserves input; retry succeeds');
      await sample.locator('#fail').check();
      await button('削除').click();
      await sample.locator('#delete-dialog').waitFor();
      await button('はい').click();
      await busy();
      await status('システムエラー');
      await heading('顧客を閲覧');
      await button('削除').click();
      await button('はい').click();
      await busy();
      await status('削除しました');
      await heading('顧客検索');
      let zeroSearchAlert;
      sample.once('dialog', async dialog => { zeroSearchAlert = [dialog.type(), dialog.message()]; await dialog.dismiss(); });
      await button('検索').click();
      assert((await sample.locator('#screen').innerText()).includes('見つかりません'), 'Deleted customer reappeared');
      assert(JSON.stringify(zeroSearchAlert) === JSON.stringify(['alert', '対象の顧客が見つかりません。']), 'Zero results must display the search alert');
      checks.push('sample: delete confirmation, failure, retry, and no resurrection on search');
      await button('データをリセット').click();
      await sample.locator('#role').selectOption('reader');
      await button('検索').click();
      await button('閲覧').click();
      await button('編集').click();
      await status('閲覧専用');
      checks.push('sample: reader cannot edit');
      await button('データをリセット').click();
      await sample.locator('#role').selectOption('editor');
      await button('検索').click();
      await button('閲覧').click();
      await button('編集').click();
      await sample.locator('#name').fill('未更新の顧客');
      sample.once('dialog', dialog => dialog.dismiss());
      await sample.locator('#settings-menu').click();
      await heading('顧客編集');
      assert(await sample.locator('#name').inputValue() === '未更新の顧客', 'Cancelled navigation must retain customer input');
      sample.once('dialog', dialog => dialog.accept());
      await sample.locator('#settings-menu').click();
      await heading('通知設定');
      checks.push('sample: customer edit navigation confirmation preserves input');
      const notification = sample.locator('#notification-enabled');
      await notification.check();
      sample.once('dialog', dialog => dialog.dismiss());
      await sample.locator('#customer-menu').click();
      assert(await notification.isChecked(), 'Cancelled navigation must retain notification input');
      sample.once('dialog', dialog => dialog.accept());
      await sample.locator('#customer-menu').click();
      await heading('顧客検索');
      await sample.locator('#settings-menu').click();
      await notification.check();
      await sample.locator('#fail').check();
      await button('通知設定を更新').click();
      await busy();
      await status('システムエラー');
      assert(await notification.isChecked(), 'Notification failure lost input');
      await button('通知設定を更新').click();
      await status('通知設定を更新しました');
      assert(await notification.isChecked(), 'Notification retry did not persist input');
      await notification.uncheck();
      await button('編集を取り消す').click();
      assert(await notification.isChecked(), 'Cancel must restore saved notification setting');
      await notification.uncheck();
      await button('通知設定を更新').click();
      await busy();
      await status('通知設定を更新しました');
      assert(!(await notification.isChecked()), 'Notification disable did not persist');
      checks.push('sample: notification navigation, failure retry, cancel restore and disable update');
      await sample.screenshot({ path: inputs.output + '/sample-app.png' });
      assert(errors.length === 0, 'Sample emitted JavaScript or console errors');
      result.passed = true;
    } finally {
      await sample.close();
    }
  } catch (error) {
    result.passed = false;
    errors.push(String(error));
  }
  return result;
}
