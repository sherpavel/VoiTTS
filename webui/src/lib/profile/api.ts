import { fail, success, unsafe, type AsyncResult } from "../err";

export type Profile = {
    name: string;
    displayName: string;
    texts: string[];
};

// An array, not a map: the order is the order they are shown in.
export type Profiles = Profile[];

const APIs = {
    get: "/api/profiles",
    order: "/api/profiles/order",
    patch: "/api/profile",
    remove: "/api/profile",
} as const;

export async function loadProfiles(): AsyncResult<Profiles> {
    const res = await unsafe(fetch(APIs.get, { method: 'GET' }));
    if (res.err) {
        return fail("fetch", res.err);
    }
    if (!res.ok.ok) {
        return fail(`fetch, server responded with: ${res.ok.statusText}`);
    }

    const parseRes = await unsafe(res.ok.json());
    if (parseRes.err) {
        return fail("JSON parse", parseRes.err);
    }

    return success(parseRes.ok);
}

export async function upsertProfile(profile: Profile): AsyncResult<void> {
    const res = await unsafe(fetch(APIs.patch, {
        method: 'PATCH',
        body: JSON.stringify(profile)
    }));
    if (res.err) {
        return fail("fetch", res.err);
    }
    if (!res.ok.ok) {
        return fail(`fetch, server responded with: ${res.ok.statusText}`);
    }
    return success(undefined);
}

export async function deleteProfile(profileName: Profile['name']): AsyncResult<void> {
    const res = await unsafe(fetch(APIs.remove, {
        method: 'DELETE',
        body: JSON.stringify({
            name: profileName
        })
    }));
    if (res.err) {
        return fail("fetch", res.err);
    }
    if (!res.ok.ok) {
        return fail(`fetch, server responded with: ${res.ok.statusText}`);
    }
    return success(undefined);
}

export async function reorderProfiles(names: Profile['name'][]): AsyncResult<void> {
    const res = await unsafe(fetch(APIs.order, {
        method: 'PUT',
        body: JSON.stringify({ names })
    }));
    if (res.err) {
        return fail("fetch", res.err);
    }
    if (res.ok.status === 409) {
        return fail("the profile list is out of date, reload the page");
    }
    if (!res.ok.ok) {
        return fail(`fetch, server responded with: ${res.ok.statusText}`);
    }
    return success(undefined);
}
