import type { Action } from "svelte/action";

export type SortableParams = {
    // Called once per drag, with the indices to move between. Never called
    // when the row was put back where it started.
    onReorder: (from: number, to: number) => void;
    // Selector for the element that starts a drag, matched within a row.
    handle?: string;
};

const DEFAULT_HANDLE = "[data-sortable-handle]";

// Drag-to-reorder for a list container. Every element child of the node is a
// row, and a row is dragged by the descendant matching `handle`.
//
// Pointer events rather than the HTML5 drag-and-drop API, which never fires on
// touch — this interface is mostly used from a phone. Give the handle
// `touch-action: none` or the browser scrolls the page instead of dragging.
//
// The action owns the rows' transform, transition and z-index while a drag is
// running and clears them when it ends, so callers must not set those
// themselves. Nothing is reordered in the DOM: onReorder is handed the indices
// and the caller reorders its own data.
//
// The parameter is read once, since actions do not re-run when their argument
// changes — pass a closure over the live state rather than a snapshot of it.
export const sortable: Action<HTMLElement, SortableParams> = (node, params) => {
    const handleSelector = params.handle ?? DEFAULT_HANDLE;

    // Row geometry, measured once per drag: mid-drag the rows carry transforms,
    // so reading them back would return the shifted positions, not the slots.
    let rows: HTMLElement[] = [];
    let rowMids: number[] = [];
    let slotHeight = 0;
    let grabbedAt = 0;

    let dragIndex = -1;
    let dropIndex = -1;
    let pointerId = -1;
    let handleEl: HTMLElement | null = null;

    // Which row, if any, the event happened inside of
    function rowOf(event: Event): { index: number; handle: HTMLElement } | null {
        const target = event.target as Element | null;
        const handle = target?.closest(handleSelector) as HTMLElement | null;
        if (!handle) return null;

        const children = Array.from(node.children) as HTMLElement[];
        const index = children.findIndex((row) => row.contains(handle));
        if (index < 0) return null;

        rows = children;
        return { index, handle };
    }

    function paint(offset: number) {
        rows.forEach((row, i) => {
            let y = 0;
            if (i === dragIndex) y = offset;
            else if (i > dragIndex && i <= dropIndex) y = -slotHeight;
            else if (i < dragIndex && i >= dropIndex) y = slotHeight;

            row.style.transform = y === 0 ? "" : `translateY(${y}px)`;
            // The dragged row tracks the pointer; the rest ease into their slot
            row.style.transition = i === dragIndex ? "none" : "transform 150ms";
            row.style.zIndex = i === dragIndex ? "10" : "";
        });
    }

    function reset() {
        for (const row of rows) {
            row.style.transform = "";
            row.style.transition = "";
            row.style.zIndex = "";
        }
        rows = [];
        rowMids = [];
        dragIndex = -1;
        dropIndex = -1;
        pointerId = -1;
        handleEl = null;
    }

    function onPointerDown(event: PointerEvent) {
        if (dragIndex >= 0) return; // a second finger during a drag
        const found = rowOf(event);
        if (!found) return;

        // Stops the browser selecting text under the drag. It also stops the
        // handle taking focus, which the keyboard path needs, so do that here.
        event.preventDefault();
        found.handle.focus();

        found.handle.setPointerCapture(event.pointerId);
        pointerId = event.pointerId;
        handleEl = found.handle;

        const rects = rows.map((row) => row.getBoundingClientRect());
        rowMids = rects.map((r) => r.top + r.height / 2);
        // The gap the list is laid out with, so the opened slot matches it
        const gap = rects.length > 1 ? rects[1].top - (rects[0].top + rects[0].height) : 0;
        slotHeight = rects[found.index].height + gap;

        grabbedAt = event.clientY;
        dragIndex = found.index;
        dropIndex = found.index;
        paint(0);
    }

    function onPointerMove(event: PointerEvent) {
        if (dragIndex < 0 || event.pointerId !== pointerId) return;

        // Relative to where the handle was grabbed, so the row moves with the
        // pointer instead of jumping to centre itself under it.
        const offset = event.clientY - grabbedAt;

        // Walk outwards while the dragged row's centre has passed a neighbour's.
        // Comparing centres rather than counting fixed steps keeps this right
        // when rows are different heights.
        const centre = rowMids[dragIndex] + offset;
        let target = dragIndex;
        while (target > 0 && centre < rowMids[target - 1]) target--;
        while (target < rowMids.length - 1 && centre > rowMids[target + 1]) target++;

        dropIndex = target;
        paint(offset);
    }

    function onPointerUp(event: PointerEvent) {
        if (dragIndex < 0 || event.pointerId !== pointerId) return;

        const from = dragIndex;
        const to = dropIndex;

        // Cleared before the callback, so the list re-renders once, already in
        // its new order, rather than flashing back through the old one first.
        if (handleEl?.hasPointerCapture(event.pointerId)) {
            handleEl.releasePointerCapture(event.pointerId);
        }
        reset();

        if (from !== to) params.onReorder(from, to);
    }

    function onPointerCancel(event: PointerEvent) {
        if (dragIndex < 0 || event.pointerId !== pointerId) return;
        reset();
    }

    // The handle is expected to be focusable, so arrows move a row without a
    // pointer. Key the list by identity and focus rides along with the row.
    function onKeyDown(event: KeyboardEvent) {
        if (event.key !== "ArrowUp" && event.key !== "ArrowDown") return;

        const found = rowOf(event);
        if (!found) return;

        const to = found.index + (event.key === "ArrowUp" ? -1 : 1);
        rows = [];
        if (to < 0 || to >= node.children.length) return;

        event.preventDefault();
        params.onReorder(found.index, to);
    }

    $effect(() => {
        // Capture retargets the move and up events to the handle, and they
        // bubble back through here, so the container is the only listener.
        node.addEventListener("pointerdown", onPointerDown);
        node.addEventListener("pointermove", onPointerMove);
        node.addEventListener("pointerup", onPointerUp);
        node.addEventListener("pointercancel", onPointerCancel);
        node.addEventListener("keydown", onKeyDown);

        return () => {
            reset();
            node.removeEventListener("pointerdown", onPointerDown);
            node.removeEventListener("pointermove", onPointerMove);
            node.removeEventListener("pointerup", onPointerUp);
            node.removeEventListener("pointercancel", onPointerCancel);
            node.removeEventListener("keydown", onKeyDown);
        };
    });
};
