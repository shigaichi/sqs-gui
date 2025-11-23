import { expect, test } from "@playwright/test";

test.describe("Create queue page", () => {
	test.beforeEach(async ({ page }) => {
		await page.goto(`/create-queue`);
	});

	test("shows default form state", async ({ page }) => {
		await expect(page.locator('[data-page="create-queue"]')).toBeVisible();
		await expect(
			page.getByRole("heading", { level: 1, name: "Create queue" }),
		).toBeVisible();

		const nameInput = page.getByLabel("Queue name");
		const typeSelect = page.getByLabel("Queue type");
		const dedupCheckbox = page.getByLabel(
			"Enable content-based deduplication (FIFO only)",
		);

		await expect(typeSelect).toHaveValue("standard");
		await expect(dedupCheckbox).toBeDisabled();
		await expect(dedupCheckbox).not.toBeChecked();
		await expect(page.getByLabel("Delay seconds")).toHaveAttribute(
			"placeholder",
			"0",
		);
		await expect(
			page.getByLabel("Message retention (seconds)"),
		).toHaveAttribute("placeholder", "345600");
		await expect(
			page.getByLabel("Visibility timeout (seconds)"),
		).toHaveAttribute("placeholder", "30");
		await expect(page.getByRole("link", { name: "Cancel" })).toHaveAttribute(
			"href",
			"/queues",
		);
		await expect(nameInput).toBeVisible();
	});

	test("switching to FIFO enforces suffix and enables deduplication", async ({
		page,
	}) => {
		const nameInput = page.getByLabel("Queue name");
		const typeSelect = page.getByLabel("Queue type");
		const dedupCheckbox = page.getByLabel(
			"Enable content-based deduplication (FIFO only)",
		);

		await nameInput.fill("orders");
		await typeSelect.selectOption("fifo");

		await expect(nameInput).toHaveValue("orders.fifo");
		await expect(dedupCheckbox).toBeEnabled();
		await expect(dedupCheckbox).not.toBeChecked();

		// Switching to FIFO again should not duplicate the suffix.
		await nameInput.fill("orders.fifo");
		await typeSelect.selectOption("fifo");
		await expect(nameInput).toHaveValue("orders.fifo");
	});

	test("switching back to Standard strips suffix and disables deduplication", async ({
		page,
	}) => {
		const nameInput = page.getByLabel("Queue name");
		const typeSelect = page.getByLabel("Queue type");
		const dedupCheckbox = page.getByLabel(
			"Enable content-based deduplication (FIFO only)",
		);

		await nameInput.fill("payments");
		await typeSelect.selectOption("fifo");
		await expect(nameInput).toHaveValue("payments.fifo");
		await expect(dedupCheckbox).toBeEnabled();

		await typeSelect.selectOption("standard");

		await expect(nameInput).toHaveValue("payments");
		await expect(dedupCheckbox).toBeDisabled();
		await expect(dedupCheckbox).not.toBeChecked();
	});

	test("blur re-applies FIFO suffix after manual removal", async ({ page }) => {
		const nameInput = page.getByLabel("Queue name");
		const typeSelect = page.getByLabel("Queue type");

		await nameInput.fill("deliveries");
		await typeSelect.selectOption("fifo");
		await expect(nameInput).toHaveValue("deliveries.fifo");

		await nameInput.fill("deliveries");
		await nameInput.blur();

		await expect(nameInput).toHaveValue("deliveries.fifo");
	});

	test("shows validation error and preserves form values on invalid submit", async ({
		page,
	}) => {
		const nameInput = page.getByLabel("Queue name");
		await nameInput.fill("invalid-delay");

		await page.getByLabel("Delay seconds").fill("9999");
		await page.getByLabel("Message retention (seconds)").fill("120");
		await page.getByLabel("Visibility timeout (seconds)").fill("45");
		await page.locator("form").evaluate((form) => {
			(form as HTMLFormElement).noValidate = true;
		});

		await page.getByRole("button", { name: "Create queue" }).click();

		await expect(
			page.getByText("Delay seconds must be between 0 and 900", {
				exact: false,
			}),
		).toBeVisible();
		await expect(nameInput).toHaveValue("invalid-delay");
		await expect(page.getByLabel("Delay seconds")).toHaveValue("9999");
		await expect(page.getByLabel("Message retention (seconds)")).toHaveValue(
			"120",
		);
		await expect(page.getByLabel("Visibility timeout (seconds)")).toHaveValue(
			"45",
		);
	});

	test("submits valid form, redirects, and shows success flash", async ({
		page,
	}) => {
		const queueName = `pw-queue-${Date.now()}`;
		await page.getByLabel("Queue name").fill(queueName);
		await page.getByLabel("Queue type").selectOption("standard");
		await page.getByLabel("Delay seconds").fill("1");
		await page.getByLabel("Message retention (seconds)").fill("120");
		await page.getByLabel("Visibility timeout (seconds)").fill("30");

		await page.getByRole("button", { name: "Create queue" }).click();

		await expect(page).toHaveURL(
			new RegExp(`/queues\\?created=${encodeURIComponent(queueName)}`),
		);
		await expect(page.locator("[data-queue-flash]")).toContainText(queueName);
		await expect(
			page.locator(`[data-queue-row][data-queue-name="${queueName}"]`),
		).toBeVisible();
	});

	test("cancel link returns to queue list without submitting", async ({
		page,
	}) => {
		await page.getByRole("link", { name: "Cancel" }).click();
		await expect(page).toHaveURL(/\/queues$/);
		await expect(page.locator('[data-page="queues"]')).toBeVisible();
	});
});
