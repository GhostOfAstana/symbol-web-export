"use strict";
const assert = require("node:assert/strict"), fs = require("node:fs");
const { chromium } = require(process.argv[3]);
(async () => {
    const browser = await chromium.launch({ headless: true, executablePath: process.argv[4] });
    try {
        const base = process.argv[2], context = await browser.newContext({ acceptDownloads: true }), other = await browser.newContext();
        const page = await context.newPage();
        await page.goto(base);
        await page.locator("#text-input").fill("hello");
        await page.locator("#banner").selectOption("standard");
        await Promise.all([page.waitForURL("**/symbol-art"), page.locator("#generate").click()]);
        const art = await page.locator("#art").textContent();
        const analysis = await (await page.request.get(base + "/export-analyzer?content=" + encodeURIComponent(art))).json();
        assert.deepEqual(analysis.content, { characters: 256, lines: 8, width: 31, uses_color: false });
        assert.deepEqual(analysis.size_estimates, { txt: 256, html: 1440, png: 14880 });
        assert.equal(analysis.recommended, "txt");
        let response = await page.request.post(base + "/export", { data: { content: "private", format: "txt" } });
        assert.equal(response.status(), 200);
        assert.equal((await (await page.request.get(base + "/export-insights")).json()).total, 1);
        assert.equal((await (await other.request.get(base + "/export-insights")).json()).total, 0);
        for (const [format, content] of [["png", "x".repeat(510)], ["pdf", "x".repeat(65000)]]) {
            response = await page.request.post(base + "/export", { data: { format, content } });
            assert.equal(response.status(), 400);
            assert.ok((await response.json()).error);
        }
        await page.locator("#text-input").fill("W".repeat(100));
        await page.locator("#banner").selectOption("standard");
        await page.locator("#generate").click();
        await page.locator("#format").selectOption("png");
        await page.locator("#download").click();
        await page.waitForFunction(() => !document.querySelector("#download").disabled);
        assert.equal(await page.locator("#status").evaluate(e => e.classList.contains("error")), true);
        assert.match(await page.locator("#status").textContent(), /4096|4 мегапикселя/);
        await page.locator("#format").selectOption("pdf");
        const promise = page.waitForEvent("download");
        await page.locator("#download").click();
        const download = await promise;
        const data = fs.readFileSync(await download.path());
        assert.equal(data.subarray(0, 4).toString(), "%PDF");
        await download.delete();
        assert.equal((await page.request.get(base + "/healthz")).status(), 200);
        console.log(JSON.stringify({ result: "ok", checks: ["hello256/8/31", "формулы1440/14880", "изоляция сессий", "слишком широкий PNG400", "слишком большой PDF400", "ошибка PNG видна в UI", "широкий PDF успешно скачан", "сервер продолжил работу"] }));
        await context.close();
        await other.close();
    }
    finally {
        await browser.close();
    }
})().catch(e => { console.error(e); process.exitCode = 1; });
