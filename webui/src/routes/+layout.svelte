<script lang="ts">
	import './layout.css';
    import { toast, Toaster } from 'svelte-sonner';
	import QuickTTS from '$lib/components/QuickTTS.svelte';
	import { profileStore } from '$lib/profile/profiles.svelte';
	import { onMount } from 'svelte';
    import LogoGithub from "$lib/assets/GitHub_Invertocat_White.png";
	import { resolve } from '$app/paths';

	let { children } = $props();

    onMount(async () => {
        const res = await profileStore.init();
        if (res.err) {
            toast.error("Failed to load profiles", { description: res.err.toString() });
        }
    });

    // iOS workaround for interactive-widget=resizes-content
    onMount(() => {
        const vv = window.visualViewport;
        if (!vv) return;
        const root = document.documentElement;
        const update = () => {
            if (vv.scale > 1.01) return;
            const inset = root.clientHeight - (vv.height + vv.offsetTop);
            root.style.setProperty('--keyboard-inset', `${Math.max(0, Math.round(inset))}px`);
        };
        update();
        vv.addEventListener('resize', update);
        vv.addEventListener('scroll', update);
        return () => {
            vv.removeEventListener('resize', update);
            vv.removeEventListener('scroll', update);
            root.style.removeProperty('--keyboard-inset');
        };
    });
</script>

<svelte:head>
    <title>VoiTTS</title>
</svelte:head>

<Toaster position='top-right' richColors />

<header class="bg-secondary flex flex-row items-center px-3 py-2">
    <a href={resolve("/")} class="inline-flex">
        <h1>VoiTTS</h1>
    </a>

    <div class="flex-1"></div>
    
    <a href="https://github.com/sherpavel/VoiTTS" target="_blank" class="block">
        <img src={LogoGithub} alt="GitHub" class="size-7">
    </a>
</header>

<!-- The bar is fixed, so reserve its height plus the keyboard's so the last of
     the content can always be scrolled clear of both. -->
<main class="pt-3 pb-[calc(var(--quick-tts-height,0px)+var(--keyboard-inset,0px))]">
    {@render children()}
</main>

<QuickTTS />
