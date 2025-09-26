import "../css/app.css";
import "../js/app";

// Helper script that enables client-side filtering on the queue list.

document.addEventListener("DOMContentLoaded", () => {
	const filterInput = document.querySelector<HTMLInputElement>("#queue-filter");
	const rows = Array.from(
		document.querySelectorAll<HTMLTableRowElement>("[data-queue-row]"),
	);
	if (!filterInput || rows.length === 0) {
		return;
	}

	const emptyState = document.createElement("tr");
	emptyState.innerHTML = `<td class="px-4 py-6 text-center text-slate-500" colspan="8">No queues match the current filter.</td>`;

	const tableBody =
		document.querySelector<HTMLTableSectionElement>("#queue-table-body");
	if (!tableBody) {
		return;
	}

	const applyFilter = () => {
		const keyword = filterInput.value.trim().toLowerCase();
		let visibleCount = 0;

		rows.forEach((row) => {
			const queueName = row.dataset.queueName ?? "";
			const matches = queueName.toLowerCase().includes(keyword);
			row.style.display = matches ? "" : "none";
			if (matches) {
				visibleCount += 1;
			}
		});

		if (visibleCount === 0) {
			if (!tableBody.contains(emptyState)) {
				tableBody.appendChild(emptyState);
			}
		} else if (tableBody.contains(emptyState)) {
			tableBody.removeChild(emptyState);
		}
	};

	filterInput.addEventListener("input", applyFilter);
});
