const lineBreakPattern = /<br\s*\/?>|\r\n?|\n/gi;

export function renderDescription(element, description, fallback = "") {
    const value = description || fallback;
    const lines = String(value).split(lineBreakPattern);
    const nodes = [];

    lines.forEach((line, index) => {
        if (index > 0) nodes.push(element.ownerDocument.createElement("br"));
        nodes.push(element.ownerDocument.createTextNode(line));
    });

    element.replaceChildren(...nodes);
}

export function descriptionForEditing(description) {
    return String(description || "").replace(/<br\s*\/?>/gi, "\n");
}
