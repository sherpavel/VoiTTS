<script lang="ts">
	import { resolve } from "$app/paths";
	import Dialog from "$lib/components/Dialog.svelte";
	import { profileStore, Status } from "$lib/profile/profiles.svelte";
	import { ChevronsUpDown, LoaderCircle, Plus } from "@lucide/svelte";
	import { sortable } from "$lib/actions/sortable.svelte";
	import { toast } from "svelte-sonner";
	import { slide } from "svelte/transition";

    let newProfileDialog = $state(false);
    let newProfileName = $state("");
    function profileDialogOpen() {
        newProfileName = "";
        newProfileDialog = true;
    }
    function profileDialogClose() {
        newProfileDialog = false;
        newProfileName = "";
    }
    async function createProfile() {
        const res = await profileStore.create(newProfileName);
        if (res.err) {
            console.error(res.err);
            toast.error("Failed to create new profile", { description: res.err.toString() });
            return;
        }
        profileDialogClose();
    }

    async function moveProfile(from: number, to: number) {
        const names = profileStore.list.map((p) => p.name);
        const [moved] = names.splice(from, 1);
        names.splice(to, 0, moved);

        const res = await profileStore.reorder(names);
        if (res.err) {
            console.error(res.err.toString());
            toast.error("Failed to reorder profiles", { description: res.err.toString() });
        }
    }
</script>

<!-- <nav class="flex justify-end px-3 mt-3">
</nav> -->

<div class="p-3">
    {#key profileStore.status}
        <div transition:slide={{ axis: 'y' }}>
            {#if profileStore.status === Status.Loading}
                <div class="w-full flex gap-1 justify-center">
                    <h1 class="text-xl">Loading profiles...</h1>
                    <LoaderCircle class="animate-spin size-8" />
                </div>
            {:else if profileStore.status === Status.Failed}
                <h1 class="text-center text-error text-lg">Failed to load profiles</h1>
            {:else if profileStore.status === Status.Done && profileStore.list.length === 0}
                <h1 class="text-center text-lg italic opacity-60">No profiles</h1>
            {/if}
        </div>
    {/key}

    {#key profileStore.status}
        <div
            class="flex flex-col gap-3"
            transition:slide={{ axis: 'y' }}
            use:sortable={{ onReorder: moveProfile }}
        >
            {#each profileStore.list as profile (profile.name)}
                <div class="flex flex-row items-stretch rounded-lg border bg-surface text-surface-foreground">
                    <a
                        href={resolve("/profile/[profile_name]", {
                            profile_name: profile.name
                        })}
                        class="block min-w-0 flex-1 p-3"
                    >
                        <h1 class="text-lg">{profile.displayName}</h1>
                    </a>

                    {#if profileStore.list.length > 1}
                        <button
                            type="button"
                            data-sortable-handle
                            aria-label="Reorder {profile.displayName}"
                            class="cursor-grab active:cursor-grabbing shrink-0 px-3 opacity-60 touch-none select-none"
                        >
                            <ChevronsUpDown class="size-5" />
                        </button>
                    {/if}
                </div>
            {/each}
        </div>
    {/key}

    <button
        type="button"
        onclick={profileDialogOpen}
        class="w-full bg-accent text-accent-foreground flex flex-row gap-3 justify-center items-center py-2 mt-10 rounded-xl"
    >
        <Plus />
        <h1>New profile</h1>
    </button>

    <Dialog
        bind:open={newProfileDialog}
        title="New profile"
    >
        {#snippet body()}
            {let creating = $state(false)}
            {const create = async () => {
                creating = true;
                await createProfile();
                creating = false;
            }}
            <div class="mb-5">
                <input
                    type="text"
                    bind:value={newProfileName}
                    disabled={creating}
                    name="profile-name" id="profile-name"
                    placeholder="Profile name"
                    class="w-full text-lg text-center"
                    onkeydown={(e) => {
                        if (e.key !== "Enter" || e.shiftKey || e.isComposing) return;
                        e.preventDefault();
                        create();
                    }}>
            </div>
            <button
                type="button"
                onclick={create}
                disabled={creating || (newProfileName.trim() === "")}
                class="flex flex-row gap-2 items-center justify-center w-full bg-accent text-accent-foreground rounded-lg py-3 disabled:opacity-60"
            >
                {#if creating}
                    <span>Creating</span>
                    <LoaderCircle class="size-5 animate-spin" />
                {:else}
                    <span>Create</span>
                {/if}
            </button>
        {/snippet}
    </Dialog>
</div>

<style lang="postcss">
    @reference "tailwindcss";

    input[type="text"] {
        background-color: var(--color-surface);
        color: var(--color-surface-foreground);
        @apply rounded-lg px-3 py-2;
        
        &::placeholder {
            @apply italic font-light;
        }
    }
</style>
