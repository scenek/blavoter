import assert from "node:assert/strict";
import test from "node:test";

import { descriptionForEditing, renderDescription } from "./description.mjs";

function descriptionElement() {
    const document = {
        createElement(tagName) {
            return { nodeType: "element", tagName: tagName.toUpperCase() };
        },
        createTextNode(textContent) {
            return { nodeType: "text", textContent };
        }
    };
    return {
        ownerDocument: document,
        children: [],
        replaceChildren(...children) {
            this.children = children;
        }
    };
}

test("renderDescription creates line breaks without interpreting other HTML", () => {
    const element = descriptionElement();

    renderDescription(element, "First<br />Second <strong>bold</strong><br>Third");

    assert.deepEqual(element.children, [
        { nodeType: "text", textContent: "First" },
        { nodeType: "element", tagName: "BR" },
        { nodeType: "text", textContent: "Second <strong>bold</strong>" },
        { nodeType: "element", tagName: "BR" },
        { nodeType: "text", textContent: "Third" }
    ]);
});

test("renderDescription also displays legacy raw newlines", () => {
    const element = descriptionElement();

    renderDescription(element, "First\r\nSecond\rThird\nFourth");

    assert.equal(element.children.filter(node => node.tagName === "BR").length, 3);
});

test("descriptionForEditing restores stored breaks to textarea newlines", () => {
    assert.equal(descriptionForEditing("First<br />Second<br>Third"), "First\nSecond\nThird");
});
