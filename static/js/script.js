"use strict";
const statusBox = document.getElementById("status");
function statusMessage(message, error = false) {
    statusBox.hidden = false;
    statusBox.textContent = message;
    statusBox.classList.toggle("error", error);
}
// HttpOnly-cookie отправляется браузером; скрипт не читает её значение.
async function refreshInsights() {
    try {
        const response = await fetch("/export-insights");
        if (!response.ok)
            throw new Error("Статистика временно недоступна.");
        const data = await response.json();
        document.getElementById("insights").textContent =
            `Загрузок: ${data.total}. TXT: ${data.counts.txt}, HTML: ${data.counts.html}, ` +
                `PDF: ${data.counts.pdf}, PNG: ${data.counts.png}. ${data.optimization}`;
    }
    catch (error) {
        statusMessage(error.message, true);
    }
}
document.getElementById("refresh-insights").addEventListener("click", refreshInsights);
refreshInsights();
document.getElementById("generator-form").addEventListener("submit", () => {
    const button = document.getElementById("generate");
    button.disabled = true;
    button.textContent = "Создаём рисунок…";
});
async function downloadFile(event) {
    event.preventDefault();
    const button = document.getElementById("download");
    button.disabled = true;
    button.textContent = "Готовим файл…";
    try {
        const body = new URLSearchParams(new FormData(event.currentTarget));
        const response = await fetch("/export", { method: "POST", body });
        if (!response.ok) {
            const error = await response.json();
            throw new Error(error.error || "Не удалось скачать файл.");
        }
        const blob = await response.blob();
        const url = URL.createObjectURL(blob);
        const link = document.createElement("a");
        link.href = url;
        link.download = `symbol-art.${body.get("format")}`;
        document.body.append(link);
        link.click();
        link.remove();
        setTimeout(() => URL.revokeObjectURL(url), 10000);
        statusMessage("Файл передан браузеру для сохранения.");
        await refreshInsights();
    }
    catch (error) {
        statusMessage(error.message, true);
    }
    finally {
        button.disabled = false;
        button.textContent = "Скачать файл";
    }
}
const exportForm = document.getElementById("export-form");
if (exportForm)
    exportForm.addEventListener("submit", downloadFile);
