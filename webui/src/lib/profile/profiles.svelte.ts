import { fail, success, type AsyncResult } from "$lib/err";
import { deleteProfile, loadProfiles, reorderProfiles, upsertProfile, type Profile, type Profiles } from "$lib/profile/api";

let profiles = $state<Profiles>([]);

export enum Status {
    None,
    Loading,
    Done,
    Failed
}
let status = $state<Status>(Status.None);

export const profileStore = {
    // In display order. Reorder with reorder(), not by mutating this.
    get list(): Profile[] {
        return profiles;
    },
    get status(): Status {
        return status;
    },

    // Call once in layout.svelte
    async init(): AsyncResult<void> {
        status = Status.Loading;

        const res = await loadProfiles();
        if (res.err) {
            status = Status.Failed;
            return fail("Failed to load profiles", res.err);
        }

        profiles = res.ok;
        status = Status.Done;
        return success(undefined);
    },

    get(name: string): Profile | null {
        return profiles.find((p) => p.name === name) ?? null;
    },

    // Creates if new or updates if exists. An edit keeps its place in the list
    async upsert(profile: Profile): AsyncResult<void> {
        const res = await upsertProfile(profile);
        if (res.err) return fail("Failed to upsert profile", res.err);

        const i = profiles.findIndex((p) => p.name === profile.name);
        if (i < 0) profiles.push(profile);
        else profiles[i] = profile;

        return success(undefined);
    },

    async create(profileName: string): AsyncResult<void> {
        if (profileName.trim() === "") return fail("Name cannot be empty");

        const profile = makeProfile(profileName, profiles);

        const res = await upsertProfile(profile);
        if (res.err) return fail("Failed to create profile", res.err);

        profiles.push(profile);

        return success(undefined);
    },

    async remove(name: string): AsyncResult<void> {
        const i = profiles.findIndex((p) => p.name === name);
        if (i < 0) return fail("Profile not found");

        const res = await deleteProfile(name);
        if (res.err) return fail("Failed to delete profile", res.err);

        profiles.splice(i, 1);

        return success(undefined);
    },

    async reorder(names: Profile['name'][]): AsyncResult<void> {
        if (names.length !== profiles.length) {
            return fail("Order must list every profile");
        }

        const byName = new Map(profiles.map((p) => [p.name, p]));
        const next: Profile[] = [];
        for (const name of names) {
            const profile = byName.get(name);
            if (!profile) return fail(`No profile named "${name}"`);
            byName.delete(name); // a name listed twice cannot resolve twice
            next.push(profile);
        }

        const previous = profiles;
        profiles = next;

        const res = await reorderProfiles(names);
        if (res.err) {
            profiles = previous;
            return fail("Failed to save the new order", res.err);
        }

        return success(undefined);
    },
};

// Create new unique profile. If uniqueness fails, appends a numbered postfix to the name key 
function makeProfile(displayName: string, existing: Profiles): Profile {
    const base = slugify(displayName) || "profile";

    let name = base;
    for (let n = 2; existing.some((p) => p.name === name); n++) {
        name = `${base}-${n}`;
    }

    return {
        name,
        displayName,
        texts: []
    };
}

function slugify(text: string): string {
    return text
        .toLowerCase()
        .replace(/[^a-z0-9]+/g, "-")
        .replace(/^-+|-+$/g, "");
}
