import "../css/app.css";
import "../js/app";

document.addEventListener("DOMContentLoaded", () => {
	const page = document.querySelector<HTMLElement>('[data-page="queue"]');
	if (!page) {
		return;
	}

	type ActiveModal = {
		element: HTMLElement;
		cleanup: () => void;
	};

	let activeModal: ActiveModal | null = null;

	const showModal = (modal: HTMLElement) => {
		if (activeModal?.element === modal) {
			return;
		}

		if (activeModal) {
			activeModal.cleanup();
		}

		const cancelButtons = Array.from(
			modal.querySelectorAll<HTMLElement>("[data-confirm-cancel]"),
		);

		const handleKeydown = (event: KeyboardEvent) => {
			if (event.key === "Escape") {
				event.preventDefault();
				cleanup();
			}
		};

		const overlayHandler = (event: MouseEvent) => {
			if (event.target === modal) {
				cleanup();
			}
		};

		const previouslyFocused = document.activeElement as HTMLElement | null;

		let cleanup: () => void = () => {};

		const cancelHandlers = cancelButtons.map((button) => {
			const handler = () => {
				cleanup();
			};
			button.addEventListener("click", handler);
			return { button, handler };
		});

		cleanup = () => {
			modal.classList.add("hidden");
			modal.classList.remove("flex");
			document.removeEventListener("keydown", handleKeydown);
			modal.removeEventListener("click", overlayHandler);
			cancelHandlers.forEach(({ button, handler }) => {
				button.removeEventListener("click", handler);
			});
			if (previouslyFocused) {
				previouslyFocused.focus();
			}
			activeModal = null;
		};

		document.addEventListener("keydown", handleKeydown);
		modal.addEventListener("click", overlayHandler);

		modal.classList.remove("hidden");
		modal.classList.add("flex");

		const submitButton = modal.querySelector<HTMLButtonElement>(
			'button[type="submit"]',
		);
		submitButton?.focus();

		activeModal = { element: modal, cleanup };
	};

	const triggers = page.querySelectorAll<HTMLElement>("[data-confirm-trigger]");
	triggers.forEach((trigger) => {
		const target = trigger.dataset.confirmTrigger;
		if (!target) {
			return;
		}

		const modal = page.querySelector<HTMLElement>(
			`[data-confirm-modal="${target}"]`,
		);
		if (!modal) {
			return;
		}

		trigger.addEventListener("click", () => {
			showModal(modal);
		});
	});
});
