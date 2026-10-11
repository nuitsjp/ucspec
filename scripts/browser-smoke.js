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
    assert(await page.locator('#nav button').count() === 4, 'Use case navigation did not render');
    assert(await page.locator('#graph svg').count() === 1, 'SVG graph did not render');
    await page.locator('[data-uc="editCustomer"]').click();
    assert(await page.locator('#scenario-list button').count() === 5, 'Unexpected edit scenarios');
    await page.locator('#scenario-list button').first().click();
    assert(await page.locator('#graph .edge.selected').count() > 0, 'Scenario path was not highlighted');
    assert(await page.locator('#detail table').count() === 1, 'Scenario table did not render');
    checks.push('renderer: use case navigation, SVG, scenario table and path highlighting');
    await page.locator('#coverage button').first().locator('xpath=..').locator('summary').first().click();
    await page.locator('#coverage button').first().click();
    assert((await page.locator('#detail h2').innerText()).includes('未網羅'), 'Missing-path candidate did not open');
    checks.push('renderer: missing-path candidate navigation');
    assert(errors.length === 0, 'Renderer emitted JavaScript or CSP errors');
    checks.push('renderer: no JavaScript or CSP errors');

    const sample = await page.context().newPage();
    try {
      observe(sample);
      await sample.setContent(inputs.sample, { waitUntil: 'load' });
      await sample.getByRole('button', { name: '検索', exact: true }).click();
      await sample.getByRole('button', { name: '閲覧', exact: true }).click();
      await sample.getByRole('button', { name: '変更', exact: true }).click();
      await sample.locator('#name').fill('');
      await sample.getByRole('button', { name: '保存', exact: true }).click();
      assert((await sample.getByRole('status').innerText()).includes('必須'), 'Empty name was not rejected');
      checks.push('sample: invalid name rejected');
      await sample.locator('#name').fill('変更後の顧客');
      await sample.locator('#fail').check();
      await sample.getByRole('button', { name: '保存', exact: true }).click();
      assert(await sample.locator('#name').inputValue() === '変更後の顧客', 'Save failure lost input');
      assert((await sample.getByRole('status').innerText()).includes('失敗'), 'Save failure was not shown');
      await sample.getByRole('button', { name: '保存', exact: true }).click();
      assert((await sample.getByRole('status').innerText()).includes('保存しました'), 'Save retry did not succeed');
      checks.push('sample: save failure preserves input; retry succeeds');
      await sample.getByRole('button', { name: '削除', exact: true }).click();
      await sample.locator('#fail').check();
      await sample.getByRole('button', { name: '削除を実行', exact: true }).click();
      assert((await sample.getByRole('status').innerText()).includes('失敗'), 'Delete failure was not shown');
      await sample.getByRole('button', { name: '削除を実行', exact: true }).click();
      assert((await sample.getByRole('status').innerText()).includes('削除しました'), 'Delete retry did not succeed');
      await sample.getByRole('button', { name: '検索', exact: true }).click();
      assert((await sample.locator('#screen').innerText()).includes('見つかりません'), 'Deleted customer reappeared');
      checks.push('sample: delete failure, retry, and no resurrection on search');
      await sample.getByRole('button', { name: 'データをリセット', exact: true }).click();
      await sample.locator('#role').selectOption('reader');
      await sample.getByRole('button', { name: '検索', exact: true }).click();
      await sample.getByRole('button', { name: '閲覧', exact: true }).click();
      await sample.getByRole('button', { name: '変更', exact: true }).click();
      assert((await sample.getByRole('status').innerText()).includes('閲覧専用'), 'Reader was allowed to edit');
      checks.push('sample: reader cannot edit');
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
