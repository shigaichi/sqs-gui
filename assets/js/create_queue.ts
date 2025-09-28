import "../css/app.css";
import "../js/app";

// Helper script for managing FIFO suffixes and checkbox state on the create queue page.

document.addEventListener("DOMContentLoaded", () => {
	const nameInput = document.querySelector<HTMLInputElement>("#queue-name");
	const typeSelect = document.querySelector<HTMLSelectElement>("#queue-type");
	const dedupCheckbox = document.querySelector<HTMLInputElement>(
		"#content-deduplication",
	);

	if (!nameInput || !typeSelect || !dedupCheckbox) {
		return;
	}

	const ensureFifoSuffix = () => {
		if (typeSelect.value !== "fifo") {
			return;
		}
		const base = nameInput.value.replace(/\.fifo$/i, "");
		nameInput.value = `${base}.fifo`;
	};

	const stripFifoSuffix = () => {
		nameInput.value = nameInput.value.replace(/\.fifo$/i, "");
	};

	const syncDeduplication = () => {
		const isFifo = typeSelect.value === "fifo";
		dedupCheckbox.disabled = !isFifo;
		if (!isFifo) {
			dedupCheckbox.checked = false;
		}
	};

	typeSelect.addEventListener("change", () => {
		if (typeSelect.value === "fifo") {
			ensureFifoSuffix();
		} else {
			stripFifoSuffix();
		}
		syncDeduplication();
	});

	nameInput.addEventListener("blur", () => {
		if (typeSelect.value === "fifo") {
			ensureFifoSuffix();
		}
	});

	syncDeduplication();
});
