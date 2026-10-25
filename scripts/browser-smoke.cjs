"use strict";
// Используйте уже доступные Playwright и браузер; скрипт ничего не устанавливает.
const assert = require("node:assert/strict"), fs = require("node:fs"), path = require("node:path");
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || process.argv[4] || "playwright");
(async () => {
    const executable = process.env.BROWSER_EXECUTABLE || process.argv[5];
    const browser = await chromium.launch({ headless: true, ...(executable ? { executablePath: executable } : {}) });
    try {
        const context = await browser.newContext({ acceptDownloads: true }), page = await context.newPage(), errors = [];
        page.on("pageerror", e => errors.push(e.message));
        const base = process.argv[2] || "http://localhost:8080";
        await page.goto(base, { waitUntil: "networkidle" });
        for (const width of [375, 768, 1920]) {
            await page.setViewportSize({ width, height: 1000 });
            assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `переполнение при ${width}`);
            if (process.argv[3])
                await page.screenshot({ path: path.join(process.argv[3], `export-${width}.png`), fullPage: true });
        }
        await page.setViewportSize({ width: 1280, height: 1000 });
        await page.locator("#text-input").fill("WELCOME");
        await page.locator("#recommend-banner").click();
        await page.waitForFunction(() => !document.querySelector("#recommend-banner").disabled);
        assert.ok(["standard", "shadow", "thinkertoy"].includes(await page.locator("#banner").inputValue()));
        await page.locator("#variations").click();
        await page.locator("#suggestions button").first().waitFor();
        assert.ok(await page.locator("#suggestions button").count() >= 3);
        await page.locator("#suggestions button").first().click();
        for (const banner of ["standard", "shadow", "thinkertoy"]) {
            await page.locator("#text-input").fill("hello");
            await page.locator("#banner").selectOption(banner);
            await page.locator("#generate").click();
            assert.ok((await page.locator("#art").textContent()).length > 30);
        }
        const art = await page.locator("#art").textContent();
        for (const format of ["txt", "html", "pdf", "png", "txt"]) {
            await page.locator("#format").selectOption(format);
            const promised = page.waitForEvent("download");
            await page.locator("#download").click();
            const download = await promised;
            assert.equal(download.suggestedFilename(), `symbol-art.${format}`);
            const data = fs.readFileSync(await download.path());
            if (format === "txt")
                assert.equal(data.toString("utf8"), art);
            if (format === "html")
                assert.match(data.toString("utf8"), /<pre/);
            if (format === "pdf")
                assert.equal(data.subarray(0, 4).toString(), "%PDF");
            if (format === "png")
                assert.deepEqual([...data.subarray(0, 8)], [137, 80, 78, 71, 13, 10, 26, 10]);
            await download.delete();
            await page.waitForFunction(() => !document.querySelector("#download").disabled);
        }
        const insights = await (await page.request.get(base + "/export-insights")).json();
        assert.equal(insights.total, 5);
        assert.equal(insights.counts.txt, 2);
        assert.equal(insights.repeat_content, true);
        assert.ok(insights.optimization);
        assert.deepEqual(errors, []);
        console.log(JSON.stringify({ result: "ok", browser: browser.version(), viewports: [375, 768, 1920], insights }));
        await context.close();
    }
    finally {
        await browser.close();
    }
})().catch(e => { console.error(e); process.exitCode = 1; });
