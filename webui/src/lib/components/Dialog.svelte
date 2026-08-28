<script lang="ts">
    import type { Snippet } from "svelte";
    import { X } from "@lucide/svelte";
    import { fade, scale } from "svelte/transition";

    let {
        open = $bindable(false),
        title,
        body,
    }: {
        open?: boolean;
        title: string;
        body: Snippet;
    } = $props();

    function close() {
        open = false;
    }
    function onKeydown(event: KeyboardEvent) {
        if (!open || event.key !== "Escape") return;
        event.preventDefault();
        close();
    }
</script>

<svelte:window onkeydown={onKeydown} />

{#if open}
    <div class="fixed inset-0 z-50 flex items-center justify-center p-5">
        <!-- Backdrop -->
        <button
            type="button"
            aria-label="Close dialog"
            class="absolute inset-0 bg-black/60"
            onclick={close}
            transition:fade={{ duration: 150 }}
        ></button>

        <!-- Panel -->
        <div
            role="dialog"
            aria-modal="true"
            aria-label={title}
            class="relative w-full max-w-md rounded-xl border border-border bg-card text-card-foreground shadow-lg"
            transition:scale={{ duration: 150, start: 0.95 }}
        >
            <div class="flex flex-row items-center gap-3 px-4 py-3">
                <h2 class="flex-1 text-lg">{title}</h2>
                <button
                    type="button"
                    aria-label="Close"
                    class="shrink-0 rounded-lg p-1"
                    onclick={close}
                >
                    <X class="size-5" />
                </button>
            </div>

            <div class="px-4 py-3">
                {@render body()}
            </div>
        </div>
    </div>
{/if}
