import "../css/app.css";
import "../js/app";

type MessageAttribute = {
	name: string;
	value: string;
};

type ReceivedMessage = {
	id: string;
	body: string;
	receiptHandle: string;
	receiveCount: number;
	attributes: MessageAttribute[];
};

type SendMessageResponse = {
	message: string;
};

type ReceiveMessagesResponse = {
	messages: ReceivedMessage[];
};

type DeleteMessageResponse = {
	message: string;
};

document.addEventListener("DOMContentLoaded", () => {
	const page = document.querySelector<HTMLElement>(
		'[data-page="send-receive"]',
	);
	if (!page) {
		return;
	}

	const queuePath = page.dataset.queueUrl;
	if (!queuePath) {
		console.warn("Queue URL missing from send/receive page dataset.");
		return;
	}

	const sendForm = page.querySelector<HTMLFormElement>("[data-send-form]");
	const feedback = page.querySelector<HTMLElement>("[data-send-feedback]");
	const supportsGroups = page.dataset.supportsGroups === "true";
	const requiresDedup = page.dataset.requiresDedup === "true";
	const addAttributeButton = page.querySelector<HTMLButtonElement>(
		"[data-attribute-add]",
	);
	const attributesContainer = page.querySelector<HTMLElement>(
		"[data-attribute-rows]",
	);
	const attributeTemplate = page.querySelector<HTMLTemplateElement>(
		"#attribute-row-template",
	);

	const receiveForm = page.querySelector<HTMLFormElement>(
		"[data-receive-form]",
	);
	const pollButton =
		receiveForm?.querySelector<HTMLButtonElement>("[data-poll-button]");
	const pollButtonDefaultLabel = pollButton?.textContent?.trim() ?? "";
	const pollButtonDisabledClasses = [
		"cursor-not-allowed",
		"opacity-60",
		"pointer-events-none",
	];
	const setPollButtonState = (isPolling: boolean) => {
		if (!pollButton) {
			return;
		}
		pollButton.disabled = isPolling;
		pollButton.textContent = isPolling
			? "Polling..."
			: pollButtonDefaultLabel || "Poll for messages";
		pollButtonDisabledClasses.forEach((className) => {
			pollButton.classList.toggle(className, isPolling);
		});
		if (isPolling) {
			pollButton.setAttribute("aria-busy", "true");
		} else {
			pollButton.removeAttribute("aria-busy");
		}
	};
	const maxMessagesInput = receiveForm?.querySelector<HTMLInputElement>(
		'[name="max_messages"]',
	);
	const waitTimeInput = receiveForm?.querySelector<HTMLInputElement>(
		'[name="wait_time_seconds"]',
	);
	const receiveList = page.querySelector<HTMLUListElement>(
		"[data-receive-list]",
	);
	const emptyState = page.querySelector<HTMLElement>("[data-receive-empty]");
	const statusBox = page.querySelector<HTMLElement>("[data-receive-status]");
	const messageTemplate = page.querySelector<HTMLTemplateElement>(
		"#receive-message-template",
	);
	const messageGroupInput = sendForm?.querySelector<HTMLInputElement>(
		'input[name="message_group_id"]',
	);
	const messageDedupInput = sendForm?.querySelector<HTMLInputElement>(
		'input[name="message_deduplication_id"]',
	);

	if (supportsGroups && messageGroupInput) {
		messageGroupInput.required = true;
	}

	if (messageDedupInput) {
		messageDedupInput.required = requiresDedup;
	}

	const successClasses = ["border-green-400", "bg-green-50", "text-green-700"];
	const errorClasses = ["border-red-400", "bg-red-50", "text-red-700"];
	const infoClasses = ["border-blue-400", "bg-blue-50", "text-blue-700"];

	const statusVariants = {
		info: ["border-slate-200", "bg-slate-50", "text-slate-700"],
		success: ["border-green-400", "bg-green-50", "text-green-700"],
		error: ["border-red-400", "bg-red-50", "text-red-700"],
	} as const;
	const statusVariantClasses = new Set(Object.values(statusVariants).flat());

	let currentMessages: ReceivedMessage[] = [];

	const postJSON = async <T>(path: string, payload: unknown): Promise<T> => {
		const response = await fetch(path, {
			method: "POST",
			headers: { "Content-Type": "application/json" },
			body: JSON.stringify(payload),
		});

		let data: unknown;
		try {
			data = await response.json();
		} catch (_error) {
			data = null;
		}

		if (!response.ok) {
			const message =
				typeof data === "object" &&
				data !== null &&
				"error" in data &&
				typeof (data as { error: unknown }).error === "string"
					? (data as { error: string }).error
					: `Request failed with status ${response.status}`;
			throw new Error(message);
		}

		return data as T;
	};

	const deleteMessageFromQueue = async (
		message: ReceivedMessage,
		button: HTMLButtonElement,
		originalLabel: string,
	) => {
		button.disabled = true;
		button.textContent = "Deleting...";
		setStatus("info", "Deleting message…");

		try {
			const response = await postJSON<DeleteMessageResponse>(
				`/queues/${queuePath}/messages/delete`,
				{ receiptHandle: message.receiptHandle },
			);
			const successMessage =
				response?.message ?? "Message deleted from the queue.";
			setStatus("success", successMessage);
			currentMessages = currentMessages.filter(
				(candidate) => candidate.receiptHandle !== message.receiptHandle,
			);
			renderMessages(currentMessages);
		} catch (error) {
			const messageText =
				error instanceof Error ? error.message : "Failed to delete message.";
			setStatus("error", messageText);
			button.disabled = false;
			button.textContent = originalLabel;
		}
	};

	const createAttributeRow = () => {
		if (!attributeTemplate || !attributesContainer) {
			return;
		}

		const fragment = attributeTemplate.content.cloneNode(
			true,
		) as DocumentFragment;
		const row = fragment.querySelector<HTMLElement>("[data-attribute-row]");
		if (!row) {
			return;
		}

		const removeButton = row.querySelector<HTMLButtonElement>(
			"[data-attribute-remove]",
		);
		if (removeButton) {
			removeButton.addEventListener("click", () => {
				row.remove();
				if (
					attributesContainer.querySelectorAll("[data-attribute-row]")
						.length === 0
				) {
					createAttributeRow();
				}
			});
		}

		attributesContainer.appendChild(fragment);
	};

	const resetAttributeRows = () => {
		if (!attributesContainer) {
			return;
		}
		attributesContainer.replaceChildren();
		createAttributeRow();
	};

	const gatherAttributes = (): MessageAttribute[] => {
		if (!attributesContainer) {
			return [];
		}

		const rows = Array.from(
			attributesContainer.querySelectorAll<HTMLElement>("[data-attribute-row]"),
		);
		const attributes: MessageAttribute[] = [];
		rows.forEach((row) => {
			const nameInput = row.querySelector<HTMLInputElement>(
				'input[name="attribute_name[]"]',
			);
			const valueInput = row.querySelector<HTMLInputElement>(
				'input[name="attribute_value[]"]',
			);
			if (!nameInput || !valueInput) {
				return;
			}
			const name = nameInput.value.trim();
			if (name === "") {
				return;
			}
			attributes.push({ name, value: valueInput.value });
		});
		return attributes;
	};

	resetAttributeRows();

	addAttributeButton?.addEventListener("click", (event) => {
		event.preventDefault();
		createAttributeRow();
	});

	sendForm?.addEventListener("reset", () => {
		window.requestAnimationFrame(() => {
			resetAttributeRows();
		});
	});

	const setFeedback = (kind: "success" | "error" | "info", message: string) => {
		if (!feedback) {
			return;
		}

		if (message.trim() === "") {
			feedback.classList.add("hidden");
			return;
		}

		feedback.textContent = message;
		feedback.classList.remove(
			"hidden",
			...successClasses,
			...errorClasses,
			...infoClasses,
		);

		if (kind === "success") {
			feedback.classList.add(...successClasses);
		} else if (kind === "error") {
			feedback.classList.add(...errorClasses);
		} else {
			feedback.classList.add(...infoClasses);
		}
	};

	const setStatus = (kind: "info" | "success" | "error", message: string) => {
		if (!statusBox) {
			return;
		}

		statusBox.textContent = message;
		statusBox.classList.remove("hidden", ...statusVariantClasses);
		statusBox.classList.add(...statusVariants[kind]);
	};

	const renderMessages = (messages: ReceivedMessage[]) => {
		if (!receiveList || !messageTemplate) {
			return;
		}

		currentMessages = [...messages];
		receiveList.innerHTML = "";
		if (messages.length === 0) {
			receiveList.classList.add("hidden");
			emptyState?.classList.remove("hidden");
			return;
		}

		const fragment = document.createDocumentFragment();
		messages.forEach((message) => {
			const content = messageTemplate.content.cloneNode(
				true,
			) as DocumentFragment;
			const idElement = content.querySelector<HTMLElement>("[data-message-id]");
			const bodyElement = content.querySelector<HTMLElement>(
				"[data-message-body]",
			);
			const countElement = content.querySelector<HTMLElement>(
				"[data-receive-count]",
			);
			const attributesElement = content.querySelector<HTMLElement>(
				"[data-message-attributes]",
			);

			if (idElement) {
				idElement.textContent = message.id;
			}
			if (bodyElement) {
				bodyElement.textContent = message.body;
			}
			if (countElement) {
				const count = message.receiveCount;
				countElement.textContent = `Received ×${count}`;
			}

			const deleteButton = content.querySelector<HTMLButtonElement>(
				"[data-message-delete]",
			);
			if (deleteButton) {
				const originalLabel = deleteButton.textContent ?? "";
				deleteButton.addEventListener("click", () => {
					void deleteMessageFromQueue(message, deleteButton, originalLabel);
				});
			}

			if (attributesElement) {
				attributesElement.innerHTML = "";
				if (message.attributes.length === 0) {
					const empty = document.createElement("p");
					empty.className = "text-xs text-slate-500";
					empty.textContent = "No message attributes returned.";
					attributesElement.appendChild(empty);
				} else {
					message.attributes.forEach((attribute) => {
						const wrapper = document.createElement("div");
						wrapper.className = "space-y-1";

						const name = document.createElement("p");
						name.className = "text-xs tracking-wide text-slate-500";
						name.textContent = attribute.name;

						const value = document.createElement("p");
						value.className = "break-all font-mono text-sm text-slate-800";
						value.textContent = attribute.value;

						wrapper.append(name, value);
						attributesElement.appendChild(wrapper);
					});
				}
			}

			fragment.appendChild(content);
		});

		receiveList.appendChild(fragment);
		receiveList.classList.remove("hidden");
		emptyState?.classList.add("hidden");
	};

	sendForm?.addEventListener("submit", async (event) => {
		event.preventDefault();
		if (!sendForm) {
			return;
		}

		const submitButton = sendForm.querySelector<HTMLButtonElement>(
			'button[type="submit"]',
		);

		const formData = new FormData(sendForm);
		const body = (formData.get("message_body") as string | null)?.trim() ?? "";
		const messageGroupId =
			(formData.get("message_group_id") as string | null)?.trim() ?? "";
		const delayRaw =
			(formData.get("delivery_delay") as string | null)?.trim() ?? "";

		if (!body) {
			setFeedback("error", "Message body is required before sending.");
			return;
		}

		let delaySeconds: number | null = null;
		if (delayRaw !== "") {
			delaySeconds = Number(delayRaw);
			if (
				Number.isNaN(delaySeconds) ||
				delaySeconds < 0 ||
				delaySeconds > 900
			) {
				setFeedback(
					"error",
					"Delivery delay must be between 0 and 900 seconds.",
				);
				return;
			}
		}

		const attributes = gatherAttributes();

		if (supportsGroups && messageGroupId === "") {
			setFeedback(
				"error",
				"Message group ID is required when sending to a FIFO queue.",
			);
			return;
		}

		const messageDeduplicationId =
			(formData.get("message_deduplication_id") as string | null)?.trim() ?? "";

		if (requiresDedup && messageDeduplicationId === "") {
			setFeedback(
				"error",
				"Message deduplication ID is required when content-based deduplication is disabled.",
			);
			return;
		}

		const payload: {
			body: string;
			messageGroupId?: string;
			messageDeduplicationId?: string;
			delaySeconds?: number;
			attributes?: MessageAttribute[];
		} = { body };

		if (messageGroupId !== "") {
			payload.messageGroupId = messageGroupId;
		}

		if (messageDeduplicationId !== "") {
			payload.messageDeduplicationId = messageDeduplicationId;
		}
		if (delaySeconds !== null) {
			payload.delaySeconds = delaySeconds;
		}
		if (attributes.length > 0) {
			payload.attributes = attributes;
		}

		try {
			if (submitButton) {
				submitButton.disabled = true;
			}
			setFeedback("info", "Sending message…");
			const response = await postJSON<SendMessageResponse>(
				`/queues/${queuePath}/messages`,
				payload,
			);
			const message =
				response?.message ?? "Message sent to the queue successfully.";
			setFeedback("success", message);
			sendForm.reset();
		} catch (error) {
			const message =
				error instanceof Error ? error.message : "Failed to send message.";
			setFeedback("error", message);
		} finally {
			if (submitButton) {
				submitButton.disabled = false;
			}
		}
	});

	receiveForm?.addEventListener("submit", async (event) => {
		event.preventDefault();
		if (!receiveForm || !pollButton) {
			return;
		}

		const formData = new FormData(receiveForm);
		const maxMessagesRaw =
			(formData.get("max_messages") as string | null)?.trim() ?? "";
		const waitTimeRaw =
			(formData.get("wait_time_seconds") as string | null)?.trim() ?? "";

		const fallbackMaxMessages = 10;
		const fallbackWaitTime = 20;

		let maxMessages = fallbackMaxMessages;
		if (maxMessagesRaw !== "") {
			const parsed = Number(maxMessagesRaw);
			if (
				Number.isNaN(parsed) ||
				!Number.isInteger(parsed) ||
				parsed < 1 ||
				parsed > 10
			) {
				setStatus(
					"error",
					"Max messages must be a whole number between 1 and 10.",
				);
				return;
			}
			maxMessages = parsed;
		} else if (maxMessagesInput) {
			maxMessagesInput.value = String(fallbackMaxMessages);
		}

		let waitTimeSeconds = fallbackWaitTime;
		if (waitTimeRaw !== "") {
			const parsed = Number(waitTimeRaw);
			if (
				Number.isNaN(parsed) ||
				!Number.isInteger(parsed) ||
				parsed < 0 ||
				parsed > 20
			) {
				setStatus(
					"error",
					"Wait time must be a whole number between 0 and 20 seconds.",
				);
				return;
			}
			waitTimeSeconds = parsed;
		} else if (waitTimeInput) {
			waitTimeInput.value = String(fallbackWaitTime);
		}

		const payload = {
			maxMessages,
			waitTimeSeconds,
		};

		setPollButtonState(true);

		setStatus("info", "Polling queue for messages…");
		receiveList?.classList.add("hidden");
		emptyState?.classList.add("hidden");

		try {
			const { messages } = await postJSON<ReceiveMessagesResponse>(
				`/queues/${queuePath}/messages/poll`,
				payload,
			);
			renderMessages(messages);
			const count = messages.length;
			if (count === 0) {
				setStatus("success", "No messages were returned.");
			} else {
				const suffix = count === 1 ? "" : "s";
				setStatus("success", `Retrieved ${count} message${suffix}.`);
			}
		} catch (error) {
			const message =
				error instanceof Error ? error.message : "Failed to poll messages.";
			setStatus("error", message);
			receiveList?.classList.add("hidden");
			emptyState?.classList.remove("hidden");
		} finally {
			setPollButtonState(false);
		}
	});
});
