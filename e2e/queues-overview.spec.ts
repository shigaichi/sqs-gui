import { expect, test } from "@playwright/test";

test.describe("Queues overview", () => {
	test("displays the main controls", async ({ page }) => {
		await page.goto(`/queues`);

		await expect(page.locator('[data-page="queues"]')).toBeVisible();
		await expect(
			page.getByRole("heading", { level: 1, name: "Queues" }),
		).toBeVisible();
		await expect(
			page
				.locator('[data-page="queues"]')
				.getByRole("link", { name: /Create queue/i }),
		).toBeVisible();
		await expect(page.getByLabel("Filter by name")).toBeVisible();
		await expect(page.locator("[data-queue-table]")).toBeVisible();
	});
});
