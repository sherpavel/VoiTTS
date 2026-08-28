<script lang="ts" module>
    let show = $state(true);

    export const quickTTSBar = {
        hide: () => show = false,
        show: () => show = true,
    }
</script>

<script lang="ts">
	import api from "$lib/api";
	import { toast } from "svelte-sonner";
	import { slide } from "svelte/transition";
    import { Send, LoaderCircle } from "@lucide/svelte";

    let barEl = $state<HTMLDivElement | null>(null);
    let quickText = $state<string>("");
    let isQuickTextSending = $state(false);

    // The bar is fixed, so it no longer occupies space. Publish its live height
    // (the textarea auto-grows) for <main> to reserve as padding.
    $effect(() => {
        const root = document.documentElement;
        const el = barEl;
        if (!el) {
            root.style.setProperty('--quick-tts-height', '0px');
            return;
        }

        const ro = new ResizeObserver(() => {
            root.style.setProperty('--quick-tts-height', `${el.offsetHeight}px`);
        });
        ro.observe(el);
        return () => {
            ro.disconnect();
            root.style.setProperty('--quick-tts-height', '0px');
        };
    });

    async function sendQuickText() {
        if (isQuickTextSending) return;

        quickText = quickText.trim();
        if (quickText === "") return;

        isQuickTextSending = true;
        
        const text = quickText;
        quickText = "";
        const res = await api.sendTTS(text);
        if (res.err) {
            quickText = text;
            console.error(res.err.toString());
            toast.error("Failed to send TTS", { description: res.err.toString() });
        }
        
        isQuickTextSending = false;
    }
    function onQuickTextKeydown(event: KeyboardEvent) {
        if (event.key !== "Enter" || event.shiftKey || event.isComposing) return;
        event.preventDefault();
        sendQuickText();
    }
</script>

<!-- Quick text -->
{#if show}
    <div
        bind:this={barEl}
        class="fixed inset-x-0 bottom-(--keyboard-inset,0px) z-40 bg-background px-2 pb-5 pt-2"
        transition:slide={{ axis: 'y' }}
    >
        <div class="flex flex-row items-end rounded-xl bg-secondary text-secondary-foreground p-1.5 shadow-lg">
            <textarea
                bind:value={quickText}
                onkeydown={onQuickTextKeydown}
                name="quick-text" id="quick-text"
                placeholder="Send instant TTS"
                class="max-h-[35dvh] min-h-10 min-w-1/2 flex-1 field-sizing-content resize-none overflow-y-auto rounded-lg px-2 py-1 bg-surface text-surface-foreground"
            ></textarea>
            {#if quickText.trim().length > 0}
                <button
                    type="button"
                    disabled={isQuickTextSending}
                    class="bg-accent text-accent-foreground rounded-lg size-10 ml-2 shrink-0 flex items-center justify-center"
                    onclick={sendQuickText}
                    transition:slide={{axis: 'x'}}
                >
                    {#if !isQuickTextSending}
                        <Send class="-translate-x-0.5 translate-y-0.5" />
                    {:else}
                        <LoaderCircle class="animate-spin" />
                    {/if}
                </button>
            {/if}
        </div>
    </div>
{/if}

<style>
    textarea {
        &:focus {
            outline: 1px solid var(--color-border);
            border: none;
        }
        &::placeholder {
            font-style: italic;
        }
    }
</style>
