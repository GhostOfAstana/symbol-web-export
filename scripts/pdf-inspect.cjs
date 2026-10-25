"use strict";
// Независимый разбор PDF использует уже доступный pdfjs-dist, не библиотеку генератора.
const assert = require("node:assert/strict"), { pathToFileURL } = require("node:url");
(async () => {
    const pdfjs = await import(process.argv[3] ? pathToFileURL(process.argv[3]).href : "pdfjs-dist/legacy/build/pdf.mjs");
    const base = process.argv[2] || "http://localhost:8080";
    for (const [content, pages, expectedX] of [["  one\n two  \n", 1, 0], [("x".repeat(301) + "\n").repeat(53), 4, 301 * 53]]) {
        const response = await fetch(base + "/export", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ content, format: "pdf" }) });
        assert.equal(response.status, 200);
        const data = new Uint8Array(await response.arrayBuffer());
        assert.equal(Number(response.headers.get("content-length")), data.length);
        const document = await pdfjs.getDocument({ data, useSystemFonts: true }).promise;
        assert.equal(document.numPages, pages);
        let text = "";
        for (let page = 1; page <= document.numPages; page++) {
            const result = await (await document.getPage(page)).getTextContent();
            text += result.items.map(item => item.str || "").join("");
        }
        if (expectedX)
            assert.equal((text.match(/x/g) || []).length, expectedX);
        else {
            assert.match(text, /one/);
            assert.match(text, /two/);
        }
        await document.destroy();
    }
    console.log(JSON.stringify({ result: "ok", reader: "pdfjs-dist", checks: ["корректный PDF", "текст one/two извлечён", "широкий многостраничный PDF4 страницы", "все15953 символа сохранены", "Content-Length совпадает"] }));
})().catch(error => { console.error(error); process.exitCode = 1; });
