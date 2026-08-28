<script lang="ts">
	import { page } from "$app/state";
	import { resolve } from "$app/paths";
	import api from "$lib/api";
	import { profileStore } from "$lib/profile/profiles.svelte";
	import { Check, ChevronLeft, ChevronsUpDown, Columns2, List, LoaderCircle, Pencil, Save, Trash2, X } from "@lucide/svelte";
	import { sortable } from "$lib/actions/sortable.svelte";
	import { toast } from "svelte-sonner";
	import { cn } from "$lib/misc";
	import { beforeNavigate, goto } from "$app/navigation";
	import { fly, slide } from "svelte/transition";
	import { quickTTSBar } from "$lib/components/QuickTTS.svelte";
	import Dialog from "$lib/components/Dialog.svelte";

    const profileName = $derived(page.params.profile_name!);
    const profile = $derived(profileStore.get(profileName));

    const editButtonClassName = "flex items-center gap-1 h-full px-2 py-1 rounded-lg";

    type Draft = { id: number; text: string };
    let nextId = 0;
    let draft = $state<Draft[]>([]);

    let isEditing = $state(false);
    $effect(() => {
        if (isEditing) quickTTSBar.hide();
        else quickTTSBar.show();
    });

    function editOpen() {
        draft = (profile?.texts ?? []).map((text) => ({ id: nextId++, text }));
        isEditing = true;
    }
    function editCancel() {
        isEditing = false;
    }
    function draftMove(from: number, to: number) {
        const [moved] = draft.splice(from, 1);
        draft.splice(to, 0, moved);
    }
    function draftAdd(text: string) {
        const trimmed = text.trim();
        if (trimmed === "") return;
        draft.push({ id: nextId++, text: trimmed });
    }
    async function editSave() {
        if (!profile) return;
        const texts = draft.map((d) => d.text.trim()).filter((t) => t !== "");
        const saveRes = await profileStore.upsert({ ...profile, texts });
        if (saveRes.err) {
            console.error(`Failed to save changes: ${saveRes.err.toString()}`);
            toast.error("Failed to save changes", { description: saveRes.err.toString() });
            return; // Keep editing on
        }
        isEditing = false;
    }

    // Block nav when in edit mode
    beforeNavigate(({cancel}) => {
        if (!isEditing) return;
        if (confirm("Discard all unsaved changes?")) return;
        cancel();
    });

    // Toggle list/2-columns view
    const columnsToggleLSKey = "app-profile-item-columns-view" as const;
    let columnsToggle = $state(localStorage.getItem(columnsToggleLSKey) === "true");
    $effect(() => {
        localStorage.setItem(columnsToggleLSKey, columnsToggle.toString());
    });

    // Profile editing
    let showRemovalDialog = $state(false);
    async function removeProfile() {
        const res = await profileStore.remove(profileName);
        if (res.err) {
            console.error(res.err.toString());
            toast.error("Failed to delete profile", { description: res.err.toString() });
            return; // Keep editing on
        }
        isEditing = false;
        goto(resolve('/'));
    }
</script>

<svelte:head>
    <title>{profile?.displayName ?? "Profile"} - VoiTTS</title>
</svelte:head>

<div class="p-3 pb-32">
    <div class="flex flex-row gap-3 items-stretch mb-3 select-none">
        <a href={resolve("/")} class="inline-flex items-center gap-1 underline text-lg">
            <ChevronLeft class="size-5" />
            Profiles
        </a>
        <div class="flex-1"></div>
        <div>
            {#if isEditing}
                <!-- Cancel -->
                <button
                    type="button"
                    class={cn(editButtonClassName, "bg-surface text-surface-foreground")}
                    onclick={editCancel}
                >
                    <X class="size-5" />
                    Cancel
                </button>
            {:else}
                <button
                    type="button"
                    class={cn(editButtonClassName, "bg-surface text-surface-foreground")}
                    onclick={editOpen}
                >
                    <Pencil class="size-5" />
                    <span>Edit</span>
                </button>
            {/if}
        </div>
        <div>
            <input
                type="checkbox"
                name="columns-toggle" id="columns-toggle"
                hidden class="hidden"
                bind:checked={columnsToggle}>
            <label
                for="columns-toggle"
                class="block bg-secondary p-1 rounded-lg"
            >
                <div class="relative flex flex-row gap-1">
                    <div
                        class="absolute top-0 left-0 size-8 rounded bg-surface transition-transform {columnsToggle ? "translate-x-9" : ""}"
                    ></div>
                    <List class="relative size-8 p-0.5 transition-colors" color={!columnsToggle ? "var(--color-surface-foreground)" : "var(--color-foreground)"} />
                    <Columns2 class="relative size-8 p-0.5 transition-colors" color={columnsToggle ? "var(--color-surface-foreground)" : "var(--color-foreground)"} />
                </div>
            </label>
        </div>
    </div>

    {#if !profile}
        <p class="text-error">No profile named "{profileName}".</p>
    {:else}
        <div class={cn("flex flex-row justify-between mb-3", isEditing ? "mb-7" : "")}>
            <h1 class="text-xl py-1">
                {profile.displayName}
            </h1>
            {#if isEditing}
                <button
                    type="button"
                    onclick={() => showRemovalDialog = true}
                    class="bg-error rounded px-4"
                    transition:slide={{ axis:'x' }}
                >
                    <Trash2 />
                </button>
            {/if}
        </div>

        <Dialog
            bind:open={showRemovalDialog}
            title="Remove {profile.displayName}?"
        >
            {#snippet body()}
                {let removing = $state(false)}
                {const className = "px-4 py-3 rounded-lg disabled:opacity-60"}
                <div class="grid grid-cols-2 gap-3">
                    <button
                        type="button"
                        onclick={() => showRemovalDialog = false}
                        disabled={removing}
                        class={cn(className, "bg-surface text-surface-foreground")}
                    >
                        Cancel
                    </button>
                    <button
                        type="button"
                        onclick={async () => {
                            removing = true;
                            await removeProfile();
                            removing = false;
                        }}
                        disabled={removing}
                        class={cn(className, "bg-error text-accent-foreground flex flex-row justify-center items-center gap-1")}
                    >
                        {#if removing}
                            <span>Removing</span>
                            <LoaderCircle class="size-5 animate-spin" />
                        {:else}
                            <span>Remove</span>
                        {/if}
                    </button>
                </div>
            {/snippet}
        </Dialog>

        {#if (isEditing ? draft.length : profile.texts.length) === 0}
            <p class="opacity-70">No phrases in this profile yet.</p>
        {:else}
            <div
                class={cn("grid gap-6", (columnsToggle && !isEditing) ? "grid-cols-2" : "grid-cols-1")}
                use:sortable={{ onReorder: draftMove }}
            >
                <!-- Default view -->
                {#if !isEditing}
                    {#each profile.texts as text, i (i)}
                        {let status = $state<'idle'|'sending'|'success'|'fail'>('idle')}
                        {let timeout: ReturnType<typeof setTimeout>}
                        {let statusColor = $derived.by(() => {
                            switch (status) {
                                case "idle": return "bg-surface text-surface-foreground";
                                case "sending": return "bg-amber-400 text-white";
                                case "success": return "bg-green-500 text-white";
                                case "fail": return "bg-error text-white";
                                default: return "bg-surface text-surface-foreground";
                            }
                        })}
                        <button
                            type="button"
                            disabled={status === 'sending'}
                            onclick={async () => {
                                if (status === 'sending') return;
                                clearTimeout(timeout);
                                status = 'sending';

                                const timeoutIdle = () => timeout = setTimeout(() => status = 'idle', 500);

                                const res = await api.sendTTS(text);
                                if (res.err) {
                                    console.error(res.err.toString());
                                    toast.error("Failed to send TTS", { description: res.err.toString() });
                                    status = 'fail';
                                    timeoutIdle();
                                    return;
                                }

                                status = 'success';
                                timeoutIdle();
                            }}
                            class={cn("block rounded-lg p-3 text-left transition-colors", statusColor)}
                        >
                            <span class="block flex-1 text-lg">{text}</span>
                        </button>
                    {/each}

                <!-- Editor view -->
                {:else}
                    {#each draft as item, i (item.id)}
                        <div
                            class="flex flex-row items-stretch rounded-lg bg-surface/80 text-surface-foreground outline outline-dashed outline-border"
                            out:fly={{x:'100%'}}
                        >
                            <button
                                type="button"
                                aria-label="Delete phrase {i + 1}"
                                class="shrink-0 self-center pl-3 pr-1 text-error"
                                onclick={() => draft.splice(i, 1)}
                            >
                                <Trash2 class="size-5" />
                            </button>
                            <textarea
                                class="block min-w-0 flex-1 h-full resize-none field-sizing-content text-lg py-3 px-2 focus:outline-none"
                                onkeydown={(e) => {
                                    if (e.key !== "Enter" || e.shiftKey || e.isComposing) return;
                                    e.preventDefault();
                                }}
                                bind:value={item.text}
                            ></textarea>
                            {#if draft.length > 1}
                                <button
                                    type="button"
                                    data-sortable-handle
                                    aria-label="Reorder phrase {i + 1}"
                                    class="cursor-grab active:cursor-grabbing shrink-0 self-center pr-3 pl-1 opacity-60 touch-none select-none"
                                >
                                    <ChevronsUpDown class="size-5" />
                                </button>
                            {/if}
                        </div>
                    {/each}
                {/if}
            </div>
        {/if}

        <!-- Create text item -->
        {#if isEditing}
            {let text = $state("")}
            {const addText = () => {
                if (text.trim() === "") return;
                draftAdd(text);
                text = "";
            }}
            <div class="mt-7 flex flex-row items-end">
                <textarea
                    placeholder="Add text"
                    class="block flex-1 resize-none field-sizing-content outline outline-dashed text-lg outline-border/50 rounded-lg px-2 py-1 focus:outline-border"
                    bind:value={text}
                    onkeydown={(e) => {
                        if (e.key !== "Enter" || e.shiftKey || e.isComposing) return;
                        e.preventDefault();
                        addText();
                    }}
                ></textarea>
                {#if text.trim().length > 0}
                    <button
                        type="button"
                        class="rounded-lg size-9 ml-3 shrink-0 flex items-center justify-center bg-accent text-accent-foreground"
                        onclick={addText}
                        transition:slide={{ axis:'x' }}
                    >
                        <Check />
                    </button>
                {/if}
            </div>
        {/if}
    {/if}

    <!-- Save button -->
    {#if isEditing}
        {let saving = $state(false)}
        <div
            class="fixed z-100 w-full left-0 bottom-[calc(var(--keyboard-inset,0px)+1.25rem)] px-3"
            transition:fly={{ y: '150%' }}
        >
            <button
                type="button"
                class={cn(editButtonClassName, "justify-center h-fit w-full px-4 py-2 bg-accent text-accent-foreground")}
                disabled={saving}
                onclick={async () => {
                    saving = true;
                    await editSave();
                    saving = false;
                }}
            >
                {#if saving}
                    <LoaderCircle class="size-5 animate-spin" />
                {:else}
                    <Save class="size-5" />
                {/if}
                <span class="text-lg">Save</span>
            </button>
        </div>
    {/if}
</div>

<style>
    textarea {
        &:focus {
            border: none;
        }
        &::placeholder {
            font-style: italic;
        }
    }
</style>
